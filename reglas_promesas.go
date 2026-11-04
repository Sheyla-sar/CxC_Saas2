package main

// reglas_promesas.go — reglas de la promesa de pago (fase 4).
// Igual que reglas.go y reglas_cobranza.go: funciones puras, sin HTTP ni SQL, con la
// configuración de la empresa como parámetro. Ningún umbral está escrito aquí.
//
// La idea central: el estado de una promesa NO se almacena, se calcula. Es la misma decisión que
// con el saldo y el estado de una cuenta (§5.E de CLAUDE.md). Una promesa guardada como "rota"
// exigiría un proceso nocturno que la marque y la confianza de que ese proceso corrió; calculada,
// siempre dice la verdad del momento en que se pregunta.

import (
	"fmt"
	"sort"
	"time"
)

// Estados de una promesa. Se calculan, nunca se guardan.
const (
	PromesaPendiente = "pendiente" // todavía no llega la fecha y aún no pagó lo prometido
	PromesaCumplida  = "cumplida"  // pagó lo prometido dentro de la ventana
	PromesaRota      = "rota"      // pasó la fecha y no alcanzó
	PromesaAnulada   = "anulada"   // se anotó mal y se dio de baja; no cuenta para nada
)

var EstadosPromesa = []string{PromesaPendiente, PromesaCumplida, PromesaRota, PromesaAnulada}

func EsEstadoPromesaValido(estado string) bool {
	return contiene(EstadosPromesa, estado)
}

// Promesa: lo que el cliente se comprometió a pagar y cuándo.
//
// Los campos "almacenados" vienen de la tabla; los "agregados" los calcula el repo (la ventana de
// cobro y lo que entró en ella); los "calculados" los llena EvaluarPromesa.
type Promesa struct {
	ID              string  `json:"id"`
	ClienteID       string  `json:"cliente_id"`
	ClienteNombre   string  `json:"cliente_nombre"`
	ClienteTelefono string  `json:"cliente_telefono"`
	CuentaID        *string `json:"cuenta_id"`
	// NumeroDocumento es el de la cuenta prometida, si la promesa apunta a una factura concreta.
	NumeroDocumento string `json:"numero_documento"`
	FechaPromesa    Fecha  `json:"fecha_promesa"`
	// MontoCentavos es opcional: sin monto, cualquier pago en la ventana cumple la promesa.
	MontoCentavos       *int64     `json:"monto_centavos"`
	Nota                string     `json:"nota"`
	RegistradoPor       string     `json:"registrado_por"`
	RegistradoPorNombre string     `json:"registrado_por_nombre"`
	Anulada             bool       `json:"anulada"`
	AnuladaPor          *string    `json:"anulada_por"`
	AnuladaPorNombre    *string    `json:"anulada_por_nombre"`
	AnuladaEn           *time.Time `json:"anulada_en"`
	MotivoAnulacion     *string    `json:"motivo_anulacion"`
	CreadoEn            time.Time  `json:"creado_en"`

	// Agregados (repo). FechaRegistro es el día en que se anotó, en la zona horaria de la empresa:
	// es el inicio de la ventana en la que un pago cuenta como cumplimiento.
	FechaRegistro  Fecha `json:"fecha_registro"`
	PagadoCentavos int64 `json:"pagado_en_ventana_centavos"`

	// Calculados (EvaluarPromesa)
	Estado          string `json:"estado"`
	DiasDeAtraso    int    `json:"dias_de_atraso"`    // días desde la fecha prometida; 0 si aún no llega
	DiasParaCumplir int    `json:"dias_para_cumplir"` // días que faltan; 0 si ya pasó
	FaltaCentavos   int64  `json:"falta_centavos"`    // lo que falta del monto prometido; 0 si no hay monto
}

// EvaluarPromesa decide el estado de una promesa. Es la ÚNICA fórmula del estado en Go.
//
//	anulada   → se anotó mal; no cuenta para nada
//	cumplida  → entró, entre el día en que se anotó y la fecha prometida inclusive, al menos lo
//	            prometido (sin monto prometido, basta cualquier pago en esa ventana)
//	pendiente → todavía no llega la fecha prometida y no alcanzó
//	rota      → ya pasó la fecha prometida y no alcanzó
//
// `hoy` llega de t.Hoy(), que es la fecha en la zona horaria de la empresa (R14): nunca la del
// servidor ni UTC.
func EvaluarPromesa(p *Promesa, hoy Fecha) {
	p.DiasDeAtraso = 0
	p.DiasParaCumplir = 0
	p.FaltaCentavos = 0
	if dias := DiasEntre(p.FechaPromesa, hoy); dias > 0 {
		p.DiasDeAtraso = dias
	} else {
		p.DiasParaCumplir = -dias
	}
	if p.MontoCentavos != nil && *p.MontoCentavos > p.PagadoCentavos {
		p.FaltaCentavos = *p.MontoCentavos - p.PagadoCentavos
	}
	switch {
	case p.Anulada:
		p.Estado = PromesaAnulada
	case PromesaAlcanzada(*p):
		p.Estado = PromesaCumplida
	case hoy.Despues(p.FechaPromesa):
		p.Estado = PromesaRota
	default:
		p.Estado = PromesaPendiente
	}
}

