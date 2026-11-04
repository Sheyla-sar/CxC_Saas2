// app.js — arranque: sesión, marca de la empresa, marco con menú según rol y navegación.

import { api, alExpirarSesion } from './api.js';
import * as estado from './estado.js';
import * as formato from './formato.js';
import * as router from './router.js';
import { h, icono, boton, avisoError, cargando, pintarMarca, restaurarMarca } from './componentes.js';

const CLAVE_TEMA = 'solvo-tema';

function nombreProducto() {
  return estado.marca()?.nombre_producto || 'Solvo';
}

function tituloDocumento(seccion) {
  const nombre = nombreProducto();
  if (!seccion || !MENU[seccion]) {
    document.title = nombre;
    return;
  }
  document.title = `${textoMenu(seccion)} · ${nombre}`;
}

function temaActual() {
  return document.documentElement.dataset.tema === 'claro' ? 'claro' : 'oscuro';
}

function alternarTema() {
  const siguiente = temaActual() === 'claro' ? 'oscuro' : 'claro';
  document.documentElement.dataset.tema = siguiente;
  try { localStorage.setItem(CLAVE_TEMA, siguiente); } catch { /* sin almacenamiento */ }
  document.querySelectorAll('[data-tema-etiqueta]').forEach((el) => {
    el.textContent = siguiente === 'claro' ? 'Oscuro' : 'Claro';
    const boton = el.closest('button');
    if (boton) boton.setAttribute('aria-label', siguiente === 'claro' ? 'Usar tema oscuro' : 'Usar tema claro');
  });
}

function logoSolvo() {
  return h('a', { class: 'marca-solvo', href: router.enlace(estado.seccionInicial()) }, [
    h('img', { class: 'marca-solvo-oscuro', src: '/logo/solvo-horizontal-oscuro.svg', alt: 'Solvo' }),
    h('img', { class: 'marca-solvo-claro', src: '/logo/solvo-horizontal-claro.svg', alt: '' }),
    h('span', { class: 'marca-producto' }, nombreProducto()),
  ]);
}

function botonTema() {
  return h('button', {
    type: 'button',
    class: 'boton boton-discreto',
    'aria-label': temaActual() === 'claro' ? 'Usar tema oscuro' : 'Usar tema claro',
    onclick: alternarTema,
  }, h('span', { dataset: { temaEtiqueta: '1' } }, temaActual() === 'claro' ? 'Oscuro' : 'Claro'));
}

import * as login from './vistas/login.js';
import * as resumen from './vistas/resumen.js';
import * as ventas from './vistas/ventas.js';
import * as cuentas from './vistas/cuentas.js';
import * as cuenta from './vistas/cuenta.js';
import * as clientes from './vistas/clientes.js';
import * as cliente from './vistas/cliente.js';
import * as alertas from './vistas/alertas.js';
import * as revisiones from './vistas/revisiones.js';
import * as cartera from './vistas/cartera.js';
import * as liquidaciones from './vistas/liquidaciones.js';
import * as promesas from './vistas/promesas.js';
import * as configuracion from './vistas/configuracion.js';

const raiz = document.getElementById('app');

// Nombre de ruta → vista y patrón del hash.
const VISTAS = {
  resumen: { patron: 'resumen', vista: resumen },
  ventas: { patron: 'ventas', vista: ventas },
  cuentas: { patron: 'cuentas', vista: cuentas },
  cuenta: { patron: 'cuentas/:id', vista: cuenta, seccion: 'cuentas' },
  clientes: { patron: 'clientes', vista: clientes },
  cliente: { patron: 'clientes/:id', vista: cliente, seccion: 'clientes' },
  alertas: { patron: 'alertas', vista: alertas },
  revisiones: { patron: 'revisiones', vista: revisiones },
  cartera: { patron: 'cartera', vista: cartera },
  liquidaciones: { patron: 'liquidaciones', vista: liquidaciones },
  promesas: { patron: 'promesas', vista: promesas },
  liquidacion: { patron: 'liquidaciones/:id', vista: { render: liquidaciones.renderDetalle }, seccion: 'liquidaciones' },
  configuracion: { patron: 'configuracion', vista: configuracion },
};

