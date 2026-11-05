package main

// repos_cobranza.go — acceso a datos de la fase 3 (cobranza en ruta y documentos).
// Mismas reglas que repos.go: empresaID como primer parámetro y filtro por empresa_id en todo query.

import (
	"database/sql"
	"errors"
)

// ─────────────────────────────────────────────────────────────────────────────
// Usuarios: edición y contraseña (gestión desde la aplicación)
// ─────────────────────────────────────────────────────────────────────────────

func (r *Repo) ActualizarUsuario(empresaID, id, nombre, rol string, activo bool) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE usuario SET nombre = $3, rol = $4, activo = $5 WHERE empresa_id = $1 AND id = $2`,
		empresaID, id, nombre, rol, activo)
	return err
}

func (r *Repo) CambiarPassword(empresaID, id, hash string) error {
	_, err := r.q.ExecContext(r.ctx,
		`UPDATE usuario SET password_hash = $3 WHERE empresa_id = $1 AND id = $2`, empresaID, id, hash)
	return err
}

// ContarAdminsActivos excluye un usuario, para no dejar a la empresa sin administrador.
func (r *Repo) ContarAdminsActivos(empresaID, exceptoID string) (int, error) {
	var n int
	err := r.q.QueryRowContext(r.ctx, `
		SELECT count(*) FROM usuario
		WHERE empresa_id = $1 AND rol = 'admin' AND activo AND id <> $2`, empresaID, exceptoID).Scan(&n)
	return n, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Anticipos (saldo a favor del cliente)
// ─────────────────────────────────────────────────────────────────────────────

const selectAnticipo = `
	SELECT a.id, a.numero, a.cliente_id, cl.nombre, a.monto_centavos, a.fecha, a.metodo, a.referencia,
	       a.nota, a.registrado_por, ur.nombre, a.liquidacion_id, a.anulado, a.anulado_por,
	       a.anulado_en, a.motivo_anulacion, a.creado_en,
	       COALESCE((SELECT SUM(p.monto_centavos) FROM pago p
	                 WHERE p.empresa_id = a.empresa_id AND p.anticipo_id = a.id AND NOT p.anulado), 0)
	FROM anticipo a
	JOIN cliente cl ON cl.empresa_id = a.empresa_id AND cl.id = a.cliente_id
	JOIN usuario ur ON ur.empresa_id = a.empresa_id AND ur.id = a.registrado_por
	WHERE a.empresa_id = $1 AND ($2::uuid IS NULL OR cl.vendedor_id = $2)`

func escanearAnticipo(s interface{ Scan(...any) error }) (Anticipo, error) {
	var a Anticipo
	err := s.Scan(&a.ID, &a.Numero, &a.ClienteID, &a.ClienteNombre, &a.MontoCentavos, &a.Fecha,
		&a.Metodo, &a.Referencia, &a.Nota, &a.RegistradoPor, &a.RegistradoPorNombre, &a.LiquidacionID,
		&a.Anulado, &a.AnuladoPor, &a.AnuladoEn, &a.MotivoAnulacion, &a.CreadoEn, &a.AplicadoCentavos)
	return a, err
}

func errAnticipoNoEncontrado() *ErrorApp {
	return errNoEncontrado("ANTICIPO_NO_ENCONTRADO", "No encontramos el anticipo indicado.")
}

// ListarAnticipos: clienteID vacío = todos los del alcance.
func (r *Repo) ListarAnticipos(empresaID string, alc Alcance, clienteID string) ([]Anticipo, error) {
	var filtro any
	if clienteID != "" {
		filtro = clienteID
	}
	filas, err := r.q.QueryContext(r.ctx,
		selectAnticipo+` AND ($3::uuid IS NULL OR a.cliente_id = $3) ORDER BY a.fecha DESC, a.creado_en DESC`,
		empresaID, alc.filtro(), filtro)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Anticipo{}
	for filas.Next() {
		a, err := escanearAnticipo(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, filas.Err()
}

func (r *Repo) ObtenerAnticipo(empresaID string, alc Alcance, id string) (Anticipo, error) {
	a, err := escanearAnticipo(r.q.QueryRowContext(r.ctx, selectAnticipo+` AND a.id = $3`, empresaID, alc.filtro(), id))
	return a, noEncontrado(err, errAnticipoNoEncontrado())
}

// BloquearAnticipo serializa las aplicaciones: dos cuentas no pueden consumir el mismo saldo.
func (r *Repo) BloquearAnticipo(empresaID, id string) error {
	var x string
	err := r.q.QueryRowContext(r.ctx,
		`SELECT id FROM anticipo WHERE empresa_id = $1 AND id = $2 FOR UPDATE`, empresaID, id).Scan(&x)
	return noEncontrado(err, errAnticipoNoEncontrado())
}

func (r *Repo) CrearAnticipo(empresaID, numero, clienteID string, monto int64, fecha Fecha,
	metodo, referencia, nota, registradoPor string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO anticipo (empresa_id, numero, cliente_id, monto_centavos, fecha, metodo,
		                      referencia, nota, registrado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`,
		empresaID, numero, clienteID, monto, fecha, metodo, referencia, nota, registradoPor).Scan(&id)
	return id, err
}

