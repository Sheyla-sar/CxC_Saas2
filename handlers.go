package main

// handlers.go — traducción HTTP. Cada handler: lee la entrada, carga datos con el Repo, delega las
// decisiones en reglas.go y guarda. No contiene umbrales ni fórmulas de negocio.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (a *App) rutas() http.Handler {
	mux := http.NewServeMux()

	// Públicas
	mux.HandleFunc("GET /api/salud", a.salud)
	mux.HandleFunc("GET /api/publico/marca", a.marcaPublica)
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.logout)

	// Autenticadas
	mux.Handle("GET /api/me", a.conTenant(hMe))
	mux.Handle("GET /api/empresa/config", a.conTenant(hLeerConfig))
	mux.Handle("PATCH /api/empresa/config", a.conTenant(soloAdmin(hActualizarConfig)))
	mux.Handle("GET /api/usuarios", a.conTenant(soloAdmin(hListarUsuarios)))
	mux.Handle("POST /api/usuarios", a.conTenant(soloAdmin(hCrearUsuario)))
	mux.Handle("PATCH /api/usuarios/{id}", a.conTenant(soloAdmin(hActualizarUsuario)))
	mux.Handle("POST /api/usuarios/{id}/contrasena", a.conTenant(soloAdmin(hReiniciarPassword)))
	mux.Handle("POST /api/me/contrasena", a.conTenant(hCambiarMiPassword))

	// Cobranza en ruta (fase 3)
	mux.Handle("GET /api/anticipos", a.conTenant(hListarAnticipos))
	mux.Handle("POST /api/clientes/{id}/anticipos", a.conTenant(hCrearAnticipo))
	mux.Handle("POST /api/anticipos/{id}/aplicar", a.conTenant(hAplicarAnticipo))
	mux.Handle("POST /api/anticipos/{id}/anular", a.conTenant(soloAdmin(hAnularAnticipo)))
	mux.Handle("POST /api/cuentas/{id}/nota-credito", a.conTenant(soloAdmin(hNotaCredito)))

	mux.Handle("GET /api/liquidaciones/corte", a.conTenant(hCorteCobrador))
	mux.Handle("POST /api/liquidaciones", a.conTenant(hCerrarLiquidacion))
	mux.Handle("GET /api/liquidaciones", a.conTenant(hListarLiquidaciones))
	mux.Handle("GET /api/liquidaciones/{id}", a.conTenant(hObtenerLiquidacion))
	mux.Handle("POST /api/liquidaciones/{id}/aprobar", a.conTenant(soloAdmin(hAprobarLiquidacion)))
	mux.Handle("POST /api/liquidaciones/{id}/rechazar", a.conTenant(soloAdmin(hRechazarLiquidacion)))

	// Adjuntos (fotos de recibos y facturas)
	mux.Handle("GET /api/adjuntos", a.conTenant(hListarAdjuntos))
	mux.Handle("POST /api/adjuntos", a.conTenant(hSubirAdjunto))
	mux.Handle("DELETE /api/adjuntos/{id}", a.conTenant(soloAdmin(hEliminarAdjunto)))
	mux.Handle("GET /api/adjuntos/{id}/archivo", a.conTenantDirecto(a.descargarAdjunto))
	mux.Handle("GET /api/adjuntos/{id}/documento.pdf", a.conTenant(hComprobantePDF))
	mux.Handle("GET /api/comprobantes.pdf", a.conTenant(hComprobantesPDF))

	// Documentos
	mux.Handle("GET /api/movimientos/{id}/recibo.pdf", a.conTenant(hReciboPDF))
	mux.Handle("GET /api/clientes/{id}/estado-cuenta.pdf", a.conTenant(hEstadoCuentaPDF))
	mux.Handle("GET /api/informe-mensual.pdf", a.conTenant(hInformeMensualPDF))

	// Importación de cartera
	mux.Handle("GET /api/importacion/plantilla.csv", a.conTenant(soloAdmin(hPlantillaImportacion)))
	mux.Handle("POST /api/importacion/cartera", a.conTenant(soloAdmin(hImportarCartera)))

	a.rutasFase4(mux)

	mux.Handle("GET /api/resumen", a.conTenant(hResumen))
	mux.Handle("GET /api/tablero", a.conTenant(hTablero))

	mux.Handle("GET /api/clientes", a.conTenant(hListarClientes))
	mux.Handle("POST /api/clientes", a.conTenant(hCrearCliente))
	mux.Handle("GET /api/clientes/{id}", a.conTenant(hObtenerCliente))
	mux.Handle("PATCH /api/clientes/{id}", a.conTenant(hActualizarCliente))
	mux.Handle("POST /api/clientes/{id}/desactivar", a.conTenant(hDesactivarCliente))

	mux.Handle("GET /api/oportunidades", a.conTenant(hListarOportunidades))
	mux.Handle("POST /api/oportunidades", a.conTenant(hCrearOportunidad))
	mux.Handle("PATCH /api/oportunidades/{id}", a.conTenant(hActualizarOportunidad))
	mux.Handle("POST /api/oportunidades/{id}/convertir", a.conTenant(hConvertirOportunidad))

	mux.Handle("GET /api/cuentas", a.conTenant(hListarCuentas))
	mux.Handle("POST /api/cuentas", a.conTenant(hCrearCuenta))
	mux.Handle("GET /api/cuentas/{id}", a.conTenant(hObtenerCuenta))
	mux.Handle("PATCH /api/cuentas/{id}/monto", a.conTenant(hEditarMonto))
	mux.Handle("POST /api/cuentas/{id}/pagos", a.conTenant(hRegistrarPago))
	mux.Handle("POST /api/cuentas/{id}/condonar", a.conTenant(soloAdmin(hCondonar)))

	mux.Handle("POST /api/movimientos/{id}/anular", a.conTenant(soloAdmin(hAnularMovimiento)))

	mux.Handle("GET /api/ajustes", a.conTenant(soloAdmin(hListarAjustes)))
	mux.Handle("POST /api/ajustes/{id}/aprobar", a.conTenant(soloAdmin(hAprobarAjuste)))
	mux.Handle("POST /api/ajustes/{id}/rechazar", a.conTenant(soloAdmin(hRechazarAjuste)))

	mux.Handle("POST /api/vendedores/{id}/reasignar-cartera", a.conTenant(soloAdmin(hReasignarCartera)))

	mux.Handle("GET /api/alertas", a.conTenant(hListarAlertas))
	mux.Handle("POST /api/alertas/marcar", a.conTenant(hMarcarAlertas))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		escribirError(w, errNoEncontrado("RUTA_NO_ENCONTRADA", "La dirección solicitada no existe."))
	})
	return conRegistro(mux)
}

// ─────────────────────────────────────────────────────────────────────────────
// Utilidades de entrada
// ─────────────────────────────────────────────────────────────────────────────

var patronUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// idDeRuta valida el {id} de la ruta. Un id mal formado se trata como inexistente (404).
func idDeRuta(r *http.Request, siNoExiste *ErrorApp) (string, error) {
	id := r.PathValue("id")
	if !patronUUID.MatchString(id) {
		return "", siNoExiste
	}
	return strings.ToLower(id), nil
}

