package main

// repos.go — todo el SQL de la aplicación.
// Reglas: cada método recibe empresaID como PRIMER parámetro y todo query filtra por
// empresa_id = $1 (además del RLS de la base). Las lecturas que dependen del rol reciben un
// Alcance: el scoping por vendedor se resuelve aquí, no en los handlers (R39).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Repo struct {
	ctx context.Context
	q   querier
}

// Alcance limita las lecturas: Total (admin) ve toda la empresa; de lo contrario solo los
// registros cuyo vendedor_id es UsuarioID (R36, R37).
type Alcance struct {
	Total     bool
	UsuarioID string
}

func AlcanceEmpresa() Alcance                  { return Alcance{Total: true} }
func AlcanceVendedor(usuarioID string) Alcance { return Alcance{UsuarioID: usuarioID} }

// filtro devuelve el valor para `($2::uuid IS NULL OR x.vendedor_id = $2)`.
func (a Alcance) filtro() any {
	if a.Total {
		return nil
	}
	return a.UsuarioID
}

func noEncontrado(err error, e *ErrorApp) error {
	if errors.Is(err, sql.ErrNoRows) {
		return e
	}
	return err
}

func textoONil(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// ─────────────────────────────────────────────────────────────────────────────
// Empresa, marca y configuración
// ─────────────────────────────────────────────────────────────────────────────

// buscarEmpresaActivaPorSlug es la única consulta sin empresa_id: sirve justamente para
// descubrirlo (login y marca pública). La tabla empresa no tiene RLS.
const camposEmpresa = `id, slug, nombre_legal, nombre_comercial, activo, creado_en, nit, nrc, direccion, telefono, plan`

func escanearEmpresa(s interface{ Scan(...any) error }) (Empresa, error) {
	var e Empresa
	err := s.Scan(&e.ID, &e.Slug, &e.NombreLegal, &e.NombreComercial, &e.Activo, &e.CreadoEn,
		&e.NIT, &e.NRC, &e.Direccion, &e.Telefono, &e.Plan)
	return e, err
}

func buscarEmpresaActivaPorSlug(ctx context.Context, q querier, slug string) (Empresa, error) {
	e, err := escanearEmpresa(q.QueryRowContext(ctx,
		`SELECT `+camposEmpresa+` FROM empresa WHERE slug = $1 AND activo`, slug))
	return e, noEncontrado(err, errNoEncontrado("EMPRESA_NO_ENCONTRADA", "No encontramos una empresa activa con ese identificador."))
}

func (r *Repo) ObtenerEmpresa(empresaID string) (Empresa, error) {
	e, err := escanearEmpresa(r.q.QueryRowContext(r.ctx,
		`SELECT `+camposEmpresa+` FROM empresa WHERE id = $1`, empresaID))
	return e, noEncontrado(err, errNoEncontrado("EMPRESA_NO_ENCONTRADA", "No encontramos la empresa."))
}

// GuardarDatosEmpresa actualiza los datos que salen en los documentos impresos.
func (r *Repo) GuardarDatosEmpresa(empresaID string, e Empresa) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE empresa SET nit = $2, nrc = $3, direccion = $4, telefono = $5 WHERE id = $1`,
		empresaID, e.NIT, e.NRC, e.Direccion, e.Telefono)
	return err
}

func (r *Repo) ObtenerMarca(empresaID string) (Marca, error) {
	var m Marca
	err := r.q.QueryRowContext(r.ctx, `
		SELECT logo_url, color_primario, color_primario_claro, nombre_producto
		FROM empresa_marca WHERE empresa_id = $1`, empresaID).
		Scan(&m.LogoURL, &m.ColorPrimario, &m.ColorPrimarioClaro, &m.NombreProducto)
	return m, noEncontrado(err, errNoEncontrado("MARCA_NO_ENCONTRADA", "La empresa no tiene marca configurada."))
}

func (r *Repo) GuardarMarca(empresaID string, m Marca) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE empresa_marca
		SET logo_url = $2, color_primario = $3, color_primario_claro = $4, nombre_producto = $5,
		    actualizado_en = now()
		WHERE empresa_id = $1`,
		empresaID, textoONil(m.LogoURL), m.ColorPrimario, m.ColorPrimarioClaro, m.NombreProducto)
	return err
}

