package main

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fecha: fecha de negocio (DATE) sin hora ni zona. Internamente medianoche UTC, lo que hace
// exactas las diferencias en días. En JSON viaja como "AAAA-MM-DD".
// ─────────────────────────────────────────────────────────────────────────────

type Fecha struct{ t time.Time }

const formatoFecha = "2006-01-02"

func NuevaFecha(anio int, mes time.Month, dia int) Fecha {
	return Fecha{time.Date(anio, mes, dia, 0, 0, 0, 0, time.UTC)}
}

func ParsearFecha(s string) (Fecha, error) {
	t, err := time.Parse(formatoFecha, s)
	if err != nil {
		return Fecha{}, err
	}
	return NuevaFecha(t.Date()), nil
}

func (f Fecha) String() string         { return f.t.Format(formatoFecha) }
func (f Fecha) SumarDias(n int) Fecha  { return Fecha{f.t.AddDate(0, 0, n)} }
func (f Fecha) SumarMeses(n int) Fecha { return Fecha{f.t.AddDate(0, n, 0)} }
func (f Fecha) InicioDeMes() Fecha     { return NuevaFecha(f.t.Year(), f.t.Month(), 1) }

// Tiempo expone la fecha como time.Time (UTC, sin hora) para formatear mes y año.
func (f Fecha) Tiempo() time.Time            { return f.t }
func (f Fecha) Despues(o Fecha) bool         { return f.t.After(o.t) }
func (f Fecha) Antes(o Fecha) bool           { return f.t.Before(o.t) }
func (f Fecha) Igual(o Fecha) bool           { return f.t.Equal(o.t) }
func (f Fecha) EsCero() bool                 { return f.t.IsZero() }
func DiasEntre(desde, hasta Fecha) int       { return int(hasta.t.Unix()/86400 - desde.t.Unix()/86400) } // sin time.Duration: no se satura a los 292 años
func (f Fecha) MarshalJSON() ([]byte, error) { return json.Marshal(f.String()) }

func (f *Fecha) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return errEntrada("FECHA_INVALIDA", "Las fechas deben enviarse como texto con formato AAAA-MM-DD.")
	}
	p, err := ParsearFecha(s)
	if err != nil {
		return errEntrada("FECHA_INVALIDA", fmt.Sprintf("La fecha %q no es válida. Use el formato AAAA-MM-DD.", s))
	}
	*f = p
	return nil
}

func (f *Fecha) Scan(src any) error {
	switch v := src.(type) {
	case time.Time:
		*f = NuevaFecha(v.Date())
		return nil
	case string:
		return f.scanTexto(v)
	case []byte:
		return f.scanTexto(string(v))
	}
	return fmt.Errorf("no se puede leer %T como Fecha", src)
}

func (f *Fecha) scanTexto(s string) error {
	if len(s) < 10 {
		return fmt.Errorf("fecha inválida %q", s)
	}
	p, err := ParsearFecha(s[:10])
	if err != nil {
		return err
	}
	*f = p
	return nil
}

func (f Fecha) Value() (driver.Value, error) { return f.String(), nil }

// ─────────────────────────────────────────────────────────────────────────────
// Empresa, marca y configuración
// ─────────────────────────────────────────────────────────────────────────────

type Empresa struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	NombreLegal     string    `json:"nombre_legal"`
	NombreComercial string    `json:"nombre_comercial"`
	Activo          bool      `json:"activo"`
	CreadoEn        time.Time `json:"creado_en"`
	// Datos que aparecen en los documentos impresos (recibo, estado de cuenta).
	NIT       string `json:"nit"`
	NRC       string `json:"nrc"`
	Direccion string `json:"direccion"`
	Telefono  string `json:"telefono"`
	// Plan contratado. Informativo: lo que habilita cada plan vive en empresa_config.
	Plan string `json:"plan"`
}

