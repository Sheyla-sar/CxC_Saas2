package main

// handlers_promesas.go — endpoints de la promesa de pago (fase 4).
// Igual que el resto de handlers: leen la entrada, cargan con el Repo, deciden con
// reglas_promesas.go y traducen a HTTP. Ninguna fórmula de negocio vive aquí.

import (
	"net/http"
	"strings"
)

// rutasPromesas registra las rutas de este grupo. La llama rutasFase4 (rutas_fase4.go).
func (a *App) rutasPromesas(mux *http.ServeMux) {
	mux.Handle("POST /api/clientes/{id}/promesas", a.conTenant(hCrearPromesa))
	mux.Handle("GET /api/promesas", a.conTenant(hListarPromesas))
	mux.Handle("GET /api/promesas/agenda", a.conTenant(hAgendaDePromesas))
	mux.Handle("POST /api/promesas/{id}/anular", a.conTenant(hAnularPromesa))
}

// promesaEvaluada recarga una promesa y le calcula el estado. Se usa después de cada escritura:
// el estado nunca se guarda, siempre se vuelve a calcular con los pagos de la ventana.
func promesaEvaluada(t *Tenant, alc Alcance, id string) (Promesa, error) {
	cfg, err := t.Config()
	if err != nil {
		return Promesa{}, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return Promesa{}, err
	}
	p, err := t.Repo.ObtenerPromesa(t.EmpresaID, alc, cfg.ZonaHoraria, id)
	if err != nil {
		return Promesa{}, err
	}
	EvaluarPromesa(&p, hoy)
	return p, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Anotar una promesa
// ─────────────────────────────────────────────────────────────────────────────

func hCrearPromesa(r *http.Request, t *Tenant) (int, any, error) {
	clienteID, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		FechaPromesa  *Fecha `json:"fecha_promesa"`
		MontoCentavos *int64 `json:"monto_centavos"`
		CuentaID      string `json:"cuenta_id"`
		Nota          string `json:"nota"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.FechaPromesa == nil {
		return 0, nil, errEntrada("FECHA_REQUERIDA", "Indique para qué día se comprometió a pagar el cliente.")
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	// El cliente se busca con el alcance del usuario: un vendedor solo anota promesas de los
	// suyos y, si no es suyo, recibe 404 (R37/R38).
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), clienteID)
	if err != nil {
		return 0, nil, err
	}
	if !cl.Activo {
		return 0, nil, errRegla("CLIENTE_INACTIVO", "El cliente está desactivado.")
	}

	var cuentaID *string
	if strings.TrimSpace(in.CuentaID) != "" {
		id, err := validarID(in.CuentaID, "cuenta_id")
		if err != nil {
			return 0, nil, err
		}
		c, err := t.Repo.ObtenerCuenta(t.EmpresaID, t.Alcance(), id)
		if err != nil {
			return 0, nil, err
		}
		if err := ValidarCuentaDeLaPromesa(c, clienteID); err != nil {
			return 0, nil, err
		}
		cuentaID = &c.ID
	}

	nota, err := ValidarPromesa(*in.FechaPromesa, hoy, in.MontoCentavos, in.Nota, cfg)
	if err != nil {
		return 0, nil, err
	}
	id, err := t.Repo.CrearPromesa(t.EmpresaID, NuevaPromesa{
		ClienteID: clienteID, CuentaID: cuentaID, FechaPromesa: *in.FechaPromesa,
		MontoCentavos: in.MontoCentavos, Nota: nota, RegistradoPor: t.Usuario.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	p, err := promesaEvaluada(t, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, p, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Listado e historial
// ─────────────────────────────────────────────────────────────────────────────

func hListarPromesas(r *http.Request, t *Tenant) (int, any, error) {
	estado := r.URL.Query().Get("estado")
	if estado == "" {
		estado = EstadoTodas
	}
	if estado != EstadoTodas && !EsEstadoPromesaValido(estado) {
		return 0, nil, errEntrada("ESTADO_INVALIDO",
			"El estado debe ser todas, pendiente, cumplida, rota o anulada.").
			con("estados_validos", append([]string{EstadoTodas}, EstadosPromesa...))
	}
	pagina, porPagina, err := ValidarPaginacion(r.URL.Query().Get("pagina"), r.URL.Query().Get("por_pagina"))
	if err != nil {
		return 0, nil, err
	}
	clienteID := strings.TrimSpace(r.URL.Query().Get("cliente_id"))
	if clienteID != "" {
		if clienteID, err = validarID(clienteID, "cliente_id"); err != nil {
			return 0, nil, err
		}
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	// El filtro por estado y la paginación los hace la base (ver repos_promesas.go); lo que
	// vuelve se re-evalúa con EvaluarPromesa, que sigue siendo la única fuente del estado.
	promesas, total, err := t.Repo.ListarPromesas(t.EmpresaID, ConsultaPromesas{
		Alcance: t.Alcance(), Zona: cfg.ZonaHoraria, ClienteID: clienteID, Estado: estado,
		Hoy: hoy, Limite: porPagina, Desde: (pagina - 1) * porPagina,
	})
	if err != nil {
		return 0, nil, err
	}
	for i := range promesas {
		EvaluarPromesa(&promesas[i], hoy)
	}
	return http.StatusOK, map[string]any{
		"estado":     estado,
		"promesas":   promesas,
		"paginacion": ArmarPaginacion(pagina, porPagina, total, len(promesas)),
	}, nil
}

// hAgendaDePromesas: lo que hay que ir a cobrar hoy. Son las promesas que todavía no se
// cumplieron y cuya fecha ya llegó o ya pasó, de la cartera del usuario, de la más atrasada a la
// de hoy. Es la pantalla con la que el cobrador arranca el día.
func hAgendaDePromesas(r *http.Request, t *Tenant) (int, any, error) {
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	promesas, _, err := t.Repo.ListarPromesas(t.EmpresaID, ConsultaPromesas{
		Alcance: t.Alcance(), Zona: cfg.ZonaHoraria, Hoy: hoy, SoloAgenda: true,
	})
	if err != nil {
		return 0, nil, err
	}
	var prometido int64
	for i := range promesas {
		EvaluarPromesa(&promesas[i], hoy)
		if promesas[i].MontoCentavos != nil {
			prometido += *promesas[i].MontoCentavos
		}
	}
	OrdenarAgenda(promesas)
	return http.StatusOK, map[string]any{
		"fecha":                    hoy,
		"promesas":                 promesas,
		"total_prometido_centavos": prometido,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Anulación
// ─────────────────────────────────────────────────────────────────────────────

func hAnularPromesa(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errPromesaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.BloquearPromesa(t.EmpresaID, id); err != nil {
		return 0, nil, err
	}
	p, err := promesaEvaluada(t, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	// Una promesa mal anotada la corrige quien la anotó; el administrador puede corregir
	// cualquiera. Un tercero no borra el registro de lo que otro escuchó en la puerta.
	if !t.EsAdmin() && p.RegistradoPor != t.Usuario.ID {
		return 0, nil, errProhibido("PROMESA_DE_OTRO_USUARIO",
			"Solo quien anotó la promesa o un administrador puede anularla.")
	}
	motivo, err := ValidarAnulacionPromesa(p, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.AnularPromesa(t.EmpresaID, id, t.Usuario.ID, motivo); err != nil {
		return 0, nil, err
	}
	p, err = promesaEvaluada(t, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, p, nil
}