// Texto e icono de cada sección del menú (qué secciones se muestran lo decide estado.secciones()).
const MENU = {
  resumen: { texto: 'Resumen', icono: 'resumen' },
  ventas: { texto: 'Ventas en proceso', icono: 'ventas' },
  cuentas: { texto: 'Cuentas', icono: 'cuentas' },
  clientes: { texto: 'Clientes', icono: 'clientes' },
  alertas: { texto: 'Alertas', icono: 'alertas' },
  revisiones: { texto: 'Revisiones', icono: 'revisiones' },
  cartera: { texto: 'Mi cartera', icono: 'cartera' },
  liquidaciones: { texto: 'Liquidación', textoVendedor: 'Mi corte', icono: 'liquidaciones' },
  promesas: { texto: 'Promesas', icono: 'pago' },
  configuracion: { texto: 'Configuración', icono: 'configuracion' },
};

// El mismo apartado se llama distinto según quién lo abre: el cobrador entrega SU corte.
function textoMenu(seccion) {
  const m = MENU[seccion];
  return !estado.esAdmin() && m.textoVendedor ? m.textoVendedor : m.texto;
}

let contenido = null;
let navegacion = null;

// ───────────── Sesión ─────────────

async function mostrarIngreso() {
  estado.cerrarSesionLocal();
  contenido = null;
  navegacion = null;
  document.querySelectorAll('.modal-fondo').forEach((m) => m.remove());
  await login.render(raiz, { alEntrar: entrar });
}

async function entrar() {
  await estado.cargarSesion();
  pintarMarca(estado.marca());
  construirMarco();
  const { nombre } = router.actual();
  if (!nombre || !estado.rutaPermitida(nombre)) router.ir(estado.seccionInicial());
  else mostrarRuta();
}

async function salir() {
  try {
    await api.logout();
  } finally {
    restaurarMarca();
    history.replaceState(null, '', location.pathname + location.search);
    mostrarIngreso();
  }
}

// ───────────── Marco: encabezado, menú y contenido ─────────────

function construirMarco() {
  const usuario = estado.usuario();
  const usuarioNodo = h('div', { class: 'encabezado-usuario' }, [
    h('div', {}, usuario.nombre),
    h('div', { class: 'texto-secundario' }, `${estado.empresa().nombre_comercial} · ${formato.rol(usuario.rol)}`),
  ]);
  navegacion = h('nav', { class: 'navegacion', 'aria-label': 'Menú principal' }, [
    h('div', { class: 'navegacion-marca' }, logoSolvo()),
    ...estado.secciones().map((s) =>
      h('a', { class: 'navegacion-item', href: router.enlace(s), dataset: { seccion: s } }, [icono(MENU[s].icono), h('span', {}, textoMenu(s))])),
    h('div', { class: 'navegacion-pie' }, [
      usuarioNodo,
      botonTema(),
      boton('Salir', { variante: 'discreto', icono: 'salir', onclick: salir }),
    ]),
  ]);
  contenido = h('main', { class: 'contenido', id: 'contenido' });

  raiz.replaceChildren(h('div', { class: 'marco' }, [
    h('header', { class: 'encabezado' }, [
      logoSolvo(),
      botonTema(),
      boton('Salir', { variante: 'discreto', icono: 'salir', onclick: salir }),
    ]),
    navegacion,
    contenido,
  ]));
}

let navegacionVigente = 0;

async function mostrarRuta() {
  if (!estado.haySesion() || !contenido) return;
  const { nombre, parametros } = router.actual();
  if (!nombre || !estado.rutaPermitida(nombre)) {
    router.ir(estado.seccionInicial());
    return;
  }
  const { vista, seccion = nombre } = VISTAS[nombre];
  tituloDocumento(seccion);
  navegacion.querySelectorAll('.navegacion-item').forEach((a) => {
    if (a.dataset.seccion === seccion) a.setAttribute('aria-current', 'page');
    else a.removeAttribute('aria-current');
  });

  // Cada navegación dibuja en su propio contenedor: si el usuario cambia de pantalla mientras
  // otra carga, la anterior termina sobre un nodo que ya no está en la página.
  const id = ++navegacionVigente;
  const destino = h('div', {}, cargando());
  contenido.replaceChildren(destino);
  window.scrollTo(0, 0);
  try {
    await vista.render(destino, parametros);
  } catch (error) {
    if (id === navegacionVigente && error.estado !== 401) destino.replaceChildren(avisoError(error));
  }
}

// ───────────── Arranque ─────────────

async function arrancar() {
  for (const [nombre, { patron }] of Object.entries(VISTAS)) router.definir(nombre, patron);
  router.alCambiar(mostrarRuta);
  alExpirarSesion(() => { if (estado.haySesion()) mostrarIngreso(); });

  try {
    await entrar();
  } catch (error) {
    if (error.estado === 401) {
      await mostrarIngreso();
    } else {
      raiz.replaceChildren(h('main', { class: 'contenido' }, avisoError(error)));
    }
  }
}

arrancar();
