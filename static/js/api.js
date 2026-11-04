// api.js — cliente HTTP. Es el ÚNICO archivo que conoce las rutas de la API.
// Devuelve el JSON tal como lo entrega el backend y, ante un error, lanza ErrorApi con el código,
// el mensaje (ya escrito para el usuario final) y el detalle que envía el servidor.

export class ErrorApi extends Error {
  constructor({ estado, codigo, mensaje, detalle }) {
    super(mensaje);
    this.estado = estado;
    this.codigo = codigo;
    this.detalle = detalle ?? {};
  }
}

// Mensajes para fallas que ocurren ANTES de tener una respuesta del backend.
const SIN_CONEXION = 'No se pudo conectar con el servidor. Revise su conexión e intente de nuevo.';
const RESPUESTA_INVALIDA = 'Ocurrió un problema inesperado. Intente de nuevo en unos momentos.';

const oyentesSesionExpirada = new Set();

/** Registra una función que se llama cuando una petición autenticada responde 401. */
export function alExpirarSesion(fn) {
  oyentesSesionExpirada.add(fn);
}

async function solicitar(metodo, ruta, cuerpo, { ignorar401 = false } = {}) {
  const opciones = { method: metodo, credentials: 'same-origin', headers: { Accept: 'application/json' } };
  if (cuerpo !== undefined) {
    opciones.headers['Content-Type'] = 'application/json';
    opciones.body = JSON.stringify(cuerpo);
  }

  let respuesta;
  try {
    respuesta = await fetch(ruta, opciones);
  } catch {
    throw new ErrorApi({ estado: 0, codigo: 'SIN_CONEXION', mensaje: SIN_CONEXION });
  }

  let datos = null;
  try {
    datos = await respuesta.json();
  } catch {
    // respuesta sin JSON: se trata abajo
  }

  if (!respuesta.ok) {
    const e = datos?.error ?? { codigo: 'RESPUESTA_INVALIDA', mensaje: RESPUESTA_INVALIDA };
    const error = new ErrorApi({ estado: respuesta.status, ...e });
    if (respuesta.status === 401 && !ignorar401) {
      oyentesSesionExpirada.forEach((fn) => fn(error));
    }
    throw error;
  }
  if (datos === null) {
    throw new ErrorApi({ estado: respuesta.status, codigo: 'RESPUESTA_INVALIDA', mensaje: RESPUESTA_INVALIDA });
  }
  return datos;
}

/** Envío de archivos (multipart). No se fija Content-Type: lo pone el navegador con su frontera. */
async function enviarArchivo(ruta, formulario) {
  let respuesta;
  try {
    respuesta = await fetch(ruta, { method: 'POST', credentials: 'same-origin', body: formulario, headers: { Accept: 'application/json' } });
  } catch {
    throw new ErrorApi({ estado: 0, codigo: 'SIN_CONEXION', mensaje: SIN_CONEXION });
  }
  let datos = null;
  try {
    datos = await respuesta.json();
  } catch {
    // respuesta sin JSON: se trata abajo
  }
  if (!respuesta.ok) {
    const e = datos?.error ?? { codigo: 'RESPUESTA_INVALIDA', mensaje: RESPUESTA_INVALIDA };
    const error = new ErrorApi({ estado: respuesta.status, ...e });
    if (respuesta.status === 401) oyentesSesionExpirada.forEach((fn) => fn(error));
    throw error;
  }
  if (datos === null) throw new ErrorApi({ estado: respuesta.status, codigo: 'RESPUESTA_INVALIDA', mensaje: RESPUESTA_INVALIDA });
  return datos;
}

function consulta(parametros) {
  const pares = Object.entries(parametros).filter(([, v]) => v !== undefined && v !== null && v !== '');
  const texto = new URLSearchParams(pares).toString();
  return texto ? `?${texto}` : '';
}

const seg = encodeURIComponent;