func (r *Repo) AnularAnticipo(empresaID, id, usuarioID, motivo string) error {
	var motivoDB any
	if motivo != "" {
		motivoDB = motivo
	}
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE anticipo SET anulado = true, anulado_por = $3, anulado_en = now(), motivo_anulacion = $4
		WHERE empresa_id = $1 AND id = $2 AND NOT anulado`, empresaID, id, usuarioID, motivoDB)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Liquidación del cobrador
// ─────────────────────────────────────────────────────────────────────────────

// PagosSinLiquidar devuelve el dinero que el cobrador aún no ha entregado. Bloquea las filas para
// que dos cierres simultáneos no incluyan el mismo pago.
func (r *Repo) PagosSinLiquidar(empresaID, cobradorID string, hasta Fecha) ([]Movimiento, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT p.id, p.cuenta_id, p.tipo, p.numero, p.monto_centavos, p.fecha, p.metodo, p.referencia,
		       p.liquidacion_id, p.anticipo_id, p.nota, p.registrado_por, ur.nombre,
		       p.anulado, p.anulado_por, ua.nombre, p.anulado_en, p.motivo_anulacion, p.creado_en
		FROM pago p
		JOIN usuario ur ON ur.empresa_id = p.empresa_id AND ur.id = p.registrado_por
		LEFT JOIN usuario ua ON ua.empresa_id = p.empresa_id AND ua.id = p.anulado_por
		WHERE p.empresa_id = $1 AND p.registrado_por = $2 AND p.tipo = 'pago' AND NOT p.anulado
		  AND p.liquidacion_id IS NULL AND p.anticipo_id IS NULL AND p.fecha <= $3
		ORDER BY p.fecha, p.creado_en
		FOR UPDATE OF p`, empresaID, cobradorID, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Movimiento{}
	for filas.Next() {
		m, err := escanearMovimiento(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, filas.Err()
}

// AnticiposSinLiquidar: el dinero de anticipos también lo entrega el cobrador.
func (r *Repo) AnticiposSinLiquidar(empresaID, cobradorID string, hasta Fecha) ([]Anticipo, error) {
	filas, err := r.q.QueryContext(r.ctx, selectAnticipo+
		` AND a.registrado_por = $3 AND NOT a.anulado AND a.liquidacion_id IS NULL AND a.fecha <= $4
		  ORDER BY a.fecha, a.creado_en`,
		empresaID, nil, cobradorID, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Anticipo{}
	for filas.Next() {
		a, err := escanearAnticipo(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, filas.Err()
}

const selectLiquidacion = `
	SELECT l.id, l.numero, l.cobrador_id, uc.nombre, l.fecha, l.estado, l.total_esperado_centavos,
	       l.total_entregado_centavos, l.cantidad_movimientos, l.nota, l.revisado_por, ur.nombre,
	       l.revisado_en, l.motivo_rechazo, l.creado_en
	FROM liquidacion l
	JOIN usuario uc ON uc.empresa_id = l.empresa_id AND uc.id = l.cobrador_id
	LEFT JOIN usuario ur ON ur.empresa_id = l.empresa_id AND ur.id = l.revisado_por
	WHERE l.empresa_id = $1 AND ($2::uuid IS NULL OR l.cobrador_id = $2)`

func escanearLiquidacion(s interface{ Scan(...any) error }) (Liquidacion, error) {
	var l Liquidacion
	err := s.Scan(&l.ID, &l.Numero, &l.CobradorID, &l.CobradorNombre, &l.Fecha, &l.Estado,
		&l.TotalEsperadoCentavos, &l.TotalEntregadoCentavos, &l.CantidadMovimientos, &l.Nota,
		&l.RevisadoPor, &l.RevisadoPorNombre, &l.RevisadoEn, &l.MotivoRechazo, &l.CreadoEn)
	return l, err
}

func errLiquidacionNoEncontrada() *ErrorApp {
	return errNoEncontrado("LIQUIDACION_NO_ENCONTRADA", "No encontramos la liquidación indicada.")
}

// ListarLiquidaciones: el admin ve todas; el cobrador, las suyas. estado vacío = todas.
func (r *Repo) ListarLiquidaciones(empresaID string, alc Alcance, estado string) ([]Liquidacion, error) {
	var filtro any
	if estado != "" {
		filtro = estado
	}
	filas, err := r.q.QueryContext(r.ctx,
		selectLiquidacion+` AND ($3::text IS NULL OR l.estado = $3) ORDER BY l.fecha DESC, l.creado_en DESC`,
		empresaID, alc.filtro(), filtro)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Liquidacion{}
	for filas.Next() {
		l, err := escanearLiquidacion(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, l)
	}
	return res, filas.Err()
}

func (r *Repo) ObtenerLiquidacion(empresaID string, alc Alcance, id string, bloquear bool) (Liquidacion, error) {
	q := selectLiquidacion + ` AND l.id = $3`
	if bloquear {
		q += ` FOR UPDATE OF l`
	}
	l, err := escanearLiquidacion(r.q.QueryRowContext(r.ctx, q, empresaID, alc.filtro(), id))
	return l, noEncontrado(err, errLiquidacionNoEncontrada())
}

func (r *Repo) CrearLiquidacion(empresaID string, l Liquidacion) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO liquidacion (empresa_id, numero, cobrador_id, fecha, total_esperado_centavos,
		                         total_entregado_centavos, cantidad_movimientos, nota)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		empresaID, l.Numero, l.CobradorID, l.Fecha, l.TotalEsperadoCentavos,
		l.TotalEntregadoCentavos, l.CantidadMovimientos, l.Nota).Scan(&id)
	return id, err
}

// AsignarLiquidacion marca los movimientos y anticipos entregados en ese corte.
func (r *Repo) AsignarLiquidacion(empresaID, liquidacionID string, pagoIDs, anticipoIDs []string) error {
	if len(pagoIDs) > 0 {
		if _, err := r.q.ExecContext(r.ctx, `
			UPDATE pago SET liquidacion_id = $3
			WHERE empresa_id = $1 AND id = ANY($2::uuid[]) AND liquidacion_id IS NULL`,
			empresaID, arregloPG(pagoIDs), liquidacionID); err != nil {
			return err
		}
	}
	if len(anticipoIDs) > 0 {
		if _, err := r.q.ExecContext(r.ctx, `
			UPDATE anticipo SET liquidacion_id = $3
			WHERE empresa_id = $1 AND id = ANY($2::uuid[]) AND liquidacion_id IS NULL`,
			empresaID, arregloPG(anticipoIDs), liquidacionID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ResolverLiquidacion(empresaID, id, estado, revisadoPor, motivo string) error {
	var motivoDB any
	if motivo != "" {
		motivoDB = motivo
	}
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE liquidacion SET estado = $3, revisado_por = $4, revisado_en = now(), motivo_rechazo = $5
		WHERE empresa_id = $1 AND id = $2 AND estado = 'entregada'`,
		empresaID, id, estado, revisadoPor, motivoDB)
	return err
}

// LiberarLiquidacion devuelve los movimientos a "sin liquidar" cuando el corte se rechaza,
// para que el cobrador pueda rehacerlo.
func (r *Repo) LiberarLiquidacion(empresaID, liquidacionID string) error {
	if _, err := r.q.ExecContext(r.ctx,
		`UPDATE pago SET liquidacion_id = NULL WHERE empresa_id = $1 AND liquidacion_id = $2`,
		empresaID, liquidacionID); err != nil {
		return err
	}
	_, err := r.q.ExecContext(r.ctx,
		`UPDATE anticipo SET liquidacion_id = NULL WHERE empresa_id = $1 AND liquidacion_id = $2`,
		empresaID, liquidacionID)
	return err
}

// MovimientosDeLiquidacion lista lo incluido en un corte ya cerrado.
func (r *Repo) MovimientosDeLiquidacion(empresaID, liquidacionID string) ([]Movimiento, error) {
	filas, err := r.q.QueryContext(r.ctx,
		selectMovimiento+` AND p.liquidacion_id = $3 ORDER BY p.fecha, p.creado_en`,
		empresaID, nil, liquidacionID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Movimiento{}
	for filas.Next() {
		m, err := escanearMovimiento(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, m)
	}
	return res, filas.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Adjuntos (fotos de recibos y facturas)
// ─────────────────────────────────────────────────────────────────────────────

const selectAdjunto = `
	SELECT a.id, a.entidad, a.entidad_id, a.titulo, a.nombre, a.clase, a.tipo_mime, a.tamano_bytes,
	       a.descripcion, a.subido_por, u.nombre, a.creado_en, a.archivo
	FROM adjunto a
	JOIN usuario u ON u.empresa_id = a.empresa_id AND u.id = a.subido_por
	WHERE a.empresa_id = $1`

func (r *Repo) escanearAdjunto(s interface{ Scan(...any) error }) (Adjunto, error) {
	var a Adjunto
	err := s.Scan(&a.ID, &a.Entidad, &a.EntidadID, &a.Titulo, &a.Nombre, &a.Clase, &a.TipoMime,
		&a.TamanoBytes, &a.Descripcion, &a.SubidoPor, &a.SubidoPorNombre, &a.CreadoEn, &a.archivo)
	if err != nil {
		return a, err
	}
	// El contenido (si está en la base) se trae solo cuando alguien lo pide: un listado de
	// adjuntos no debe cargar todas las fotos en memoria. Usa la misma transacción (y por tanto
	// el mismo app.empresa_id) que la consulta que lo trajo.
	id, q, ctx := a.ID, r.q, r.ctx
	a.leerDeBase = func() ([]byte, error) {
		var datos []byte
		err := q.QueryRowContext(ctx, `SELECT contenido FROM adjunto WHERE id = $1`, id).Scan(&datos)
		if err == nil && datos == nil {
			err = sql.ErrNoRows
		}
		return datos, err
	}
	return a, nil
}

func (r *Repo) ListarAdjuntos(empresaID, entidad, entidadID string) ([]Adjunto, error) {
	filas, err := r.q.QueryContext(r.ctx,
		selectAdjunto+` AND a.entidad = $2 AND a.entidad_id = $3 ORDER BY a.creado_en`,
		empresaID, entidad, entidadID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Adjunto{}
	for filas.Next() {
		a, err := r.escanearAdjunto(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, filas.Err()
}

// ListarComprobantes es el listado SIN firmas: lo que se archiva, se imprime y se entrega como
// documento. La firma del cliente no es un comprobante suelto, sale dentro del recibo.
func (r *Repo) ListarComprobantes(empresaID, entidad, entidadID string) ([]Adjunto, error) {
	lista, err := r.ListarAdjuntos(empresaID, entidad, entidadID)
	if err != nil {
		return nil, err
	}
	res := make([]Adjunto, 0, len(lista))
	for _, a := range lista {
		if !a.EsFirma() {
			res = append(res, a)
		}
	}
	return res, nil
}

// FirmaDeMovimiento devuelve la firma del cliente en ese movimiento, si la tiene. Se toma la
// ÚLTIMA: al volver a firmar se reemplaza la anterior, y si alguna quedara rezagada manda la
// más reciente, que es la que el cliente trazó de verdad.
func (r *Repo) FirmaDeMovimiento(empresaID, movimientoID string) (Adjunto, bool, error) {
	a, err := r.escanearAdjunto(r.q.QueryRowContext(r.ctx,
		selectAdjunto+` AND a.entidad = 'movimiento' AND a.entidad_id = $2 AND a.clase = $3
		                ORDER BY a.creado_en DESC LIMIT 1`,
		empresaID, movimientoID, ClaseFirma))
	if errors.Is(err, sql.ErrNoRows) {
		return Adjunto{}, false, nil
	}
	return a, err == nil, err
}

func (r *Repo) ObtenerAdjunto(empresaID, id string) (Adjunto, error) {
	a, err := r.escanearAdjunto(r.q.QueryRowContext(r.ctx, selectAdjunto+` AND a.id = $2`, empresaID, id))
	return a, noEncontrado(err, errNoEncontrado("ADJUNTO_NO_ENCONTRADO", "No encontramos el archivo indicado."))
}

func (r *Repo) CrearAdjunto(empresaID string, a Adjunto) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO adjunto (empresa_id, entidad, entidad_id, titulo, nombre, clase, tipo_mime,
		                     tamano_bytes, archivo, descripcion, subido_por, contenido)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
		empresaID, a.Entidad, a.EntidadID, a.Titulo, a.Nombre, claseOComprobante(a.Clase),
		a.TipoMime, a.TamanoBytes, a.archivo, a.Descripcion, a.SubidoPor, a.contenido).Scan(&id)
	return id, err
}

// claseOComprobante: quien no dice nada está subiendo un comprobante, que es el caso de siempre.
func claseOComprobante(clase string) string {
	if clase == ClaseFirma {
		return ClaseFirma
	}
	return ClaseComprobante
}

func (r *Repo) EliminarAdjunto(empresaID, id string) error {
	_, err := r.q.ExecContext(r.ctx, `DELETE FROM adjunto WHERE empresa_id = $1 AND id = $2`, empresaID, id)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Importación de cartera
// ─────────────────────────────────────────────────────────────────────────────

// ClientesPorCodigo mapea código del sistema de origen → cliente, para no duplicar al reimportar.
func (r *Repo) ClientesPorCodigo(empresaID string) (map[string]Cliente, error) {
	filas, err := r.q.QueryContext(r.ctx, selectCliente+` AND c.codigo_externo <> ''`, empresaID, nil)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := map[string]Cliente{}
	for filas.Next() {
		c, err := escanearCliente(filas)
		if err != nil {
			return nil, err
		}
		res[c.CodigoExterno] = c
	}
	return res, filas.Err()
}

// DocumentosExistentes devuelve qué números de documento ya están cargados.
func (r *Repo) DocumentosExistentes(empresaID string, numeros []string) (map[string]bool, error) {
	res := map[string]bool{}
	if len(numeros) == 0 {
		return res, nil
	}
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT numero_documento FROM cuenta
		WHERE empresa_id = $1 AND numero_documento = ANY($2::text[])`, empresaID, arregloPG(numeros))
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	for filas.Next() {
		var n string
		if err := filas.Scan(&n); err != nil {
			return nil, err
		}
		res[n] = true
	}
	return res, filas.Err()
}

// ContarUsuariosActivos se usa para aplicar el límite del plan contratado (R77).
func (r *Repo) ContarUsuariosActivos(empresaID string) (int, error) {
	var n int
	err := r.q.QueryRowContext(r.ctx,
		`SELECT COUNT(*) FROM usuario WHERE empresa_id = $1 AND activo`, empresaID).Scan(&n)
	return n, err
}
