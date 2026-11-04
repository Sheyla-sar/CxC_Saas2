package main

// handlers_documentos.go — descarga de recibos y estados de cuenta en PDF, e importación de la
// cartera inicial desde un archivo CSV (lo que el cliente exporta de su Excel o de su sistema).

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// PDF
// ─────────────────────────────────────────────────────────────────────────────

// escribirPDF manda el archivo al navegador. Se usa después de confirmar la transacción.
func escribirPDF(w http.ResponseWriter, nombre string, contenido []byte) {
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", nombre))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(contenido)
}

// archivoPDF es lo que devuelven los handlers de documentos: el middleware lo escribe tal cual.
type archivoPDF struct {
	Nombre    string
	Contenido []byte
}

// hReciboPDF arma el comprobante de un pago o nota de crédito.
func hReciboPDF(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("MOVIMIENTO_NO_ENCONTRADO", "No encontramos el movimiento indicado."))
	if err != nil {
		return 0, nil, err
	}
	mov, err := t.Repo.ObtenerMovimiento(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	cuenta, err := cuentaEvaluada(t, t.Alcance(), mov.CuentaID)
	if err != nil {
		return 0, nil, err
	}
	cliente, err := t.Repo.ObtenerCliente(t.EmpresaID, AlcanceEmpresa(), cuenta.ClienteID)
	if err != nil {
		return 0, nil, err
	}
	empresa, marca, cfg, err := datosDeMarca(t)
	if err != nil {
		return 0, nil, err
	}
	adjuntos, err := t.Repo.ListarAdjuntos(t.EmpresaID, "movimiento", mov.ID)
	if err != nil {
		return 0, nil, err
	}
	// El saldo anterior es el actual más lo que este movimiento rebajó (si no está anulado).
	saldoAntes := cuenta.SaldoCentavos
	if !mov.Anulado {
		saldoAntes += mov.MontoCentavos
	}
	contenido, err := ReciboPDF(DatosRecibo{
		Empresa: empresa, Marca: marca, Config: cfg, Cliente: cliente, Cuenta: cuenta,
		Movimiento: mov, SaldoAntes: saldoAntes, SaldoActual: cuenta.SaldoCentavos, Adjuntos: adjuntos,
	})
	if err != nil {
		return 0, nil, err
	}
	nombre := "recibo.pdf"
	if mov.Numero != nil {
		nombre = *mov.Numero + ".pdf"
	}
	return http.StatusOK, archivoPDF{nombre, contenido}, nil
}