// validarID valida un id recibido en el cuerpo (400 si está mal formado).
func validarID(id, campo string) (string, error) {
	id = strings.TrimSpace(id)
	if !patronUUID.MatchString(id) {
		return "", errEntrada("ID_INVALIDO", fmt.Sprintf("El campo %q debe ser un identificador válido.", campo)).con("campo", campo)
	}
	return strings.ToLower(id), nil
}

// leerJSON decodifica el cuerpo rechazando campos desconocidos. Un cuerpo vacío se acepta (cada
// handler valida sus campos obligatorios).
func leerJSON(r *http.Request, destino any) error {
	cuerpo, err := io.ReadAll(r.Body) // limitado a 1 MB por conRegistro
	if err != nil {
		return traducirErrorJSON(err)
	}
	// Postgres no admite el carácter nulo en texto: sin esta revisión llegaba como error 500.
	if bytes.Contains(bytes.ToLower(cuerpo), []byte(`\u0000`)) {
		return errEntrada("TEXTO_INVALIDO", "El texto enviado contiene caracteres no permitidos.")
	}
	dec := json.NewDecoder(bytes.NewReader(cuerpo))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return traducirErrorJSON(err)
	}
	if dec.More() {
		return errEntrada("JSON_INVALIDO", "El cuerpo de la petición debe contener un solo objeto JSON.")
	}
	return nil
}

func traducirErrorJSON(err error) error {
	var app *ErrorApp
	var sintaxis *json.SyntaxError
	var tipo *json.UnmarshalTypeError
	var tamano *http.MaxBytesError
	switch {
	case errors.As(err, &app):
		return app
	case errors.As(err, &tamano):
		return errEntrada("CUERPO_DEMASIADO_GRANDE", "El cuerpo de la petición es demasiado grande.")
	case errors.As(err, &tipo) && tipo.Field == "":
		return errEntrada("JSON_INVALIDO", "El cuerpo de la petición debe ser un objeto JSON.")
	case errors.As(err, &tipo):
		return errEntrada("TIPO_INVALIDO", fmt.Sprintf("El campo %q tiene un tipo de dato incorrecto.", tipo.Field)).con("campo", tipo.Field)
	case errors.As(err, &sintaxis), errors.Is(err, io.ErrUnexpectedEOF):
		return errEntrada("JSON_INVALIDO", "El cuerpo de la petición no es un JSON válido.")
	case strings.Contains(err.Error(), "unknown field"):
		campo := err.Error()[strings.Index(err.Error(), "unknown field")+len("unknown field"):]
		campo = strings.Trim(strings.TrimSpace(campo), `"`)
		return errEntrada("CAMPO_DESCONOCIDO", fmt.Sprintf("El campo %q no es reconocido.", campo)).con("campo", campo)
	}
	return errEntrada("JSON_INVALIDO", "El cuerpo de la petición no es un JSON válido.")
}

// ─────────────────────────────────────────────────────────────────────────────
// Públicos y sesión
// ─────────────────────────────────────────────────────────────────────────────

type respuestaMarcaPublica struct {
	Slug            string `json:"slug"`
	NombreComercial string `json:"nombre_comercial"`
	Marca
}

// marcaPublica: sin sesión, para pintar la pantalla de login de cada empresa.
func (a *App) marcaPublica(w http.ResponseWriter, r *http.Request) {
	slug := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("slug")))
	if slug == "" {
		slug = slugSolicitado(r, a.cfg.DominioBase)
	}
	if slug == "" {
		escribirError(w, errEntrada("SLUG_REQUERIDO", "Indique la empresa con el parámetro slug."))
		return
	}
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		escribirError(w, err)
		return
	}
	defer tx.Rollback()
	empresa, err := buscarEmpresaActivaPorSlug(ctx, tx, slug)
	if err != nil {
		escribirError(w, err)
		return
	}
	if err := establecerEmpresa(ctx, tx, empresa.ID); err != nil {
		escribirError(w, err)
		return
	}
	marca, err := (&Repo{ctx: ctx, q: tx}).ObtenerMarca(empresa.ID)
	if err != nil {
		escribirError(w, err)
		return
	}
	escribirJSON(w, http.StatusOK, respuestaMarcaPublica{empresa.Slug, empresa.NombreComercial, marca})
}