func (r *Repo) ObtenerConfig(empresaID string) (ConfigEmpresa, error) {
	var c ConfigEmpresa
	var plazos, metodos string
	err := r.q.QueryRowContext(r.ctx, `
		SELECT moneda, simbolo_moneda, zona_horaria, dias_aviso_previo,
		       array_to_string(plazos_credito_permitidos, ','),
		       permitir_sobrepago, exigir_revision_ajuste_monto, exigir_motivo_anulacion,
		       longitud_minima_motivo, umbral_comportamiento_bueno, umbral_comportamiento_regular,
		       minimo_cuentas_para_historial, cuentas_evaluadas_comportamiento,
		       exigir_limite_credito, permitir_condonaciones,
		       array_to_string(metodos_pago, ','), exigir_liquidacion_cobrador, permitir_adjuntos,
		       tamano_maximo_adjunto_mb, prefijo_recibo, prefijo_nota_credito, prefijo_anticipo,
		       prefijo_liquidacion, limite_usuarios, actualizado_en
		FROM empresa_config WHERE empresa_id = $1`, empresaID).
		Scan(&c.Moneda, &c.SimboloMoneda, &c.ZonaHoraria, &c.DiasAvisoPrevio, &plazos,
			&c.PermitirSobrepago, &c.ExigirRevisionAjusteMonto, &c.ExigirMotivoAnulacion,
			&c.LongitudMinimaMotivo, &c.UmbralComportamientoBueno, &c.UmbralComportamientoRegular,
			&c.MinimoCuentasParaHistorial, &c.CuentasEvaluadasComportamiento,
			&c.ExigirLimiteCredito, &c.PermitirCondonaciones,
			&metodos, &c.ExigirLiquidacionCobrador, &c.PermitirAdjuntos,
			&c.TamanoMaximoAdjuntoMB, &c.PrefijoRecibo, &c.PrefijoNotaCredito, &c.PrefijoAnticipo,
			&c.PrefijoLiquidacion, &c.LimiteUsuarios, &c.ActualizadoEn)
	if err != nil {
		return c, noEncontrado(err, errNoEncontrado("CONFIGURACION_NO_ENCONTRADA", "La empresa no tiene configuración."))
	}
	c.PlazosCreditoPermitidos = []int{}
	for _, p := range strings.Split(plazos, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			c.PlazosCreditoPermitidos = append(c.PlazosCreditoPermitidos, n)
		}
	}
	c.MetodosPago = []string{}
	for _, m := range strings.Split(metodos, ",") {
		if m = strings.TrimSpace(m); m != "" {
			c.MetodosPago = append(c.MetodosPago, m)
		}
	}
	return c, nil
}

func (r *Repo) GuardarConfig(empresaID string, c ConfigEmpresa) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE empresa_config SET
		    moneda = $2, simbolo_moneda = $3, zona_horaria = $4, dias_aviso_previo = $5,
		    plazos_credito_permitidos = $6::int[], permitir_sobrepago = $7,
		    exigir_revision_ajuste_monto = $8, exigir_motivo_anulacion = $9,
		    longitud_minima_motivo = $10, umbral_comportamiento_bueno = $11,
		    umbral_comportamiento_regular = $12, minimo_cuentas_para_historial = $13,
		    cuentas_evaluadas_comportamiento = $14, exigir_limite_credito = $15,
		    permitir_condonaciones = $16, metodos_pago = $17::text[],
		    exigir_liquidacion_cobrador = $18, permitir_adjuntos = $19,
		    tamano_maximo_adjunto_mb = $20, prefijo_recibo = $21, prefijo_nota_credito = $22,
		    prefijo_anticipo = $23, prefijo_liquidacion = $24, limite_usuarios = $25,
		    actualizado_en = now()
		WHERE empresa_id = $1`,
		empresaID, c.Moneda, c.SimboloMoneda, c.ZonaHoraria, c.DiasAvisoPrevio,
		arregloPG(c.PlazosCreditoPermitidos), c.PermitirSobrepago,
		c.ExigirRevisionAjusteMonto, c.ExigirMotivoAnulacion,
		c.LongitudMinimaMotivo, c.UmbralComportamientoBueno,
		c.UmbralComportamientoRegular, c.MinimoCuentasParaHistorial,
		c.CuentasEvaluadasComportamiento, c.ExigirLimiteCredito, c.PermitirCondonaciones,
		arregloPG(c.MetodosPago), c.ExigirLiquidacionCobrador, c.PermitirAdjuntos,
		c.TamanoMaximoAdjuntoMB, c.PrefijoRecibo, c.PrefijoNotaCredito, c.PrefijoAnticipo,
		c.PrefijoLiquidacion, c.LimiteUsuarios)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Correlativos de documentos (recibos, notas de crédito, anticipos, liquidaciones)
// ─────────────────────────────────────────────────────────────────────────────

// ReservarNumero toma el siguiente correlativo del tipo indicado. El UPDATE bloquea la fila, así
// que dos cobradores simultáneos nunca reciben el mismo número de recibo.
func (r *Repo) ReservarNumero(empresaID, tipo, prefijo string) (string, error) {
	var siguiente int64
	err := r.q.QueryRowContext(r.ctx, `
		UPDATE correlativo SET siguiente = siguiente + 1
		WHERE empresa_id = $1 AND tipo = $2
		RETURNING siguiente - 1`, empresaID, tipo).Scan(&siguiente)
	if errors.Is(err, sql.ErrNoRows) {
		// Empresa creada antes de que existiera este tipo de documento.
		if _, err := r.q.ExecContext(r.ctx,
			`INSERT INTO correlativo (empresa_id, tipo, siguiente) VALUES ($1, $2, 2)`, empresaID, tipo); err != nil {
			return "", err
		}
		siguiente = 1
	} else if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%06d", prefijo, siguiente), nil
}

// CrearCorrelativos inicializa los contadores de una empresa nueva.
func (r *Repo) CrearCorrelativos(empresaID string) error {
	_, err := r.q.ExecContext(r.ctx, `
		INSERT INTO correlativo (empresa_id, tipo)
		SELECT $1, t FROM unnest(ARRAY['recibo', 'nota_credito', 'anticipo', 'liquidacion']) t
		ON CONFLICT DO NOTHING`, empresaID)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Alta de empresas (provision.go)
// ─────────────────────────────────────────────────────────────────────────────

// CrearEmpresa inserta la fila de empresa (sin RLS). Devuelve el nuevo empresa_id.
func (r *Repo) CrearEmpresa(slug, nombreLegal, nombreComercial string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO empresa (slug, nombre_legal, nombre_comercial) VALUES ($1, $2, $3) RETURNING id`,
		slug, nombreLegal, nombreComercial).Scan(&id)
	return id, err
}

