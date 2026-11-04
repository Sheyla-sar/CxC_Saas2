package main

// handlers_informe.go — GET /api/informe-mensual.pdf: el informe que recibe el dueño cada mes.
// Es el mismo material del tablero, pero impreso y comparado contra el mes anterior, para quien
// no entra al sistema todos los días. Como el resto de los documentos, no guarda nada.

import (
	"fmt"
	"net/http"
	"strings"
)

// Cuántas filas entran en los cuadros de detalle del informe antes de volverse ilegible.
const (
	salidasEnInforme  = 10
	deudoresEnInforme = 10
)

var nombreMes = [...]string{
	"enero", "febrero", "marzo", "abril", "mayo", "junio",
	"julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre",
}

// mesSolicitado interpreta ?mes=AAAA-MM. Sin parámetro, el mes en curso.
func mesSolicitado(r *http.Request, hoy Fecha) (desde, hasta Fecha, etiqueta string, err error) {
	texto := strings.TrimSpace(r.URL.Query().Get("mes"))
	inicio := hoy.InicioDeMes()
	if texto != "" {
		f, errF := ParsearFecha(texto + "-01")
		if errF != nil {
			return desde, hasta, "", errEntrada("MES_INVALIDO", "Indique el mes como AAAA-MM, por ejemplo 2026-09.")
		}
		if err := ValidarFecha(f, "mes"); err != nil {
			return desde, hasta, "", err
		}
		inicio = f.InicioDeMes()
	}
	if inicio.Despues(hoy) {
		return desde, hasta, "", errRegla("MES_FUTURO", "Todavía no hay informe de ese mes.")
	}
	fin := inicio.SumarMeses(1).SumarDias(-1)
	if fin.Despues(hoy) {
		fin = hoy // el mes en curso se informa hasta hoy
	}
	return inicio, fin, etiquetaDeMes(inicio), nil
}

func etiquetaDeMes(f Fecha) string {
	t := f.Tiempo()
	return fmt.Sprintf("%s de %d", nombreMes[int(t.Month())-1], t.Year())
}

func hInformeMensualPDF(r *http.Request, t *Tenant) (int, any, error) {
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	desde, hasta, mes, err := mesSolicitado(r, hoy)
	if err != nil {
		return 0, nil, err
	}
	empresa, marca, cfg, err := datosDeMarca(t)
	if err != nil {
		return 0, nil, err
	}
	alc := t.Alcance()

	cuentas, err := cuentasEvaluadas(t, alc, "")
	if err != nil {
		return 0, nil, err
	}
	resumen := ResumirCartera(cuentas, deudoresEnInforme)

	// Actividad del mes y del mes anterior, para comparar.
	anterior := desde.SumarMeses(-1)
	dias, err := t.Repo.CobranzaPorDia(t.EmpresaID, alc, anterior, hasta)
	if err != nil {
		return 0, nil, err
	}
	var cobradoMes, cobradoAnte int64
	for _, d := range dias {
		switch {
		case !d.Fecha.Antes(desde):
			cobradoMes += d.MontoCentavos
		case !d.Fecha.Antes(anterior):
			cobradoAnte += d.MontoCentavos
		}
	}

	porMetodo, err := t.Repo.CobranzaPorMetodo(t.EmpresaID, alc, desde, hasta)
	if err != nil {
		return 0, nil, err
	}
	salidas, err := t.Repo.SalidasSinCobro(t.EmpresaID, alc, desde, hasta, salidasEnInforme)
	if err != nil {
		return 0, nil, err
	}
	totales, err := t.Repo.TotalesPorTipo(t.EmpresaID, alc, desde, hasta)
	if err != nil {
		return 0, nil, err
	}
	venta, err := t.Repo.VentaEmitida(t.EmpresaID, alc, desde, hasta)
	if err != nil {
		return 0, nil, err
	}
	liquidaciones, err := t.Repo.LiquidacionesDelPeriodo(t.EmpresaID, desde, hasta)
	if err != nil {
		return 0, nil, err
	}

	datos := DatosInformeMensual{
		Empresa: empresa, Marca: marca, Config: cfg,
		Mes: mes, Desde: desde, Hasta: hasta, Hoy: hoy,
		Antiguedad:    AntiguedadSaldos(cuentas),
		SaldoTotal:    resumen.TotalPorCobrarCentavos,
		SaldoVencido:  resumen.TotalVencidoCentavos,
		CobradoMes:    cobradoMes,
		CobradoAnte:   cobradoAnte,
		PorMetodo:     porMetodo,
		Venta:         venta,
		Salidas:       salidas,
		TotalSalidas:  totales[TipoNotaCredito] + totales[TipoCondonacion],
		Deudores:      resumen.TopDeudores,
		Puntos:        PuntosDeAtencion(cuentas, liquidaciones, cfg),
		Liquidaciones: len(liquidaciones),
	}

	// El cuadro de cobradores solo tiene sentido para quien ve a todo el equipo.
	if t.EsAdmin() {
		cobrado, err := t.Repo.CobranzaPorUsuario(t.EmpresaID, desde, hasta)
		if err != nil {
			return 0, nil, err
		}
		diferencias, err := t.Repo.DiferenciasPorCobrador(t.EmpresaID, desde)
		if err != nil {
			return 0, nil, err
		}
		carteras := map[string]CarteraVendedor{}
		for _, c := range CarteraPorVendedor(cuentas) {
			carteras[c.VendedorID] = c
		}
		for _, c := range cobrado {
			cartera := carteras[c.UsuarioID]
			if c.CobradoCentavos == 0 && cartera.SaldoCentavos == 0 {
				continue // ni cobró ni tiene cartera: no aporta nada al informe
			}
			diferencia, tiene := diferencias[c.UsuarioID]
			datos.Cobradores = append(datos.Cobradores, CobradorInforme{
				Nombre: c.Nombre, CobradoCentavos: c.CobradoCentavos,
				CarteraCentavos: cartera.SaldoCentavos, VencidoCentavos: cartera.VencidoCentavos,
				Clientes: cartera.Clientes, Diferencia: diferencia, TieneLiquidado: tiene,
			})
		}
	}

	pdf, err := InformeMensualPDF(datos)
	if err != nil {
		return 0, nil, err
	}
	nombre := fmt.Sprintf("informe-%s.pdf", desde.String()[:7])
	return http.StatusOK, archivoPDF{nombre, pdf}, nil
}
