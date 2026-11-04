// formato.js — dinero, fechas y etiquetas.
// Es el ÚNICO lugar que convierte centavos a texto y texto a centavos. No calcula saldos ni
// estados: solo presenta los valores que ya vienen calculados del backend.

let simboloMoneda = '';

/** Lo llama estado.js al cargar la configuración de la empresa. */
export function configurarMoneda(simbolo) {
  simboloMoneda = simbolo ?? '';
}

export function simbolo() {
  return simboloMoneda;
}

// ───────────── Dinero ─────────────

function conMiles(enteros) {
  return String(enteros).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
}

/** 123456 → "$1,234.56" con el símbolo de la empresa. Aritmética entera: nunca pasa por decimales. */
export function dinero(centavos) {
  if (centavos === null || centavos === undefined) return '—';
  const negativo = centavos < 0;
  const absoluto = Math.abs(centavos);
  const enteros = Math.trunc(absoluto / 100);
  const resto = absoluto % 100;
  return `${negativo ? '-' : ''}${simboloMoneda}${conMiles(enteros)}.${String(resto).padStart(2, '0')}`;
}

/** Valor para prellenar un campo de monto: 123456 → "1234.56" (sin símbolo ni separadores). */
export function montoParaCampo(centavos) {
  if (centavos === null || centavos === undefined) return '';
  const enteros = Math.trunc(Math.abs(centavos) / 100);
  const resto = Math.abs(centavos) % 100;
  return `${centavos < 0 ? '-' : ''}${enteros}.${String(resto).padStart(2, '0')}`;
}

/**
 * Texto escrito por el usuario → centavos enteros. Acepta "1234", "1,234.5", "$1,234.56".
 * Devuelve null si el texto no es un monto válido con hasta dos decimales.
 */
export function aCentavos(texto) {
  let limpio = String(texto ?? '').trim();
  if (simboloMoneda) limpio = limpio.split(simboloMoneda).join('');
  limpio = limpio.replace(/[\s,]/g, '');
  if (!/^\d+(\.\d{0,2})?$/.test(limpio)) return null;
  const [enteros, decimales = ''] = limpio.split('.');
  const centavos = Number(enteros) * 100 + Number(decimales.padEnd(2, '0'));
  return Number.isSafeInteger(centavos) ? centavos : null;
}

// ───────────── Fechas ─────────────
// Las fechas de negocio llegan como "AAAA-MM-DD" y se muestran sin convertir de zona horaria.

const formatoFecha = new Intl.DateTimeFormat('es', { day: 'numeric', month: 'short', year: 'numeric', timeZone: 'UTC' });
const formatoFechaHora = new Intl.DateTimeFormat('es', { day: 'numeric', month: 'short', year: 'numeric', hour: 'numeric', minute: '2-digit' });
// Sin año: para los ejes de los gráficos, donde el año se repetiría en cada marca.
const formatoFechaCorta = new Intl.DateTimeFormat('es', { day: 'numeric', month: 'short', timeZone: 'UTC' });

export function fecha(iso) {
  if (!iso) return '—';
  const [a, m, d] = iso.slice(0, 10).split('-').map(Number);
  return formatoFecha.format(new Date(Date.UTC(a, m - 1, d)));
}

export function fechaCorta(iso) {
  if (!iso) return '';
  const [a, m, d] = iso.slice(0, 10).split('-').map(Number);
  return formatoFechaCorta.format(new Date(Date.UTC(a, m - 1, d)));
}

export function fechaHora(marcaDeTiempo) {
  if (!marcaDeTiempo) return '—';
  return formatoFechaHora.format(new Date(marcaDeTiempo));
}

function dias(n) {
  return n === 1 ? '1 día' : `${n} días`;
}

/** Texto del vencimiento a partir de los campos calculados por el backend. */
export function vencimiento(cuenta) {
  if (cuenta.estado === 'pagado') return `Pagada · vencimiento ${fecha(cuenta.fecha_vencimiento)}`;
  if (cuenta.estado === 'vencido') return `Vencida hace ${dias(cuenta.dias_vencida)}`;
  if (cuenta.dias_para_vencer === 0) return 'Vence hoy';
  if (cuenta.dias_para_vencer === 1) return 'Vence mañana';
  return `Vence en ${dias(cuenta.dias_para_vencer)}`;
}

export function avisoPrevio(diasAviso) {
  return `“Por vencer” incluye las cuentas que vencen en los próximos ${dias(diasAviso)}.`;
}

export function plazo(diasPlazo) {
  return `${diasPlazo} días`;
}

// ───────────── Etiquetas ─────────────

const ETIQUETAS_ESTADO = { todas: 'Todas', al_dia: 'Al día', por_vencer: 'Por vencer', vencido: 'Vencida', pagado: 'Pagada' };
const ETIQUETAS_FILTRO = { todas: 'Todas', al_dia: 'Al día', por_vencer: 'Por vencer', vencido: 'Vencidas', pagado: 'Pagadas' };
const ETIQUETAS_ETAPA = { nuevo: 'Nuevo', contactado: 'Contactado', negociacion: 'Negociación', ganado: 'Ganado', perdido: 'Perdido' };
const ETIQUETAS_MOVIMIENTO = { pago: 'Pago', condonacion: 'Condonación', nota_credito: 'Nota de crédito' };
const ETIQUETAS_AJUSTE = { pendiente: 'Pendiente de revisión', aprobado: 'Aprobado', rechazado: 'Rechazado' };
const ETIQUETAS_ROL = { admin: 'Administración', vendedor: 'Ventas' };
const ETIQUETAS_COMPORTAMIENTO = { bueno: 'Buen pagador', regular: 'Pagador regular', malo: 'Mal pagador' };