type Marca struct {
	LogoURL            *string `json:"logo_url"`
	ColorPrimario      string  `json:"color_primario"`
	ColorPrimarioClaro string  `json:"color_primario_claro"`
	NombreProducto     string  `json:"nombre_producto"`
}

// ConfigEmpresa son los parámetros de negocio de una empresa. Toda regla de reglas.go que tenga un
// umbral lo lee de aquí. Los valores por defecto están en los DEFAULT de 001_esquema.sql.
type ConfigEmpresa struct {
	Moneda                         string   `json:"moneda"`
	SimboloMoneda                  string   `json:"simbolo_moneda"`
	ZonaHoraria                    string   `json:"zona_horaria"`
	DiasAvisoPrevio                int      `json:"dias_aviso_previo"`
	PlazosCreditoPermitidos        []int    `json:"plazos_credito_permitidos"`
	PermitirSobrepago              bool     `json:"permitir_sobrepago"`
	ExigirRevisionAjusteMonto      bool     `json:"exigir_revision_ajuste_monto"`
	ExigirMotivoAnulacion          bool     `json:"exigir_motivo_anulacion"`
	LongitudMinimaMotivo           int      `json:"longitud_minima_motivo"`
	UmbralComportamientoBueno      int      `json:"umbral_comportamiento_bueno"`
	UmbralComportamientoRegular    int      `json:"umbral_comportamiento_regular"`
	MinimoCuentasParaHistorial     int      `json:"minimo_cuentas_para_historial"`
	CuentasEvaluadasComportamiento int      `json:"cuentas_evaluadas_comportamiento"`
	ExigirLimiteCredito            bool     `json:"exigir_limite_credito"`
	PermitirCondonaciones          bool     `json:"permitir_condonaciones"`
	MetodosPago                    []string `json:"metodos_pago"`
	ExigirLiquidacionCobrador      bool     `json:"exigir_liquidacion_cobrador"`
	PermitirAdjuntos               bool     `json:"permitir_adjuntos"`
	TamanoMaximoAdjuntoMB          int      `json:"tamano_maximo_adjunto_mb"`
	PrefijoRecibo                  string   `json:"prefijo_recibo"`
	PrefijoNotaCredito             string   `json:"prefijo_nota_credito"`
	PrefijoAnticipo                string   `json:"prefijo_anticipo"`
	PrefijoLiquidacion             string   `json:"prefijo_liquidacion"`
	// LimiteUsuarios es el máximo de usuarios ACTIVOS del plan contratado. 0 = sin límite.
	LimiteUsuarios int       `json:"limite_usuarios"`
	ActualizadoEn  time.Time `json:"actualizado_en"`
}

// ─────────────────────────────────────────────────────────────────────────────
// Entidades de negocio
// ─────────────────────────────────────────────────────────────────────────────

type Usuario struct {
	ID           string    `json:"id"`
	Nombre       string    `json:"nombre"`
	Email        string    `json:"email"`
	Rol          string    `json:"rol"`
	Activo       bool      `json:"activo"`
	CreadoEn     time.Time `json:"creado_en"`
	PasswordHash string    `json:"-"`
}

type Cliente struct {
	ID                    string    `json:"id"`
	Nombre                string    `json:"nombre"`
	Contacto              string    `json:"contacto"`
	Telefono              string    `json:"telefono"`
	PlazoCreditoDias      int       `json:"plazo_credito_dias"`
	LimiteCreditoCentavos int64     `json:"limite_credito_centavos"`
	VendedorID            string    `json:"vendedor_id"`
	VendedorNombre        string    `json:"vendedor_nombre"`
	CodigoExterno         string    `json:"codigo_externo"`
	Activo                bool      `json:"activo"`
	CreadoEn              time.Time `json:"creado_en"`
}

