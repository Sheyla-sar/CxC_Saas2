// estado.js — sesión, configuración de la empresa y caché en memoria.
// Aquí se decide QUÉ se muestra según el rol y la configuración (permisos de interfaz). Ocultar
// opciones es cortesía: la restricción real la aplica el backend en cada petición.

import { api } from './api.js';
import { configurarMoneda } from './formato.js';

const sesion = {
  usuario: null,   // { id, nombre, email, rol, ... }
  empresa: null,   // { id, slug, nombre_comercial }
  marca: null,     // { logo_url, color_primario, color_primario_claro, nombre_producto }
  config: null,    // empresa_config completa
  etapas: [],      // etapas de venta en orden, tal como las define el backend
  estados: [],     // estados de cuenta, tal como los define el backend
};

// ───────────── Sesión ─────────────

/** Carga usuario y configuración. Lanza ErrorApi (401 si no hay sesión). */
export async function cargarSesion() {
  const me = await api.me();
  const conf = await api.configuracion();
  sesion.usuario = me.usuario;
  sesion.empresa = me.empresa;
  sesion.marca = conf.marca;
  sesion.config = conf.config;
  sesion.etapas = conf.etapas_oportunidad;
  sesion.estados = conf.estados_cuenta;
  configurarMoneda(conf.config.simbolo_moneda);
  recordarSlug(me.empresa.slug);
  cache.clear();
}

export function cerrarSesionLocal() {
  Object.assign(sesion, { usuario: null, empresa: null, marca: null, config: null, etapas: [], estados: [] });
  configurarMoneda('');
  cache.clear();
}

export const haySesion = () => sesion.usuario !== null;
export const usuario = () => sesion.usuario;
export const empresa = () => sesion.empresa;
export const marca = () => sesion.marca;
export const config = () => sesion.config;
export const etapas = () => sesion.etapas;
export const estadosCuenta = () => sesion.estados;
export const esAdmin = () => sesion.usuario?.rol === 'admin';

// ───────────── Permisos de interfaz (derivados de rol + configuración) ─────────────

export function puede(accion) {
  const c = sesion.config ?? {};
  switch (accion) {
    case 'condonar': return esAdmin() && c.permitir_condonaciones === true;
    case 'anular': return esAdmin();
    case 'revisar_ajustes': return esAdmin() && c.exigir_revision_ajuste_monto === true;
    case 'reasignar_cartera': return esAdmin();
    case 'asignar_limite_credito': return esAdmin();
    case 'nota_credito': return esAdmin();
    case 'gestionar_usuarios': return esAdmin();
    case 'importar_cartera': return esAdmin();
    case 'aprobar_liquidaciones': return esAdmin();
    case 'adjuntar': return c.permitir_adjuntos === true;
    case 'liquidar': return c.exigir_liquidacion_cobrador === true;
    default: return false;
  }
}

/** Secciones del menú, en orden, según el rol y la configuración de la empresa. */
export function secciones() {
  if (!esAdmin()) {
    const suyas = ['cartera', 'promesas'];
    if (puede('liquidar')) suyas.push('liquidaciones');
    return [...suyas, 'alertas'];
  }
  const lista = ['resumen', 'ventas', 'cuentas', 'clientes', 'promesas'];
  if (puede('liquidar')) lista.push('liquidaciones');
  lista.push('alertas');
  if (puede('revisar_ajustes')) lista.push('revisiones');
  lista.push('configuracion');
  return lista;
}

export const seccionInicial = () => secciones()[0];

/** Rutas de detalle a las que se llega desde otras pantallas (el backend decide si hay acceso). */
const RUTAS_DE_DETALLE = ['cuenta', 'cliente', 'liquidacion'];

export function rutaPermitida(nombre) {
  return secciones().includes(nombre) || RUTAS_DE_DETALLE.includes(nombre);
}

/** Formas de pago que la empresa habilitó, en el orden en que las configuró. */
export function metodosPago() {
  return sesion.config?.metodos_pago ?? [];
}

/** Longitud mínima que la empresa exige para los motivos. */
export function motivoMinimo(obligatorio = true) {
  const minimo = sesion.config?.longitud_minima_motivo ?? 0;
  return obligatorio ? Math.max(minimo, 1) : 0;
}

// ───────────── Caché en memoria ─────────────
// Guarda lecturas que se repiten entre pantallas (listas para selectores). Cualquier cambio
// registrado por el usuario llama a invalidar() para no mostrar datos viejos.

const cache = new Map();

export async function enCache(clave, cargar) {
  if (!cache.has(clave)) {
    const promesa = cargar();
    cache.set(clave, promesa);
    promesa.catch(() => cache.delete(clave));
  }
  return cache.get(clave);
}

export function invalidar() {
  cache.clear();
}

// ───────────── Preferencias del navegador ─────────────

const CLAVE_SLUG = 'ultima-empresa';

export function ultimoSlug() {
  try {
    return localStorage.getItem(CLAVE_SLUG) ?? '';
  } catch {
    return '';
  }
}

export function recordarSlug(slug) {
  try {
    localStorage.setItem(CLAVE_SLUG, slug);
  } catch {
    // sin almacenamiento disponible: no es necesario
  }
}

// El valor del filtro que no filtra. Lo define el backend (EstadoTodas); aquí se nombra una vez.
const TODAS = 'todas';
export const filtroTodas = () => TODAS;

let filtroCuentas = TODAS;
export const filtroCuentasActual = () => filtroCuentas;
export function fijarFiltroCuentas(valor) {
  filtroCuentas = valor;
}
