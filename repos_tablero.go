package main

// repos_tablero.go — agregaciones para los tableros. Todo lo que se suma aquí sale de los mismos
// movimientos que ya registra la aplicación: no hay cifras guardadas aparte que se puedan
// desincronizar. Como el resto del repo, cada método recibe empresaID como primer parámetro.

// PuntoCobranza es un día de la serie de cobranza.
type PuntoCobranza struct {
	Fecha         Fecha `json:"fecha"`
	MontoCentavos int64 `json:"monto_centavos"`
	Cantidad      int   `json:"cantidad"`
}

// CobranzaPorDia suma los pagos no anulados de la cartera del alcance, día por día. Se atribuye
// al vendedor DUEÑO de la cuenta (es cobranza de su cartera), no a quien recibió el dinero.
func (r *Repo) CobranzaPorDia(empresaID string, alc Alcance, desde, hasta Fecha) ([]PuntoCobranza, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT p.fecha, SUM(p.monto_centavos), COUNT(*)
		FROM pago p
		JOIN cuenta c ON c.empresa_id = p.empresa_id AND c.id = p.cuenta_id
		WHERE p.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND p.tipo = 'pago' AND NOT p.anulado AND p.fecha BETWEEN $3 AND $4
		GROUP BY p.fecha
		ORDER BY p.fecha`, empresaID, alc.filtro(), desde, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []PuntoCobranza{}
	for filas.Next() {
		var p PuntoCobranza
		if err := filas.Scan(&p.Fecha, &p.MontoCentavos, &p.Cantidad); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, filas.Err()
}

// CobranzaUsuario es lo que un cobrador recibió en el período.
type CobranzaUsuario struct {
	UsuarioID       string `json:"usuario_id"`
	Nombre          string `json:"nombre"`
	CobradoCentavos int64  `json:"cobrado_centavos"`
	CantidadPagos   int    `json:"cantidad_pagos"`
}

// CobranzaPorUsuario agrupa por quien RECIBIÓ el dinero (registrado_por), que es lo que después
// tiene que liquidar. Se excluyen las aplicaciones de anticipo: ahí no entra dinero nuevo.
func (r *Repo) CobranzaPorUsuario(empresaID string, desde, hasta Fecha) ([]CobranzaUsuario, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT u.id, u.nombre,
		       COALESCE(SUM(p.monto_centavos) FILTER (WHERE p.id IS NOT NULL), 0),
		       COUNT(p.id)
		FROM usuario u
		LEFT JOIN pago p ON p.empresa_id = u.empresa_id AND p.registrado_por = u.id
		     AND p.tipo = 'pago' AND NOT p.anulado AND p.anticipo_id IS NULL
		     AND p.fecha BETWEEN $2 AND $3
		WHERE u.empresa_id = $1 AND u.activo
		GROUP BY u.id, u.nombre
		ORDER BY 3 DESC, u.nombre`, empresaID, desde, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []CobranzaUsuario{}
	for filas.Next() {
		var c CobranzaUsuario
		if err := filas.Scan(&c.UsuarioID, &c.Nombre, &c.CobradoCentavos, &c.CantidadPagos); err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, filas.Err()
}

// TotalesPorTipo suma los movimientos del período por tipo (pago, condonacion, nota_credito)
// dentro de la cartera del alcance. Sirve para ver cuánto saldo se fue sin entrar dinero.
func (r *Repo) TotalesPorTipo(empresaID string, alc Alcance, desde, hasta Fecha) (map[string]int64, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT p.tipo, SUM(p.monto_centavos)
		FROM pago p
		JOIN cuenta c ON c.empresa_id = p.empresa_id AND c.id = p.cuenta_id
		WHERE p.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND NOT p.anulado AND p.fecha BETWEEN $3 AND $4
		GROUP BY p.tipo`, empresaID, alc.filtro(), desde, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := map[string]int64{}
	for filas.Next() {
		var tipo string
		var monto int64
		if err := filas.Scan(&tipo, &monto); err != nil {
			return nil, err
		}
		res[tipo] = monto
	}
	return res, filas.Err()
}

// DiferenciasPorCobrador acumula, desde una fecha, lo que faltó o sobró en las liquidaciones de
// cada cobrador (entregado − esperado). Las rechazadas no cuentan: su dinero volvió a quedar
// pendiente de entregar y se contará en la liquidación que lo reemplace.
func (r *Repo) DiferenciasPorCobrador(empresaID string, desde Fecha) (map[string]int64, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT l.cobrador_id, SUM(l.total_entregado_centavos - l.total_esperado_centavos)
		FROM liquidacion l
		WHERE l.empresa_id = $1 AND l.fecha >= $2 AND l.estado <> 'rechazada'
		GROUP BY l.cobrador_id`, empresaID, desde)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := map[string]int64{}
	for filas.Next() {
		var id string
		var diferencia int64
		if err := filas.Scan(&id, &diferencia); err != nil {
			return nil, err
		}
		res[id] = diferencia
	}
	return res, filas.Err()
}