// CrearMarcaYConfig crea las filas de marca y configuración con los DEFAULT del esquema.
func (r *Repo) CrearMarcaYConfig(empresaID, nombreProducto string) error {
	if _, err := r.q.ExecContext(r.ctx,
		`INSERT INTO empresa_marca (empresa_id, nombre_producto) VALUES ($1, $2)`, empresaID, nombreProducto); err != nil {
		return err
	}
	_, err := r.q.ExecContext(r.ctx, `INSERT INTO empresa_config (empresa_id) VALUES ($1)`, empresaID)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Usuarios
// ─────────────────────────────────────────────────────────────────────────────

const selectUsuario = `SELECT id, nombre, email, rol, activo, creado_en, password_hash FROM usuario`

func escanearUsuario(s interface{ Scan(...any) error }) (Usuario, error) {
	var u Usuario
	err := s.Scan(&u.ID, &u.Nombre, &u.Email, &u.Rol, &u.Activo, &u.CreadoEn, &u.PasswordHash)
	return u, err
}

func errUsuarioNoEncontrado() *ErrorApp {
	return errNoEncontrado("USUARIO_NO_ENCONTRADO", "No encontramos el usuario indicado.")
}

func (r *Repo) CrearUsuario(empresaID, nombre, email, hash, rol string) (Usuario, error) {
	u, err := escanearUsuario(r.q.QueryRowContext(r.ctx, `
		INSERT INTO usuario (empresa_id, nombre, email, password_hash, rol)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, nombre, email, rol, activo, creado_en, password_hash`,
		empresaID, nombre, email, hash, rol))
	return u, err
}

func (r *Repo) BuscarUsuarioPorEmail(empresaID, email string) (Usuario, error) {
	u, err := escanearUsuario(r.q.QueryRowContext(r.ctx,
		selectUsuario+` WHERE empresa_id = $1 AND email = $2`, empresaID, email))
	return u, noEncontrado(err, errUsuarioNoEncontrado())
}

func (r *Repo) ObtenerUsuario(empresaID, id string) (Usuario, error) {
	u, err := escanearUsuario(r.q.QueryRowContext(r.ctx,
		selectUsuario+` WHERE empresa_id = $1 AND id = $2`, empresaID, id))
	return u, noEncontrado(err, errUsuarioNoEncontrado())
}

func (r *Repo) ListarUsuarios(empresaID string) ([]Usuario, error) {
	filas, err := r.q.QueryContext(r.ctx, selectUsuario+` WHERE empresa_id = $1 ORDER BY nombre`, empresaID)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Usuario{}
	for filas.Next() {
		u, err := escanearUsuario(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, u)
	}
	return res, filas.Err()
}

// ─────────────────────────────────────────────────────────────────────────────
// Clientes
// ─────────────────────────────────────────────────────────────────────────────

const selectCliente = `
	SELECT c.id, c.nombre, c.contacto, c.telefono, c.plazo_credito_dias, c.limite_credito_centavos,
	       c.vendedor_id, u.nombre, c.codigo_externo, c.activo, c.creado_en
	FROM cliente c
	JOIN usuario u ON u.empresa_id = c.empresa_id AND u.id = c.vendedor_id
	WHERE c.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)`

func escanearCliente(s interface{ Scan(...any) error }) (Cliente, error) {
	var c Cliente
	err := s.Scan(&c.ID, &c.Nombre, &c.Contacto, &c.Telefono, &c.PlazoCreditoDias, &c.LimiteCreditoCentavos,
		&c.VendedorID, &c.VendedorNombre, &c.CodigoExterno, &c.Activo, &c.CreadoEn)
	return c, err
}

func errClienteNoEncontrado() *ErrorApp {
	return errNoEncontrado("CLIENTE_NO_ENCONTRADO", "No encontramos el cliente solicitado.")
}

func (r *Repo) ListarClientes(empresaID string, alc Alcance, incluirInactivos bool) ([]Cliente, error) {
	filas, err := r.q.QueryContext(r.ctx, selectCliente+` AND (c.activo OR $3) ORDER BY c.nombre`,
		empresaID, alc.filtro(), incluirInactivos)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Cliente{}
	for filas.Next() {
		c, err := escanearCliente(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, filas.Err()
}

func (r *Repo) ObtenerCliente(empresaID string, alc Alcance, id string) (Cliente, error) {
	c, err := escanearCliente(r.q.QueryRowContext(r.ctx, selectCliente+` AND c.id = $3`, empresaID, alc.filtro(), id))
	return c, noEncontrado(err, errClienteNoEncontrado())
}

// BloquearCliente toma el lock de fila del cliente. Serializa las operaciones que dependen de su
// deuda total: crear cuentas (límite de crédito, R5) y desactivarlo (R3). Sin este lock, ventas
// simultáneas superaban el límite y un cliente podía quedar desactivado con saldo.
// Orden de locks en toda la app: oportunidad → cliente → cuenta.
func (r *Repo) BloquearCliente(empresaID string, alc Alcance, id string) error {
	var x string
	err := r.q.QueryRowContext(r.ctx, `
		SELECT id FROM cliente
		WHERE empresa_id = $1 AND ($2::uuid IS NULL OR vendedor_id = $2) AND id = $3
		FOR UPDATE`, empresaID, alc.filtro(), id).Scan(&x)
	return noEncontrado(err, errClienteNoEncontrado())
}

// SerializarReasignaciones evita que dos reasignaciones de la misma empresa se crucen (y se
// bloqueen mutuamente al actualizar los mismos clientes en distinto orden).
func (r *Repo) SerializarReasignaciones(empresaID string) error {
	_, err := r.q.ExecContext(r.ctx, `SELECT pg_advisory_xact_lock(hashtextextended('reasignacion:' || $1, 0))`, empresaID)
	return err
}

func (r *Repo) CrearCliente(empresaID string, d DatosCliente, vendedorID string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias,
		                     limite_credito_centavos, vendedor_id, codigo_externo)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		empresaID, d.Nombre, d.Contacto, d.Telefono, d.PlazoCreditoDias, d.LimiteCreditoCentavos,
		vendedorID, d.CodigoExterno).Scan(&id)
	return id, err
}

func (r *Repo) ActualizarCliente(empresaID, id string, d DatosCliente, vendedorID string, activo bool) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE cliente SET nombre = $3, contacto = $4, telefono = $5, plazo_credito_dias = $6,
		                   limite_credito_centavos = $7, vendedor_id = $8, activo = $9
		WHERE empresa_id = $1 AND id = $2`,
		empresaID, id, d.Nombre, d.Contacto, d.Telefono, d.PlazoCreditoDias, d.LimiteCreditoCentavos, vendedorID, activo)
	return err
}

func (r *Repo) ReasignarClientes(empresaID, origen, destino string) (int, error) {
	res, err := r.q.ExecContext(r.ctx,
		`UPDATE cliente SET vendedor_id = $3 WHERE empresa_id = $1 AND vendedor_id = $2`, empresaID, origen, destino)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	return int(n), err
}

// ─────────────────────────────────────────────────────────────────────────────
// Oportunidades
// ─────────────────────────────────────────────────────────────────────────────

const selectOportunidad = `
	SELECT o.id, o.cliente_id, c.nombre, o.monto_estimado_centavos, o.etapa, o.notas,
	       o.vendedor_id, u.nombre, o.cuenta_id, o.creado_en, o.actualizado_en
	FROM oportunidad o
	JOIN cliente c ON c.empresa_id = o.empresa_id AND c.id = o.cliente_id
	JOIN usuario u ON u.empresa_id = o.empresa_id AND u.id = o.vendedor_id
	WHERE o.empresa_id = $1 AND ($2::uuid IS NULL OR o.vendedor_id = $2)`

func escanearOportunidad(s interface{ Scan(...any) error }) (Oportunidad, error) {
	var o Oportunidad
	err := s.Scan(&o.ID, &o.ClienteID, &o.ClienteNombre, &o.MontoEstimadoCentavos, &o.Etapa, &o.Notas,
		&o.VendedorID, &o.VendedorNombre, &o.CuentaID, &o.CreadoEn, &o.ActualizadoEn)
	return o, err
}

func (r *Repo) ListarOportunidades(empresaID string, alc Alcance) ([]Oportunidad, error) {
	filas, err := r.q.QueryContext(r.ctx, selectOportunidad+` ORDER BY o.actualizado_en DESC`, empresaID, alc.filtro())
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Oportunidad{}
	for filas.Next() {
		o, err := escanearOportunidad(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, o)
	}
	return res, filas.Err()
}

// ObtenerOportunidad con bloquear=true toma un lock de fila (conversión concurrente).
func (r *Repo) ObtenerOportunidad(empresaID string, alc Alcance, id string, bloquear bool) (Oportunidad, error) {
	q := selectOportunidad + ` AND o.id = $3`
	if bloquear {
		q += ` FOR UPDATE OF o`
	}
	o, err := escanearOportunidad(r.q.QueryRowContext(r.ctx, q, empresaID, alc.filtro(), id))
	return o, noEncontrado(err, errNoEncontrado("OPORTUNIDAD_NO_ENCONTRADA", "No encontramos la oportunidad solicitada."))
}

func (r *Repo) CrearOportunidad(empresaID, clienteID string, monto int64, etapa, notas, vendedorID string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		empresaID, clienteID, monto, etapa, notas, vendedorID).Scan(&id)
	return id, err
}

func (r *Repo) ActualizarOportunidad(empresaID string, o Oportunidad) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE oportunidad SET monto_estimado_centavos = $3, etapa = $4, notas = $5, actualizado_en = now()
		WHERE empresa_id = $1 AND id = $2`,
		empresaID, o.ID, o.MontoEstimadoCentavos, o.Etapa, o.Notas)
	return err
}