// PromesaAlcanzada responde si lo que entró en la ventana basta para darla por cumplida.
// Sin monto prometido, cualquier pago alcanza: lo que el cliente dijo fue "le pago el viernes",
// no "le pago $100 el viernes", y castigarlo por abonar menos sería inventarle una condición.
func PromesaAlcanzada(p Promesa) bool {
	if p.MontoCentavos == nil {
		return p.PagadoCentavos > 0
	}
	return p.PagadoCentavos >= *p.MontoCentavos
}

// ValidarPromesa revisa lo que se anota al crearla. La fecha prometida no puede ser anterior a
// hoy (una promesa es sobre el futuro; para registrar lo que ya pasó está el pago) y el monto,
// si se indica, tiene que ser positivo y estar dentro de los límites de la plataforma.
func ValidarPromesa(fecha, hoy Fecha, monto *int64, nota string, cfg ConfigEmpresa) (string, error) {
	if err := ValidarFecha(fecha, "fecha_promesa"); err != nil {
		return "", err
	}
	if fecha.Antes(hoy) {
		return "", errRegla("FECHA_PASADA",
			"La fecha prometida ya pasó. Anote el día en que el cliente se compromete a pagar.").
			con("hoy", hoy.String()).con("fecha_promesa", fecha.String())
	}
	if monto != nil {
		if *monto <= 0 {
			return "", errEntrada("MONTO_INVALIDO", "El monto prometido debe ser mayor a cero.")
		}
		if err := ValidarMonto(*monto, "monto_centavos", cfg); err != nil {
			return "", err
		}
	}
	return ValidarTexto(nota, "nota", LargoMaximoNota)
}

// ValidarCuentaDeLaPromesa: si la promesa apunta a una factura, esa factura tiene que ser del
// mismo cliente. Si no, el cumplimiento se mediría contra pagos de otro.
func ValidarCuentaDeLaPromesa(c Cuenta, clienteID string) error {
	if c.ClienteID != clienteID {
		return errRegla("CUENTA_DE_OTRO_CLIENTE",
			"La cuenta indicada es de otro cliente. La promesa solo puede apuntar a una cuenta del mismo cliente.")
	}
	return nil
}

// ValidarAnulacionPromesa — una promesa no se borra: se anula y queda en el historial con su
// motivo. Anularla dos veces no tiene sentido.
func ValidarAnulacionPromesa(p Promesa, motivo string, cfg ConfigEmpresa) (string, error) {
	if p.Anulada {
		return "", errRegla("PROMESA_YA_ANULADA", "Esta promesa ya fue anulada.")
	}
	return ValidarMotivo(motivo, cfg.ExigirMotivoAnulacion, cfg)
}

// ─────────────────────────────────────────────────────────────────────────────
// Cumplimiento: el dato por el que existe toda esta pantalla
// ─────────────────────────────────────────────────────────────────────────────

// CumplimientoPromesas resume cuántas promesas cumplió un cliente y cuántas rompió. Las
// pendientes no cuentan todavía (no se sabe) y las anuladas no cuentan nunca.
type CumplimientoPromesas struct {
	Cumplidas  int     `json:"cumplidas"`
	Rotas      int     `json:"rotas"`
	Pendientes int     `json:"pendientes"`
	Evaluadas  int     `json:"evaluadas"`  // cumplidas + rotas
	Porcentaje float64 `json:"porcentaje"` // cumplidas / evaluadas, con un decimal
}

// ResumirCumplimiento cuenta promesas YA evaluadas con EvaluarPromesa: no vuelve a decidir nada.
func ResumirCumplimiento(promesas []Promesa) CumplimientoPromesas {
	var r CumplimientoPromesas
	for _, p := range promesas {
		switch p.Estado {
		case PromesaCumplida:
			r.Cumplidas++
		case PromesaRota:
			r.Rotas++
		case PromesaPendiente:
			r.Pendientes++
		}
	}
	r.Evaluadas = r.Cumplidas + r.Rotas
	r.Porcentaje = PorcentajeDe(int64(r.Cumplidas), int64(r.Evaluadas))
	return r
}

// OrdenarAgenda deja primero lo más atrasado: la promesa que se rompió hace más días es la que
// hay que ir a ver hoy. Con la misma fecha, ordena por nombre del cliente para que la lista no
// cambie de orden entre dos consultas iguales.
func OrdenarAgenda(promesas []Promesa) {
	sort.SliceStable(promesas, func(i, j int) bool {
		if !promesas[i].FechaPromesa.Igual(promesas[j].FechaPromesa) {
			return promesas[i].FechaPromesa.Antes(promesas[j].FechaPromesa)
		}
		return promesas[i].ClienteNombre < promesas[j].ClienteNombre
	})
}

// TextoPromesa describe una promesa en una línea, para los mensajes al usuario final.
func TextoPromesa(p Promesa, cfg ConfigEmpresa) string {
	if p.MontoCentavos == nil {
		return fmt.Sprintf("%s se comprometió a pagar el %s.", p.ClienteNombre, p.FechaPromesa)
	}
	return fmt.Sprintf("%s se comprometió a pagar %s el %s.",
		p.ClienteNombre, FormatearMonto(*p.MontoCentavos, cfg), p.FechaPromesa)
}