// ContarLiquidacionesEntregadas cuenta las liquidaciones que esperan que un administrador las
// apruebe o las rechace.
func (r *Repo) ContarLiquidacionesEntregadas(empresaID string) (int, error) {
	var n int
	err := r.q.QueryRowContext(r.ctx, `
		SELECT COUNT(*) FROM liquidacion WHERE empresa_id = $1 AND estado = 'entregada'`,
		empresaID).Scan(&n)
	return n, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Informe mensual del dueño
// ─────────────────────────────────────────────────────────────────────────────

// CobranzaPorMetodo desglosa por forma de pago lo cobrado en el período.
func (r *Repo) CobranzaPorMetodo(empresaID string, alc Alcance, desde, hasta Fecha) ([]TotalPorMetodo, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT p.metodo, COUNT(*), SUM(p.monto_centavos)
		FROM pago p
		JOIN cuenta c ON c.empresa_id = p.empresa_id AND c.id = p.cuenta_id
		WHERE p.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND p.tipo = 'pago' AND NOT p.anulado AND p.fecha BETWEEN $3 AND $4
		GROUP BY p.metodo
		ORDER BY 3 DESC`, empresaID, alc.filtro(), desde, hasta)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []TotalPorMetodo{}
	for filas.Next() {
		var t TotalPorMetodo
		if err := filas.Scan(&t.Metodo, &t.Cantidad, &t.MontoCentavos); err != nil {
			return nil, err
		}
		res = append(res, t)
	}
	return res, filas.Err()
}

// SalidaSinCobro es una nota de crédito o una condonación: saldo que se fue sin entrar dinero.
type SalidaSinCobro struct {
	Tipo          string
	Numero        string
	Fecha         Fecha
	ClienteNombre string
	MontoCentavos int64
	Motivo        string
	Autorizo      string
}

// SalidasSinCobro lista las notas de crédito y condonaciones del período, de mayor a menor.
func (r *Repo) SalidasSinCobro(empresaID string, alc Alcance, desde, hasta Fecha, limite int) ([]SalidaSinCobro, error) {
	filas, err := r.q.QueryContext(r.ctx, `
		SELECT p.tipo, COALESCE(p.numero, ''), p.fecha, cl.nombre, p.monto_centavos,
		       COALESCE(p.nota, ''), u.nombre
		FROM pago p
		JOIN cuenta c  ON c.empresa_id = p.empresa_id AND c.id = p.cuenta_id
		JOIN cliente cl ON cl.empresa_id = c.empresa_id AND cl.id = c.cliente_id
		JOIN usuario u ON u.empresa_id = p.empresa_id AND u.id = p.registrado_por
		WHERE p.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND p.tipo IN ('nota_credito', 'condonacion') AND NOT p.anulado
		  AND p.fecha BETWEEN $3 AND $4
		ORDER BY p.monto_centavos DESC
		LIMIT $5`, empresaID, alc.filtro(), desde, hasta, limite)
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []SalidaSinCobro{}
	for filas.Next() {
		var s SalidaSinCobro
		if err := filas.Scan(&s.Tipo, &s.Numero, &s.Fecha, &s.ClienteNombre, &s.MontoCentavos, &s.Motivo, &s.Autorizo); err != nil {
			return nil, err
		}
		res = append(res, s)
	}
	return res, filas.Err()
}

// VentaDelPeriodo es la cartera que se emitió en el período (cuentas nuevas).
type VentaDelPeriodo struct {
	Cuentas        int   `json:"cuentas"`
	MontoCentavos  int64 `json:"monto_centavos"`
	ClientesNuevos int   `json:"clientes_nuevos"`
}

// VentaEmitida cuenta las cuentas creadas y los clientes dados de alta en el período.
func (r *Repo) VentaEmitida(empresaID string, alc Alcance, desde, hasta Fecha) (VentaDelPeriodo, error) {
	var v VentaDelPeriodo
	err := r.q.QueryRowContext(r.ctx, `
		SELECT COUNT(*), COALESCE(SUM(c.monto_total_centavos), 0)
		FROM cuenta c
		WHERE c.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND c.fecha_emision BETWEEN $3 AND $4`,
		empresaID, alc.filtro(), desde, hasta).Scan(&v.Cuentas, &v.MontoCentavos)
	if err != nil {
		return v, err
	}
	err = r.q.QueryRowContext(r.ctx, `
		SELECT COUNT(*)
		FROM cliente cl
		WHERE cl.empresa_id = $1 AND ($2::uuid IS NULL OR cl.vendedor_id = $2)
		  AND cl.creado_en::date BETWEEN $3 AND $4`,
		empresaID, alc.filtro(), desde, hasta).Scan(&v.ClientesNuevos)
	return v, err
}

// LiquidacionesDelPeriodo devuelve las liquidaciones cerradas entre dos fechas.
func (r *Repo) LiquidacionesDelPeriodo(empresaID string, desde, hasta Fecha) ([]Liquidacion, error) {
	filas, err := r.q.QueryContext(r.ctx,
		selectLiquidacion+` AND l.fecha BETWEEN $3 AND $4 ORDER BY l.fecha, l.creado_en`,
		empresaID, nil, desde, hasta)
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