// VincularCuentaOportunidad enlaza la cuenta y deja la oportunidad con el mismo cliente de la cuenta
// (la conversión puede emitirse a otro cliente): venta ganada y cuenta nunca quedan desalineadas.
func (r *Repo) VincularCuentaOportunidad(empresaID, oportunidadID, cuentaID string) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE oportunidad o SET cuenta_id = c.id, cliente_id = c.cliente_id, actualizado_en = now()
		FROM cuenta c
		WHERE o.empresa_id = $1 AND o.id = $2 AND o.cuenta_id IS NULL
		  AND c.empresa_id = $1 AND c.id = $3`,
		empresaID, oportunidadID, cuentaID)
	return err
}

func (r *Repo) ReasignarOportunidades(empresaID string, ids []string, destino string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE oportunidad SET vendedor_id = $3, actualizado_en = now()
		WHERE empresa_id = $1 AND id = ANY($2::uuid[])`, empresaID, arregloPG(ids), destino)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Cuentas. El repo suma los movimientos no anulados; saldo y estado los calcula reglas.go.
// ─────────────────────────────────────────────────────────────────────────────

const selectCuenta = `
	SELECT c.id, c.cliente_id, cl.nombre, cl.contacto, cl.telefono, c.oportunidad_id,
	       c.numero_documento, c.monto_total_centavos, c.fecha_emision, c.fecha_vencimiento,
	       c.vendedor_id, u.nombre, c.requiere_revision, c.creado_en,
	       COALESCE(SUM(p.monto_centavos) FILTER (WHERE p.tipo = 'pago' AND NOT p.anulado), 0),
	       COALESCE(SUM(p.monto_centavos) FILTER (WHERE p.tipo = 'condonacion' AND NOT p.anulado), 0),
	       COALESCE(SUM(p.monto_centavos) FILTER (WHERE p.tipo = 'nota_credito' AND NOT p.anulado), 0),
	       MAX(p.fecha) FILTER (WHERE p.tipo = 'pago' AND NOT p.anulado)
	FROM cuenta c
	JOIN cliente cl ON cl.empresa_id = c.empresa_id AND cl.id = c.cliente_id
	JOIN usuario u  ON u.empresa_id = c.empresa_id AND u.id = c.vendedor_id
	LEFT JOIN pago p ON p.empresa_id = c.empresa_id AND p.cuenta_id = c.id
	WHERE c.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)`

