package main

// handlers_exportacion.go — lo que sale del sistema hacia afuera: el mensaje de cobro que el
// dueño manda por su propio WhatsApp, y los dos CSV que el contador abre en Excel.
//
// Dos decisiones que gobiernan todo este archivo:
//
//  1. El sistema NO manda nada por su cuenta. `/api/clientes/{id}/mensaje-cobro` devuelve el texto
//     y el enlace `wa.me`; quien aprieta "Enviar" es la persona, desde su teléfono y su número.
//     No se usa la API de WhatsApp ni se contrata un intermediario.
//  2. El texto lo redacta el BACKEND. En el navegador no viven ni el formato del dinero
//     (FormatearMonto), ni la zona horaria, ni las etiquetas: el frontend solo abre el enlace
//     (§11: "el navegador nunca calcula"). Lo mismo vale para los CSV.

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func (a *App) rutasExportacion(mux *http.ServeMux) {
	mux.Handle("GET /api/clientes/{id}/mensaje-cobro", a.conTenant(hMensajeDeCobro))
	mux.Handle("GET /api/cuentas.csv", a.conTenant(hCarteraCSV))
	mux.Handle("GET /api/antiguedad.csv", a.conTenant(hAntiguedadCSV))
}

// ─────────────────────────────────────────────────────────────────────────────
// Mensaje de cobro por WhatsApp
// ─────────────────────────────────────────────────────────────────────────────

// codigoPaisPorDefecto es el que se antepone cuando el teléfono del cliente viene sin código de
// país, que es como lo escribe todo el mundo acá ("7845-2210"). Hoy el producto se vende en un
// solo país; el día que se venda en dos, esto se muda a `empresa_config` con su DEFAULT en la
// migración (principio rector de §1) y deja de ser una constante.
const codigoPaisPorDefecto = "503"

// largoNumeroLocal es la cantidad de dígitos de un número salvadoreño sin código de país.
const largoNumeroLocal = 8

// maximoDocumentosEnMensaje acota la lista del mensaje: un WhatsApp con cuarenta facturas no lo
// lee nadie. El resto se resume en una línea y el detalle completo va en el estado de cuenta.
const maximoDocumentosEnMensaje = 12

type respuestaMensajeCobro struct {
	ClienteID string `json:"cliente_id"`
	Telefono  string `json:"telefono"` // normalizado, con código de país: +50378452210
	Texto     string `json:"texto"`    // sin codificar, para mostrarlo tal cual
	URL       string `json:"url"`      // wa.me con el texto ya codificado
}

// hMensajeDeCobro arma el recordatorio de cobro de un cliente y el enlace para abrirlo en WhatsApp.
func hMensajeDeCobro(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	// El cliente se busca con el alcance del usuario (un vendedor no puede cobrarle a un cliente
	// ajeno: 404), pero sus cuentas se leen completas, igual que en el estado de cuenta (R70):
	// el saldo que se le informa al cliente es el que él debe, no el pedazo que ve un vendedor.
	cliente, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	telefono, ok := normalizarTelefono(cliente.Telefono)
	if !ok {
		return 0, nil, errRegla("CLIENTE_SIN_TELEFONO",
			fmt.Sprintf("%s no tiene teléfono registrado. Agréguele el número al cliente para poder enviarle el mensaje por WhatsApp.",
				cliente.Nombre))
	}
	empresa, _, cfg, err := datosDeMarca(t)
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	texto := TextoMensajeCobro(empresa, cliente, cuentas, cfg)
	return http.StatusOK, respuestaMensajeCobro{
		ClienteID: cliente.ID,
		Telefono:  "+" + telefono,
		Texto:     texto,
		URL:       "https://wa.me/" + telefono + "?text=" + url.QueryEscape(texto),
	}, nil
}

// normalizarTelefono deja el número como lo quiere wa.me: solo dígitos y con código de país.
// Quita espacios, guiones, puntos y paréntesis; entiende el prefijo internacional escrito como
// "+503" o como "00503". Si quedan los 8 dígitos de un número local, le antepone el código de
// país. Devuelve ok=false si no quedó nada utilizable.
func normalizarTelefono(crudo string) (string, bool) {
	var digitos strings.Builder
	for _, c := range crudo {
		if c >= '0' && c <= '9' {
			digitos.WriteRune(c)
		}
	}
	n := digitos.String()
	n = strings.TrimPrefix(n, "00") // prefijo internacional marcado a la antigua
	switch {
	case n == "":
		return "", false
	case len(n) == largoNumeroLocal:
		return codigoPaisPorDefecto + n, true
	case len(n) < largoNumeroLocal:
		return "", false // un número incompleto no sirve para nada
	}
	return n, true // ya trae código de país
}

