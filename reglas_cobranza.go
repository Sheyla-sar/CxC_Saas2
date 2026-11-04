package main

// reglas_cobranza.go — reglas de negocio de la fase 3 (cobranza en ruta y documentos).
// Igual que reglas.go: funciones puras, sin HTTP ni SQL, con la configuración de la empresa
// como parámetro. Ningún umbral ni catálogo está escrito aquí: todo viene de ConfigEmpresa.

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Métodos de pago
// ─────────────────────────────────────────────────────────────────────────────

// ValidarMetodoPago exige que la forma de pago esté entre las que la empresa configuró.
// Vacío = el primer método configurado (el más común, normalmente efectivo).
func ValidarMetodoPago(metodo string, cfg ConfigEmpresa) (string, error) {
	m := strings.ToLower(strings.TrimSpace(metodo))
	if m == "" {
		if len(cfg.MetodosPago) == 0 {
			return "", errEntrada("METODO_REQUERIDO", "Indique la forma de pago.")
		}
		return cfg.MetodosPago[0], nil
	}
	for _, permitido := range cfg.MetodosPago {
		if permitido == m {
			return m, nil
		}
	}
	return "", errRegla("METODO_NO_PERMITIDO",
		fmt.Sprintf("La forma de pago %q no está entre las que maneja su empresa.", metodo)).
		con("metodos_permitidos", cfg.MetodosPago)
}

// ─────────────────────────────────────────────────────────────────────────────
// Notas de crédito (R20 bis): devolución documentada que rebaja el saldo
// ─────────────────────────────────────────────────────────────────────────────