const agruparCuenta = ` GROUP BY c.id, cl.id, u.id`

func escanearCuenta(s interface{ Scan(...any) error }) (Cuenta, error) {
	var c Cuenta
	err := s.Scan(&c.ID, &c.ClienteID, &c.ClienteNombre, &c.ClienteContacto, &c.ClienteTelefono, &c.OportunidadID,
		&c.NumeroDocumento, &c.MontoTotalCentavos, &c.FechaEmision, &c.FechaVencimiento, &c.VendedorID,
		&c.VendedorNombre, &c.RequiereRevision, &c.CreadoEn, &c.PagadoCentavos, &c.CondonadoCentavos, &c.NotaCreditoCentavos, &c.UltimoPago)
	return c, err
}

func errCuentaNoEncontrada() *ErrorApp {
	return errNoEncontrado("CUENTA_NO_ENCONTRADA", "No encontramos la cuenta solicitada.")
}

// ListarCuentas devuelve las cuentas del alcance. Si clienteID no es vacío, solo las de ese cliente.
func (r *Repo) ListarCuentas(empresaID string, alc Alcance, clienteID string) ([]Cuenta, error) {
	q := selectCuenta + ` AND ($3::uuid IS NULL OR c.cliente_id = $3)` + agruparCuenta +
		` ORDER BY c.fecha_vencimiento, c.creado_en`
	var filtroCliente any
	if clienteID != "" {
		filtroCliente = clienteID
	}
	filas, err := r.q.QueryContext(r.ctx, q, empresaID, alc.filtro(), filtroCliente)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Cuenta{}
	for filas.Next() {
		c, err := escanearCuenta(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, filas.Err()
}

func (r *Repo) ObtenerCuenta(empresaID string, alc Alcance, id string) (Cuenta, error) {
	c, err := escanearCuenta(r.q.QueryRowContext(r.ctx, selectCuenta+` AND c.id = $3`+agruparCuenta,
		empresaID, alc.filtro(), id))
	return c, noEncontrado(err, errCuentaNoEncontrada())
}

// BloquearCuenta toma el lock de fila de la cuenta antes de operar sobre su saldo, para que dos
// pagos simultáneos no lean el mismo saldo.
func (r *Repo) BloquearCuenta(empresaID string, alc Alcance, id string) error {
	var x string
	err := r.q.QueryRowContext(r.ctx, `
		SELECT id FROM cuenta
		WHERE empresa_id = $1 AND ($2::uuid IS NULL OR vendedor_id = $2) AND id = $3
		FOR UPDATE`, empresaID, alc.filtro(), id).Scan(&x)
	return noEncontrado(err, errCuentaNoEncontrada())
}

func (r *Repo) CrearCuenta(empresaID, clienteID string, oportunidadID *string, monto int64,
	emision, vencimiento Fecha, vendedorID, numeroDocumento string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO cuenta (empresa_id, cliente_id, oportunidad_id, monto_total_centavos,
		                    fecha_emision, fecha_vencimiento, vendedor_id, numero_documento)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
		empresaID, clienteID, textoONil(oportunidadID), monto, emision, vencimiento, vendedorID,
		numeroDocumento).Scan(&id)
	return id, err
}

func (r *Repo) ActualizarMontoCuenta(empresaID, id string, monto int64, requiereRevision bool) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE cuenta SET monto_total_centavos = $3, requiere_revision = $4
		WHERE empresa_id = $1 AND id = $2`, empresaID, id, monto, requiereRevision)
	return err
}

func (r *Repo) MarcarRevisionCuenta(empresaID, id string, requiereRevision bool) error {
	_, err := r.q.ExecContext(r.ctx,
		`UPDATE cuenta SET requiere_revision = $3 WHERE empresa_id = $1 AND id = $2`, empresaID, id, requiereRevision)
	return err
}

func (r *Repo) ReasignarCuentas(empresaID string, ids []string, destino string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.q.ExecContext(r.ctx,
		`UPDATE cuenta SET vendedor_id = $3 WHERE empresa_id = $1 AND id = ANY($2::uuid[])`,
		empresaID, arregloPG(ids), destino)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Movimientos (tabla pago)
// ─────────────────────────────────────────────────────────────────────────────

const selectMovimiento = `
	SELECT p.id, p.cuenta_id, p.tipo, p.numero, p.monto_centavos, p.fecha, p.metodo, p.referencia,
	       p.liquidacion_id, p.anticipo_id, p.nota, p.registrado_por, ur.nombre,
	       p.anulado, p.anulado_por, ua.nombre, p.anulado_en, p.motivo_anulacion, p.creado_en
	FROM pago p
	JOIN cuenta c ON c.empresa_id = p.empresa_id AND c.id = p.cuenta_id
	JOIN usuario ur ON ur.empresa_id = p.empresa_id AND ur.id = p.registrado_por
	LEFT JOIN usuario ua ON ua.empresa_id = p.empresa_id AND ua.id = p.anulado_por
	WHERE p.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)`

func escanearMovimiento(s interface{ Scan(...any) error }) (Movimiento, error) {
	var m Movimiento
	err := s.Scan(&m.ID, &m.CuentaID, &m.Tipo, &m.Numero, &m.MontoCentavos, &m.Fecha, &m.Metodo,
		&m.Referencia, &m.LiquidacionID, &m.AnticipoID, &m.Nota, &m.RegistradoPor,
		&m.RegistradoPorNombre, &m.Anulado, &m.AnuladoPor, &m.AnuladoPorNombre, &m.AnuladoEn,
		&m.MotivoAnulacion, &m.CreadoEn)
	return m, err
}

func (r *Repo) ListarMovimientos(empresaID string, alc Alcance, cuentaID string) ([]Movimiento, error) {
	filas, err := r.q.QueryContext(r.ctx, selectMovimiento+` AND p.cuenta_id = $3 ORDER BY p.fecha, p.creado_en`,
		empresaID, alc.filtro(), cuentaID)
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

func (r *Repo) ObtenerMovimiento(empresaID string, alc Alcance, id string) (Movimiento, error) {
	m, err := escanearMovimiento(r.q.QueryRowContext(r.ctx, selectMovimiento+` AND p.id = $3`, empresaID, alc.filtro(), id))
	return m, noEncontrado(err, errNoEncontrado("MOVIMIENTO_NO_ENCONTRADO", "No encontramos el pago o condonación indicado."))
}

// NuevoMovimiento son los datos de un pago, condonación o nota de crédito.
type NuevoMovimiento struct {
	CuentaID      string
	Tipo          string
	Numero        string // recibo o número de nota de crédito; vacío = sin numerar
	MontoCentavos int64
	Fecha         Fecha
	Metodo        string
	Referencia    string
	Nota          string
	RegistradoPor string
	AnticipoID    *string // si proviene de aplicar un anticipo
}

func (r *Repo) CrearMovimiento(empresaID string, m NuevoMovimiento) (string, error) {
	var id string
	var numero any
	if m.Numero != "" {
		numero = m.Numero
	}
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO pago (empresa_id, cuenta_id, tipo, numero, monto_centavos, fecha, metodo,
		                  referencia, nota, registrado_por, anticipo_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING id`,
		empresaID, m.CuentaID, m.Tipo, numero, m.MontoCentavos, m.Fecha, m.Metodo, m.Referencia,
		m.Nota, m.RegistradoPor, textoONil(m.AnticipoID)).Scan(&id)
	return id, err
}

