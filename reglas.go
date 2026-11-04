package main

// reglas.go — lógica de negocio pura. Sin HTTP y sin SQL.
// Cada función recibe datos ya cargados y la ConfigEmpresa, y devuelve resultados o *ErrorApp.
// Ningún umbral de negocio está escrito aquí como constante: todos vienen de ConfigEmpresa.
// Los números de regla (R#) corresponden a CLAUDE.md §7.

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	RolAdmin    = "admin"
	RolVendedor = "vendedor"

	EstadoAlDia     = "al_dia"
	EstadoPorVencer = "por_vencer"
	EstadoVencido   = "vencido"
	EstadoPagado    = "pagado"
	// EstadoTodas no es un estado: es el valor del filtro que no filtra.
	EstadoTodas = "todas"

	TipoPago        = "pago"
	TipoCondonacion = "condonacion"

	AjustePendiente = "pendiente"
	AjusteAprobado  = "aprobado"
	AjusteRechazado = "rechazado"

	EtapaNuevo       = "nuevo"
	EtapaContactado  = "contactado"
	EtapaNegociacion = "negociacion"
	EtapaGanado      = "ganado"
	EtapaPerdido     = "perdido"

	ComportamientoBueno        = "bueno"
	ComportamientoRegular      = "regular"
	ComportamientoMalo         = "malo"
	ComportamientoSinHistorial = "sin_historial"
)

// R6: las cinco etapas del pipeline, en orden.
var EtapasOportunidad = []string{EtapaNuevo, EtapaContactado, EtapaNegociacion, EtapaGanado, EtapaPerdido}

var EstadosCuenta = []string{EstadoAlDia, EstadoPorVencer, EstadoVencido, EstadoPagado}

// ─────────────────────────────────────────────────────────────────────────────
// Tiempo
// ─────────────────────────────────────────────────────────────────────────────

var cacheZonas sync.Map // nombre → *time.Location

func cargarZona(zona string) (*time.Location, error) {
	if loc, ok := cacheZonas.Load(zona); ok {
		return loc.(*time.Location), nil
	}
	loc, err := time.LoadLocation(zona)
	if err != nil {
		return nil, err
	}
	cacheZonas.Store(zona, loc)
	return loc, nil
}