// ValidarNotaCredito funciona como la condonación, pero representa una devolución de mercadería
// o una corrección de factura, y lleva su propio correlativo. Igual que la condonación, no cuenta
// como pago para el comportamiento del cliente.
func ValidarNotaCredito(monto *int64, motivo string, fecha, hoy Fecha, saldo int64, cfg ConfigEmpresa) (int64, string, error) {
	m, err := ValidarMotivo(motivo, true, cfg)
	if err != nil {
		return 0, "", err
	}
	if saldo <= 0 {
		return 0, "", errRegla("CUENTA_SIN_SALDO", "La cuenta no tiene saldo pendiente que rebajar.")
	}
	valor := saldo
	if monto != nil {
		valor = *monto
	}
	if valor <= 0 {
		return 0, "", errEntrada("MONTO_INVALIDO", "El monto de la nota de crédito debe ser mayor a cero.")
	}
	if fecha.Despues(hoy) {
		return 0, "", errRegla("FECHA_FUTURA", "La fecha de la nota de crédito no puede ser posterior a hoy.")
	}
	if err := ValidarFecha(fecha, "fecha"); err != nil {
		return 0, "", err
	}
	if valor > saldo {
		return 0, "", errRegla("NOTA_CREDITO_EXCEDE_SALDO",
			fmt.Sprintf("La nota de crédito de %s supera el saldo pendiente de %s.",
				FormatearMonto(valor, cfg), FormatearMonto(saldo, cfg))).
			con("saldo_pendiente_centavos", saldo).
			con("monto_solicitado_centavos", valor)
	}
	return valor, m, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Anticipos: dinero del cliente sin factura asignada
// ─────────────────────────────────────────────────────────────────────────────

// EvaluarAnticipo calcula el saldo a favor que queda disponible.
func EvaluarAnticipo(a *Anticipo) {
	a.DisponibleCentavos = 0
	if !a.Anulado {
		a.DisponibleCentavos = a.MontoCentavos - a.AplicadoCentavos
	}
}

// ValidarAnticipo — monto positivo, fecha no futura y forma de pago válida.
func ValidarAnticipo(monto int64, fecha, hoy Fecha, metodo string, cfg ConfigEmpresa) (string, error) {
	if monto <= 0 {
		return "", errEntrada("MONTO_INVALIDO", "El monto del anticipo debe ser mayor a cero.")
	}
	if err := ValidarMonto(monto, "monto_centavos", cfg); err != nil {
		return "", err
	}
	if fecha.Despues(hoy) {
		return "", errRegla("FECHA_FUTURA", "La fecha del anticipo no puede ser posterior a hoy.")
	}
	if err := ValidarFecha(fecha, "fecha"); err != nil {
		return "", err
	}
	return ValidarMetodoPago(metodo, cfg)
}

// ValidarAplicacionAnticipo — cuánto se puede aplicar de un anticipo a una cuenta.
// Sin monto explícito aplica lo menor entre el disponible y el saldo de la cuenta.
func ValidarAplicacionAnticipo(a Anticipo, c Cuenta, monto *int64, cfg ConfigEmpresa) (int64, error) {
	if a.Anulado {
		return 0, errRegla("ANTICIPO_ANULADO", "Este anticipo fue anulado y ya no tiene saldo a favor.")
	}
	if a.ClienteID != c.ClienteID {
		return 0, errRegla("ANTICIPO_DE_OTRO_CLIENTE",
			"El anticipo pertenece a otro cliente. Solo puede aplicarse a cuentas del mismo cliente.")
	}
	if a.DisponibleCentavos <= 0 {
		return 0, errRegla("ANTICIPO_SIN_SALDO", "Este anticipo ya se aplicó por completo.")
	}
	if c.SaldoCentavos <= 0 {
		return 0, errRegla("CUENTA_SIN_SALDO", "La cuenta ya no tiene saldo pendiente.")
	}
	valor := min(a.DisponibleCentavos, c.SaldoCentavos)
	if monto != nil {
		valor = *monto
	}
	if valor <= 0 {
		return 0, errEntrada("MONTO_INVALIDO", "El monto a aplicar debe ser mayor a cero.")
	}
	if valor > a.DisponibleCentavos {
		return 0, errRegla("ANTICIPO_EXCEDE_DISPONIBLE",
			fmt.Sprintf("El anticipo %s solo tiene %s disponibles.", a.Numero, FormatearMonto(a.DisponibleCentavos, cfg))).
			con("disponible_centavos", a.DisponibleCentavos).
			con("monto_solicitado_centavos", valor)
	}
	// Aplicar un anticipo nunca genera sobrepago: es dinero ya cobrado.
	if valor > c.SaldoCentavos {
		return 0, errRegla("SOBREPAGO",
			fmt.Sprintf("Aplicar %s supera el saldo pendiente de %s.",
				FormatearMonto(valor, cfg), FormatearMonto(c.SaldoCentavos, cfg))).
			con("saldo_pendiente_centavos", c.SaldoCentavos).
			con("monto_recibido_centavos", valor)
	}
	return valor, nil
}

// ValidarAnulacionAnticipo — no se puede anular si ya se aplicó a cuentas.
func ValidarAnulacionAnticipo(a Anticipo, motivo string, cfg ConfigEmpresa) (string, error) {
	if a.Anulado {
		return "", errRegla("ANTICIPO_YA_ANULADO", "Este anticipo ya fue anulado.")
	}
	if a.AplicadoCentavos > 0 {
		return "", errRegla("ANTICIPO_YA_APLICADO",
			fmt.Sprintf("El anticipo ya tiene %s aplicados a cuentas. Anule primero esos movimientos.",
				FormatearMonto(a.AplicadoCentavos, cfg))).
			con("aplicado_centavos", a.AplicadoCentavos)
	}
	return ValidarMotivo(motivo, cfg.ExigirMotivoAnulacion, cfg)
}

// ─────────────────────────────────────────────────────────────────────────────
// Liquidación del cobrador
// ─────────────────────────────────────────────────────────────────────────────

// CorteCobrador es lo que el cobrador tiene pendiente de entregar.
type CorteCobrador struct {
	CobradorID            string           `json:"cobrador_id"`
	CobradorNombre        string           `json:"cobrador_nombre"`
	Fecha                 Fecha            `json:"fecha"`
	TotalEsperadoCentavos int64            `json:"total_esperado_centavos"`
	CantidadMovimientos   int              `json:"cantidad_movimientos"`
	PorMetodo             []TotalPorMetodo `json:"por_metodo"`
	Movimientos           []Movimiento     `json:"movimientos"`
	Anticipos             []Anticipo       `json:"anticipos"`
}

// ArmarCorte suma lo cobrado y lo desglosa por forma de pago. Solo agrupa datos que ya vienen
// del repo: no recalcula montos.
func ArmarCorte(cobrador Usuario, hoy Fecha, movimientos []Movimiento, anticipos []Anticipo) CorteCobrador {
	corte := CorteCobrador{
		CobradorID: cobrador.ID, CobradorNombre: cobrador.Nombre, Fecha: hoy,
		Movimientos: movimientos, Anticipos: anticipos,
	}
	totales := map[string]*TotalPorMetodo{}
	orden := []string{}
	acumular := func(metodo string, monto int64) {
		t, ok := totales[metodo]
		if !ok {
			t = &TotalPorMetodo{Metodo: metodo}
			totales[metodo] = t
			orden = append(orden, metodo)
		}
		t.Cantidad++
		t.MontoCentavos += monto
		corte.TotalEsperadoCentavos += monto
		corte.CantidadMovimientos++
	}
	for _, m := range movimientos {
		acumular(m.Metodo, m.MontoCentavos)
	}
	for _, a := range anticipos {
		acumular(a.Metodo, a.MontoCentavos)
	}
	corte.PorMetodo = []TotalPorMetodo{}
	for _, m := range orden {
		corte.PorMetodo = append(corte.PorMetodo, *totales[m])
	}
	return corte
}

// ValidarCierreLiquidacion — el cobrador declara cuánto entrega; la diferencia queda registrada
// y es el admin quien la aprueba o la rechaza.
func ValidarCierreLiquidacion(corte CorteCobrador, entregado int64, nota string, cfg ConfigEmpresa) (string, error) {
	if corte.CantidadMovimientos == 0 {
		return "", errRegla("SIN_COBROS_PENDIENTES", "No hay cobros pendientes de entregar.")
	}
	if entregado < 0 {
		return "", errEntrada("MONTO_INVALIDO", "El monto entregado no puede ser negativo.")
	}
	if err := ValidarMonto(entregado, "total_entregado_centavos", cfg); err != nil {
		return "", err
	}
	n, err := ValidarTexto(nota, "nota", LargoMaximoNota)
	if err != nil {
		return "", err
	}
	// Si hay faltante o sobrante, se exige explicación: es el control del corte de caja.
	if entregado != corte.TotalEsperadoCentavos && n == "" {
		return "", errEntrada("EXPLICACION_REQUERIDA",
			fmt.Sprintf("Lo entregado (%s) no coincide con lo cobrado (%s). Explique la diferencia.",
				FormatearMonto(entregado, cfg), FormatearMonto(corte.TotalEsperadoCentavos, cfg)))
	}
	return n, nil
}

// ValidarResolucionLiquidacion — solo se resuelve una liquidación entregada.
func ValidarResolucionLiquidacion(l Liquidacion, aprobar bool, motivo string, cfg ConfigEmpresa) (string, error) {
	if l.Estado != LiquidacionEntregada {
		return "", errRegla("LIQUIDACION_YA_RESUELTA", fmt.Sprintf("Esta liquidación ya fue %s.", l.Estado)).
			con("estado", l.Estado)
	}
	if aprobar {
		return "", nil
	}
	return ValidarMotivo(motivo, true, cfg)
}

// EvaluarLiquidacion calcula la diferencia entre lo cobrado y lo entregado.
func EvaluarLiquidacion(l *Liquidacion) {
	l.DiferenciaCentavos = l.TotalEntregadoCentavos - l.TotalEsperadoCentavos
}

// ─────────────────────────────────────────────────────────────────────────────
// Adjuntos
// ─────────────────────────────────────────────────────────────────────────────

// ValidarAdjunto revisa que la empresa acepte adjuntos, que el tipo de archivo sea de los
// permitidos y que no exceda el tamaño configurado. Devuelve la extensión que corresponde.
func ValidarAdjunto(entidad, tipoMime string, tamanoBytes int64, cfg ConfigEmpresa) (string, error) {
	if !cfg.PermitirAdjuntos {
		return "", errRegla("ADJUNTOS_DESHABILITADOS", "Su empresa no tiene habilitados los archivos adjuntos.")
	}
	if !contiene(EntidadesConAdjunto, entidad) {
		return "", errEntrada("ENTIDAD_INVALIDA", "No se pueden adjuntar archivos a ese tipo de registro.").
			con("entidades_validas", EntidadesConAdjunto)
	}
	extension, ok := TiposMimeAdjunto[tipoMime]
	if !ok {
		return "", errEntrada("TIPO_ARCHIVO_INVALIDO",
			"Solo se aceptan fotos JPG o PNG y archivos PDF.").con("tipo_recibido", tipoMime)
	}
	maximo := int64(cfg.TamanoMaximoAdjuntoMB) * 1024 * 1024
	if tamanoBytes <= 0 {
		return "", errEntrada("ARCHIVO_VACIO", "El archivo está vacío.")
	}
	if tamanoBytes > maximo {
		return "", errRegla("ARCHIVO_DEMASIADO_GRANDE",
			fmt.Sprintf("El archivo pesa más de %d MB. Tome la foto con menor resolución o comprímala.", cfg.TamanoMaximoAdjuntoMB)).
			con("maximo_mb", cfg.TamanoMaximoAdjuntoMB)
	}
	return extension, nil
}

func contiene(lista []string, valor string) bool {
	for _, x := range lista {
		if x == valor {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Antigüedad de saldos (aging): base del estado de cuenta y de los tableros
// ─────────────────────────────────────────────────────────────────────────────

type TramoAntiguedad struct {
	Etiqueta      string `json:"etiqueta"`
	DesdeDias     int    `json:"desde_dias"`
	HastaDias     int    `json:"hasta_dias"` // -1 = sin tope
	SaldoCentavos int64  `json:"saldo_centavos"`
	Cantidad      int    `json:"cantidad"`
}

// AntiguedadSaldos clasifica el saldo pendiente por días de vencido. Recibe cuentas ya evaluadas
// y usa los días que calculó EvaluarCuenta; no recalcula fechas.
func AntiguedadSaldos(cuentas []Cuenta) []TramoAntiguedad {
	tramos := []TramoAntiguedad{
		{Etiqueta: "Por vencer", DesdeDias: -1, HastaDias: 0},
		{Etiqueta: "1 a 30 días", DesdeDias: 1, HastaDias: 30},
		{Etiqueta: "31 a 60 días", DesdeDias: 31, HastaDias: 60},
		{Etiqueta: "61 a 90 días", DesdeDias: 61, HastaDias: 90},
		{Etiqueta: "Más de 90 días", DesdeDias: 91, HastaDias: -1},
	}
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		indice := 0 // aún no vencida
		switch dias := c.DiasVencida; {
		case dias <= 0:
			indice = 0
		case dias <= 30:
			indice = 1
		case dias <= 60:
			indice = 2
		case dias <= 90:
			indice = 3
		default:
			indice = 4
		}
		tramos[indice].SaldoCentavos += c.SaldoCentavos
		tramos[indice].Cantidad++
	}
	return tramos
}

// ─────────────────────────────────────────────────────────────────────────────
// Etiquetas para los documentos impresos (el frontend tiene las suyas en formato.js)
// ─────────────────────────────────────────────────────────────────────────────

func EtiquetaEstadoCuenta(estado string) string {
	switch estado {
	case EstadoAlDia:
		return "Al día"
	case EstadoPorVencer:
		return "Por vencer"
	case EstadoVencido:
		return "Vencida"
	case EstadoPagado:
		return "Pagada"
	}
	return estado
}

func EtiquetaTipoMovimiento(tipo string) string {
	switch tipo {
	case TipoPago:
		return "Pago"
	case TipoCondonacion:
		return "Condonación"
	case TipoNotaCredito:
		return "Nota de crédito"
	}
	return tipo
}

// EtiquetaMetodoPago: la empresa define el catálogo; aquí solo se le pone un nombre presentable.
func EtiquetaMetodoPago(metodo string) string {
	switch metodo {
	case "efectivo":
		return "Efectivo"
	case "transferencia":
		return "Transferencia"
	case "cheque":
		return "Cheque"
	case "deposito":
		return "Depósito"
	case "tarjeta":
		return "Tarjeta"
	case MetodoAnticipo:
		return "Anticipo aplicado"
	case "":
		return "Sin especificar"
	}
	return strings.ToUpper(metodo[:1]) + metodo[1:]
}

// EtiquetaDiferencia explica en palabras una diferencia de liquidación.
func EtiquetaDiferencia(centavos int64, cfg ConfigEmpresa) string {
	switch {
	case centavos == 0:
		return "Cuadra"
	case centavos < 0:
		return "Faltan " + FormatearMonto(-centavos, cfg)
	}
	return "Sobran " + FormatearMonto(centavos, cfg)
}

func TextoComportamiento(c Comportamiento) string {
	switch c.Clasificacion {
	case ComportamientoSinHistorial:
		return "sin historial suficiente para evaluar"
	case ComportamientoBueno:
		return fmt.Sprintf("buen pagador (promedio %.1f días de atraso)", valorODefecto(c.PromedioDiasAtraso))
	case ComportamientoRegular:
		return fmt.Sprintf("pagador regular (promedio %.1f días de atraso)", valorODefecto(c.PromedioDiasAtraso))
	case ComportamientoMalo:
		return fmt.Sprintf("mal pagador (promedio %.1f días de atraso)", valorODefecto(c.PromedioDiasAtraso))
	}
	return c.Clasificacion
}

func valorODefecto(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// ─────────────────────────────────────────────────────────────────────────────
// Usuarios (gestión desde la aplicación)
// ─────────────────────────────────────────────────────────────────────────────

const LargoMinimoPassword = 8

var RolesValidos = []string{RolAdmin, RolVendedor}

type DatosUsuario struct {
	Nombre, Email, Rol string
	Activo             bool
}

func ValidarDatosUsuario(d DatosUsuario) (DatosUsuario, error) {
	var err error
	if d.Nombre, err = ValidarTexto(d.Nombre, "nombre", 120); err != nil {
		return d, err
	}
	if d.Nombre == "" {
		return d, errEntrada("NOMBRE_REQUERIDO", "Escriba el nombre del usuario.")
	}
	d.Email = strings.ToLower(strings.TrimSpace(d.Email))
	if d.Email != "" && !patronEmail.MatchString(d.Email) {
		return d, errEntrada("CORREO_INVALIDO", "El correo no tiene un formato válido.")
	}
	if !contiene(RolesValidos, d.Rol) {
		return d, errEntrada("ROL_INVALIDO", "El rol debe ser administrador o vendedor.").con("roles_validos", RolesValidos)
	}
	return d, nil
}

func ValidarPassword(clave string) error {
	if len([]rune(clave)) < LargoMinimoPassword {
		return errEntrada("CONTRASENA_CORTA",
			fmt.Sprintf("La contraseña debe tener al menos %d caracteres.", LargoMinimoPassword)).
			con("longitud_minima", LargoMinimoPassword)
	}
	if len(clave) > 72 { // límite de bcrypt
		return errEntrada("CONTRASENA_LARGA", "La contraseña no puede superar 72 caracteres.")
	}
	return nil
}

// ValidarCambioDeRol evita que la empresa quede sin administrador activo.
func ValidarCambioDeRol(objetivo Usuario, nuevo DatosUsuario, adminsRestantes int) error {
	quedaSinAdmin := objetivo.Rol == RolAdmin && objetivo.Activo &&
		(nuevo.Rol != RolAdmin || !nuevo.Activo) && adminsRestantes == 0
	if quedaSinAdmin {
		return errRegla("ULTIMO_ADMIN",
			"Es el único administrador activo. Nombre a otro administrador antes de cambiar este usuario.")
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tableros. Funciones puras sobre cuentas YA evaluadas: usan el saldo, el estado y los días
// que calculó EvaluarCuenta. Nunca recalculan fechas ni vuelven a mirar la base.
// ─────────────────────────────────────────────────────────────────────────────

// TramoVencimiento responde "¿cuánto me entra si todos pagan a tiempo?" para un corte de días.
// Es acumulado: el tramo de 30 días incluye lo que vence en 7 y en 15.
type TramoVencimiento struct {
	Dias          int   `json:"dias"`
	Cantidad      int   `json:"cantidad"`
	SaldoCentavos int64 `json:"saldo_centavos"`
}

// ProximosVencimientos suma el saldo que vence dentro de cada corte, sin contar lo ya vencido.
func ProximosVencimientos(cuentas []Cuenta, cortes []int) []TramoVencimiento {
	tramos := make([]TramoVencimiento, len(cortes))
	for i, d := range cortes {
		tramos[i].Dias = d
	}
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 || c.Estado == EstadoVencido {
			continue
		}
		for i, corte := range cortes {
			if c.DiasParaVencer <= corte {
				tramos[i].SaldoCentavos += c.SaldoCentavos
				tramos[i].Cantidad++
			}
		}
	}
	return tramos
}

// CarteraVendedor es la cartera viva de un vendedor: lo que tiene por cobrar y lo que se le atrasó.
type CarteraVendedor struct {
	VendedorID      string  `json:"vendedor_id"`
	Nombre          string  `json:"nombre"`
	SaldoCentavos   int64   `json:"saldo_centavos"`
	VencidoCentavos int64   `json:"vencido_centavos"`
	Clientes        int     `json:"clientes"`
	CuentasVencidas int     `json:"cuentas_vencidas"`
	PorcentajeMora  float64 `json:"porcentaje_mora"` // vencido / saldo, redondeado a un decimal
}

// CarteraPorVendedor agrupa las cuentas con saldo por vendedor, de mayor a menor mora.
func CarteraPorVendedor(cuentas []Cuenta) []CarteraVendedor {
	indice := map[string]*CarteraVendedor{}
	clientes := map[string]map[string]bool{}
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		v, ok := indice[c.VendedorID]
		if !ok {
			v = &CarteraVendedor{VendedorID: c.VendedorID, Nombre: c.VendedorNombre}
			indice[c.VendedorID] = v
			clientes[c.VendedorID] = map[string]bool{}
		}
		v.SaldoCentavos += c.SaldoCentavos
		if c.Estado == EstadoVencido {
			v.VencidoCentavos += c.SaldoCentavos
			v.CuentasVencidas++
		}
		clientes[c.VendedorID][c.ClienteID] = true
	}
	res := make([]CarteraVendedor, 0, len(indice))
	for id, v := range indice {
		v.Clientes = len(clientes[id])
		if v.SaldoCentavos > 0 {
			v.PorcentajeMora = math.Round(float64(v.VencidoCentavos)*1000/float64(v.SaldoCentavos)) / 10
		}
		res = append(res, *v)
	}
	sort.Slice(res, func(i, j int) bool {
		if res[i].VencidoCentavos != res[j].VencidoCentavos {
			return res[i].VencidoCentavos > res[j].VencidoCentavos
		}
		return res[i].Nombre < res[j].Nombre
	})
	return res
}

// ─────────────────────────────────────────────────────────────────────────────
// Informe mensual del dueño
// ─────────────────────────────────────────────────────────────────────────────

// PorcentajeDe devuelve parte/total en porcentaje, con un decimal. Total cero = 0.
func PorcentajeDe(parte, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(parte)*1000/float64(total)) / 10
}

// ConcentracionCartera: qué parte del saldo está en los `top` clientes más grandes.
func ConcentracionCartera(cuentas []Cuenta, top int) (parte, total int64, clientes int) {
	saldos := []int64{}
	for id, s := range SaldosPorCliente(cuentas) {
		_ = id
		if s.SaldoCentavos > 0 {
			saldos = append(saldos, s.SaldoCentavos)
			total += s.SaldoCentavos
		}
	}
	sort.Slice(saldos, func(i, j int) bool { return saldos[i] > saldos[j] })
	if top > len(saldos) {
		top = len(saldos)
	}
	for i := 0; i < top; i++ {
		parte += saldos[i]
	}
	return parte, total, len(saldos)
}

// PuntosDeAtencion arma las frases del informe. Son HECHOS derivados de los datos, nunca juicios
// ni recomendaciones: el dueño saca sus conclusiones. Si un hecho no aplica, no se escribe.
func PuntosDeAtencion(cuentas []Cuenta, liquidaciones []Liquidacion, cfg ConfigEmpresa) []string {
	var puntos []string
	tramos := AntiguedadSaldos(cuentas)
	var total int64
	for _, t := range tramos {
		total += t.SaldoCentavos
	}
	var vencido int64
	for _, t := range tramos[1:] {
		vencido += t.SaldoCentavos
	}
	if total > 0 && vencido > 0 {
		puntos = append(puntos, fmt.Sprintf(
			"El %s de la cartera está vencida: %s de %s.",
			formatearPorcentaje(PorcentajeDe(vencido, total)), FormatearMonto(vencido, cfg), FormatearMonto(total, cfg)))
	}
	if viejo := tramos[len(tramos)-1]; viejo.SaldoCentavos > 0 {
		puntos = append(puntos, fmt.Sprintf(
			"%s en %d cuenta(s) pasa de los 90 días de vencida.",
			FormatearMonto(viejo.SaldoCentavos, cfg), viejo.Cantidad))
	}
	if parte, suma, clientes := ConcentracionCartera(cuentas, 3); clientes >= 3 && suma > 0 {
		puntos = append(puntos, fmt.Sprintf(
			"Los 3 clientes más grandes concentran el %s de lo que se debe (%s de %s entre %d clientes).",
			formatearPorcentaje(PorcentajeDe(parte, suma)), FormatearMonto(parte, cfg), FormatearMonto(suma, cfg), clientes))
	}
	var faltante int64
	var conFaltante int
	for _, l := range liquidaciones {
		if l.Estado != LiquidacionRechazada && l.DiferenciaCentavos < 0 {
			faltante -= l.DiferenciaCentavos
			conFaltante++
		}
	}
	if conFaltante > 0 {
		puntos = append(puntos, fmt.Sprintf(
			"%d liquidación(es) del mes cerraron con faltante, por %s en total.",
			conFaltante, FormatearMonto(faltante, cfg)))
	}
	return puntos
}

// formatearPorcentaje: 8.4 → "8,4 %" (coma decimal, como se escribe en español).
func formatearPorcentaje(n float64) string {
	return strings.Replace(strconv.FormatFloat(n, 'f', -1, 64), ".", ",", 1) + " %"
}

// ValidarAltaUsuario — R77. El plan contratado limita cuántos usuarios ACTIVOS puede tener la
// empresa (`limite_usuarios`, 0 = sin límite). Se cuenta al crear y al reactivar, no al editar.
func ValidarAltaUsuario(activos int, cfg ConfigEmpresa) error {
	if cfg.LimiteUsuarios <= 0 || activos < cfg.LimiteUsuarios {
		return nil
	}
	return errRegla("LIMITE_USUARIOS",
		fmt.Sprintf("Su plan permite %d usuario(s) activo(s) y ya tiene %d. Desactive uno o amplíe su plan.",
			cfg.LimiteUsuarios, activos)).
		con("limite_usuarios", cfg.LimiteUsuarios).
		con("usuarios_activos", activos)
}