func (r *Repo) AnularMovimiento(empresaID, id, usuarioID, motivo string) error {
	var motivoDB any
	if motivo != "" {
		motivoDB = motivo
	}
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE pago SET anulado = true, anulado_por = $3, anulado_en = now(), motivo_anulacion = $4
		WHERE empresa_id = $1 AND id = $2 AND NOT anulado`, empresaID, id, usuarioID, motivoDB)
	return err
}

// ─────────────────────────────────────────────────────────────────────────────
// Ajustes de monto
// ─────────────────────────────────────────────────────────────────────────────

const selectAjuste = `
	SELECT a.id, a.cuenta_id, cl.nombre, a.monto_anterior_centavos, a.monto_nuevo_centavos, a.motivo,
	       a.solicitado_por, us.nombre, a.creado_en, a.estado, a.revisado_por, ur.nombre, a.revisado_en
	FROM ajuste_cuenta a
	JOIN cuenta c   ON c.empresa_id = a.empresa_id AND c.id = a.cuenta_id
	JOIN cliente cl ON cl.empresa_id = a.empresa_id AND cl.id = c.cliente_id
	JOIN usuario us ON us.empresa_id = a.empresa_id AND us.id = a.solicitado_por
	LEFT JOIN usuario ur ON ur.empresa_id = a.empresa_id AND ur.id = a.revisado_por
	WHERE a.empresa_id = $1`

func escanearAjuste(s interface{ Scan(...any) error }) (Ajuste, error) {
	var a Ajuste
	err := s.Scan(&a.ID, &a.CuentaID, &a.ClienteNombre, &a.MontoAnteriorCentavos, &a.MontoNuevoCentavos, &a.Motivo,
		&a.SolicitadoPor, &a.SolicitadoPorNombre, &a.CreadoEn, &a.Estado, &a.RevisadoPor, &a.RevisadoPorNombre, &a.RevisadoEn)
	return a, err
}

func (r *Repo) listarAjustes(q string, args ...any) ([]Ajuste, error) {
	filas, err := r.q.QueryContext(r.ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Ajuste{}
	for filas.Next() {
		a, err := escanearAjuste(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, a)
	}
	return res, filas.Err()
}

// ListarAjustes filtra por estado; estado vacío = todos.
func (r *Repo) ListarAjustes(empresaID, estado string) ([]Ajuste, error) {
	var filtro any
	if estado != "" {
		filtro = estado
	}
	return r.listarAjustes(selectAjuste+` AND ($2::text IS NULL OR a.estado = $2) ORDER BY a.creado_en DESC`, empresaID, filtro)
}

func (r *Repo) ListarAjustesDeCuenta(empresaID, cuentaID string) ([]Ajuste, error) {
	return r.listarAjustes(selectAjuste+` AND a.cuenta_id = $2 ORDER BY a.creado_en DESC`, empresaID, cuentaID)
}

func (r *Repo) ObtenerAjuste(empresaID, id string, bloquear bool) (Ajuste, error) {
	q := selectAjuste + ` AND a.id = $2`
	if bloquear {
		q += ` FOR UPDATE OF a`
	}
	a, err := escanearAjuste(r.q.QueryRowContext(r.ctx, q, empresaID, id))
	return a, noEncontrado(err, errNoEncontrado("AJUSTE_NO_ENCONTRADO", "No encontramos el ajuste indicado."))
}

func (r *Repo) CrearAjuste(empresaID, cuentaID string, anterior, nuevo int64, motivo, solicitadoPor, estado string) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO ajuste_cuenta (empresa_id, cuenta_id, monto_anterior_centavos, monto_nuevo_centavos,
		                           motivo, solicitado_por, estado, revisado_en)
		VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $7 = 'pendiente' THEN NULL ELSE now() END)
		RETURNING id`,
		empresaID, cuentaID, anterior, nuevo, motivo, solicitadoPor, estado).Scan(&id)
	return id, err
}