func hMe(r *http.Request, t *Tenant) (int, any, error) {
	e, err := t.Repo.ObtenerEmpresa(t.EmpresaID)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, respuestaSesion{t.Usuario, empresaPublica{e.ID, e.Slug, e.NombreComercial}}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Configuración de la empresa
// ─────────────────────────────────────────────────────────────────────────────

type respuestaConfig struct {
	Empresa Empresa       `json:"empresa"`
	Marca   Marca         `json:"marca"`
	Config  ConfigEmpresa `json:"config"`
	// Datos derivados para que la interfaz se adapte sin reglas propias.
	EtapasOportunidad []string `json:"etapas_oportunidad"`
	EstadosCuenta     []string `json:"estados_cuenta"`
}

func leerConfigCompleta(t *Tenant) (respuestaConfig, error) {
	e, err := t.Repo.ObtenerEmpresa(t.EmpresaID)
	if err != nil {
		return respuestaConfig{}, err
	}
	m, err := t.Repo.ObtenerMarca(t.EmpresaID)
	if err != nil {
		return respuestaConfig{}, err
	}
	c, err := t.Config()
	if err != nil {
		return respuestaConfig{}, err
	}
	return respuestaConfig{e, m, c, EtapasOportunidad, EstadosCuenta}, nil
}

func hLeerConfig(r *http.Request, t *Tenant) (int, any, error) {
	res, err := leerConfigCompleta(t)
	return http.StatusOK, res, err
}

// hActualizarConfig aplica un PATCH parcial: se parte de los valores actuales y solo se pisan los
// campos enviados. Los nombres de campo son los mismos que devuelve el GET.
func hActualizarConfig(r *http.Request, t *Tenant) (int, any, error) {
	var in struct {
		Marca  json.RawMessage `json:"marca"`
		Config json.RawMessage `json:"config"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if len(in.Marca) == 0 && len(in.Config) == 0 {
		return 0, nil, errEntrada("SIN_CAMBIOS", "Envíe al menos uno de los bloques 'marca' o 'config'.")
	}
	if len(in.Marca) > 0 {
		m, err := t.Repo.ObtenerMarca(t.EmpresaID)
		if err != nil {
			return 0, nil, err
		}
		if err := decodificarParcial(in.Marca, &m); err != nil {
			return 0, nil, err
		}
		if m, err = ValidarMarca(m); err != nil {
			return 0, nil, err
		}
		if err := t.Repo.GuardarMarca(t.EmpresaID, m); err != nil {
			return 0, nil, err
		}
	}
	if len(in.Config) > 0 {
		c, err := t.Config()
		if err != nil {
			return 0, nil, err
		}
		if err := decodificarParcial(in.Config, &c); err != nil {
			return 0, nil, err
		}
		if c, err = ValidarConfig(c); err != nil {
			return 0, nil, err
		}
		if err := t.Repo.GuardarConfig(t.EmpresaID, c); err != nil {
			return 0, nil, err
		}
		t.cfg = nil
	}
	res, err := leerConfigCompleta(t)
	return http.StatusOK, res, err
}

// decodificarParcial aplica un objeto JSON sobre un struct existente, rechazando campos
// desconocidos. Los campos no enviados conservan su valor.
func decodificarParcial(raw json.RawMessage, destino any) error {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		return traducirErrorJSON(err)
	}
	return nil
}

func hListarUsuarios(r *http.Request, t *Tenant) (int, any, error) {
	us, err := t.Repo.ListarUsuarios(t.EmpresaID)
	return http.StatusOK, map[string]any{"usuarios": us}, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Carga de cuentas evaluadas (saldo/estado calculados por reglas.go)
// ─────────────────────────────────────────────────────────────────────────────

func cuentasEvaluadas(t *Tenant, alc Alcance, clienteID string) ([]Cuenta, error) {
	cfg, err := t.Config()
	if err != nil {
		return nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return nil, err
	}
	cuentas, err := t.Repo.ListarCuentas(t.EmpresaID, alc, clienteID)
	if err != nil {
		return nil, err
	}
	for i := range cuentas {
		EvaluarCuenta(&cuentas[i], hoy, cfg)
	}
	return cuentas, nil
}

func cuentaEvaluada(t *Tenant, alc Alcance, id string) (Cuenta, error) {
	cfg, err := t.Config()
	if err != nil {
		return Cuenta{}, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return Cuenta{}, err
	}
	c, err := t.Repo.ObtenerCuenta(t.EmpresaID, alc, id)
	if err != nil {
		return Cuenta{}, err
	}
	EvaluarCuenta(&c, hoy, cfg)
	return c, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Resumen
// ─────────────────────────────────────────────────────────────────────────────

type respuestaResumen struct {
	Fecha Fecha `json:"fecha"`
	Resumen
	AjustesPendientes *int `json:"ajustes_pendientes_revision,omitempty"`
}

func hResumen(r *http.Request, t *Tenant) (int, any, error) {
	top := 5
	if v := r.URL.Query().Get("top"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 50 {
			return 0, nil, errEntrada("PARAMETRO_INVALIDO", "El parámetro top debe ser un número entre 1 y 50.")
		}
		top = n
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, t.Alcance(), "")
	if err != nil {
		return 0, nil, err
	}
	res := respuestaResumen{Fecha: hoy, Resumen: ResumirCartera(cuentas, top)}
	if t.EsAdmin() {
		n, err := t.Repo.ContarAjustesPendientes(t.EmpresaID, "")
		if err != nil {
			return 0, nil, err
		}
		res.AjustesPendientes = &n
	}
	return http.StatusOK, res, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Clientes
// ─────────────────────────────────────────────────────────────────────────────

type clienteConSaldo struct {
	Cliente
	SaldoCliente
	// CuentasAbiertasLista solo viaja con ?con_cuentas=true (la pantalla del cobrador, que
	// muestra las cuentas debajo de cada cliente). Sin el parámetro, no se envía.
	CuentasAbiertasLista []CuentaEnCartera `json:"cuentas_abiertas_lista,omitempty"`
}

func hListarClientes(r *http.Request, t *Tenant) (int, any, error) {
	incluirInactivos := r.URL.Query().Get("incluir_inactivos") == "true"
	conCuentas := r.URL.Query().Get("con_cuentas") == "true"
	pagina, porPagina, err := ValidarPaginacion(r.URL.Query().Get("pagina"), r.URL.Query().Get("por_pagina"))
	if err != nil {
		return 0, nil, err
	}
	clientes, total, err := t.Repo.ListarClientesPagina(t.EmpresaID, t.Alcance(), incluirInactivos,
		r.URL.Query().Get("buscar"), porPagina, (pagina-1)*porPagina)
	if err != nil {
		return 0, nil, err
	}
	// Solo se leen las cuentas de los clientes de ESTA página; el saldo de cada uno lo sigue
	// calculando reglas.go, no la base.
	ids := make([]string, 0, len(clientes))
	for _, c := range clientes {
		ids = append(ids, c.ID)
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := t.Repo.CuentasDeClientes(t.EmpresaID, t.Alcance(), ids)
	if err != nil {
		return 0, nil, err
	}
	for i := range cuentas {
		EvaluarCuenta(&cuentas[i], hoy, cfg)
	}
	saldos := SaldosPorCliente(cuentas)
	abiertas := map[string][]CuentaEnCartera{}
	if conCuentas {
		for _, c := range cuentas {
			if c.SaldoCentavos > 0 {
				abiertas[c.ClienteID] = append(abiertas[c.ClienteID], c.EnCartera())
			}
		}
	}
	res := make([]clienteConSaldo, 0, len(clientes))
	for _, c := range clientes {
		fila := clienteConSaldo{Cliente: c, SaldoCliente: saldos[c.ID]}
		if conCuentas {
			fila.CuentasAbiertasLista = abiertas[c.ID]
			if fila.CuentasAbiertasLista == nil {
				fila.CuentasAbiertasLista = []CuentaEnCartera{}
			}
		}
		res = append(res, fila)
	}
	return http.StatusOK, map[string]any{
		"clientes":   res,
		"paginacion": ArmarPaginacion(pagina, porPagina, total, len(clientes)),
	}, nil
}

type entradaCliente struct {
	Nombre                *string `json:"nombre"`
	Contacto              *string `json:"contacto"`
	Telefono              *string `json:"telefono"`
	PlazoCreditoDias      *int    `json:"plazo_credito_dias"`
	LimiteCreditoCentavos *int64  `json:"limite_credito_centavos"`
	VendedorID            *string `json:"vendedor_id"`
	Activo                *bool   `json:"activo"`
}

// validarVendedorAsignable comprueba que el vendedor exista en la empresa y esté activo.
func validarVendedorAsignable(t *Tenant, id string) error {
	id, err := validarID(id, "vendedor_id")
	if err != nil {
		return err
	}
	u, err := t.Repo.ObtenerUsuario(t.EmpresaID, id)
	if err != nil {
		return err
	}
	return ValidarAsignable(u)
}

func hCrearCliente(r *http.Request, t *Tenant) (int, any, error) {
	var in entradaCliente
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	d := DatosCliente{Nombre: valor(in.Nombre), Contacto: valor(in.Contacto), Telefono: valor(in.Telefono),
		PlazoCreditoDias: valor(in.PlazoCreditoDias), LimiteCreditoCentavos: valor(in.LimiteCreditoCentavos)}
	if in.LimiteCreditoCentavos != nil && *in.LimiteCreditoCentavos != 0 && !t.EsAdmin() {
		return 0, nil, errSoloAdmin()
	}
	if d, err = ValidarDatosCliente(d, cfg); err != nil {
		return 0, nil, err
	}
	vendedorID, err := ElegirVendedor(t.Usuario, in.VendedorID, t.Usuario.ID)
	if err != nil {
		return 0, nil, err
	}
	if err := validarVendedorAsignable(t, vendedorID); err != nil {
		return 0, nil, err
	}
	id, err := t.Repo.CrearCliente(t.EmpresaID, d, vendedorID)
	if err != nil {
		return 0, nil, err
	}
	c, err := t.Repo.ObtenerCliente(t.EmpresaID, AlcanceEmpresa(), id)
	return http.StatusCreated, c, err
}

type respuestaCliente struct {
	Cliente Cliente `json:"cliente"`
	SaldoCliente
	Credito        CreditoCliente `json:"credito"`
	Comportamiento Comportamiento `json:"comportamiento_pago"`
	Cuentas        []Cuenta       `json:"cuentas"`
}

func hObtenerCliente(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	// Comportamiento y crédito son propiedades del cliente: se calculan con todas sus cuentas.
	todas, err := cuentasEvaluadas(t, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	saldo := SaldosPorCliente(todas)[id]
	visibles := todas
	if !t.EsAdmin() {
		visibles = []Cuenta{}
		for _, c := range todas {
			if c.VendedorID == t.Usuario.ID {
				visibles = append(visibles, c)
			}
		}
	}
	return http.StatusOK, respuestaCliente{
		Cliente:        cl,
		SaldoCliente:   saldo,
		Credito:        CalcularCredito(cl.LimiteCreditoCentavos, saldo.SaldoCentavos),
		Comportamiento: CalcularComportamiento(todas, cfg),
		Cuentas:        visibles,
	}, nil
}

func hActualizarCliente(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in entradaCliente
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	// Lock: la desactivación (y el nuevo límite) no pueden cruzarse con una venta en curso.
	if err := t.Repo.BloquearCliente(t.EmpresaID, t.Alcance(), id); err != nil {
		return 0, nil, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if !t.EsAdmin() && ((in.LimiteCreditoCentavos != nil && *in.LimiteCreditoCentavos != cl.LimiteCreditoCentavos) ||
		(in.VendedorID != nil && !strings.EqualFold(*in.VendedorID, cl.VendedorID))) {
		return 0, nil, errSoloAdmin()
	}

	d := DatosCliente{
		Nombre: valorO(in.Nombre, cl.Nombre), Contacto: valorO(in.Contacto, cl.Contacto),
		Telefono: valorO(in.Telefono, cl.Telefono), PlazoCreditoDias: valorO(in.PlazoCreditoDias, cl.PlazoCreditoDias),
		LimiteCreditoCentavos: valorO(in.LimiteCreditoCentavos, cl.LimiteCreditoCentavos),
	}
	if d, err = ValidarDatosCliente(d, cfg); err != nil {
		return 0, nil, err
	}
	vendedorID := valorO(in.VendedorID, cl.VendedorID)
	if vendedorID != cl.VendedorID {
		if err := validarVendedorAsignable(t, vendedorID); err != nil {
			return 0, nil, err
		}
	}
	activo := valorO(in.Activo, cl.Activo)
	if cl.Activo && !activo {
		if err := validarDesactivacion(t, id, cfg); err != nil {
			return 0, nil, err
		}
	}
	if err := t.Repo.ActualizarCliente(t.EmpresaID, id, d, strings.ToLower(vendedorID), activo); err != nil {
		return 0, nil, err
	}
	c, err := t.Repo.ObtenerCliente(t.EmpresaID, AlcanceEmpresa(), id)
	return http.StatusOK, c, err
}

func validarDesactivacion(t *Tenant, clienteID string, cfg ConfigEmpresa) error {
	cuentas, err := cuentasEvaluadas(t, AlcanceEmpresa(), clienteID)
	if err != nil {
		return err
	}
	return ValidarDesactivacion(SaldosPorCliente(cuentas)[clienteID], cfg)
}

func hDesactivarCliente(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	// Lock antes de revisar el saldo: ninguna venta puede entrar entre la revisión y la desactivación.
	if err := t.Repo.BloquearCliente(t.EmpresaID, t.Alcance(), id); err != nil {
		return 0, nil, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if err := validarDesactivacion(t, id, cfg); err != nil {
		return 0, nil, err
	}
	d := DatosCliente{
		Nombre: cl.Nombre, Contacto: cl.Contacto, Telefono: cl.Telefono,
		PlazoCreditoDias: cl.PlazoCreditoDias, LimiteCreditoCentavos: cl.LimiteCreditoCentavos,
		CodigoExterno: cl.CodigoExterno,
	}
	if err := t.Repo.ActualizarCliente(t.EmpresaID, id, d, cl.VendedorID, false); err != nil {
		return 0, nil, err
	}
	cl.Activo = false
	return http.StatusOK, cl, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Oportunidades
// ─────────────────────────────────────────────────────────────────────────────

type grupoEtapa struct {
	Etapa                 string        `json:"etapa"`
	Cantidad              int           `json:"cantidad"`
	TotalEstimadoCentavos int64         `json:"total_estimado_centavos"`
	Oportunidades         []Oportunidad `json:"oportunidades"`
}

func hListarOportunidades(r *http.Request, t *Tenant) (int, any, error) {
	ops, err := t.Repo.ListarOportunidades(t.EmpresaID, t.Alcance())
	if err != nil {
		return 0, nil, err
	}
	grupos := make([]grupoEtapa, len(EtapasOportunidad))
	indice := map[string]int{}
	for i, e := range EtapasOportunidad {
		grupos[i] = grupoEtapa{Etapa: e, Oportunidades: []Oportunidad{}}
		indice[e] = i
	}
	for _, o := range ops {
		g := &grupos[indice[o.Etapa]]
		g.Oportunidades = append(g.Oportunidades, o)
		g.Cantidad++
		g.TotalEstimadoCentavos += o.MontoEstimadoCentavos
	}
	return http.StatusOK, map[string]any{"etapas": grupos}, nil
}

func hCrearOportunidad(r *http.Request, t *Tenant) (int, any, error) {
	var in struct {
		ClienteID             string  `json:"cliente_id"`
		MontoEstimadoCentavos int64   `json:"monto_estimado_centavos"`
		Etapa                 string  `json:"etapa"`
		Notas                 string  `json:"notas"`
		VendedorID            *string `json:"vendedor_id"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	clienteID, err := validarID(in.ClienteID, "cliente_id")
	if err != nil {
		return 0, nil, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), clienteID)
	if err != nil {
		return 0, nil, err
	}
	if !cl.Activo {
		return 0, nil, errRegla("CLIENTE_INACTIVO", "El cliente está desactivado.")
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	notas, err := ValidarDatosOportunidad(in.MontoEstimadoCentavos, in.Notas, cfg)
	if err != nil {
		return 0, nil, err
	}
	if in.Etapa == "" {
		in.Etapa = EtapaNuevo
	}
	if err := ValidarCambioEtapa(Oportunidad{}, in.Etapa); err != nil {
		return 0, nil, err
	}
	if in.Etapa == EtapaGanado {
		return 0, nil, errEntrada("ETAPA_INICIAL_INVALIDA", "Cree la oportunidad en una etapa abierta y luego márquela como ganada.")
	}
	vendedorID, err := ElegirVendedor(t.Usuario, in.VendedorID, cl.VendedorID)
	if err != nil {
		return 0, nil, err
	}
	if err := validarVendedorAsignable(t, vendedorID); err != nil {
		return 0, nil, err
	}
	id, err := t.Repo.CrearOportunidad(t.EmpresaID, clienteID, in.MontoEstimadoCentavos, in.Etapa,
		notas, strings.ToLower(vendedorID))
	if err != nil {
		return 0, nil, err
	}
	o, err := t.Repo.ObtenerOportunidad(t.EmpresaID, AlcanceEmpresa(), id, false)
	return http.StatusCreated, o, err
}

type respuestaOportunidad struct {
	Oportunidad Oportunidad `json:"oportunidad"`
	// Conversion trae los datos precargados cuando la oportunidad queda ganada y sin cuenta (R7).
	Conversion *Precarga `json:"conversion"`
}

func hActualizarOportunidad(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("OPORTUNIDAD_NO_ENCONTRADA", "No encontramos la oportunidad solicitada."))
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Etapa                 *string `json:"etapa"`
		Notas                 *string `json:"notas"`
		MontoEstimadoCentavos *int64  `json:"monto_estimado_centavos"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	op, err := t.Repo.ObtenerOportunidad(t.EmpresaID, t.Alcance(), id, true)
	if err != nil {
		return 0, nil, err
	}
	if in.Etapa != nil {
		if err := ValidarCambioEtapa(op, *in.Etapa); err != nil {
			return 0, nil, err
		}
		op.Etapa = *in.Etapa
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	op.MontoEstimadoCentavos = valorO(in.MontoEstimadoCentavos, op.MontoEstimadoCentavos)
	if op.Notas, err = ValidarDatosOportunidad(op.MontoEstimadoCentavos, valorO(in.Notas, op.Notas), cfg); err != nil {
		return 0, nil, err
	}
	if err := t.Repo.ActualizarOportunidad(t.EmpresaID, op); err != nil {
		return 0, nil, err
	}
	op, err = t.Repo.ObtenerOportunidad(t.EmpresaID, AlcanceEmpresa(), id, false)
	if err != nil {
		return 0, nil, err
	}
	res := respuestaOportunidad{Oportunidad: op}
	if op.Etapa == EtapaGanado && op.CuentaID == nil {
		cl, err := t.Repo.ObtenerCliente(t.EmpresaID, AlcanceEmpresa(), op.ClienteID)
		if err != nil {
			return 0, nil, err
		}
		cfg, err := t.Config()
		if err != nil {
			return 0, nil, err
		}
		hoy, err := t.Hoy()
		if err != nil {
			return 0, nil, err
		}
		p := PrecargarConversion(op, cl, hoy, cfg)
		res.Conversion = &p
	}
	return http.StatusOK, res, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Cuentas
// ─────────────────────────────────────────────────────────────────────────────

type entradaCuenta struct {
	ClienteID          string  `json:"cliente_id"`
	NumeroDocumento    string  `json:"numero_documento"`
	MontoTotalCentavos *int64  `json:"monto_total_centavos"`
	FechaEmision       *Fecha  `json:"fecha_emision"`
	FechaVencimiento   *Fecha  `json:"fecha_vencimiento"`
	VendedorID         *string `json:"vendedor_id"`
}

// esDocumentoDuplicado detecta el choque con el índice único de número de documento.
func esDocumentoDuplicado(err error) bool {
	return err != nil && strings.Contains(err.Error(), "idx_cuenta_documento_unico")
}

type respuestaCuentaCreada struct {
	Cuenta      Cuenta             `json:"cuenta"`
	Advertencia *AdvertenciaLimite `json:"advertencia"`
}

// crearCuenta es el camino único para crear cuentas: venta directa (R10) o conversión (R8).
func crearCuenta(t *Tenant, clienteID string, monto int64, emision, vencimiento *Fecha,
	vendedorSolicitado *string, vendedorPorDefecto string, oportunidadID *string,
	numeroDocumento string) (respuestaCuentaCreada, error) {
	var res respuestaCuentaCreada
	cfg, err := t.Config()
	if err != nil {
		return res, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return res, err
	}
	// Lock del cliente antes de leer su deuda: dos ventas simultáneas no pueden ver el mismo cupo.
	if err := t.Repo.BloquearCliente(t.EmpresaID, t.Alcance(), clienteID); err != nil {
		return res, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), clienteID)
	if err != nil {
		return res, err
	}
	// El límite de crédito es del cliente: considera todas sus cuentas, sin importar el vendedor.
	delCliente, err := cuentasEvaluadas(t, AlcanceEmpresa(), clienteID)
	if err != nil {
		return res, err
	}
	plan, err := PlanificarCuenta(SolicitudCuenta{
		Cliente: cl, MontoCentavos: monto, FechaEmision: emision, FechaVencimiento: vencimiento,
		SaldoAbiertoCliente: SaldosPorCliente(delCliente)[clienteID].SaldoCentavos, Hoy: hoy,
	}, cfg)
	if err != nil {
		return res, err
	}
	if vendedorPorDefecto == "" {
		vendedorPorDefecto = cl.VendedorID
	}
	vendedorID, err := ElegirVendedor(t.Usuario, vendedorSolicitado, vendedorPorDefecto)
	if err != nil {
		return res, err
	}
	if err := validarVendedorAsignable(t, vendedorID); err != nil {
		return res, err
	}
	documento, err := ValidarTexto(numeroDocumento, "número de documento", LargoMaximoCodigo)
	if err != nil {
		return res, err
	}
	id, err := t.Repo.CrearCuenta(t.EmpresaID, clienteID, oportunidadID, monto, plan.FechaEmision,
		plan.FechaVencimiento, strings.ToLower(vendedorID), documento)
	if err != nil {
		if esDocumentoDuplicado(err) {
			return res, errRegla("DOCUMENTO_DUPLICADO",
				fmt.Sprintf("Ya existe una cuenta con el documento %s.", documento)).con("numero_documento", documento)
		}
		return res, err
	}
	if oportunidadID != nil {
		if err := t.Repo.VincularCuentaOportunidad(t.EmpresaID, *oportunidadID, id); err != nil {
			return res, err
		}
	}
	res.Cuenta, err = cuentaEvaluada(t, AlcanceEmpresa(), id)
	res.Advertencia = plan.Advertencia
	return res, err
}

func hCrearCuenta(r *http.Request, t *Tenant) (int, any, error) {
	var in entradaCuenta
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	clienteID, err := validarID(in.ClienteID, "cliente_id")
	if err != nil {
		return 0, nil, err
	}
	if in.MontoTotalCentavos == nil {
		return 0, nil, errEntrada("MONTO_REQUERIDO", "Indique el monto total de la cuenta en centavos.")
	}
	res, err := crearCuenta(t, clienteID, *in.MontoTotalCentavos, in.FechaEmision, in.FechaVencimiento,
		in.VendedorID, "", nil, in.NumeroDocumento)
	return http.StatusCreated, res, err
}

func hConvertirOportunidad(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("OPORTUNIDAD_NO_ENCONTRADA", "No encontramos la oportunidad solicitada."))
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		// ClienteID permite emitir la cuenta a otro cliente que el de la oportunidad (p. ej. otra
		// sucursal o razón social del mismo comprador). Vacío = el cliente de la oportunidad.
		ClienteID          *string `json:"cliente_id"`
		NumeroDocumento    string  `json:"numero_documento"`
		MontoTotalCentavos *int64  `json:"monto_total_centavos"`
		FechaEmision       *Fecha  `json:"fecha_emision"`
		FechaVencimiento   *Fecha  `json:"fecha_vencimiento"`
		VendedorID         *string `json:"vendedor_id"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	// Lock de la oportunidad: dos conversiones simultáneas no pueden crear dos cuentas (R9).
	op, err := t.Repo.ObtenerOportunidad(t.EmpresaID, t.Alcance(), id, true)
	if err != nil {
		return 0, nil, err
	}
	if err := ValidarConversion(op); err != nil {
		return 0, nil, err
	}
	monto := op.MontoEstimadoCentavos
	if in.MontoTotalCentavos != nil {
		monto = *in.MontoTotalCentavos
	}
	clienteID := op.ClienteID
	if in.ClienteID != nil && strings.TrimSpace(*in.ClienteID) != "" {
		// crearCuenta busca el cliente con el alcance del usuario: un vendedor solo puede usar los
		// suyos (404 si no), y se aplican plazo, límite de crédito y cliente activo del cliente elegido.
		if clienteID, err = validarID(*in.ClienteID, "cliente_id"); err != nil {
			return 0, nil, err
		}
	}
	res, err := crearCuenta(t, clienteID, monto, in.FechaEmision, in.FechaVencimiento, in.VendedorID,
		op.VendedorID, &op.ID, in.NumeroDocumento)
	return http.StatusCreated, res, err
}

func hListarCuentas(r *http.Request, t *Tenant) (int, any, error) {
	estado := r.URL.Query().Get("estado")
	if estado == "" {
		estado = EstadoTodas
	}
	if estado != EstadoTodas && !EsEstadoCuentaValido(estado) {
		return 0, nil, errEntrada("ESTADO_INVALIDO", "El estado debe ser todas, al_dia, por_vencer, vencido o pagado.").
			con("estados_validos", append([]string{EstadoTodas}, EstadosCuenta...))
	}
	pagina, porPagina, err := ValidarPaginacion(r.URL.Query().Get("pagina"), r.URL.Query().Get("por_pagina"))
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	// El filtro y la paginación los hace la base (ver repos_paginacion.go); lo que se devuelve
	// se vuelve a evaluar con reglas.go, que sigue siendo la única fuente del saldo y el estado.
	cuentas, total, err := t.Repo.ListarCuentasPagina(t.EmpresaID, ConsultaCuentas{
		Alcance: t.Alcance(), Estado: estado, Buscar: r.URL.Query().Get("buscar"),
		Hoy: hoy, DiasAviso: cfg.DiasAvisoPrevio,
		Limite: porPagina, Desde: (pagina - 1) * porPagina,
	})
	if err != nil {
		return 0, nil, err
	}
	for i := range cuentas {
		EvaluarCuenta(&cuentas[i], hoy, cfg)
	}
	return http.StatusOK, map[string]any{
		"estado":     estado,
		"cuentas":    cuentas,
		"paginacion": ArmarPaginacion(pagina, porPagina, total, len(cuentas)),
	}, nil
}

type respuestaDetalleCuenta struct {
	Cuenta      Cuenta       `json:"cuenta"`
	Movimientos []Movimiento `json:"movimientos"`
	Ajustes     []Ajuste     `json:"ajustes"`
}

func detalleCuenta(t *Tenant, id string) (respuestaDetalleCuenta, error) {
	c, err := cuentaEvaluada(t, t.Alcance(), id)
	if err != nil {
		return respuestaDetalleCuenta{}, err
	}
	movs, err := t.Repo.ListarMovimientos(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return respuestaDetalleCuenta{}, err
	}
	ajustes, err := t.Repo.ListarAjustesDeCuenta(t.EmpresaID, id)
	if err != nil {
		return respuestaDetalleCuenta{}, err
	}
	return respuestaDetalleCuenta{c, movs, ajustes}, nil
}

func hObtenerCuenta(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errCuentaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	res, err := detalleCuenta(t, id)
	return http.StatusOK, res, err
}

// bloquearYEvaluar toma el lock de la cuenta y la devuelve evaluada.
func bloquearYEvaluar(t *Tenant, id string) (Cuenta, ConfigEmpresa, Fecha, error) {
	cfg, err := t.Config()
	if err != nil {
		return Cuenta{}, cfg, Fecha{}, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return Cuenta{}, cfg, hoy, err
	}
	if err := t.Repo.BloquearCuenta(t.EmpresaID, t.Alcance(), id); err != nil {
		return Cuenta{}, cfg, hoy, err
	}
	c, err := cuentaEvaluada(t, t.Alcance(), id)
	return c, cfg, hoy, err
}

type respuestaAjuste struct {
	Ajuste Ajuste `json:"ajuste"`
	Cuenta Cuenta `json:"cuenta"`
}

func hEditarMonto(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errCuentaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		MontoTotalCentavos *int64 `json:"monto_total_centavos"`
		Motivo             string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.MontoTotalCentavos == nil {
		return 0, nil, errEntrada("MONTO_REQUERIDO", "Indique el nuevo monto total de la cuenta en centavos.")
	}
	c, cfg, _, err := bloquearYEvaluar(t, id)
	if err != nil {
		return 0, nil, err
	}
	plan, err := PlanificarAjuste(c, *in.MontoTotalCentavos, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}
	ajusteID, err := t.Repo.CrearAjuste(t.EmpresaID, id, c.MontoTotalCentavos, *in.MontoTotalCentavos,
		plan.Motivo, t.Usuario.ID, plan.Estado)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.ActualizarMontoCuenta(t.EmpresaID, id, *in.MontoTotalCentavos, plan.RequiereRevision); err != nil {
		return 0, nil, err
	}
	return respuestaDeAjuste(t, ajusteID, id, http.StatusOK)
}

func respuestaDeAjuste(t *Tenant, ajusteID, cuentaID string, estado int) (int, any, error) {
	a, err := t.Repo.ObtenerAjuste(t.EmpresaID, ajusteID, false)
	if err != nil {
		return 0, nil, err
	}
	c, err := cuentaEvaluada(t, AlcanceEmpresa(), cuentaID)
	return estado, respuestaAjuste{a, c}, err
}

type respuestaMovimiento struct {
	Movimiento Movimiento `json:"movimiento"`
	Cuenta     Cuenta     `json:"cuenta"`
}

func respuestaDeMovimiento(t *Tenant, movID, cuentaID string, estado int) (int, any, error) {
	m, err := t.Repo.ObtenerMovimiento(t.EmpresaID, AlcanceEmpresa(), movID)
	if err != nil {
		return 0, nil, err
	}
	c, err := cuentaEvaluada(t, AlcanceEmpresa(), cuentaID)
	return estado, respuestaMovimiento{m, c}, err
}

func hRegistrarPago(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errCuentaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		MontoCentavos *int64 `json:"monto_centavos"`
		Fecha         *Fecha `json:"fecha"`
		Nota          string `json:"nota"`
		Metodo        string `json:"metodo"`
		Referencia    string `json:"referencia"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.MontoCentavos == nil {
		return 0, nil, errEntrada("MONTO_REQUERIDO", "Indique el monto del pago en centavos.")
	}
	c, cfg, hoy, err := bloquearYEvaluar(t, id)
	if err != nil {
		return 0, nil, err
	}
	fecha := valorO(in.Fecha, hoy)
	if err := ValidarPago(*in.MontoCentavos, fecha, hoy, c.SaldoCentavos, cfg); err != nil {
		return 0, nil, err
	}
	nota, err := ValidarTexto(in.Nota, "nota", LargoMaximoNota)
	if err != nil {
		return 0, nil, err
	}
	metodo, err := ValidarMetodoPago(in.Metodo, cfg)
	if err != nil {
		return 0, nil, err
	}
	referencia, err := ValidarTexto(in.Referencia, "referencia", LargoMaximoCodigo)
	if err != nil {
		return 0, nil, err
	}
	// Cada pago lleva su recibo numerado: es el comprobante que se le deja al cliente.
	numero, err := t.Repo.ReservarNumero(t.EmpresaID, "recibo", cfg.PrefijoRecibo)
	if err != nil {
		return 0, nil, err
	}
	movID, err := t.Repo.CrearMovimiento(t.EmpresaID, NuevoMovimiento{
		CuentaID: id, Tipo: TipoPago, Numero: numero, MontoCentavos: *in.MontoCentavos, Fecha: fecha,
		Metodo: metodo, Referencia: referencia, Nota: nota, RegistradoPor: t.Usuario.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	return respuestaDeMovimiento(t, movID, id, http.StatusCreated)
}

func hCondonar(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errCuentaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		MontoCentavos *int64 `json:"monto_centavos"`
		Motivo        string `json:"motivo"`
		Fecha         *Fecha `json:"fecha"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	c, cfg, hoy, err := bloquearYEvaluar(t, id)
	if err != nil {
		return 0, nil, err
	}
	fecha := valorO(in.Fecha, hoy)
	monto, motivo, err := ValidarCondonacion(in.MontoCentavos, in.Motivo, fecha, hoy, c.SaldoCentavos, cfg)
	if err != nil {
		return 0, nil, err
	}
	movID, err := t.Repo.CrearMovimiento(t.EmpresaID, NuevoMovimiento{
		CuentaID: id, Tipo: TipoCondonacion, MontoCentavos: monto, Fecha: fecha,
		Metodo: TipoCondonacion, Nota: motivo, RegistradoPor: t.Usuario.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	return respuestaDeMovimiento(t, movID, id, http.StatusCreated)
}

func hAnularMovimiento(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("MOVIMIENTO_NO_ENCONTRADO", "No encontramos el pago o condonación indicado."))
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	mov, err := t.Repo.ObtenerMovimiento(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	// Lock de la cuenta y relectura del movimiento: evita anular dos veces en paralelo.
	if _, _, _, err := bloquearYEvaluar(t, mov.CuentaID); err != nil {
		return 0, nil, err
	}
	if mov, err = t.Repo.ObtenerMovimiento(t.EmpresaID, t.Alcance(), id); err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	motivo, err := ValidarAnulacion(mov, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.AnularMovimiento(t.EmpresaID, id, t.Usuario.ID, motivo); err != nil {
		return 0, nil, err
	}
	return respuestaDeMovimiento(t, id, mov.CuentaID, http.StatusOK)
}

// ─────────────────────────────────────────────────────────────────────────────
// Ajustes
// ─────────────────────────────────────────────────────────────────────────────

func hListarAjustes(r *http.Request, t *Tenant) (int, any, error) {
	estado := r.URL.Query().Get("estado")
	switch estado {
	case "":
		estado = AjustePendiente
	case "todos", AjustePendiente, AjusteAprobado, AjusteRechazado:
	default:
		return 0, nil, errEntrada("ESTADO_INVALIDO", "El estado debe ser pendiente, aprobado, rechazado o todos.")
	}
	filtro := estado
	if estado == "todos" {
		filtro = ""
	}
	ajustes, err := t.Repo.ListarAjustes(t.EmpresaID, filtro)
	return http.StatusOK, map[string]any{"estado": estado, "ajustes": ajustes}, err
}

func errAjusteNoEncontrado() *ErrorApp {
	return errNoEncontrado("AJUSTE_NO_ENCONTRADO", "No encontramos el ajuste indicado.")
}

// resolverAjuste aprueba o rechaza. Al rechazar, el monto vuelve al anterior (R33).
func resolverAjuste(r *http.Request, t *Tenant, aprobar bool) (int, any, error) {
	id, err := idDeRuta(r, errAjusteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAjuste(t.EmpresaID, id, false)
	if err != nil {
		return 0, nil, err
	}
	c, cfg, _, err := bloquearYEvaluar(t, a.CuentaID)
	if err != nil {
		return 0, nil, err
	}
	if a, err = t.Repo.ObtenerAjuste(t.EmpresaID, id, true); err != nil {
		return 0, nil, err
	}
	if err := ValidarResolucionAjuste(a); err != nil {
		return 0, nil, err
	}
	estado := AjusteAprobado
	if !aprobar {
		if err := ValidarReversionAjuste(a, c, cfg); err != nil {
			return 0, nil, err
		}
		estado = AjusteRechazado
	}
	if err := t.Repo.ResolverAjuste(t.EmpresaID, id, estado, t.Usuario.ID); err != nil {
		return 0, nil, err
	}
	pendientes, err := t.Repo.ContarAjustesPendientes(t.EmpresaID, a.CuentaID)
	if err != nil {
		return 0, nil, err
	}
	monto := c.MontoTotalCentavos
	if !aprobar {
		monto = a.MontoAnteriorCentavos
	}
	if err := t.Repo.ActualizarMontoCuenta(t.EmpresaID, a.CuentaID, monto, pendientes > 0); err != nil {
		return 0, nil, err
	}
	return respuestaDeAjuste(t, id, a.CuentaID, http.StatusOK)
}

func hAprobarAjuste(r *http.Request, t *Tenant) (int, any, error) { return resolverAjuste(r, t, true) }
func hRechazarAjuste(r *http.Request, t *Tenant) (int, any, error) {
	return resolverAjuste(r, t, false)
}

// ─────────────────────────────────────────────────────────────────────────────
// Reasignación de cartera (R40–R42)
// ─────────────────────────────────────────────────────────────────────────────

func hReasignarCartera(r *http.Request, t *Tenant) (int, any, error) {
	origenID, err := idDeRuta(r, errUsuarioNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		VendedorDestinoID string `json:"vendedor_destino_id"`
		Motivo            string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	destinoID, err := validarID(in.VendedorDestinoID, "vendedor_destino_id")
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	origen, err := t.Repo.ObtenerUsuario(t.EmpresaID, origenID)
	if err != nil {
		return 0, nil, err
	}
	destino, err := t.Repo.ObtenerUsuario(t.EmpresaID, destinoID)
	if err != nil {
		return 0, nil, err
	}
	motivo, err := ValidarReasignacion(origen, destino, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}

	if err := t.Repo.SerializarReasignaciones(t.EmpresaID); err != nil {
		return 0, nil, err
	}
	// Se usa el alcance "como si fuera el vendedor de origen" para listar exactamente su cartera.
	carteraOrigen := AlcanceVendedor(origen.ID)
	ops, err := t.Repo.ListarOportunidades(t.EmpresaID, carteraOrigen)
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, carteraOrigen, "")
	if err != nil {
		return 0, nil, err
	}
	opIDs, cuentaIDs := SeleccionarCartera(ops, cuentas)

	// Mismo orden de locks que la conversión (oportunidad → cliente → cuenta) para no bloquearse
	// mutuamente con una venta que se está convirtiendo en ese momento.
	if err := t.Repo.ReasignarOportunidades(t.EmpresaID, opIDs, destino.ID); err != nil {
		return 0, nil, err
	}
	nClientes, err := t.Repo.ReasignarClientes(t.EmpresaID, origen.ID, destino.ID)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.ReasignarCuentas(t.EmpresaID, cuentaIDs, destino.ID); err != nil {
		return 0, nil, err
	}
	reg, err := t.Repo.RegistrarReasignacion(t.EmpresaID, Reasignacion{
		VendedorOrigen: origen.ID, VendedorDestino: destino.ID,
		CantidadClientes: nClientes, CantidadOportunidades: len(opIDs), CantidadCuentas: len(cuentaIDs),
		Motivo: motivo, RealizadoPor: t.Usuario.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{
		"reasignacion": reg,
		"mensaje": fmt.Sprintf("Se transfirieron %d cliente(s), %d oportunidad(es) abiertas y %d cuenta(s) con saldo de %s a %s. Las cuentas ya pagadas conservan su vendedor original.",
			nClientes, len(opIDs), len(cuentaIDs), origen.Nombre, destino.Nombre),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Alertas (R46)
// ─────────────────────────────────────────────────────────────────────────────

type alerta struct {
	CuentaID         string `json:"cuenta_id"`
	Tipo             string `json:"tipo"`
	ClienteID        string `json:"cliente_id"`
	ClienteNombre    string `json:"cliente_nombre"`
	ClienteContacto  string `json:"cliente_contacto"`
	ClienteTelefono  string `json:"cliente_telefono"`
	VendedorID       string `json:"vendedor_id"`
	VendedorNombre   string `json:"vendedor_nombre"`
	SaldoCentavos    int64  `json:"saldo_centavos"`
	SaldoTexto       string `json:"saldo_texto"`
	FechaVencimiento Fecha  `json:"fecha_vencimiento"`
	DiasParaVencer   int    `json:"dias_para_vencer"`
	DiasVencida      int    `json:"dias_vencida"`
	EntregadaHoy     bool   `json:"entregada_hoy"`
}

func hListarAlertas(r *http.Request, t *Tenant) (int, any, error) {
	soloPendientes := r.URL.Query().Get("solo_pendientes") == "true"
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, t.Alcance(), "")
	if err != nil {
		return 0, nil, err
	}
	entregadas, err := t.Repo.AlertasEntregadas(t.EmpresaID, hoy)
	if err != nil {
		return 0, nil, err
	}
	res := []alerta{}
	for _, c := range cuentas {
		tipo, ok := TipoAlerta(c)
		if !ok {
			continue
		}
		entregada := entregadas[claveAlerta(c.ID, tipo)]
		if soloPendientes && entregada {
			continue
		}
		res = append(res, alerta{
			CuentaID: c.ID, Tipo: tipo, ClienteID: c.ClienteID, ClienteNombre: c.ClienteNombre,
			ClienteContacto: c.ClienteContacto, ClienteTelefono: c.ClienteTelefono,
			VendedorID: c.VendedorID, VendedorNombre: c.VendedorNombre,
			SaldoCentavos: c.SaldoCentavos, SaldoTexto: FormatearMonto(c.SaldoCentavos, cfg),
			FechaVencimiento: c.FechaVencimiento, DiasParaVencer: c.DiasParaVencer, DiasVencida: c.DiasVencida,
			EntregadaHoy: entregada,
		})
	}
	return http.StatusOK, map[string]any{"fecha": hoy, "alertas": res}, nil
}

type omitida struct {
	CuentaID string `json:"cuenta_id"`
	Razon    string `json:"razon"`
}

// hMarcarAlertas registra la entrega. El tipo lo decide el servidor con el estado actual de la
// cuenta, para que el consumidor externo no pueda registrar un tipo inconsistente.
func hMarcarAlertas(r *http.Request, t *Tenant) (int, any, error) {
	var in struct {
		CuentaIDs []string `json:"cuenta_ids"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if len(in.CuentaIDs) == 0 {
		return 0, nil, errEntrada("CUENTAS_REQUERIDAS", "Envíe la lista cuenta_ids con las cuentas cuyo aviso ya se entregó.")
	}
	if len(in.CuentaIDs) > 500 {
		return 0, nil, errEntrada("DEMASIADAS_CUENTAS", "Envíe como máximo 500 cuentas por llamada.")
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	registradas, yaRegistradas, omitidas := []string{}, []string{}, []omitida{}
	for _, crudo := range in.CuentaIDs {
		id, err := validarID(crudo, "cuenta_ids")
		if err != nil {
			omitidas = append(omitidas, omitida{crudo, "identificador inválido"})
			continue
		}
		c, err := cuentaEvaluada(t, t.Alcance(), id)
		var e *ErrorApp
		if errors.As(err, &e) {
			omitidas = append(omitidas, omitida{id, "cuenta no encontrada"})
			continue
		} else if err != nil {
			return 0, nil, err
		}
		tipo, ok := TipoAlerta(c)
		if !ok {
			omitidas = append(omitidas, omitida{id, "la cuenta ya no requiere aviso hoy"})
			continue
		}
		nueva, err := t.Repo.RegistrarAlerta(t.EmpresaID, id, tipo, hoy)
		if err != nil {
			return 0, nil, err
		}
		if nueva {
			registradas = append(registradas, id)
		} else {
			yaRegistradas = append(yaRegistradas, id)
		}
	}
	return http.StatusOK, map[string]any{
		"fecha": hoy, "registradas": registradas, "ya_registradas": yaRegistradas, "omitidas": omitidas,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Punteros opcionales
// ─────────────────────────────────────────────────────────────────────────────

func valor[T any](p *T) T {
	var cero T
	if p == nil {
		return cero
	}
	return *p
}

func valorO[T any](p *T, porDefecto T) T {
	if p == nil {
		return porDefecto
	}
	return *p
}

// ─────────────────────────────────────────────────────────────────────────────
// Salud: lo que consulta el monitoreo. Sin sesión y sin datos de ninguna empresa.
// ─────────────────────────────────────────────────────────────────────────────

// version la fija el compilador: go build -ldflags "-X main.version=2026.10.15".
var version = "dev"

var arranque = time.Now()

// salud responde 200 si la aplicación puede hablar con la base, y 503 si no. No revela nada:
// ni versiones de la base, ni nombres de empresas, ni cuántos usuarios hay.
func (a *App) salud(w http.ResponseWriter, r *http.Request) {
	ctx, cancelar := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancelar()

	cuerpo := map[string]any{
		"version":         version,
		"segundos_en_pie": int(time.Since(arranque).Seconds()),
		"base_de_datos":   "ok",
		"estado":          "ok",
	}
	if err := a.db.PingContext(ctx); err != nil {
		log.Printf("salud: la base no responde: %v", err)
		cuerpo["base_de_datos"] = "sin_respuesta"
		cuerpo["estado"] = "degradado"
		escribirJSON(w, http.StatusServiceUnavailable, cuerpo)
		return
	}
	escribirJSON(w, http.StatusOK, cuerpo)
}