// hEstadoCuentaPDF arma el estado de cuenta del cliente, con sus documentos pendientes.
func hEstadoCuentaPDF(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	cliente, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	empresa, marca, cfg, err := datosDeMarca(t)
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cuentas, err := cuentasEvaluadas(t, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	anticipos, err := anticiposEvaluados(t, id)
	if err != nil {
		return 0, nil, err
	}
	saldo := SaldosPorCliente(cuentas)[id]
	datos := DatosEstadoCuenta{
		Empresa: empresa, Marca: marca, Config: cfg, Cliente: cliente, Hoy: hoy, Cuentas: cuentas,
		Saldo:          saldo,
		Credito:        CalcularCredito(cliente.LimiteCreditoCentavos, saldo.SaldoCentavos),
		Comportamiento: CalcularComportamiento(cuentas, cfg),
		Antiguedad:     AntiguedadSaldos(cuentas),
		Anticipos:      anticipos,
		Movimientos:    map[string][]Movimiento{},
		ConDetalle:     r.URL.Query().Get("detalle") == "true",
	}
	if datos.ConDetalle {
		for _, c := range cuentas {
			if c.SaldoCentavos <= 0 {
				continue
			}
			movs, err := t.Repo.ListarMovimientos(t.EmpresaID, AlcanceEmpresa(), c.ID)
			if err != nil {
				return 0, nil, err
			}
			datos.Movimientos[c.ID] = movs
		}
	}
	// Fotos de facturas y recibos: se anexan si se piden explícitamente.
	if r.URL.Query().Get("fotos") == "true" {
		vistos := map[string]bool{}
		for _, c := range cuentas {
			if c.SaldoCentavos <= 0 {
				continue
			}
			adjuntos, err := t.Repo.ListarAdjuntos(t.EmpresaID, "cuenta", c.ID)
			if err != nil {
				return 0, nil, err
			}
			for _, a := range adjuntos {
				if !vistos[a.ID] {
					vistos[a.ID] = true
					datos.Adjuntos = append(datos.Adjuntos, a)
				}
			}
		}
	}
	contenido, err := EstadoCuentaPDF(datos)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, archivoPDF{"estado-de-cuenta.pdf", contenido}, nil
}

func datosDeMarca(t *Tenant) (Empresa, Marca, ConfigEmpresa, error) {
	empresa, err := t.Repo.ObtenerEmpresa(t.EmpresaID)
	if err != nil {
		return Empresa{}, Marca{}, ConfigEmpresa{}, err
	}
	marca, err := t.Repo.ObtenerMarca(t.EmpresaID)
	if err != nil {
		return empresa, Marca{}, ConfigEmpresa{}, err
	}
	cfg, err := t.Config()
	return empresa, marca, cfg, err
}

// ─────────────────────────────────────────────────────────────────────────────
// Importación de cartera (CSV)
// ─────────────────────────────────────────────────────────────────────────────

// columnasImportacion es el formato de la plantilla. El orden no importa: se leen por nombre.
var columnasImportacion = []string{
	"codigo_cliente", "nombre_cliente", "contacto", "telefono", "plazo_dias", "limite_credito",
	"numero_documento", "fecha_emision", "fecha_vencimiento", "monto", "abonado",
}

const plantillaImportacion = `codigo_cliente,nombre_cliente,contacto,telefono,plazo_dias,limite_credito,numero_documento,fecha_emision,fecha_vencimiento,monto,abonado
C-001,Tienda La Bendición,Rosa Hernández,7845-2210,30,5000.00,FAC-00123,2026-09-01,2026-10-01,1250.00,250.00
C-002,Abarrotería San José,José Alfaro,7012-3345,15,0,FAC-00124,2026-09-15,2026-09-30,675.50,0
`

// FilaImportacion es una línea del archivo ya interpretada.
type FilaImportacion struct {
	Linea            int      `json:"linea"`
	CodigoCliente    string   `json:"codigo_cliente"`
	NombreCliente    string   `json:"nombre_cliente"`
	NumeroDocumento  string   `json:"numero_documento"`
	MontoCentavos    int64    `json:"monto_centavos"`
	AbonadoCentavos  int64    `json:"abonado_centavos"`
	SaldoCentavos    int64    `json:"saldo_centavos"`
	FechaEmision     Fecha    `json:"fecha_emision"`
	FechaVencimiento Fecha    `json:"fecha_vencimiento"`
	PlazoDias        int      `json:"plazo_dias"`
	LimiteCentavos   int64    `json:"limite_credito_centavos"`
	Contacto         string   `json:"contacto"`
	Telefono         string   `json:"telefono"`
	ClienteNuevo     bool     `json:"cliente_nuevo"`
	Errores          []string `json:"errores"`
	Omitida          bool     `json:"omitida"` // documento ya cargado
}

type ResumenImportacion struct {
	Filas             []FilaImportacion `json:"filas"`
	TotalFilas        int               `json:"total_filas"`
	FilasValidas      int               `json:"filas_validas"`
	FilasConError     int               `json:"filas_con_error"`
	FilasOmitidas     int               `json:"filas_omitidas"`
	ClientesNuevos    int               `json:"clientes_nuevos"`
	TotalCarteraCents int64             `json:"total_cartera_centavos"`
	Aplicada          bool              `json:"aplicada"`
	CuentasCreadas    int               `json:"cuentas_creadas"`
	ClientesCreados   int               `json:"clientes_creados"`
}

// hPlantillaImportacion entrega el CSV de ejemplo que el cliente llena con su cartera.
func hPlantillaImportacion(r *http.Request, t *Tenant) (int, any, error) {
	return http.StatusOK, archivoCSV{"plantilla-cartera.csv", []byte(plantillaImportacion)}, nil
}

type archivoCSV struct {
	Nombre    string
	Contenido []byte
}

// hImportarCartera lee el CSV y, si se pide aplicar, crea clientes y cuentas. Primero se revisa
// (aplicar=false) para ver qué pasaría; recién después se confirma.
func hImportarCartera(r *http.Request, t *Tenant) (int, any, error) {
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return 0, nil, errEntrada("ARCHIVO_INVALIDO", "No se pudo leer el archivo. Suba el CSV de la plantilla.")
	}
	defer r.MultipartForm.RemoveAll()
	aplicar := r.FormValue("aplicar") == "true"

	archivo, _, err := r.FormFile("archivo")
	if err != nil {
		return 0, nil, errEntrada("ARCHIVO_REQUERIDO", "Adjunte el archivo CSV con la cartera.")
	}
	defer archivo.Close()
	contenido, err := io.ReadAll(io.LimitReader(archivo, 8<<20))
	if err != nil {
		return 0, nil, err
	}

	filas, err := leerCSVCartera(contenido, cfg, hoy)
	if err != nil {
		return 0, nil, err
	}

	// Qué ya existe: clientes por código y documentos ya cargados.
	clientes, err := t.Repo.ClientesPorCodigo(t.EmpresaID)
	if err != nil {
		return 0, nil, err
	}
	documentos := make([]string, 0, len(filas))
	for _, f := range filas {
		if f.NumeroDocumento != "" {
			documentos = append(documentos, f.NumeroDocumento)
		}
	}
	yaCargados, err := t.Repo.DocumentosExistentes(t.EmpresaID, documentos)
	if err != nil {
		return 0, nil, err
	}

	res := ResumenImportacion{TotalFilas: len(filas)}
	vistosEnArchivo := map[string]bool{}
	nuevosCodigos := map[string]bool{}
	for i := range filas {
		f := &filas[i]
		if f.NumeroDocumento != "" {
			if yaCargados[f.NumeroDocumento] || vistosEnArchivo[f.NumeroDocumento] {
				f.Omitida = true
			}
			vistosEnArchivo[f.NumeroDocumento] = true
		}
		if _, existe := clientes[f.CodigoCliente]; !existe && f.CodigoCliente != "" {
			f.ClienteNuevo = !nuevosCodigos[f.CodigoCliente]
			nuevosCodigos[f.CodigoCliente] = true
		}
		switch {
		case len(f.Errores) > 0:
			res.FilasConError++
		case f.Omitida:
			res.FilasOmitidas++
		default:
			res.FilasValidas++
			res.TotalCarteraCents += f.SaldoCentavos
			if f.ClienteNuevo {
				res.ClientesNuevos++
			}
		}
	}
	res.Filas = filas

	if !aplicar {
		return http.StatusOK, res, nil
	}
	if res.FilasConError > 0 {
		return 0, nil, errRegla("ARCHIVO_CON_ERRORES",
			fmt.Sprintf("El archivo tiene %d fila(s) con errores. Corrija el archivo y vuelva a subirlo.", res.FilasConError)).
			con("filas_con_error", res.FilasConError)
	}

	for i := range filas {
		f := &filas[i]
		if f.Omitida || len(f.Errores) > 0 {
			continue
		}
		cliente, existe := clientes[f.CodigoCliente]
		if !existe {
			datos := DatosCliente{
				Nombre: f.NombreCliente, Contacto: f.Contacto, Telefono: f.Telefono,
				PlazoCreditoDias: f.PlazoDias, LimiteCreditoCentavos: f.LimiteCentavos,
				CodigoExterno: f.CodigoCliente,
			}
			if datos, err = ValidarDatosCliente(datos, cfg); err != nil {
				return 0, nil, err
			}
			nuevoID, err := t.Repo.CrearCliente(t.EmpresaID, datos, t.Usuario.ID)
			if err != nil {
				return 0, nil, err
			}
			cliente, err = t.Repo.ObtenerCliente(t.EmpresaID, AlcanceEmpresa(), nuevoID)
			if err != nil {
				return 0, nil, err
			}
			clientes[f.CodigoCliente] = cliente
			res.ClientesCreados++
		}
		cuentaID, err := t.Repo.CrearCuenta(t.EmpresaID, cliente.ID, nil, f.MontoCentavos,
			f.FechaEmision, f.FechaVencimiento, cliente.VendedorID, f.NumeroDocumento)
		if err != nil {
			return 0, nil, err
		}
		res.CuentasCreadas++
		// Lo ya abonado entra como un pago histórico, para que el saldo cuadre.
		if f.AbonadoCentavos > 0 {
			if _, err := t.Repo.CrearMovimiento(t.EmpresaID, NuevoMovimiento{
				CuentaID: cuentaID, Tipo: TipoPago, MontoCentavos: f.AbonadoCentavos,
				Fecha: f.FechaEmision, Metodo: cfg.MetodosPago[0],
				Nota: "Abono previo, importado de la cartera inicial", RegistradoPor: t.Usuario.ID,
			}); err != nil {
				return 0, nil, err
			}
		}
	}
	res.Aplicada = true
	return http.StatusOK, res, nil
}