// HoyEn devuelve la fecha de hoy en la zona horaria de la empresa (R14). `ahora` se recibe como
// parámetro para que la función sea pura.
func HoyEn(zona string, ahora time.Time) (Fecha, error) {
	loc, err := cargarZona(zona)
	if err != nil {
		return Fecha{}, fmt.Errorf("zona horaria %q inválida: %w", zona, err)
	}
	return NuevaFecha(ahora.In(loc).Date()), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Formato para mensajes al usuario final
// ─────────────────────────────────────────────────────────────────────────────

// FormatearMonto convierte centavos en texto legible con el símbolo de la empresa: $1,234.56.
func FormatearMonto(centavos int64, cfg ConfigEmpresa) string {
	signo := ""
	if centavos < 0 {
		signo = "-"
		centavos = -centavos
	}
	enteros := strconv.FormatInt(centavos/100, 10)
	var b strings.Builder
	for i, r := range enteros {
		if i > 0 && (len(enteros)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s%s%s.%02d", signo, cfg.SimboloMoneda, b.String(), centavos%100)
}

// ─────────────────────────────────────────────────────────────────────────────
// Límites de plataforma (no de negocio): protegen la aritmética y la base de datos, no cambian
// entre empresas. Con montos de hasta mil millones de dólares, sumar decenas de miles de cuentas
// no desborda int64 ni supera los enteros exactos de JavaScript (2^53).
// ─────────────────────────────────────────────────────────────────────────────

const (
	MontoMaximoCentavos = 100_000_000_000 // $1,000,000,000.00
	LargoMaximoMotivo   = 500
	LargoMaximoNota     = 1000
	LargoMaximoTitulo   = 120
	LargoMaximoContacto = 120
	LargoMaximoTelefono = 40
	LargoMaximoCodigo   = 60 // código externo y número de documento
	LargoMaximoMetodo   = 30
)

// Tipos de movimiento y estados de liquidación (fase 3).
const (
	TipoNotaCredito = "nota_credito"

	LiquidacionEntregada = "entregada"
	LiquidacionAprobada  = "aprobada"
	LiquidacionRechazada = "rechazada"

	// MetodoAnticipo identifica los movimientos que no son dinero nuevo, sino aplicación de un
	// anticipo ya cobrado: no se liquidan otra vez.
	MetodoAnticipo = "anticipo"
)

// EntidadesConAdjunto: a qué se le pueden pegar fotos o PDF.
var EntidadesConAdjunto = []string{"cuenta", "movimiento", "liquidacion", "anticipo", "cliente"}

var TiposMimeAdjunto = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

var (
	fechaMinima = NuevaFecha(1900, time.January, 1)
	fechaMaxima = NuevaFecha(2199, time.December, 31)
)

// ValidarMonto rechaza montos fuera del rango que la plataforma puede sumar sin desbordes.
// La positividad la valida cada regla con su propio mensaje.
func ValidarMonto(centavos int64, campo string, cfg ConfigEmpresa) error {
	if centavos > MontoMaximoCentavos || centavos < -MontoMaximoCentavos {
		return errEntrada("MONTO_FUERA_DE_RANGO",
			fmt.Sprintf("El monto no puede superar %s.", FormatearMonto(MontoMaximoCentavos, cfg))).
			con("campo", campo).con("maximo_centavos", int64(MontoMaximoCentavos))
	}
	return nil
}

// ValidarFecha rechaza fechas de negocio absurdas (años 0001 o 9999), que romperían cálculos de días.
func ValidarFecha(f Fecha, campo string) error {
	if f.Antes(fechaMinima) || f.Despues(fechaMaxima) {
		return errEntrada("FECHA_FUERA_DE_RANGO",
			fmt.Sprintf("La fecha %s no es válida: debe estar entre %s y %s.", f, fechaMinima, fechaMaxima)).
			con("campo", campo)
	}
	return nil
}

// ValidarTexto limpia espacios y limita el largo de un texto libre.
func ValidarTexto(valor, campo string, maximo int) (string, error) {
	v := strings.TrimSpace(valor)
	if utf8.RuneCountInString(v) > maximo {
		return v, errEntrada("TEXTO_DEMASIADO_LARGO",
			fmt.Sprintf("El campo %s no puede superar %d caracteres.", campo, maximo)).
			con("campo", campo).con("maximo", maximo)
	}
	return v, nil
}

// ValidarMotivo exige un texto con la longitud mínima configurada. Si no es obligatorio y viene
// vacío, se acepta. Devuelve el motivo sin espacios sobrantes.
func ValidarMotivo(motivo string, obligatorio bool, cfg ConfigEmpresa) (string, error) {
	m, err := ValidarTexto(motivo, "motivo", LargoMaximoMotivo)
	if err != nil {
		return "", err
	}
	if !obligatorio && m == "" {
		return "", nil
	}
	minimo := max(cfg.LongitudMinimaMotivo, 1)
	if n := utf8.RuneCountInString(m); n < minimo {
		return "", errEntrada("MOTIVO_INSUFICIENTE",
			fmt.Sprintf("Escriba un motivo de al menos %d caracteres para dejar constancia de la razón.", minimo)).
			con("longitud_minima", minimo).con("longitud_recibida", n)
	}
	return m, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Clientes y crédito (R1–R5)
// ─────────────────────────────────────────────────────────────────────────────

// ValidarPlazo — R2.
func ValidarPlazo(plazo int, cfg ConfigEmpresa) error {
	if plazo <= 0 {
		return errEntrada("PLAZO_INVALIDO", "El plazo de crédito debe ser un número de días mayor a cero.")
	}
	for _, p := range cfg.PlazosCreditoPermitidos {
		if p == plazo {
			return nil
		}
	}
	return errRegla("PLAZO_NO_PERMITIDO",
		fmt.Sprintf("El plazo de %d días no está entre los plazos de crédito que maneja su empresa.", plazo)).
		con("plazo_solicitado_dias", plazo).
		con("plazos_permitidos", cfg.PlazosCreditoPermitidos)
}

// DatosCliente son los campos editables de un cliente, ya combinados (crear o actualizar).
type DatosCliente struct {
	Nombre, Contacto, Telefono string
	PlazoCreditoDias           int
	LimiteCreditoCentavos      int64
	CodigoExterno              string // código en el sistema del que se importó
}

// ValidarDatosCliente — R1, R2, R4.
func ValidarDatosCliente(d DatosCliente, cfg ConfigEmpresa) (DatosCliente, error) {
	d.Nombre = strings.TrimSpace(d.Nombre)
	d.Contacto = strings.TrimSpace(d.Contacto)
	d.Telefono = strings.TrimSpace(d.Telefono)
	if d.Nombre == "" {
		return d, errEntrada("NOMBRE_REQUERIDO", "Escriba el nombre del cliente.")
	}
	if utf8.RuneCountInString(d.Nombre) > 200 {
		return d, errEntrada("NOMBRE_DEMASIADO_LARGO", "El nombre del cliente no puede superar 200 caracteres.")
	}
	if d.LimiteCreditoCentavos < 0 {
		return d, errEntrada("LIMITE_INVALIDO", "El límite de crédito no puede ser negativo. Use 0 para indicar que no tiene límite.")
	}
	if err := ValidarMonto(d.LimiteCreditoCentavos, "limite_credito_centavos", cfg); err != nil {
		return d, err
	}
	var err error
	if d.Contacto, err = ValidarTexto(d.Contacto, "contacto", LargoMaximoContacto); err != nil {
		return d, err
	}
	if d.Telefono, err = ValidarTexto(d.Telefono, "teléfono", LargoMaximoTelefono); err != nil {
		return d, err
	}
	if d.CodigoExterno, err = ValidarTexto(d.CodigoExterno, "código", LargoMaximoCodigo); err != nil {
		return d, err
	}
	if err := ValidarPlazo(d.PlazoCreditoDias, cfg); err != nil {
		return d, err
	}
	return d, nil
}

// SaldoCliente resume la deuda de un cliente a partir de sus cuentas ya evaluadas.
type SaldoCliente struct {
	SaldoCentavos        int64 `json:"saldo_centavos"`
	SaldoVencidoCentavos int64 `json:"saldo_vencido_centavos"`
	CuentasAbiertas      int   `json:"cuentas_abiertas"`
	CuentasVencidas      int   `json:"cuentas_vencidas"`
}

// SaldosPorCliente agrupa por cliente las cuentas con saldo pendiente (> 0).
func SaldosPorCliente(cuentas []Cuenta) map[string]SaldoCliente {
	res := map[string]SaldoCliente{}
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		s := res[c.ClienteID]
		s.SaldoCentavos += c.SaldoCentavos
		s.CuentasAbiertas++
		if c.Estado == EstadoVencido {
			s.SaldoVencidoCentavos += c.SaldoCentavos
			s.CuentasVencidas++
		}
		res[c.ClienteID] = s
	}
	return res
}

// ValidarDesactivacion — R3.
func ValidarDesactivacion(s SaldoCliente, cfg ConfigEmpresa) error {
	if s.CuentasAbiertas > 0 {
		return errRegla("CLIENTE_CON_SALDO",
			fmt.Sprintf("No se puede desactivar el cliente: tiene %d cuenta(s) abierta(s) con saldo de %s.",
				s.CuentasAbiertas, FormatearMonto(s.SaldoCentavos, cfg))).
			con("saldo_pendiente_centavos", s.SaldoCentavos).
			con("cuentas_abiertas", s.CuentasAbiertas)
	}
	return nil
}

// CreditoCliente — R4: 0 significa sin límite; el disponible puede quedar negativo si ya se excedió.
type CreditoCliente struct {
	LimiteCreditoCentavos int64  `json:"limite_credito_centavos"`
	SinLimite             bool   `json:"sin_limite"`
	SaldoAbiertoCentavos  int64  `json:"saldo_abierto_centavos"`
	DisponibleCentavos    *int64 `json:"disponible_centavos"`
}

func CalcularCredito(limite, saldoAbierto int64) CreditoCliente {
	c := CreditoCliente{LimiteCreditoCentavos: limite, SinLimite: limite == 0, SaldoAbiertoCentavos: saldoAbierto}
	if limite > 0 {
		d := limite - saldoAbierto
		c.DisponibleCentavos = &d
	}
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// Cuenta por cobrar (R11–R14)
// ─────────────────────────────────────────────────────────────────────────────

// CalcularVencimiento — R11.
func CalcularVencimiento(emision Fecha, plazoDias int) Fecha { return emision.SumarDias(plazoDias) }

// CalcularSaldo — R12: monto total menos pagos y condonaciones no anulados.
func CalcularSaldo(montoTotal, pagado, condonado, notasCredito int64) int64 {
	return montoTotal - pagado - condonado - notasCredito
}

// CalcularEstado — R13. `hoy` debe venir de HoyEn con la zona de la empresa (R14).
func CalcularEstado(saldo int64, vencimiento, hoy Fecha, cfg ConfigEmpresa) string {
	switch {
	case saldo <= 0:
		return EstadoPagado
	case hoy.Despues(vencimiento):
		return EstadoVencido
	case DiasEntre(hoy, vencimiento) <= cfg.DiasAvisoPrevio:
		return EstadoPorVencer
	default:
		return EstadoAlDia
	}
}

// EvaluarCuenta llena los campos calculados de una cuenta.
func EvaluarCuenta(c *Cuenta, hoy Fecha, cfg ConfigEmpresa) {
	c.SaldoCentavos = CalcularSaldo(c.MontoTotalCentavos, c.PagadoCentavos, c.CondonadoCentavos, c.NotaCreditoCentavos)
	c.Estado = CalcularEstado(c.SaldoCentavos, c.FechaVencimiento, hoy, cfg)
	c.DiasParaVencer = DiasEntre(hoy, c.FechaVencimiento)
	c.DiasVencida = 0
	if c.Estado == EstadoVencido {
		c.DiasVencida = -c.DiasParaVencer
	}
}

func EsEstadoCuentaValido(e string) bool {
	for _, x := range EstadosCuenta {
		if x == e {
			return true
		}
	}
	return false
}

// SolicitudCuenta reúne lo necesario para decidir si se puede crear una cuenta.
type SolicitudCuenta struct {
	Cliente             Cliente
	MontoCentavos       int64
	FechaEmision        *Fecha // nil = hoy
	FechaVencimiento    *Fecha // nil = emisión + plazo del cliente
	SaldoAbiertoCliente int64  // suma de saldos > 0 del cliente antes de esta cuenta
	Hoy                 Fecha
}

type PlanCuenta struct {
	FechaEmision     Fecha
	FechaVencimiento Fecha
	Advertencia      *AdvertenciaLimite
}

// AdvertenciaLimite acompaña la respuesta cuando se excede el límite y la empresa no lo exige (R5).
type AdvertenciaLimite struct {
	Codigo                string `json:"codigo"`
	Mensaje               string `json:"mensaje"`
	LimiteCreditoCentavos int64  `json:"limite_credito_centavos"`
	SaldoAbiertoCentavos  int64  `json:"saldo_abierto_centavos"`
	MontoNuevoCentavos    int64  `json:"monto_nuevo_centavos"`
	ExcesoCentavos        int64  `json:"exceso_centavos"`
}

// PlanificarCuenta — R5, R10, R11.
func PlanificarCuenta(s SolicitudCuenta, cfg ConfigEmpresa) (PlanCuenta, error) {
	var p PlanCuenta
	if s.MontoCentavos <= 0 {
		return p, errEntrada("MONTO_INVALIDO", "El monto de la cuenta debe ser mayor a cero.")
	}
	if err := ValidarMonto(s.MontoCentavos, "monto_total_centavos", cfg); err != nil {
		return p, err
	}
	for campo, f := range map[string]*Fecha{"fecha_emision": s.FechaEmision, "fecha_vencimiento": s.FechaVencimiento} {
		if f != nil {
			if err := ValidarFecha(*f, campo); err != nil {
				return p, err
			}
		}
	}
	if !s.Cliente.Activo {
		return p, errRegla("CLIENTE_INACTIVO", "El cliente está desactivado. Reactívelo antes de registrarle una venta al crédito.")
	}

	p.FechaEmision = s.Hoy
	if s.FechaEmision != nil {
		p.FechaEmision = *s.FechaEmision
	}
	if s.FechaVencimiento != nil {
		if s.FechaVencimiento.Antes(p.FechaEmision) {
			return p, errEntrada("FECHA_VENCIMIENTO_INVALIDA", "La fecha de vencimiento no puede ser anterior a la fecha de emisión.")
		}
		p.FechaVencimiento = *s.FechaVencimiento
	} else {
		if err := ValidarPlazo(s.Cliente.PlazoCreditoDias, cfg); err != nil {
			return p, err
		}
		p.FechaVencimiento = CalcularVencimiento(p.FechaEmision, s.Cliente.PlazoCreditoDias)
	}

	limite := s.Cliente.LimiteCreditoCentavos
	if limite > 0 && s.SaldoAbiertoCliente+s.MontoCentavos > limite {
		exceso := s.SaldoAbiertoCliente + s.MontoCentavos - limite
		if cfg.ExigirLimiteCredito {
			return p, errRegla("LIMITE_CREDITO_EXCEDIDO",
				fmt.Sprintf("Esta venta supera el límite de crédito del cliente (%s) por %s. Disponible: %s.",
					FormatearMonto(limite, cfg), FormatearMonto(exceso, cfg),
					FormatearMonto(max(limite-s.SaldoAbiertoCliente, 0), cfg))).
				con("limite_credito_centavos", limite).
				con("saldo_abierto_centavos", s.SaldoAbiertoCliente).
				con("monto_nuevo_centavos", s.MontoCentavos).
				con("exceso_centavos", exceso)
		}
		p.Advertencia = &AdvertenciaLimite{
			Codigo: "LIMITE_CREDITO_EXCEDIDO",
			Mensaje: fmt.Sprintf("La cuenta se registró, pero el cliente quedó %s por encima de su límite de crédito de %s.",
				FormatearMonto(exceso, cfg), FormatearMonto(limite, cfg)),
			LimiteCreditoCentavos: limite,
			SaldoAbiertoCentavos:  s.SaldoAbiertoCliente,
			MontoNuevoCentavos:    s.MontoCentavos,
			ExcesoCentavos:        exceso,
		}
	}
	return p, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Oportunidades (R6–R9)
// ─────────────────────────────────────────────────────────────────────────────

func EsEtapaValida(e string) bool {
	for _, x := range EtapasOportunidad {
		if x == e {
			return true
		}
	}
	return false
}

// ValidarCambioEtapa — R6, R9: una oportunidad convertida queda en 'ganado' para siempre.
func ValidarCambioEtapa(op Oportunidad, nueva string) error {
	if !EsEtapaValida(nueva) {
		return errEntrada("ETAPA_INVALIDA", "La etapa indicada no existe.").con("etapas_validas", EtapasOportunidad)
	}
	if op.CuentaID != nil && nueva != EtapaGanado {
		return errRegla("OPORTUNIDAD_YA_CONVERTIDA",
			"Esta oportunidad ya se convirtió en una cuenta por cobrar y no puede cambiar de etapa.").
			con("cuenta_id", *op.CuentaID)
	}
	return nil
}

// ValidarDatosOportunidad valida monto estimado y notas. Devuelve las notas limpias.
func ValidarDatosOportunidad(montoEstimado int64, notas string, cfg ConfigEmpresa) (string, error) {
	if montoEstimado < 0 {
		return "", errEntrada("MONTO_INVALIDO", "El monto estimado no puede ser negativo.")
	}
	if err := ValidarMonto(montoEstimado, "monto_estimado_centavos", cfg); err != nil {
		return "", err
	}
	return ValidarTexto(notas, "notas", LargoMaximoNota)
}

// ValidarConversion — R8, R9.
func ValidarConversion(op Oportunidad) error {
	if op.CuentaID != nil {
		return errRegla("OPORTUNIDAD_YA_CONVERTIDA", "Esta oportunidad ya se convirtió en una cuenta por cobrar.").
			con("cuenta_id", *op.CuentaID)
	}
	if op.Etapa != EtapaGanado {
		return errRegla("OPORTUNIDAD_NO_GANADA", "Solo se pueden convertir en cuenta las oportunidades ganadas.").
			con("etapa_actual", op.Etapa)
	}
	return nil
}

// EsOportunidadAbierta — R41: sigue en el pipeline (o ganada pero aún sin cuenta).
func EsOportunidadAbierta(op Oportunidad) bool {
	switch op.Etapa {
	case EtapaNuevo, EtapaContactado, EtapaNegociacion:
		return true
	case EtapaGanado:
		return op.CuentaID == nil
	}
	return false
}

// Precarga son los datos sugeridos para el formulario de conversión (R7). No crea nada.
type Precarga struct {
	OportunidadID         string `json:"oportunidad_id"`
	ClienteID             string `json:"cliente_id"`
	ClienteNombre         string `json:"cliente_nombre"`
	MontoSugeridoCentavos int64  `json:"monto_sugerido_centavos"`
	FechaEmision          Fecha  `json:"fecha_emision"`
	PlazoCreditoDias      int    `json:"plazo_credito_dias"`
	PlazoPermitido        bool   `json:"plazo_permitido"`
	FechaVencimiento      Fecha  `json:"fecha_vencimiento"`
	LimiteCreditoCentavos int64  `json:"limite_credito_centavos"`
}

func PrecargarConversion(op Oportunidad, cl Cliente, hoy Fecha, cfg ConfigEmpresa) Precarga {
	return Precarga{
		OportunidadID:         op.ID,
		ClienteID:             cl.ID,
		ClienteNombre:         cl.Nombre,
		MontoSugeridoCentavos: op.MontoEstimadoCentavos,
		FechaEmision:          hoy,
		PlazoCreditoDias:      cl.PlazoCreditoDias,
		PlazoPermitido:        ValidarPlazo(cl.PlazoCreditoDias, cfg) == nil,
		FechaVencimiento:      CalcularVencimiento(hoy, cl.PlazoCreditoDias),
		LimiteCreditoCentavos: cl.LimiteCreditoCentavos,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Pagos, condonaciones y anulaciones (R15–R28)
// ─────────────────────────────────────────────────────────────────────────────

// ValidarPago — R15, R16, R18.
func ValidarPago(monto int64, fecha, hoy Fecha, saldo int64, cfg ConfigEmpresa) error {
	if monto <= 0 {
		return errEntrada("MONTO_INVALIDO", "El monto del pago debe ser mayor a cero.")
	}
	if err := ValidarMonto(monto, "monto_centavos", cfg); err != nil {
		return err
	}
	// La regla de negocio (R16) tiene prioridad; el rango solo atrapa fechas pasadas absurdas.
	if fecha.Despues(hoy) {
		return errRegla("FECHA_FUTURA", "La fecha del pago no puede ser posterior a hoy.").
			con("fecha_recibida", fecha).con("hoy", hoy)
	}
	if err := ValidarFecha(fecha, "fecha"); err != nil {
		return err
	}
	if !cfg.PermitirSobrepago && monto > saldo {
		return errRegla("SOBREPAGO",
			fmt.Sprintf("El pago de %s es mayor que el saldo pendiente de %s. Registre como máximo el saldo pendiente.",
				FormatearMonto(monto, cfg), FormatearMonto(saldo, cfg))).
			con("saldo_pendiente_centavos", saldo).
			con("monto_recibido_centavos", monto)
	}
	return nil
}

// ValidarCondonacion — R21 (parte de configuración), R22, R23. El rol se valida en el handler.
// Si monto es nil, se condona el saldo completo. Devuelve monto y motivo definitivos.
func ValidarCondonacion(monto *int64, motivo string, fecha, hoy Fecha, saldo int64, cfg ConfigEmpresa) (int64, string, error) {
	if !cfg.PermitirCondonaciones {
		return 0, "", errRegla("CONDONACIONES_DESHABILITADAS", "Su empresa no tiene habilitadas las condonaciones de saldo.")
	}
	m, err := ValidarMotivo(motivo, true, cfg)
	if err != nil {
		return 0, "", err
	}
	if saldo <= 0 {
		return 0, "", errRegla("CUENTA_SIN_SALDO", "La cuenta no tiene saldo pendiente que condonar.")
	}
	valor := saldo
	if monto != nil {
		valor = *monto
	}
	if valor <= 0 {
		return 0, "", errEntrada("MONTO_INVALIDO", "El monto a condonar debe ser mayor a cero.")
	}
	if fecha.Despues(hoy) {
		return 0, "", errRegla("FECHA_FUTURA", "La fecha de la condonación no puede ser posterior a hoy.")
	}
	if err := ValidarFecha(fecha, "fecha"); err != nil {
		return 0, "", err
	}
	if valor > saldo {
		return 0, "", errRegla("CONDONACION_EXCEDE_SALDO",
			fmt.Sprintf("No se puede condonar %s: el saldo pendiente es %s.", FormatearMonto(valor, cfg), FormatearMonto(saldo, cfg))).
			con("saldo_pendiente_centavos", saldo).
			con("monto_solicitado_centavos", valor)
	}
	return valor, m, nil
}

// ValidarAnulacion — R25–R28 (el rol se valida en el handler).
func ValidarAnulacion(mov Movimiento, motivo string, cfg ConfigEmpresa) (string, error) {
	if mov.Anulado {
		return "", errRegla("MOVIMIENTO_YA_ANULADO", "Este movimiento ya fue anulado anteriormente.")
	}
	return ValidarMotivo(motivo, cfg.ExigirMotivoAnulacion, cfg)
}

// ─────────────────────────────────────────────────────────────────────────────
// Ajustes de monto (R29–R34)
// ─────────────────────────────────────────────────────────────────────────────

type PlanAjuste struct {
	Motivo           string
	Estado           string // pendiente si la empresa exige revisión; aprobado automático si no
	RequiereRevision bool
}

func totalAplicado(c Cuenta) int64 {
	return c.PagadoCentavos + c.CondonadoCentavos + c.NotaCreditoCentavos
}

func errMontoMenorAPagado(c Cuenta, monto int64, cfg ConfigEmpresa) *ErrorApp {
	return errRegla("MONTO_MENOR_A_PAGADO",
		fmt.Sprintf("El monto no puede quedar en %s porque la cuenta ya tiene %s aplicados entre pagos, notas de crédito y condonaciones.",
			FormatearMonto(monto, cfg), FormatearMonto(totalAplicado(c), cfg))).
		con("monto_solicitado_centavos", monto).
		con("total_pagado_centavos", c.PagadoCentavos).
		con("total_condonado_centavos", c.CondonadoCentavos).
		con("total_nota_credito_centavos", c.NotaCreditoCentavos).
		con("total_aplicado_centavos", totalAplicado(c))
}

// PlanificarAjuste — R29–R32, R34. `c` debe estar evaluada.
func PlanificarAjuste(c Cuenta, montoNuevo int64, motivo string, cfg ConfigEmpresa) (PlanAjuste, error) {
	var p PlanAjuste
	if montoNuevo <= 0 {
		return p, errEntrada("MONTO_INVALIDO", "El nuevo monto de la cuenta debe ser mayor a cero.")
	}
	if err := ValidarMonto(montoNuevo, "monto_total_centavos", cfg); err != nil {
		return p, err
	}
	if montoNuevo == c.MontoTotalCentavos {
		return p, errEntrada("MONTO_SIN_CAMBIO", "El nuevo monto es igual al monto actual de la cuenta.")
	}
	m, err := ValidarMotivo(motivo, true, cfg)
	if err != nil {
		return p, err
	}
	if montoNuevo < totalAplicado(c) {
		return p, errMontoMenorAPagado(c, montoNuevo, cfg)
	}
	p.Motivo = m
	if cfg.ExigirRevisionAjusteMonto {
		p.Estado = AjustePendiente
		p.RequiereRevision = true
	} else {
		p.Estado = AjusteAprobado
		p.RequiereRevision = c.RequiereRevision // puede haber ajustes pendientes de antes
	}
	return p, nil
}

// ValidarResolucionAjuste — R33: solo se resuelven ajustes pendientes.
func ValidarResolucionAjuste(a Ajuste) error {
	if a.Estado != AjustePendiente {
		return errRegla("AJUSTE_YA_RESUELTO", fmt.Sprintf("Este ajuste ya fue %s.", a.Estado)).con("estado", a.Estado)
	}
	return nil
}

// ValidarReversionAjuste — R33, R34: al rechazar se vuelve al monto anterior, siempre que el ajuste
// sea el último aplicado y el monto anterior no quede por debajo de lo ya aplicado.
func ValidarReversionAjuste(a Ajuste, c Cuenta, cfg ConfigEmpresa) error {
	if c.MontoTotalCentavos != a.MontoNuevoCentavos {
		return errRegla("AJUSTE_SUPERADO",
			"La cuenta tuvo otro cambio de monto después de este ajuste. Resuelva primero el ajuste más reciente.").
			con("monto_actual_centavos", c.MontoTotalCentavos).
			con("monto_del_ajuste_centavos", a.MontoNuevoCentavos)
	}
	if a.MontoAnteriorCentavos < totalAplicado(c) {
		return errMontoMenorAPagado(c, a.MontoAnteriorCentavos, cfg)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Roles y asignación de vendedor (R35–R38)
// ─────────────────────────────────────────────────────────────────────────────

// ElegirVendedor decide a quién se asigna un recurso nuevo. El vendedor siempre se asigna a sí
// mismo; el admin puede elegir, o se usa el valor por defecto (p. ej. el vendedor del cliente).
func ElegirVendedor(u Usuario, solicitado *string, porDefecto string) (string, error) {
	if u.Rol != RolAdmin {
		if solicitado != nil && *solicitado != u.ID {
			return "", errSoloAdmin()
		}
		return u.ID, nil
	}
	if solicitado != nil && *solicitado != "" {
		return *solicitado, nil
	}
	return porDefecto, nil
}

// ValidarAsignable: solo se asignan recursos a usuarios activos.
func ValidarAsignable(u Usuario) error {
	if !u.Activo {
		return errRegla("USUARIO_INACTIVO", fmt.Sprintf("%s está desactivado y no puede recibir clientes ni cuentas.", u.Nombre))
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Reasignación de cartera (R40–R42)
// ─────────────────────────────────────────────────────────────────────────────

func ValidarReasignacion(origen, destino Usuario, motivo string, cfg ConfigEmpresa) (string, error) {
	if origen.ID == destino.ID {
		return "", errEntrada("MISMO_VENDEDOR", "El vendedor de origen y el de destino deben ser distintos.")
	}
	if err := ValidarAsignable(destino); err != nil {
		return "", err
	}
	return ValidarMotivo(motivo, true, cfg)
}

// SeleccionarCartera — R41: qué se mueve. Clientes: todos. Oportunidades: abiertas.
// Cuentas: solo con saldo pendiente; las pagadas conservan su vendedor (historial).
func SeleccionarCartera(ops []Oportunidad, cuentas []Cuenta) (opIDs, cuentaIDs []string) {
	for _, o := range ops {
		if EsOportunidadAbierta(o) {
			opIDs = append(opIDs, o.ID)
		}
	}
	for _, c := range cuentas {
		if c.SaldoCentavos > 0 {
			cuentaIDs = append(cuentaIDs, c.ID)
		}
	}
	return opIDs, cuentaIDs
}

// ─────────────────────────────────────────────────────────────────────────────
// Comportamiento de pago (R43–R45)
// ─────────────────────────────────────────────────────────────────────────────

type Comportamiento struct {
	PromedioDiasAtraso *float64 `json:"promedio_dias_atraso"`
	Clasificacion      string   `json:"clasificacion"`
	CuentasEvaluadas   int      `json:"cuentas_evaluadas"`
}

// CalcularComportamiento recibe las cuentas del cliente YA evaluadas. Considera las cuentas pagadas
// que tienen al menos un pago real (UltimoPago solo cuenta movimientos tipo 'pago', R24/R44).
func CalcularComportamiento(cuentas []Cuenta, cfg ConfigEmpresa) Comportamiento {
	var pagadas []Cuenta
	for _, c := range cuentas {
		if c.SaldoCentavos <= 0 && c.UltimoPago != nil {
			pagadas = append(pagadas, c)
		}
	}
	sort.Slice(pagadas, func(i, j int) bool {
		if !pagadas[i].UltimoPago.Igual(*pagadas[j].UltimoPago) {
			return pagadas[i].UltimoPago.Despues(*pagadas[j].UltimoPago)
		}
		return pagadas[i].FechaVencimiento.Despues(pagadas[j].FechaVencimiento)
	})
	if len(pagadas) > cfg.CuentasEvaluadasComportamiento {
		pagadas = pagadas[:cfg.CuentasEvaluadasComportamiento]
	}

	res := Comportamiento{CuentasEvaluadas: len(pagadas)}
	if len(pagadas) < cfg.MinimoCuentasParaHistorial || len(pagadas) == 0 {
		res.Clasificacion = ComportamientoSinHistorial
		return res
	}
	total := 0
	for _, c := range pagadas {
		total += max(DiasEntre(c.FechaVencimiento, *c.UltimoPago), 0)
	}
	promedio := math.Round(float64(total)/float64(len(pagadas))*10) / 10
	res.PromedioDiasAtraso = &promedio

	switch {
	case promedio < float64(cfg.UmbralComportamientoBueno):
		res.Clasificacion = ComportamientoBueno
	case promedio <= float64(cfg.UmbralComportamientoRegular):
		res.Clasificacion = ComportamientoRegular
	default:
		res.Clasificacion = ComportamientoMalo
	}
	return res
}

// ─────────────────────────────────────────────────────────────────────────────
// Alertas (R46)
// ─────────────────────────────────────────────────────────────────────────────

// TipoAlerta indica si una cuenta evaluada requiere aviso hoy y de qué tipo.
func TipoAlerta(c Cuenta) (string, bool) {
	if c.SaldoCentavos <= 0 {
		return "", false
	}
	if c.Estado == EstadoPorVencer || c.Estado == EstadoVencido {
		return c.Estado, true
	}
	return "", false
}

// ─────────────────────────────────────────────────────────────────────────────
// Resumen de cartera
// ─────────────────────────────────────────────────────────────────────────────

type ConteoEstado struct {
	Estado        string `json:"estado"`
	Cantidad      int    `json:"cantidad"`
	SaldoCentavos int64  `json:"saldo_centavos"`
}

type Deudor struct {
	ClienteID     string `json:"cliente_id"`
	ClienteNombre string `json:"cliente_nombre"`
	SaldoCliente
}

type Resumen struct {
	TotalPorCobrarCentavos int64          `json:"total_por_cobrar_centavos"`
	TotalAlDiaCentavos     int64          `json:"total_al_dia_centavos"` // no vencido: al_dia + por_vencer
	TotalPorVencerCentavos int64          `json:"total_por_vencer_centavos"`
	TotalVencidoCentavos   int64          `json:"total_vencido_centavos"`
	CuentasPorEstado       []ConteoEstado `json:"cuentas_por_estado"`
	TopDeudores            []Deudor       `json:"top_deudores"`
}

// ResumirCartera recibe cuentas evaluadas.
func ResumirCartera(cuentas []Cuenta, top int) Resumen {
	var r Resumen
	conteo := map[string]*ConteoEstado{}
	for _, e := range EstadosCuenta {
		ce := &ConteoEstado{Estado: e}
		conteo[e] = ce
	}
	nombres := map[string]string{}
	for _, c := range cuentas {
		ce := conteo[c.Estado]
		ce.Cantidad++
		if c.SaldoCentavos > 0 {
			ce.SaldoCentavos += c.SaldoCentavos
			r.TotalPorCobrarCentavos += c.SaldoCentavos
			switch c.Estado {
			case EstadoVencido:
				r.TotalVencidoCentavos += c.SaldoCentavos
			case EstadoPorVencer:
				r.TotalPorVencerCentavos += c.SaldoCentavos
				r.TotalAlDiaCentavos += c.SaldoCentavos
			default:
				r.TotalAlDiaCentavos += c.SaldoCentavos
			}
		}
		nombres[c.ClienteID] = c.ClienteNombre
	}
	for _, e := range EstadosCuenta {
		r.CuentasPorEstado = append(r.CuentasPorEstado, *conteo[e])
	}

	r.TopDeudores = []Deudor{}
	for id, s := range SaldosPorCliente(cuentas) {
		r.TopDeudores = append(r.TopDeudores, Deudor{ClienteID: id, ClienteNombre: nombres[id], SaldoCliente: s})
	}
	sort.Slice(r.TopDeudores, func(i, j int) bool {
		a, b := r.TopDeudores[i], r.TopDeudores[j]
		if a.SaldoCentavos != b.SaldoCentavos {
			return a.SaldoCentavos > b.SaldoCentavos
		}
		return a.ClienteNombre < b.ClienteNombre
	})
	if len(r.TopDeudores) > top {
		r.TopDeudores = r.TopDeudores[:top]
	}
	return r
}

// ─────────────────────────────────────────────────────────────────────────────
// Configuración y marca de la empresa
// ─────────────────────────────────────────────────────────────────────────────

var (
	patronColor  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	patronMoneda = regexp.MustCompile(`^[A-Z]{3}$`)
	// Las formas de pago se guardan normalizadas (minúsculas, sin espacios): son identificadores,
	// no textos para el usuario. La etiqueta visible la ponen formato.js y EtiquetaMetodoPago.
	patronMetodoPago = regexp.MustCompile(`^[a-z][a-z0-9_]{1,29}$`)
)

// Topes de PLATAFORMA de la configuración de documentos y adjuntos.
const (
	TamanoMaximoAdjuntoMB = 25 // el servidor acepta hasta 26 MB por petición
	LargoMaximoPrefijo    = 10
)

// ValidarConfig valida y normaliza (plazos ordenados y sin duplicados).
func ValidarConfig(c ConfigEmpresa) (ConfigEmpresa, error) {
	invalido := func(campo, msg string) (ConfigEmpresa, error) {
		return c, errEntrada("CONFIGURACION_INVALIDA", msg).con("campo", campo)
	}
	c.Moneda = strings.ToUpper(strings.TrimSpace(c.Moneda))
	c.SimboloMoneda = strings.TrimSpace(c.SimboloMoneda)
	if !patronMoneda.MatchString(c.Moneda) {
		return invalido("moneda", "La moneda debe ser un código ISO de tres letras, por ejemplo USD.")
	}
	if n := utf8.RuneCountInString(c.SimboloMoneda); n < 1 || n > 5 {
		return invalido("simbolo_moneda", "El símbolo de moneda debe tener entre 1 y 5 caracteres.")
	}
	if _, err := cargarZona(c.ZonaHoraria); err != nil {
		return invalido("zona_horaria", "La zona horaria no es válida. Use un nombre como America/El_Salvador.")
	}
	if c.DiasAvisoPrevio < 0 || c.DiasAvisoPrevio > 365 {
		return invalido("dias_aviso_previo", "Los días de aviso previo deben estar entre 0 y 365.")
	}
	if len(c.PlazosCreditoPermitidos) == 0 {
		return invalido("plazos_credito_permitidos", "Debe haber al menos un plazo de crédito permitido.")
	}
	vistos := map[int]bool{}
	var plazos []int
	for _, p := range c.PlazosCreditoPermitidos {
		if p <= 0 || p > 365 {
			return invalido("plazos_credito_permitidos", "Cada plazo de crédito debe estar entre 1 y 365 días.")
		}
		if !vistos[p] {
			vistos[p] = true
			plazos = append(plazos, p)
		}
	}
	sort.Ints(plazos)
	c.PlazosCreditoPermitidos = plazos
	if c.LongitudMinimaMotivo < 0 || c.LongitudMinimaMotivo > 500 {
		return invalido("longitud_minima_motivo", "La longitud mínima del motivo debe estar entre 0 y 500 caracteres.")
	}
	if c.UmbralComportamientoBueno < 0 || c.UmbralComportamientoBueno > c.UmbralComportamientoRegular {
		return invalido("umbral_comportamiento_bueno", "El umbral de comportamiento bueno no puede ser negativo ni mayor que el umbral regular.")
	}
	if c.MinimoCuentasParaHistorial < 1 {
		return invalido("minimo_cuentas_para_historial", "El mínimo de cuentas para historial debe ser al menos 1.")
	}
	if c.CuentasEvaluadasComportamiento < c.MinimoCuentasParaHistorial || c.CuentasEvaluadasComportamiento > 100 {
		return invalido("cuentas_evaluadas_comportamiento", "Las cuentas evaluadas deben estar entre el mínimo para historial y 100.")
	}

	// Fase 3. Sin estas comprobaciones, un PATCH podría dejar la empresa sin formas de pago
	// (ningún cobro se podría registrar) o con un tope de adjunto imposible de cumplir.
	metodos := []string{}
	vistosMetodo := map[string]bool{}
	for _, m := range c.MetodosPago {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if !patronMetodoPago.MatchString(m) {
			return invalido("metodos_pago", "Cada forma de pago debe escribirse en minúsculas, sin espacios ni acentos (por ejemplo: transferencia).")
		}
		if m == MetodoAnticipo {
			return invalido("metodos_pago", "“anticipo” no se configura: lo usa el sistema al aplicar un anticipo a una cuenta.")
		}
		if !vistosMetodo[m] {
			vistosMetodo[m] = true
			metodos = append(metodos, m)
		}
	}
	if len(metodos) == 0 {
		return invalido("metodos_pago", "Debe haber al menos una forma de pago habilitada.")
	}
	if len(metodos) > 20 {
		return invalido("metodos_pago", "No puede haber más de 20 formas de pago.")
	}
	c.MetodosPago = metodos
	if c.TamanoMaximoAdjuntoMB < 1 || c.TamanoMaximoAdjuntoMB > TamanoMaximoAdjuntoMB {
		return invalido("tamano_maximo_adjunto_mb",
			fmt.Sprintf("El tamaño máximo de un adjunto debe estar entre 1 y %d MB.", TamanoMaximoAdjuntoMB))
	}
	for campo, prefijo := range map[string]*string{
		"prefijo_recibo": &c.PrefijoRecibo, "prefijo_nota_credito": &c.PrefijoNotaCredito,
		"prefijo_anticipo": &c.PrefijoAnticipo, "prefijo_liquidacion": &c.PrefijoLiquidacion,
	} {
		*prefijo = strings.TrimSpace(*prefijo)
		if utf8.RuneCountInString(*prefijo) > LargoMaximoPrefijo {
			return invalido(campo, fmt.Sprintf("El prefijo no puede pasar de %d caracteres.", LargoMaximoPrefijo))
		}
	}
	if c.LimiteUsuarios < 0 || c.LimiteUsuarios > 1000 {
		return invalido("limite_usuarios", "El límite de usuarios debe estar entre 0 (sin límite) y 1000.")
	}
	return c, nil
}

func ValidarMarca(m Marca) (Marca, error) {
	invalido := func(campo, msg string) (Marca, error) {
		return m, errEntrada("MARCA_INVALIDA", msg).con("campo", campo)
	}
	m.NombreProducto = strings.TrimSpace(m.NombreProducto)
	if n := utf8.RuneCountInString(m.NombreProducto); n < 1 || n > 80 {
		return invalido("nombre_producto", "El nombre del producto debe tener entre 1 y 80 caracteres.")
	}
	if !patronColor.MatchString(m.ColorPrimario) {
		return invalido("color_primario", "El color primario debe tener el formato #RRGGBB.")
	}
	if !patronColor.MatchString(m.ColorPrimarioClaro) {
		return invalido("color_primario_claro", "El color primario claro debe tener el formato #RRGGBB.")
	}
	m.ColorPrimario = strings.ToUpper(m.ColorPrimario)
	m.ColorPrimarioClaro = strings.ToUpper(m.ColorPrimarioClaro)
	if m.LogoURL != nil {
		u := strings.TrimSpace(*m.LogoURL)
		if u == "" {
			m.LogoURL = nil
		} else {
			parsed, err := url.Parse(u)
			if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
				return invalido("logo_url", "La dirección del logo debe ser una URL que empiece con https://.")
			}
			m.LogoURL = &u
		}
	}
	return m, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Paginación — R78
// ─────────────────────────────────────────────────────────────────────────────

// Límites de PLATAFORMA de los listados. No van a empresa_config: no cambian ninguna regla de
// negocio, solo cuánto viaja por la red en cada petición.
const (
	PorPaginaPorDefecto = 50
	PorPaginaMaximo     = 200
)

// Paginacion viaja en la respuesta de los listados para que la interfaz sepa si hay más.
type Paginacion struct {
	Pagina    int  `json:"pagina"`
	PorPagina int  `json:"por_pagina"`
	Total     int  `json:"total"`
	Paginas   int  `json:"paginas"`
	Desde     int  `json:"desde"` // número del primer elemento de la página (1-based)
	Hasta     int  `json:"hasta"`
	HayMas    bool `json:"hay_mas"`
}

// ValidarPaginacion interpreta ?pagina= y ?por_pagina=. Vacíos = primera página con el tamaño
// por defecto.
func ValidarPaginacion(pagina, porPagina string) (int, int, error) {
	p, err := enteroPositivo(pagina, 1, "pagina")
	if err != nil {
		return 0, 0, err
	}
	n, err := enteroPositivo(porPagina, PorPaginaPorDefecto, "por_pagina")
	if err != nil {
		return 0, 0, err
	}
	if n > PorPaginaMaximo {
		return 0, 0, errEntrada("PARAMETRO_INVALIDO",
			fmt.Sprintf("por_pagina no puede ser mayor que %d.", PorPaginaMaximo)).
			con("maximo", PorPaginaMaximo)
	}
	return p, n, nil
}

func enteroPositivo(texto string, porDefecto int, campo string) (int, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return porDefecto, nil
	}
	n, err := strconv.Atoi(texto)
	if err != nil || n < 1 {
		return 0, errEntrada("PARAMETRO_INVALIDO",
			fmt.Sprintf("%s debe ser un número mayor que cero.", campo)).con("campo", campo)
	}
	return n, nil
}

// ArmarPaginacion calcula los datos que acompañan a una página ya traída de la base.
func ArmarPaginacion(pagina, porPagina, total, enLaPagina int) Paginacion {
	paginas := (total + porPagina - 1) / porPagina
	desde, hasta := 0, 0
	if enLaPagina > 0 {
		desde = (pagina-1)*porPagina + 1
		hasta = desde + enLaPagina - 1
	}
	return Paginacion{
		Pagina: pagina, PorPagina: porPagina, Total: total, Paginas: paginas,
		Desde: desde, Hasta: hasta, HayMas: pagina < paginas,
	}
}