type Oportunidad struct {
	ID                    string    `json:"id"`
	ClienteID             string    `json:"cliente_id"`
	ClienteNombre         string    `json:"cliente_nombre"`
	MontoEstimadoCentavos int64     `json:"monto_estimado_centavos"`
	Etapa                 string    `json:"etapa"`
	Notas                 string    `json:"notas"`
	VendedorID            string    `json:"vendedor_id"`
	VendedorNombre        string    `json:"vendedor_nombre"`
	CuentaID              *string   `json:"cuenta_id"`
	CreadoEn              time.Time `json:"creado_en"`
	ActualizadoEn         time.Time `json:"actualizado_en"`
}

// Cuenta: los campos "almacenados" vienen de la tabla; los "agregados" los suma el repo a partir de
// los movimientos no anulados; los "calculados" los llena reglas.EvaluarCuenta. Saldo y estado
// nunca se guardan en la base.
type Cuenta struct {
	ID                 string    `json:"id"`
	ClienteID          string    `json:"cliente_id"`
	ClienteNombre      string    `json:"cliente_nombre"`
	ClienteContacto    string    `json:"cliente_contacto"`
	ClienteTelefono    string    `json:"cliente_telefono"`
	OportunidadID      *string   `json:"oportunidad_id"`
	NumeroDocumento    string    `json:"numero_documento"`
	MontoTotalCentavos int64     `json:"monto_total_centavos"`
	FechaEmision       Fecha     `json:"fecha_emision"`
	FechaVencimiento   Fecha     `json:"fecha_vencimiento"`
	VendedorID         string    `json:"vendedor_id"`
	VendedorNombre     string    `json:"vendedor_nombre"`
	RequiereRevision   bool      `json:"requiere_revision"`
	CreadoEn           time.Time `json:"creado_en"`

	// Agregados (repo)
	PagadoCentavos    int64 `json:"total_pagado_centavos"`
	CondonadoCentavos int64 `json:"total_condonado_centavos"`
	// NotaCreditoCentavos rebaja el saldo igual que una condonación, pero es un documento
	// comercial (devolución, descuento posterior), no un perdón de deuda: se informa aparte.
	NotaCreditoCentavos int64  `json:"total_nota_credito_centavos"`
	UltimoPago          *Fecha `json:"fecha_ultimo_pago"`

	// Calculados (reglas.EvaluarCuenta)
	SaldoCentavos  int64  `json:"saldo_centavos"`
	Estado         string `json:"estado"`
	DiasParaVencer int    `json:"dias_para_vencer"`
	DiasVencida    int    `json:"dias_vencida"`
}

// Movimiento es una fila de la tabla pago: un pago, una condonación o una nota de crédito.
type Movimiento struct {
	ID                  string     `json:"id"`
	CuentaID            string     `json:"cuenta_id"`
	Tipo                string     `json:"tipo"`
	Numero              *string    `json:"numero"`
	MontoCentavos       int64      `json:"monto_centavos"`
	Fecha               Fecha      `json:"fecha"`
	Metodo              string     `json:"metodo"`
	Referencia          string     `json:"referencia"`
	LiquidacionID       *string    `json:"liquidacion_id"`
	AnticipoID          *string    `json:"anticipo_id"`
	Nota                string     `json:"nota"`
	RegistradoPor       string     `json:"registrado_por"`
	RegistradoPorNombre string     `json:"registrado_por_nombre"`
	Anulado             bool       `json:"anulado"`
	AnuladoPor          *string    `json:"anulado_por"`
	AnuladoPorNombre    *string    `json:"anulado_por_nombre"`
	AnuladoEn           *time.Time `json:"anulado_en"`
	MotivoAnulacion     *string    `json:"motivo_anulacion"`
	CreadoEn            time.Time  `json:"creado_en"`
}