func (r *Repo) ResolverAjuste(empresaID, id, estado, revisadoPor string) error {
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE ajuste_cuenta SET estado = $3, revisado_por = $4, revisado_en = now()
		WHERE empresa_id = $1 AND id = $2 AND estado = 'pendiente'`, empresaID, id, estado, revisadoPor)
	return err
}

func (r *Repo) ContarAjustesPendientes(empresaID, cuentaID string) (int, error) {
	var n int
	var filtro any
	if cuentaID != "" {
		filtro = cuentaID
	}
	err := r.q.QueryRowContext(r.ctx, `
		SELECT count(*) FROM ajuste_cuenta
		WHERE empresa_id = $1 AND estado = 'pendiente' AND ($2::uuid IS NULL OR cuenta_id = $2)`,
		empresaID, filtro).Scan(&n)
	return n, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Reasignación de cartera
// ─────────────────────────────────────────────────────────────────────────────

func (r *Repo) RegistrarReasignacion(empresaID string, x Reasignacion) (Reasignacion, error) {
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO reasignacion_cartera (empresa_id, vendedor_origen, vendedor_destino, cantidad_clientes,
		                                  cantidad_oportunidades, cantidad_cuentas, motivo, realizado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id, creado_en`,
		empresaID, x.VendedorOrigen, x.VendedorDestino, x.CantidadClientes, x.CantidadOportunidades,
		x.CantidadCuentas, x.Motivo, x.RealizadoPor).Scan(&x.ID, &x.CreadoEn)
	return x, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Alertas entregadas
