package main

// handlers_tablero.go — GET /api/tablero: todo lo que necesita la pantalla de indicadores en una
// sola petición. No guarda nada ni expone cifras nuevas: agrega lo que ya está registrado.
// El navegador solo dibuja; ningún total se calcula allá.

import "net/http"

// Cortes de la proyección de cobro, en días. Son tramos de presentación (no reglas de negocio),
// por eso viven aquí y no en empresa_config.
var cortesProyeccion = []int{7, 15, 30}

// diasSerieCobranza es el largo de la serie diaria que se dibuja en el tablero.
const diasSerieCobranza = 30

type totalesCobranza struct {
	HoyCentavos         int64 `json:"hoy_centavos"`
	SemanaCentavos      int64 `json:"semana_centavos"`
	MesCentavos         int64 `json:"mes_centavos"`
	MesAnteriorCentavos int64 `json:"mes_anterior_centavos"`
}

type fugaDeSaldo struct {
	CondonadoCentavos   int64 `json:"condonado_centavos"`
	NotaCreditoCentavos int64 `json:"nota_credito_centavos"`
}

// cobradorTablero mezcla lo que el cobrador recibió con la cartera que tiene asignada y con las
// diferencias de sus liquidaciones: las tres cosas que se miran juntas para evaluarlo.
type cobradorTablero struct {
	UsuarioID          string  `json:"usuario_id"`
	Nombre             string  `json:"nombre"`
	CobradoMesCentavos int64   `json:"cobrado_mes_centavos"`
	CantidadPagos      int     `json:"cantidad_pagos"`
	SaldoCentavos      int64   `json:"cartera_centavos"`
	VencidoCentavos    int64   `json:"vencido_centavos"`
	Clientes           int     `json:"clientes"`
	PorcentajeMora     float64 `json:"porcentaje_mora"`
	DiferenciaCentavos int64   `json:"diferencia_liquidaciones_centavos"`
	TieneLiquidaciones bool    `json:"tiene_liquidaciones"`
}

type respuestaTablero struct {
	Fecha                   Fecha              `json:"fecha"`
	Antiguedad              []TramoAntiguedad  `json:"antiguedad"`
	Proximos                []TramoVencimiento `json:"proximos_vencimientos"`
	Cobranza                totalesCobranza    `json:"cobranza"`
	Serie                   []PuntoCobranza    `json:"serie_cobranza"`
	FugaMes                 fugaDeSaldo        `json:"fuga_mes"`
	Cobradores              []cobradorTablero  `json:"cobradores,omitempty"`
	LiquidacionesPendientes *int               `json:"liquidaciones_pendientes,omitempty"`
}

func hTablero(r *http.Request, t *Tenant) (int, any, error) {
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, t.Alcance(), "")
	if err != nil {
		return 0, nil, err
	}

	inicioMes := hoy.InicioDeMes()
	mesAnterior := inicioMes.SumarMeses(-1)
	desdeSerie := hoy.SumarDias(-(diasSerieCobranza - 1))
	// Una sola lectura cubre la serie y los totales: el período arranca en el día más antiguo
	// que necesite cualquiera de los dos.
	desde := desdeSerie
	if mesAnterior.Antes(desde) {
		desde = mesAnterior
	}
	dias, err := t.Repo.CobranzaPorDia(t.EmpresaID, t.Alcance(), desde, hoy)
	if err != nil {
		return 0, nil, err
	}

	res := respuestaTablero{
		Fecha:      hoy,
		Antiguedad: AntiguedadSaldos(cuentas),
		Proximos:   ProximosVencimientos(cuentas, cortesProyeccion),
		Serie:      []PuntoCobranza{},
	}
	inicioSemana := hoy.SumarDias(-6) // últimos 7 días, incluido hoy
	porDia := map[string]PuntoCobranza{}
	for _, p := range dias {
		porDia[p.Fecha.String()] = p
		if p.Fecha.Igual(hoy) {
			res.Cobranza.HoyCentavos += p.MontoCentavos
		}
		if !p.Fecha.Antes(inicioSemana) {
			res.Cobranza.SemanaCentavos += p.MontoCentavos
		}
		if !p.Fecha.Antes(inicioMes) {
			res.Cobranza.MesCentavos += p.MontoCentavos
		} else if !p.Fecha.Antes(mesAnterior) {
			res.Cobranza.MesAnteriorCentavos += p.MontoCentavos
		}
	}
	// La serie sale completa, con los días sin cobranza en cero: el navegador no arma fechas.
	for f := desdeSerie; !f.Despues(hoy); f = f.SumarDias(1) {
		punto, hubo := porDia[f.String()]
		if !hubo {
			punto = PuntoCobranza{Fecha: f}
		}
		res.Serie = append(res.Serie, punto)
	}

	totales, err := t.Repo.TotalesPorTipo(t.EmpresaID, t.Alcance(), inicioMes, hoy)
	if err != nil {
		return 0, nil, err
	}
	res.FugaMes = fugaDeSaldo{
		CondonadoCentavos:   totales[TipoCondonacion],
		NotaCreditoCentavos: totales[TipoNotaCredito],
	}

	if !t.EsAdmin() {
		return http.StatusOK, res, nil
	}

	// Tablero de cobradores: solo para administración.
	cobrado, err := t.Repo.CobranzaPorUsuario(t.EmpresaID, inicioMes, hoy)
	if err != nil {
		return 0, nil, err
	}
	diferencias, err := t.Repo.DiferenciasPorCobrador(t.EmpresaID, inicioMes)
	if err != nil {
		return 0, nil, err
	}
	carteras := map[string]CarteraVendedor{}
	for _, c := range CarteraPorVendedor(cuentas) {
		carteras[c.VendedorID] = c
	}
	res.Cobradores = []cobradorTablero{}
	for _, c := range cobrado {
		cartera := carteras[c.UsuarioID]
		diferencia, tiene := diferencias[c.UsuarioID]
		res.Cobradores = append(res.Cobradores, cobradorTablero{
			UsuarioID: c.UsuarioID, Nombre: c.Nombre,
			CobradoMesCentavos: c.CobradoCentavos, CantidadPagos: c.CantidadPagos,
			SaldoCentavos: cartera.SaldoCentavos, VencidoCentavos: cartera.VencidoCentavos,
			Clientes: cartera.Clientes, PorcentajeMora: cartera.PorcentajeMora,
			DiferenciaCentavos: diferencia, TieneLiquidaciones: tiene,
		})
	}
	n, err := t.Repo.ContarLiquidacionesEntregadas(t.EmpresaID)
	if err != nil {
		return 0, nil, err
	}
	res.LiquidacionesPendientes = &n
	return http.StatusOK, res, nil
}