export const api = {
  // Sesión y empresa
  marcaPublica: (slug) => solicitar('GET', `/api/publico/marca${consulta({ slug })}`),
  login: (slug, email, password) => solicitar('POST', '/api/login', { slug, email, password }, { ignorar401: true }),
  logout: () => solicitar('POST', '/api/logout'),
  me: () => solicitar('GET', '/api/me', undefined, { ignorar401: true }),
  configuracion: () => solicitar('GET', '/api/empresa/config'),
  usuarios: () => solicitar('GET', '/api/usuarios'),

  resumen: () => solicitar('GET', '/api/resumen'),

  // Clientes. Los listados vienen paginados: la respuesta trae { clientes, paginacion }.
  clientes: ({ pagina, porPagina, buscar, conCuentas, incluirInactivos } = {}) =>
    solicitar('GET', `/api/clientes${consulta({
      pagina, por_pagina: porPagina, buscar,
      con_cuentas: conCuentas ? 'true' : '', incluir_inactivos: incluirInactivos ? 'true' : '',
    })}`),
  cliente: (id) => solicitar('GET', `/api/clientes/${seg(id)}`),
  crearCliente: (datos) => solicitar('POST', '/api/clientes', datos),

  // Ventas en proceso
  oportunidades: () => solicitar('GET', '/api/oportunidades'),
  crearOportunidad: (datos) => solicitar('POST', '/api/oportunidades', datos),
  cambiarEtapa: (id, etapa) => solicitar('PATCH', `/api/oportunidades/${seg(id)}`, { etapa }),
  convertirOportunidad: (id, datos) => solicitar('POST', `/api/oportunidades/${seg(id)}/convertir`, datos),

  // Cuentas y movimientos
  cuentas: (estado, { pagina, porPagina, buscar } = {}) =>
    solicitar('GET', `/api/cuentas${consulta({ estado, pagina, por_pagina: porPagina, buscar })}`),
  cuenta: (id) => solicitar('GET', `/api/cuentas/${seg(id)}`),
  registrarPago: (cuentaId, datos) => solicitar('POST', `/api/cuentas/${seg(cuentaId)}/pagos`, datos),
  condonar: (cuentaId, datos) => solicitar('POST', `/api/cuentas/${seg(cuentaId)}/condonar`, datos),
  editarMonto: (cuentaId, datos) => solicitar('PATCH', `/api/cuentas/${seg(cuentaId)}/monto`, datos),
  anularMovimiento: (movimientoId, motivo) => solicitar('POST', `/api/movimientos/${seg(movimientoId)}/anular`, { motivo }),

  // Revisiones
  ajustesPendientes: () => solicitar('GET', '/api/ajustes?estado=pendiente'),
  aprobarAjuste: (id) => solicitar('POST', `/api/ajustes/${seg(id)}/aprobar`),
  rechazarAjuste: (id) => solicitar('POST', `/api/ajustes/${seg(id)}/rechazar`),

  reasignarCartera: (origenId, destinoId, motivo) =>
    solicitar('POST', `/api/vendedores/${seg(origenId)}/reasignar-cartera`, { vendedor_destino_id: destinoId, motivo }),

  alertas: () => solicitar('GET', '/api/alertas'),

  // ───────────── Fase 3 ─────────────

  tablero: () => solicitar('GET', '/api/tablero'),

  // Usuarios
  crearUsuario: (datos) => solicitar('POST', '/api/usuarios', datos),
  actualizarUsuario: (id, datos) => solicitar('PATCH', `/api/usuarios/${seg(id)}`, datos),
  reiniciarPassword: (id, password) => solicitar('POST', `/api/usuarios/${seg(id)}/contrasena`, { password }),
  cambiarMiPassword: (actual, nueva) => solicitar('POST', '/api/me/contrasena', { password_actual: actual, password_nueva: nueva }),

  // Anticipos (dinero del cliente sin factura asignada)
  anticipos: (clienteId, soloDisponibles) => solicitar('GET', `/api/anticipos${consulta({ cliente_id: clienteId, solo_disponibles: soloDisponibles ? 'true' : '' })}`),
  crearAnticipo: (clienteId, datos) => solicitar('POST', `/api/clientes/${seg(clienteId)}/anticipos`, datos),
  aplicarAnticipo: (id, datos) => solicitar('POST', `/api/anticipos/${seg(id)}/aplicar`, datos),
  anularAnticipo: (id, motivo) => solicitar('POST', `/api/anticipos/${seg(id)}/anular`, { motivo }),

  notaCredito: (cuentaId, datos) => solicitar('POST', `/api/cuentas/${seg(cuentaId)}/nota-credito`, datos),

  // Liquidación del cobrador
  corte: (cobradorId) => solicitar('GET', `/api/liquidaciones/corte${consulta({ cobrador_id: cobradorId })}`),
  cerrarLiquidacion: (datos) => solicitar('POST', '/api/liquidaciones', datos),
  liquidaciones: (estado) => solicitar('GET', `/api/liquidaciones${consulta({ estado })}`),
  liquidacion: (id) => solicitar('GET', `/api/liquidaciones/${seg(id)}`),
  aprobarLiquidacion: (id) => solicitar('POST', `/api/liquidaciones/${seg(id)}/aprobar`, {}),
  rechazarLiquidacion: (id, motivo) => solicitar('POST', `/api/liquidaciones/${seg(id)}/rechazar`, { motivo }),

  // Adjuntos (fotos de recibos y facturas)
  adjuntos: (entidad, entidadId) => solicitar('GET', `/api/adjuntos${consulta({ entidad, entidad_id: entidadId })}`),
  subirAdjunto: (formulario) => enviarArchivo('/api/adjuntos', formulario),
  eliminarAdjunto: (id) => solicitar('DELETE', `/api/adjuntos/${seg(id)}`),
  urlAdjunto: (id) => `/api/adjuntos/${seg(id)}/archivo`,
  // El comprobante como documento: la foto con su título, su descripción y quién la subió.
  urlComprobante: (id) => `/api/adjuntos/${seg(id)}/documento.pdf`,
  urlComprobantes: (entidad, entidadId) =>
    `/api/comprobantes.pdf${consulta({ entidad, entidad_id: entidadId })}`,

  // Documentos para imprimir
  urlRecibo: (movimientoId) => `/api/movimientos/${seg(movimientoId)}/recibo.pdf`,
  urlInformeMensual: (mes) => `/api/informe-mensual.pdf${consulta({ mes })}`,
  urlEstadoCuenta: (clienteId, { detalle = false, fotos = false } = {}) =>
    `/api/clientes/${seg(clienteId)}/estado-cuenta.pdf${consulta({ detalle: detalle ? 'true' : '', fotos: fotos ? 'true' : '' })}`,

  // ───────────── Fase 4 ─────────────

  // Promesas de pago: lo que el cliente se comprometió a pagar y cuándo.
  promesas: ({ estado, pagina, porPagina, clienteId } = {}) =>
    solicitar('GET', `/api/promesas${consulta({ estado, pagina, por_pagina: porPagina, cliente_id: clienteId })}`),
  agendaDePromesas: () => solicitar('GET', '/api/promesas/agenda'),
  crearPromesa: (clienteId, datos) => solicitar('POST', `/api/clientes/${seg(clienteId)}/promesas`, datos),
  anularPromesa: (id, motivo) => solicitar('POST', `/api/promesas/${seg(id)}/anular`, { motivo }),

  // El mensaje de cobro lo arma el backend (ahí viven el formato del dinero y los textos).
  mensajeDeCobro: (clienteId) => solicitar('GET', `/api/clientes/${seg(clienteId)}/mensaje-cobro`),

  // Exportaciones que el contador abre en Excel.
  urlCarteraCSV: ({ estado, buscar } = {}) => `/api/cuentas.csv${consulta({ estado, buscar })}`,
  urlAntiguedadCSV: () => '/api/antiguedad.csv',

  // Firma del cliente en el recibo (se guarda como adjunto del movimiento).
  guardarFirma: (movimientoId, formulario) => enviarArchivo(`/api/movimientos/${seg(movimientoId)}/firma`, formulario),

  // Importación de cartera
  urlPlantillaImportacion: () => '/api/importacion/plantilla.csv',
  importarCartera: (archivo, aplicar) => {
    const f = new FormData();
    f.append('archivo', archivo);
    if (aplicar) f.append('aplicar', 'true');
    return enviarArchivo('/api/importacion/cartera', f);
  },
};