type Ajuste struct {
	ID                    string     `json:"id"`
	CuentaID              string     `json:"cuenta_id"`
	ClienteNombre         string     `json:"cliente_nombre"`
	MontoAnteriorCentavos int64      `json:"monto_anterior_centavos"`
	MontoNuevoCentavos    int64      `json:"monto_nuevo_centavos"`
	Motivo                string     `json:"motivo"`
	SolicitadoPor         string     `json:"solicitado_por"`
	SolicitadoPorNombre   string     `json:"solicitado_por_nombre"`
	CreadoEn              time.Time  `json:"creado_en"`
	Estado                string     `json:"estado"`
	RevisadoPor           *string    `json:"revisado_por"`
	RevisadoPorNombre     *string    `json:"revisado_por_nombre"`
	RevisadoEn            *time.Time `json:"revisado_en"`
}

// Anticipo: dinero recibido del cliente sin factura asignada. Lo aplicado se calcula sumando los
// movimientos que lo referencian; nunca se descuenta del monto.
type Anticipo struct {
	ID                  string     `json:"id"`
	Numero              string     `json:"numero"`
	ClienteID           string     `json:"cliente_id"`
	ClienteNombre       string     `json:"cliente_nombre"`
	MontoCentavos       int64      `json:"monto_centavos"`
	Fecha               Fecha      `json:"fecha"`
	Metodo              string     `json:"metodo"`
	Referencia          string     `json:"referencia"`
	Nota                string     `json:"nota"`
	RegistradoPor       string     `json:"registrado_por"`
	RegistradoPorNombre string     `json:"registrado_por_nombre"`
	LiquidacionID       *string    `json:"liquidacion_id"`
	Anulado             bool       `json:"anulado"`
	AnuladoPor          *string    `json:"anulado_por"`
	AnuladoEn           *time.Time `json:"anulado_en"`
	MotivoAnulacion     *string    `json:"motivo_anulacion"`
	CreadoEn            time.Time  `json:"creado_en"`

	// Agregado por el repo: lo ya aplicado a cuentas.
	AplicadoCentavos int64 `json:"aplicado_centavos"`
	// Calculado por reglas.go
	DisponibleCentavos int64 `json:"disponible_centavos"`
}

// Liquidacion: corte de caja del cobrador. Agrupa los pagos que entregó.
type Liquidacion struct {
	ID                     string     `json:"id"`
	Numero                 string     `json:"numero"`
	CobradorID             string     `json:"cobrador_id"`
	CobradorNombre         string     `json:"cobrador_nombre"`
	Fecha                  Fecha      `json:"fecha"`
	Estado                 string     `json:"estado"`
	TotalEsperadoCentavos  int64      `json:"total_esperado_centavos"`
	TotalEntregadoCentavos int64      `json:"total_entregado_centavos"`
	DiferenciaCentavos     int64      `json:"diferencia_centavos"`
	CantidadMovimientos    int        `json:"cantidad_movimientos"`
	Nota                   string     `json:"nota"`
	RevisadoPor            *string    `json:"revisado_por"`
	RevisadoPorNombre      *string    `json:"revisado_por_nombre"`
	RevisadoEn             *time.Time `json:"revisado_en"`
	MotivoRechazo          *string    `json:"motivo_rechazo"`
	CreadoEn               time.Time  `json:"creado_en"`
}

// TotalPorMetodo desglosa lo cobrado por forma de pago (efectivo, transferencia, ...).
type TotalPorMetodo struct {
	Metodo        string `json:"metodo"`
	Cantidad      int    `json:"cantidad"`
	MontoCentavos int64  `json:"monto_centavos"`
}

// Clases de adjunto. El comprobante se archiva y se imprime como documento; la firma se dibuja
// dentro del recibo del movimiento al que pertenece y en ningún otro lado.
const (
	ClaseComprobante = "comprobante"
	ClaseFirma       = "firma"
)