// ─────────────────────────────────────────────────────────────────────────────

func claveAlerta(cuentaID, tipo string) string { return cuentaID + "|" + tipo }

// AlertasEntregadas devuelve el conjunto cuenta|tipo ya entregado en la fecha indicada.
func (r *Repo) AlertasEntregadas(empresaID string, fecha Fecha) (map[string]bool, error) {
	filas, err := r.q.QueryContext(r.ctx,
		`SELECT cuenta_id, tipo FROM alerta_generada WHERE empresa_id = $1 AND fecha_generada = $2`, empresaID, fecha)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := map[string]bool{}
	for filas.Next() {
		var cuentaID, tipo string
		if err := filas.Scan(&cuentaID, &tipo); err != nil {
			return nil, err
		}
		res[claveAlerta(cuentaID, tipo)] = true
	}
	return res, filas.Err()
}

// RegistrarAlerta es idempotente: devuelve false si ya estaba registrada ese día.
func (r *Repo) RegistrarAlerta(empresaID, cuentaID, tipo string, fecha Fecha) (bool, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO alerta_generada (empresa_id, cuenta_id, tipo, fecha_generada)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (cuenta_id, tipo, fecha_generada) DO NOTHING
		RETURNING id`, empresaID, cuentaID, tipo, fecha).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