// TextoMensajeCobro redacta el recordatorio. Es texto, no reglas de negocio: no decide nada, solo
// pone en palabras lo que ya calcularon `EvaluarCuenta` y `FormatearMonto`. Por eso vive acá y no
// en reglas.go, igual que los rótulos del informe mensual.
//
// Tono fijado con el cliente (§11): se trata de USTED, sin emojis, sin signos de admiración y sin
// sonar a cobrador. Quien manda esto es el dueño del negocio, que quiere cobrar Y seguir
// vendiéndole al mismo cliente la semana entrante.
func TextoMensajeCobro(empresa Empresa, cliente Cliente, cuentas []Cuenta, cfg ConfigEmpresa) string {
	var vencidas, pendientes []Cuenta
	var saldo, saldoVencido int64
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		saldo += c.SaldoCentavos
		if c.Estado == EstadoVencido {
			vencidas = append(vencidas, c)
			saldoVencido += c.SaldoCentavos
		} else {
			pendientes = append(pendientes, c)
		}
	}
	// Lo más atrasado primero: es de lo que hay que hablar.
	sort.SliceStable(vencidas, func(i, j int) bool { return vencidas[i].DiasVencida > vencidas[j].DiasVencida })
	sort.SliceStable(pendientes, func(i, j int) bool {
		return pendientes[i].FechaVencimiento.Antes(pendientes[j].FechaVencimiento)
	})

	var m strings.Builder
	fmt.Fprintf(&m, "Buen día, %s.\n", cliente.Nombre)
	fmt.Fprintf(&m, "Le escribimos de %s.\n\n", empresa.NombreComercial)

	// Cliente sin saldo: el mensaje es un agradecimiento, no un cobro.
	if len(vencidas) == 0 && len(pendientes) == 0 {
		m.WriteString("A la fecha su cuenta con nosotros está al día: no tenemos documentos pendientes de pago.\n\n")
		m.WriteString("Le agradecemos su cumplimiento y quedamos a sus órdenes para cuando necesite hacer su próximo pedido.")
		return m.String()
	}

	fmt.Fprintf(&m, "Este es el estado de su cuenta. Saldo pendiente: %s en %s.\n",
		FormatearMonto(saldo, cfg), pluralDocumentos(len(vencidas)+len(pendientes)))

	if len(vencidas) > 0 {
		fmt.Fprintf(&m, "\nDocumentos vencidos (%s):\n", FormatearMonto(saldoVencido, cfg))
		for _, c := range recortar(vencidas) {
			fmt.Fprintf(&m, "- %s, venció el %s, %s de atraso, %s\n",
				etiquetaDocumento(c), fechaCorta(c.FechaVencimiento), pluralDias(c.DiasVencida),
				FormatearMonto(c.SaldoCentavos, cfg))
		}
		m.WriteString(resto(len(vencidas)))
		m.WriteString("\nLe agradecemos indicarnos qué día podemos pasar a recibir su pago. ")
		m.WriteString("Si ya lo canceló, le pedimos hacer caso omiso de este mensaje y compartirnos el comprobante. ")
		m.WriteString("Quedamos a sus órdenes.")
		return m.String()
	}

	// Hay saldo, pero nada vencido: recordatorio cordial, no reclamo.
	m.WriteString("\nDocumentos por vencer:\n")
	for _, c := range recortar(pendientes) {
		fmt.Fprintf(&m, "- %s, vence el %s, %s\n",
			etiquetaDocumento(c), fechaCorta(c.FechaVencimiento), FormatearMonto(c.SaldoCentavos, cfg))
	}
	m.WriteString(resto(len(pendientes)))
	m.WriteString("\nNinguno está vencido a la fecha. Le agradecemos su puntualidad y quedamos a sus órdenes.")
	return m.String()
}

func recortar(cuentas []Cuenta) []Cuenta {
	if len(cuentas) > maximoDocumentosEnMensaje {
		return cuentas[:maximoDocumentosEnMensaje]
	}
	return cuentas
}

func resto(total int) string {
	if total <= maximoDocumentosEnMensaje {
		return ""
	}
	return fmt.Sprintf("- y %s más\n", pluralDocumentos(total-maximoDocumentosEnMensaje))
}

// etiquetaDocumento nombra la cuenta como la nombraría el cliente: por su factura. Si la cuenta se
// cargó sin número de documento, se la identifica por su fecha de emisión.
func etiquetaDocumento(c Cuenta) string {
	if c.NumeroDocumento != "" {
		return c.NumeroDocumento
	}
	return "Documento del " + fechaCorta(c.FechaEmision)
}