// Adjunto: foto o PDF de un recibo, factura o comprobante. El archivo vive en disco.
type Adjunto struct {
	ID        string `json:"id"`
	Entidad   string `json:"entidad"`
	EntidadID string `json:"entidad_id"`
	// Titulo es cómo se llama el documento para la gente; Nombre es el nombre del archivo que
	// venía del teléfono, que casi nunca dice nada.
	Titulo string `json:"titulo"`
	Nombre string `json:"nombre"`
	// Clase distingue el comprobante que se archiva de la firma del cliente: la firma no se
	// lista ni se anexa como página, se dibuja dentro del recibo.
	Clase           string    `json:"clase"`
	TipoMime        string    `json:"tipo_mime"`
	TamanoBytes     int64     `json:"tamano_bytes"`
	Descripcion     string    `json:"descripcion"`
	SubidoPor       string    `json:"subido_por"`
	SubidoPorNombre string    `json:"subido_por_nombre"`
	CreadoEn        time.Time `json:"creado_en"`
	// archivo es la ruta en disco: nunca se expone en la API.
	archivo string
}

// ComoSeLlama devuelve el título si lo tiene y, si no, el nombre del archivo: en los listados y
// en los PDF siempre hay que mostrar algo.
func (a Adjunto) ComoSeLlama() string {
	if strings.TrimSpace(a.Titulo) != "" {
		return a.Titulo
	}
	return a.Nombre
}

// EsImagen: los PDF solo pueden incrustar fotos; un adjunto que ya es PDF se entrega aparte.
func (a Adjunto) EsImagen() bool {
	return a.TipoMime == "image/jpeg" || a.TipoMime == "image/png"
}

// EsFirma: la firma del cliente no es un comprobante más. Sale impresa dentro del recibo, en su
// espacio de firma, y por eso se excluye de la galería, del anexo de fotos y del paquete de
// comprobantes.
func (a Adjunto) EsFirma() bool {
	return a.Clase == ClaseFirma
}

type Reasignacion struct {
	ID                    string    `json:"id"`
	VendedorOrigen        string    `json:"vendedor_origen"`
	VendedorDestino       string    `json:"vendedor_destino"`
	CantidadClientes      int       `json:"cantidad_clientes"`
	CantidadOportunidades int       `json:"cantidad_oportunidades"`
	CantidadCuentas       int       `json:"cantidad_cuentas"`
	Motivo                string    `json:"motivo"`
	RealizadoPor          string    `json:"realizado_por"`
	CreadoEn              time.Time `json:"creado_en"`
}

// CuentaEnCartera es la versión reducida de una cuenta que viaja dentro del listado de clientes
// (?con_cuentas=true, la pantalla del cobrador). Lleva solo lo que esa pantalla dibuja: mandar la
// cuenta completa multiplicaba por nueve el peso de la respuesta en una ruta con muchas cuentas.
type CuentaEnCartera struct {
	ID               string `json:"id"`
	ClienteID        string `json:"cliente_id"`
	ClienteNombre    string `json:"cliente_nombre"`
	NumeroDocumento  string `json:"numero_documento"`
	SaldoCentavos    int64  `json:"saldo_centavos"`
	Estado           string `json:"estado"`
	FechaVencimiento Fecha  `json:"fecha_vencimiento"`
	DiasParaVencer   int    `json:"dias_para_vencer"`
	DiasVencida      int    `json:"dias_vencida"`
	RequiereRevision bool   `json:"requiere_revision"`
}

// EnCartera reduce una cuenta ya evaluada a lo que muestra la pantalla del cobrador.
func (c Cuenta) EnCartera() CuentaEnCartera {
	return CuentaEnCartera{
		ID: c.ID, ClienteID: c.ClienteID, ClienteNombre: c.ClienteNombre,
		NumeroDocumento: c.NumeroDocumento, SaldoCentavos: c.SaldoCentavos, Estado: c.Estado,
		FechaVencimiento: c.FechaVencimiento, DiasParaVencer: c.DiasParaVencer,
		DiasVencida: c.DiasVencida, RequiereRevision: c.RequiereRevision,
	}
}