// leerCSVCartera interpreta el archivo y deja los errores por fila, sin detenerse en el primero.
func leerCSVCartera(contenido []byte, cfg ConfigEmpresa, hoy Fecha) ([]FilaImportacion, error) {
	texto := strings.TrimPrefix(string(contenido), string(rune(0xFEFF))) // BOM que agrega Excel
	lector := csv.NewReader(strings.NewReader(texto))
	lector.FieldsPerRecord = -1
	lector.TrimLeadingSpace = true
	registros, err := lector.ReadAll()
	if err != nil {
		return nil, errEntrada("CSV_INVALIDO", "El archivo no se pudo leer como CSV. Use la plantilla y guarde como CSV.")
	}
	if len(registros) < 2 {
		return nil, errEntrada("ARCHIVO_VACIO", "El archivo no tiene filas de datos.")
	}
	indice := map[string]int{}
	for i, columna := range registros[0] {
		indice[normalizarColumna(columna)] = i
	}
	for _, requerida := range []string{"nombre_cliente", "numero_documento", "fecha_emision", "monto"} {
		if _, ok := indice[requerida]; !ok {
			return nil, errEntrada("COLUMNA_FALTANTE",
				fmt.Sprintf("Falta la columna %q. Descargue la plantilla para ver el formato.", requerida)).
				con("columnas_esperadas", columnasImportacion)
		}
	}

	filas := make([]FilaImportacion, 0, len(registros)-1)
	for n, registro := range registros[1:] {
		campo := func(nombre string) string {
			i, ok := indice[nombre]
			if !ok || i >= len(registro) {
				return ""
			}
			return strings.TrimSpace(registro[i])
		}
		if strings.TrimSpace(strings.Join(registro, "")) == "" {
			continue // línea en blanco
		}
		f := FilaImportacion{
			Linea:           n + 2,
			CodigoCliente:   campo("codigo_cliente"),
			NombreCliente:   campo("nombre_cliente"),
			NumeroDocumento: campo("numero_documento"),
			Contacto:        campo("contacto"),
			Telefono:        campo("telefono"),
		}
		agregarError := func(formato string, args ...any) {
			f.Errores = append(f.Errores, fmt.Sprintf(formato, args...))
		}
		if f.NombreCliente == "" {
			agregarError("falta el nombre del cliente")
		}
		if f.CodigoCliente == "" {
			f.CodigoCliente = f.NombreCliente // sin código, se agrupa por nombre
		}
		f.MontoCentavos = leerMonto(campo("monto"), "monto", agregarError)
		f.AbonadoCentavos = leerMonto(campo("abonado"), "abonado", agregarError)
		f.LimiteCentavos = leerMonto(campo("limite_credito"), "límite de crédito", agregarError)
		if f.MontoCentavos <= 0 && !tieneError(f.Errores, "monto") {
			agregarError("el monto debe ser mayor a cero")
		}
		if f.AbonadoCentavos > f.MontoCentavos {
			agregarError("lo abonado no puede ser mayor que el monto")
		}
		f.SaldoCentavos = f.MontoCentavos - f.AbonadoCentavos

		f.PlazoDias = 30
		if texto := campo("plazo_dias"); texto != "" {
			if n, err := strconv.Atoi(texto); err == nil {
				f.PlazoDias = n
			} else {
				agregarError("el plazo debe ser un número de días")
			}
		}
		if emision, err := ParsearFecha(campo("fecha_emision")); err == nil {
			f.FechaEmision = emision
		} else {
			agregarError("la fecha de emisión debe tener formato AAAA-MM-DD")
		}
		if texto := campo("fecha_vencimiento"); texto != "" {
			if vencimiento, err := ParsearFecha(texto); err == nil {
				f.FechaVencimiento = vencimiento
			} else {
				agregarError("la fecha de vencimiento debe tener formato AAAA-MM-DD")
			}
		} else if !f.FechaEmision.EsCero() {
			f.FechaVencimiento = CalcularVencimiento(f.FechaEmision, f.PlazoDias)
		}
		if !f.FechaEmision.EsCero() && !f.FechaVencimiento.EsCero() && f.FechaVencimiento.Antes(f.FechaEmision) {
			agregarError("el vencimiento no puede ser anterior a la emisión")
		}
		if !f.FechaEmision.EsCero() {
			if err := ValidarFecha(f.FechaEmision, "fecha_emision"); err != nil {
				agregarError("la fecha de emisión está fuera de rango")
			}
		}
		if f.MontoCentavos > MontoMaximoCentavos {
			agregarError("el monto supera el máximo permitido")
		}
		filas = append(filas, f)
	}
	if len(filas) == 0 {
		return nil, errEntrada("ARCHIVO_VACIO", "El archivo no tiene filas de datos.")
	}
	return filas, nil
}