export const estadoCuenta = (e) => ETIQUETAS_ESTADO[e] ?? e;
export const filtroEstado = (e) => ETIQUETAS_FILTRO[e] ?? e;
export const etapa = (e) => ETIQUETAS_ETAPA[e] ?? e;
export const tipoMovimiento = (t) => ETIQUETAS_MOVIMIENTO[t] ?? t;
export const estadoAjuste = (e) => ETIQUETAS_AJUSTE[e] ?? e;
export const rol = (r) => ETIQUETAS_ROL[r] ?? r;

// Formas de pago y liquidación (fase 3). El catálogo de formas de pago lo define cada empresa:
// aquí solo se les pone un nombre presentable; una forma no listada se muestra tal cual.
const ETIQUETAS_METODO = {
  efectivo: 'Efectivo', transferencia: 'Transferencia', cheque: 'Cheque',
  deposito: 'Depósito', tarjeta: 'Tarjeta', anticipo: 'Anticipo aplicado',
};
const ETIQUETAS_LIQUIDACION = { entregada: 'Esperando aprobación', aprobada: 'Aprobada', rechazada: 'Rechazada' };

export function metodoPago(m) {
  if (!m) return '—';
  return ETIQUETAS_METODO[m] ?? (m.charAt(0).toUpperCase() + m.slice(1));
}
export const estadoLiquidacion = (e) => ETIQUETAS_LIQUIDACION[e] ?? e;

/** 8.4 → "8,4 %". El backend ya envía el número redondeado. */
export function porcentaje(n) {
  if (n === null || n === undefined) return '—';
  return `${String(n).replace('.', ',')} %`;
}

/** Diferencia de una liquidación: "Cuadra", "Faltan $5.00" o "Sobran $2.00". */
export function diferenciaLiquidacion(centavos) {
  if (centavos === 0) return 'Cuadra';
  return centavos < 0 ? `Faltan ${dinero(-centavos)}` : `Sobran ${dinero(centavos)}`;
}

/** Una línea legible del comportamiento de pago que calcula el backend. */
export function comportamiento(c) {
  if (!c || c.clasificacion === 'sin_historial') {
    return `Sin historial suficiente (${c?.cuentas_evaluadas ?? 0} cuenta(s) pagada(s) evaluada(s))`;
  }
  const etiqueta = ETIQUETAS_COMPORTAMIENTO[c.clasificacion] ?? c.clasificacion;
  const promedio = c.promedio_dias_atraso;
  const detalle = promedio === 0
    ? 'paga a tiempo'
    : `paga en promedio con ${String(promedio).replace('.', ',')} días de atraso`;
  return `${etiqueta}: ${detalle} (últimas ${c.cuentas_evaluadas} cuentas)`;
}

// ───────────── Promesas de pago (fase 4) ─────────────
// El estado lo calcula el backend (pendiente, cumplida, rota, anulada); aquí solo se le pone
// nombre y se arma la línea que explica el cuándo con los días que ya vienen contados.

const ETIQUETAS_PROMESA = { pendiente: 'Pendiente', cumplida: 'Cumplió', rota: 'No cumplió', anulada: 'Anulada' };
const ETIQUETAS_FILTRO_PROMESA = { todas: 'Todas', pendiente: 'Pendientes', cumplida: 'Cumplidas', rota: 'No cumplidas', anulada: 'Anuladas' };

export const estadoPromesa = (e) => ETIQUETAS_PROMESA[e] ?? e;
export const filtroPromesa = (e) => ETIQUETAS_FILTRO_PROMESA[e] ?? e;

/** Línea de "para cuándo" de una promesa, a partir de los días que contó el backend. */
export function cuandoPromesa(p) {
  if (p.estado === 'anulada') return `Anulada · era para el ${fecha(p.fecha_promesa)}`;
  if (p.estado === 'cumplida') return `Cumplió: abonó ${dinero(p.pagado_en_ventana_centavos)}`;
  if (p.estado === 'rota') {
    if (p.dias_de_atraso === 1) return 'Prometió pagar ayer';
    return `Prometió pagar hace ${dias(p.dias_de_atraso)}`;
  }
  if (p.dias_para_cumplir === 0) return 'Prometió pagar hoy';
  if (p.dias_para_cumplir === 1) return 'Prometió pagar mañana';
  return `Prometió pagar en ${dias(p.dias_para_cumplir)}`;
}

/** Lo que prometió, en texto: el monto o, si no dijo monto, que fue un abono sin cifra. */
export function montoPromesa(p) {
  return p.monto_centavos ? dinero(p.monto_centavos) : 'Sin monto';
}