// fechaCorta es la fecha como se lee acá (día/mes/año), no el AAAA-MM-DD de la API.
func fechaCorta(f Fecha) string { return f.Tiempo().Format("02/01/2006") }

func pluralDocumentos(n int) string {
	if n == 1 {
		return "1 documento"
	}
	return strconv.Itoa(n) + " documentos"
}

func pluralDias(n int) string {
	if n == 1 {
		return "1 día"
	}
	return strconv.Itoa(n) + " días"
}

// ─────────────────────────────────────────────────────────────────────────────
// Exportación a Excel (CSV)
// ─────────────────────────────────────────────────────────────────────────────

// maximoFilasExportacion es un techo de plataforma, no de empresa: protege la memoria del proceso
// y el Excel de quien abre el archivo. Con más cartera que esto, lo que se necesita es un filtro.
const maximoFilasExportacion = 20000

// escribirCSV arma el archivo con BOM UTF-8 al inicio.
//
// El BOM no es decoración: Excel en Windows abre un CSV sin BOM con la página de códigos del
// sistema y "Bendición" sale "Bendición". El contador no reporta eso como un problema del CSV,
// lo reporta como que el sistema está mal hecho.
//
// Separador coma, decimales con punto y montos sin símbolo (columna "Moneda" aparte): así Excel
// los reconoce como números y los puede sumar. Fechas en AAAA-MM-DD.
func escribirCSV(nombre string, encabezados []string, filas [][]string) (archivoCSV, error) {
	var b bytes.Buffer
	b.Write([]byte{0xEF, 0xBB, 0xBF}) // BOM UTF-8
	w := csv.NewWriter(&b)
	if err := w.Write(encabezados); err != nil {
		return archivoCSV{}, err
	}
	for _, f := range filas {
		if err := w.Write(f); err != nil {
			return archivoCSV{}, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return archivoCSV{}, err
	}
	return archivoCSV{nombre, b.Bytes()}, nil
}

// montoPlano escribe los centavos como número para Excel: sin símbolo, sin separador de miles y
// con punto decimal. El símbolo de la empresa va en su propia columna.
func montoPlano(centavos int64) string {
	signo := ""
	if centavos < 0 {
		signo = "-"
		centavos = -centavos
	}
	return fmt.Sprintf("%s%d.%02d", signo, centavos/100, centavos%100)
}

// codigosDeCliente devuelve id → código externo, para la columna "Código" del CSV. Se arma con
// ListarClientes, que ya respeta el alcance del usuario; un cliente fuera del alcance (cuenta
// heredada tras una reasignación) sale con el código en blanco, nunca con el de otro.
func codigosDeCliente(t *Tenant) (map[string]string, error) {
	clientes, err := t.Repo.ListarClientes(t.EmpresaID, t.Alcance(), true)
	if err != nil {
		return nil, err
	}
	res := make(map[string]string, len(clientes))
	for _, c := range clientes {
		res[c.ID] = c.CodigoExterno
	}
	return res, nil
}

// hCarteraCSV exporta la cartera con los MISMOS filtros que GET /api/cuentas (R78): el contador
// baja exactamente lo que está viendo en pantalla. El alcance también es el mismo: un vendedor
// exporta su cartera y nada más.
func hCarteraCSV(r *http.Request, t *Tenant) (int, any, error) {
	filtro := r.URL.Query().Get("estado")
	if filtro == "" {
		filtro = EstadoTodas
	}
	if filtro != EstadoTodas && !EsEstadoCuentaValido(filtro) {
		return 0, nil, errEntrada("ESTADO_INVALIDO", "El estado debe ser todas, al_dia, por_vencer, vencido o pagado.").
			con("estados_validos", append([]string{EstadoTodas}, EstadosCuenta...))
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	buscar := r.URL.Query().Get("buscar")
	// Se reusa la consulta paginada de /api/cuentas en vez de escribir otra: así el filtro por
	// estado y la búsqueda del CSV son, literalmente, los mismos de la pantalla. El "límite" es
	// el techo de exportación, no una página.
	cuentas, total, err := t.Repo.ListarCuentasPagina(t.EmpresaID, ConsultaCuentas{
		Alcance: t.Alcance(), Estado: filtro, Buscar: buscar,
		Hoy: hoy, DiasAviso: cfg.DiasAvisoPrevio,
		Limite: maximoFilasExportacion, Desde: 0,
	})
	if err != nil {
		return 0, nil, err
	}
	if total > maximoFilasExportacion {
		return 0, nil, errRegla("EXPORTACION_DEMASIADO_GRANDE",
			fmt.Sprintf("La exportación tiene %d cuentas y el máximo por archivo es %d. Filtre por estado o use la búsqueda para bajarla por partes.",
				total, maximoFilasExportacion)).
			con("total", total).con("maximo", maximoFilasExportacion)
	}
	codigos, err := codigosDeCliente(t)
	if err != nil {
		return 0, nil, err
	}

	filas := make([][]string, 0, len(cuentas))
	for i := range cuentas {
		EvaluarCuenta(&cuentas[i], hoy, cfg) // el saldo y el estado salen de reglas.go, no del SQL
		c := cuentas[i]
		filas = append(filas, []string{
			c.ClienteNombre,
			codigos[c.ClienteID],
			c.NumeroDocumento,
			c.FechaEmision.String(),
			c.FechaVencimiento.String(),
			EtiquetaEstadoCuenta(c.Estado),
			strconv.Itoa(c.DiasVencida),
			montoPlano(c.MontoTotalCentavos),
			montoPlano(c.PagadoCentavos),
			montoPlano(c.CondonadoCentavos),
			montoPlano(c.NotaCreditoCentavos),
			montoPlano(c.SaldoCentavos),
			cfg.Moneda,
			c.VendedorNombre,
		})
	}
	// "Condonado" y "Nota de crédito" van aparte para que la resta cuadre en la hoja:
	// Monto − Pagado − Condonado − Nota de crédito = Saldo (R12). Sin esas dos columnas el
	// contador ve un saldo que no le da y abre un caso.
	archivo, err := escribirCSV(
		fmt.Sprintf("cartera-%s.csv", hoy),
		[]string{"Cliente", "Código", "Documento", "Emisión", "Vencimiento", "Estado",
			"Días de atraso", "Monto", "Pagado", "Condonado", "Nota de crédito", "Saldo",
			"Moneda", "Vendedor"},
		filas)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, archivo, nil
}

// hAntiguedadCSV exporta una fila por cliente con su saldo repartido en los tramos de antigüedad.
//
// No calcula nada propio: los tramos los arma `AntiguedadSaldos` (reglas_cobranza.go) y los
// totales `SaldosPorCliente` (reglas.go), las mismas funciones que alimentan el tablero, el
// estado de cuenta y el informe mensual. Si alguien cambia los cortes de antigüedad, este CSV
// cambia con ellos y no queda contando distinto que el resto del sistema.
func hAntiguedadCSV(r *http.Request, t *Tenant) (int, any, error) {
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	// Como /api/resumen y el informe mensual (§10), la antigüedad se calcula sobre toda la
	// cartera del alcance: es un corte por cliente, no se puede paginar sin romperlo.
	cuentas, err := cuentasEvaluadas(t, t.Alcance(), "")
	if err != nil {
		return 0, nil, err
	}
	codigos, err := codigosDeCliente(t)
	if err != nil {
		return 0, nil, err
	}

	saldos := SaldosPorCliente(cuentas)
	porCliente := map[string][]Cuenta{}
	nombres := map[string]string{}
	vendedores := map[string]string{}
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		porCliente[c.ClienteID] = append(porCliente[c.ClienteID], c)
		nombres[c.ClienteID] = c.ClienteNombre
		vendedores[c.ClienteID] = c.VendedorNombre
	}
	ids := make([]string, 0, len(porCliente))
	for id := range porCliente {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return nombres[ids[i]] < nombres[ids[j]] })

	// Los rótulos de las columnas son los de los propios tramos: una sola fuente.
	encabezados := []string{"Cliente", "Código"}
	for _, tramo := range AntiguedadSaldos(nil) {
		encabezados = append(encabezados, tramo.Etiqueta)
	}
	encabezados = append(encabezados, "Total", "Documentos", "Moneda", "Vendedor")

	filas := make([][]string, 0, len(ids))
	for _, id := range ids {
		fila := []string{nombres[id], codigos[id]}
		for _, tramo := range AntiguedadSaldos(porCliente[id]) {
			fila = append(fila, montoPlano(tramo.SaldoCentavos))
		}
		s := saldos[id]
		fila = append(fila, montoPlano(s.SaldoCentavos), strconv.Itoa(s.CuentasAbiertas),
			cfg.Moneda, vendedores[id])
		filas = append(filas, fila)
	}
	archivo, err := escribirCSV(fmt.Sprintf("antiguedad-%s.csv", hoy), encabezados, filas)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, archivo, nil
}