func tieneError(errores []string, texto string) bool {
	for _, e := range errores {
		if strings.Contains(e, texto) {
			return true
		}
	}
	return false
}

// leerMonto acepta "1,250.00", "1250", "$1,250.00" y vacío (= 0).
func leerMonto(texto, campo string, agregarError func(string, ...any)) int64 {
	if strings.TrimSpace(texto) == "" {
		return 0
	}
	limpio := strings.NewReplacer("$", "", ",", "", " ", "").Replace(texto)
	partes := strings.SplitN(limpio, ".", 2)
	entero, err := strconv.ParseInt(partes[0], 10, 64)
	if err != nil {
		agregarError("el %s no es un número válido", campo)
		return 0
	}
	var centavos int64
	if len(partes) == 2 {
		decimales := (partes[1] + "00")[:2]
		if centavos, err = strconv.ParseInt(decimales, 10, 64); err != nil {
			agregarError("el %s no es un número válido", campo)
			return 0
		}
	}
	if entero < 0 {
		agregarError("el %s no puede ser negativo", campo)
		return 0
	}
	return entero*100 + centavos
}

func normalizarColumna(c string) string {
	c = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(c, string(rune(0xFEFF)))))
	reemplazos := strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", " ", "_", "-", "_")
	return reemplazos.Replace(c)
}
