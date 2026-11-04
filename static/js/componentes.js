// componentes.js — piezas reutilizables de interfaz. Solo construyen DOM: no llaman a la API
// ni deciden reglas. Las clases CSS describen la función del elemento, no su apariencia.

import * as formato from './formato.js';

// ───────────── Construcción de elementos ─────────────

/** h('div', { class: 'x', onclick: fn }, [hijos]) — los hijos pueden ser texto, nodos o null. */
export function h(etiqueta, atributos = {}, hijos = []) {
  const el = document.createElement(etiqueta);
  for (const [clave, valor] of Object.entries(atributos ?? {})) {
    if (valor === null || valor === undefined || valor === false) continue;
    if (clave.startsWith('on') && typeof valor === 'function') {
      el.addEventListener(clave.slice(2), valor);
    } else if (clave === 'class') {
      el.className = valor;
    } else if (clave === 'dataset') {
      Object.assign(el.dataset, valor);
    } else if (clave in el && typeof valor !== 'string') {
      el[clave] = valor;
    } else {
      el.setAttribute(clave, valor === true ? '' : valor);
    }
  }
  agregar(el, hijos);
  return el;
}

function agregar(el, hijos) {
  for (const hijo of [].concat(hijos)) {
    if (hijo === null || hijo === undefined || hijo === false) continue;
    el.append(hijo instanceof Node ? hijo : document.createTextNode(String(hijo)));
  }
}

// ───────────── Iconos (Lucide, SVG inline) ─────────────

const ICONOS = {
  resumen: '<rect width="7" height="9" x="3" y="3" rx="1"/><rect width="7" height="5" x="14" y="3" rx="1"/><rect width="7" height="9" x="14" y="12" rx="1"/><rect width="7" height="5" x="3" y="16" rx="1"/>',
  ventas: '<path d="M6 5v11"/><path d="M12 5v6"/><path d="M18 5v14"/>',
  cuentas: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>',
  clientes: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/>',
  alertas: '<path d="M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9"/><path d="M10.3 21a1.94 1.94 0 0 0 3.4 0"/>',
  revisiones: '<rect width="8" height="4" x="8" y="2" rx="1" ry="1"/><path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2"/><path d="m9 14 2 2 4-4"/>',
  cartera: '<path d="M19 7V4a1 1 0 0 0-1-1H5a2 2 0 0 0 0 4h15a1 1 0 0 1 1 1v4h-3a2 2 0 0 0 0 4h3a1 1 0 0 0 1-1v-2a1 1 0 0 0-1-1"/><path d="M3 5v14a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1v-4"/>',
  salir: '<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" x2="9" y1="12" y2="12"/>',
  agregar: '<path d="M5 12h14"/><path d="M12 5v14"/>',
  cerrar: '<path d="M18 6 6 18"/><path d="m6 6 12 12"/>',
  volver: '<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>',
  revision: '<path d="M4 15s1-1 4-1 5 2 8 2 4-1 4-1V3s-1 1-4 1-5-2-8-2-4 1-4 1z"/><line x1="4" x2="4" y1="22" y2="15"/>',
  advertencia: '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  pago: '<rect width="20" height="14" x="2" y="5" rx="2"/><line x1="2" x2="22" y1="10" y2="10"/>',
  reasignar: '<path d="m16 3 4 4-4 4"/><path d="M20 7H4"/><path d="m8 21-4-4 4-4"/><path d="M4 17h16"/>',
  liquidaciones: '<path d="M12 2v20"/><path d="M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"/>',
  usuarios: '<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><line x1="19" x2="19" y1="8" y2="14"/><line x1="22" x2="16" y1="11" y2="11"/>',
  configuracion: '<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
  foto: '<path d="M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z"/><circle cx="12" cy="13" r="3"/>',
  documento: '<path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4a2 2 0 0 0 2 2h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/>',
  importar: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="17 8 12 3 7 8"/><line x1="12" x2="12" y1="3" y2="15"/>',
  anticipo: '<circle cx="8" cy="8" r="6"/><path d="M18.09 10.37A6 6 0 1 1 10.34 18"/><path d="M7 6h1v4"/><path d="m16.71 13.88.7.71-2.82 2.82"/>',
  aprobar: '<path d="M21.801 10A10 10 0 1 1 17 3.335"/><path d="m9 11 3 3L22 4"/>',
  rechazar: '<circle cx="12" cy="12" r="10"/><path d="m15 9-6 6"/><path d="m9 9 6 6"/>',
  descargar: '<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/>',
  firma: '<path d="M12 20h9"/><path d="M16.376 3.622a1 1 0 0 1 3.002 3.002L7.368 18.635a2 2 0 0 1-.855.506l-2.872.838a.5.5 0 0 1-.62-.62l.838-2.872a2 2 0 0 1 .506-.854z"/>',
  basura: '<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>',
};

export function icono(nombre, clase = '') {
  const span = document.createElement('span');
  span.innerHTML = `<svg class="icono ${clase}" viewBox="0 0 24 24" aria-hidden="true">${ICONOS[nombre] ?? ''}</svg>`;
  return span.firstElementChild;
}

// ───────────── Marca de la empresa ─────────────

const VARIABLES_MARCA = { '--color-primario': 'color_primario', '--color-primario-claro': 'color_primario_claro' };

/** Escribe los colores de la empresa en :root y el nombre del producto en el título. */
export function pintarMarca(marca) {
  const raiz = document.documentElement.style;
  for (const [variable, campo] of Object.entries(VARIABLES_MARCA)) {
    if (marca?.[campo]) raiz.setProperty(variable, marca[campo]);
    else raiz.removeProperty(variable);
  }
  document.title = marca?.nombre_producto || 'Solvo';
}

/** Vuelve a los colores de respaldo del CSS. */
export function restaurarMarca() {
  pintarMarca(null);
}

export function logoOProducto(marca, clase) {
  return h('div', { class: clase }, [
    marca?.logo_url ? h('img', { src: marca.logo_url, alt: '' }) : null,
    h('span', { class: `${clase === 'encabezado-marca' ? 'encabezado-producto' : 'ingreso-producto'}` }, marca?.nombre_producto ?? ''),
  ]);
}

// ───────────── Piezas básicas ─────────────

export function boton(texto, { variante = '', tipo = 'button', icono: nombreIcono, onclick, deshabilitado, etiqueta } = {}) {
  return h('button', {
    type: tipo,
    class: `boton ${variante ? `boton-${variante}` : ''}`.trim(),
    onclick,
    disabled: deshabilitado === true,
    'aria-label': etiqueta,
  }, [nombreIcono ? icono(nombreIcono, 'icono-pequeno') : null, texto]);
}

export function pildoraEstado(estado) {
  return h('span', { class: `pildora sv-estado estado-${estado}` }, formato.estadoCuenta(estado));
}

export function marcaRevision() {
  return h('span', { class: 'pildora sv-estado marca-revision', title: 'El monto de esta cuenta fue editado y espera revisión' },
    'En revisión');
}

/** Separa ".50" para poder achicar los centavos. El texto sigue siendo el de formato.dinero. */
function cifra(texto, clase) {
  const punto = typeof texto === 'string' ? texto.lastIndexOf('.') : -1;
  const centavos = punto >= 0 ? texto.slice(punto + 1) : '';
  if (!/^\d{2}$/.test(centavos)) return h('span', { class: clase }, texto);
  return h('span', { class: clase }, [
    texto.slice(0, punto),
    h('span', { class: 'monto-centavos' }, texto.slice(punto)),
  ]);
}

export function monto(centavos, clase = 'monto') {
  return cifra(formato.dinero(centavos), clase);
}

export function dato(etiqueta, valor) {
  return h('div', { class: 'dato' }, [
    h('span', { class: 'dato-etiqueta' }, etiqueta),
    h('span', { class: 'dato-valor' }, valor),
  ]);
}

export function tarjeta(hijos, clase = '') {
  return h('section', { class: `tarjeta ${clase}`.trim() }, hijos);
}

function tituloVisual(titulo) {
  if (typeof titulo !== 'string') return titulo;
  const partes = titulo.trim().split(/\s+/);
  if (partes.length < 2) return h('em', {}, titulo);
  const acento = partes.pop();
  return [partes.join(' ') + ' ', h('em', {}, acento)];
}

export function encabezadoPantalla(titulo, acciones = []) {
  return h('header', { class: 'pantalla-encabezado' }, [
    h('h1', {}, tituloVisual(titulo)),
    acciones.length ? h('div', { class: 'pantalla-acciones' }, acciones) : null,
  ]);
}

export function enlaceVolver(href, texto) {
  return h('a', { class: 'volver', href }, [icono('volver', 'icono-pequeno'), texto]);
}

export function vacio(texto) {
  return h('p', { class: 'vacio' }, texto);
}

export function cargando() {
  return h('p', { class: 'cargando' }, 'Cargando…');
}

/**
 * Tabla genérica. columnas: [{ titulo, celda: (fila) => nodo|texto, clase }].
 * opciones.alElegir(fila) vuelve cada fila navegable (clic o Enter).
 */
export function tabla(columnas, filas, { alElegir, claseFila } = {}) {
  const encabezado = h('tr', {}, columnas.map((c) => h('th', { class: c.clase, scope: 'col' }, c.titulo)));
  const cuerpo = filas.map((fila) => {
    const tr = h('tr', { class: [alElegir ? 'fila-navegable' : '', claseFila?.(fila) ?? ''].join(' ').trim() },
      columnas.map((c) => h('td', { class: c.clase }, c.celda(fila))));
    if (alElegir) {
      tr.tabIndex = 0;
      tr.addEventListener('click', (ev) => {
        if (!ev.target.closest('button, a')) alElegir(fila);
      });
      tr.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter' && ev.target === tr) alElegir(fila);
      });
    }
    return tr;
  });
  return h('div', { class: 'tabla-contenedor' }, h('table', { class: 'tabla' }, [h('thead', {}, encabezado), h('tbody', {}, cuerpo)]));
}

// ───────────── Avisos ─────────────
// tipo: error (rojo, la operación NO se hizo), advertencia (amarillo, SÍ se hizo), exito, informativo.

export function aviso(tipo, contenido, acciones = []) {
  return h('div', { class: `aviso aviso-${tipo}`, role: tipo === 'error' ? 'alert' : 'status' }, [
    ...[].concat(contenido),
    acciones.length ? h('div', { class: 'aviso-acciones' }, acciones) : null,
  ]);
}

/** Aviso de error con el mensaje del backend tal cual. */
export function avisoError(error, extra = []) {
  return aviso('error', [h('p', {}, error?.message ?? String(error)), ...extra]);
}

let temporizadorNotificacion;
export function notificar(texto) {
  document.querySelector('.notificacion')?.remove();
  const el = aviso('exito', h('p', {}, texto));
  el.classList.add('notificacion');
  document.body.append(el);
  clearTimeout(temporizadorNotificacion);
  temporizadorNotificacion = setTimeout(() => el.remove(), 4000);
}

// ───────────── Campos de formulario ─────────────

let contadorCampos = 0;
const nuevoId = () => `campo-${++contadorCampos}`;

export function campo(etiqueta, control, ayuda) {
  if (!control.id) control.id = nuevoId();
  return h('div', { class: 'campo' }, [
    h('label', { for: control.id }, etiqueta),
    control,
    ayuda ? h('p', { class: 'campo-ayuda' }, ayuda) : null,
  ]);
}

export function campoTexto(etiqueta, { valor = '', requerido = false, tipo = 'text', autocompletar, ayuda } = {}) {
  const input = h('input', { type: tipo, value: valor, required: requerido, autocomplete: autocompletar });
  return { elemento: campo(etiqueta, input, ayuda), valor: () => input.value.trim(), control: input };
}

export function campoSeleccion(etiqueta, opciones, { valor, ayuda, vacia } = {}) {
  const select = h('select', {}, [
    vacia ? h('option', { value: '' }, vacia) : null,
    ...opciones.map((o) => h('option', { value: o.valor, selected: o.valor === valor }, o.texto)),
  ]);
  return { elemento: campo(etiqueta, select, ayuda), valor: () => select.value, control: select };
}

/** Campo de mes (AAAA-MM). El valor inicial lo decide quien llama, con una fecha del backend. */
export function campoMes(etiqueta, { valor = '', ayuda } = {}) {
  const input = h('input', { type: 'month', value: valor });
  return { elemento: campo(etiqueta, input, ayuda), valor: () => input.value, control: input };
}

export function campoFecha(etiqueta, { valor = '', ayuda } = {}) {
  const input = h('input', { type: 'date', value: valor });
  return { elemento: campo(etiqueta, input, ayuda), valor: () => input.value, control: input };
}

/** Campo de monto con el símbolo de la empresa. centavos() devuelve null si no es válido. */
export function campoMonto(etiqueta, { centavos, ayuda, placeholder } = {}) {
  const input = h('input', {
    type: 'text', inputmode: 'decimal', autocomplete: 'off', placeholder,
    value: centavos === undefined || centavos === null ? '' : formato.montoParaCampo(centavos),
  });
  input.id = nuevoId();
  const elemento = h('div', { class: 'campo' }, [
    h('label', { for: input.id }, etiqueta),
    h('div', { class: 'campo-monto' }, [h('span', { class: 'campo-monto-simbolo' }, formato.simbolo()), input]),
    ayuda ? h('p', { class: 'campo-ayuda' }, ayuda) : null,
  ]);
  return {
    elemento,
    control: input,
    vacio: () => input.value.trim() === '',
    centavos: () => formato.aCentavos(input.value),
    fijar: (c) => { input.value = formato.montoParaCampo(c); input.dispatchEvent(new Event('input', { bubbles: true })); },
  };
}

/** Campo de motivo con contador: muestra cuántos caracteres faltan para el mínimo configurado. */
export function campoMotivo(etiqueta, minimo, { obligatorio = true } = {}) {
  const area = h('textarea', { maxlength: 500 });
  const ayuda = h('p', { class: 'campo-ayuda', 'aria-live': 'polite' });
  area.id = nuevoId();
  const largo = () => [...area.value.trim()].length;
  const valido = () => (!obligatorio && largo() === 0) || largo() >= minimo;
  const actualizar = () => {
    const faltan = minimo - largo();
    if (!obligatorio && largo() === 0) {
      ayuda.textContent = 'Opcional.';
      ayuda.className = 'campo-ayuda';
    } else if (faltan > 0) {
      ayuda.textContent = `Faltan ${faltan} caracteres (mínimo ${minimo}).`;
      ayuda.className = 'campo-ayuda motivo-incompleto';
    } else {
      ayuda.textContent = `Motivo suficiente (${largo()} caracteres).`;
      ayuda.className = 'campo-ayuda motivo-completo';
    }
  };
  area.addEventListener('input', actualizar);
  actualizar();
  return {
    elemento: h('div', { class: 'campo' }, [h('label', { for: area.id }, etiqueta), area, ayuda]),
    valor: () => area.value.trim(),
    valido,
    control: area,
  };
}

/**
 * campoFirma(etiqueta, { ayuda, alto, textoBorrar })
 * Lienzo donde el cliente firma con el dedo (teléfono) o con el mouse (escritorio), y un botón
 * para borrar y volver a firmar. Devuelve { elemento, control, vacio(), limpiar(), png() };
 * png() entrega una promesa con el Blob PNG, que es lo que se sube al servidor.
 *
 * Se escuchan eventos *pointer* y no touch+mouse por separado: un solo juego de eventos cubre
 * dedo, lápiz y ratón, y así no hay dos caminos que mantener.
 * Los trazos se guardan como puntos y el lienzo se repinta a partir de ellos: eso permite
 * redimensionar (girar el teléfono, abrir el teclado) sin perder lo que el cliente ya firmó.
 */
export function campoFirma(etiqueta, { ayuda, alto = 180, textoBorrar = 'Borrar y volver a firmar' } = {}) {
  const lienzo = h('canvas', { class: 'lienzo-firma', role: 'img', 'aria-label': etiqueta });
  // Sin touch-action: none la página se desplaza mientras el dedo arrastra y la firma sale
  // cortada. Es comportamiento, no apariencia, y por eso se fija acá y no en la hoja de estilos.
  lienzo.style.touchAction = 'none';
  const pincel = lienzo.getContext('2d');
  const TINTA = '#1F2933';
  const GROSOR = 2.2; // milímetro y medio en pantalla: grosor constante, como un bolígrafo
  const trazos = []; // cada trazo es una lista de puntos en píxeles CSS
  let trazoActual = null;

  const estiloDeTrazo = () => {
    pincel.lineWidth = GROSOR;
    pincel.lineCap = 'round';
    pincel.lineJoin = 'round';
    pincel.strokeStyle = TINTA;
    pincel.fillStyle = TINTA;
  };

  const medidas = () => ({
    ancho: lienzo.clientWidth || 320,
    altura: lienzo.clientHeight || alto,
  });

  const repintar = () => {
    const { ancho, altura } = medidas();
    pincel.clearRect(0, 0, ancho, altura);
    estiloDeTrazo();
    for (const trazo of trazos) {
      if (trazo.length === 1) {
        // Un toque sin arrastre (el punto de una "i") también es parte de la firma.
        pincel.beginPath();
        pincel.arc(trazo[0].x, trazo[0].y, GROSOR / 2, 0, Math.PI * 2);
        pincel.fill();
        continue;
      }
      pincel.beginPath();
      pincel.moveTo(trazo[0].x, trazo[0].y);
      for (let i = 1; i < trazo.length; i += 1) pincel.lineTo(trazo[i].x, trazo[i].y);
      pincel.stroke();
    }
  };

  const medir = () => {
    if (!lienzo.clientHeight) lienzo.style.height = `${alto}px`;
    const { ancho, altura } = medidas();
    // El lienzo se dibuja a la resolución REAL de la pantalla: en un teléfono moderno cada
    // píxel CSS son dos o tres de verdad, y sin esto la firma sale pixelada en el recibo.
    const densidad = window.devicePixelRatio || 1;
    lienzo.width = Math.round(ancho * densidad);
    lienzo.height = Math.round(altura * densidad);
    pincel.setTransform(densidad, 0, 0, densidad, 0, 0);
    repintar();
  };

  const avisarCambio = () => lienzo.dispatchEvent(new Event('change', { bubbles: true }));

  const punto = (ev) => {
    const caja = lienzo.getBoundingClientRect();
    return { x: ev.clientX - caja.left, y: ev.clientY - caja.top };
  };

  lienzo.addEventListener('pointerdown', (ev) => {
    ev.preventDefault();
    lienzo.setPointerCapture?.(ev.pointerId);
    trazoActual = [punto(ev)];
    trazos.push(trazoActual);
    repintar();
    avisarCambio();
  });
  lienzo.addEventListener('pointermove', (ev) => {
    if (!trazoActual) return;
    const anterior = trazoActual[trazoActual.length - 1];
    const actual = punto(ev);
    trazoActual.push(actual);
    // Se dibuja solo el tramo nuevo: repintar todo en cada movimiento haría saltar el trazo.
    estiloDeTrazo();
    pincel.beginPath();
    pincel.moveTo(anterior.x, anterior.y);
    pincel.lineTo(actual.x, actual.y);
    pincel.stroke();
  });
  const terminarTrazo = () => {
    if (!trazoActual) return;
    trazoActual = null;
    avisarCambio();
  };
  lienzo.addEventListener('pointerup', terminarTrazo);
  lienzo.addEventListener('pointercancel', terminarTrazo);

  // El lienzo nace sin tamaño (todavía no está en la página): se mide cuando el navegador lo
  // coloca y cada vez que cambia de tamaño.
  if (window.ResizeObserver) new ResizeObserver(medir).observe(lienzo);

  const limpiar = () => {
    trazos.length = 0;
    trazoActual = null;
    repintar();
    avisarCambio();
  };

  const elemento = h('div', { class: 'campo campo-firma' }, [
    h('label', {}, etiqueta),
    lienzo,
    h('div', { class: 'firma-acciones' }, [
      boton(textoBorrar, { variante: 'discreto', icono: 'basura', onclick: limpiar }),
    ]),
    ayuda ? h('p', { class: 'campo-ayuda' }, ayuda) : null,
  ]);

  return {
    elemento,
    control: lienzo,
    vacio: () => trazos.length === 0,
    limpiar,
    // El PNG va con fondo transparente a propósito: se imprime encima de la línea de firma del
    // recibo, y un fondo blanco le pondría un recuadro encima.
    png: () => new Promise((entregar) => lienzo.toBlob((blob) => entregar(blob), 'image/png')),
  };
}

// ───────────── Gráficos (SVG y CSS, sin librerías) ─────────────
// Solo DIBUJAN lo que ya viene calculado del backend: ninguna de estas funciones suma ni
// clasifica nada. El ancho de cada barra es proporción visual, no un dato.

/**
 * Barras horizontales. filas: [{ etiqueta, valor, texto, clase, alElegir }].
 * `valor` es el número con el que se compara (el mayor ocupa todo el ancho).
 */
export function barras(filas, { claseExtra = '' } = {}) {
  const maximo = Math.max(1, ...filas.map((f) => Math.abs(f.valor ?? 0)));
  return h('div', { class: `barras ${claseExtra}`.trim() }, filas.map((f) => {
    const valor = Math.abs(f.valor ?? 0);
    // Un valor en cero no dibuja barra: una marca mínima haría ver saldo donde no lo hay.
    const ancho = valor === 0 ? '0' : `${Math.max(valor / maximo * 100, 2)}%`;
    const cuerpo = [
      h('div', { class: 'barra-etiqueta' }, f.etiqueta),
      h('div', { class: 'barra-pista' }, valor === 0 ? null : h('div', { class: `barra-relleno ${f.clase ?? ''}`.trim(), style: `width:${ancho}` })),
      h('div', { class: 'barra-valor monto' }, f.texto),
    ];
    return f.alElegir
      ? h('button', { type: 'button', class: 'barra barra-navegable', onclick: f.alElegir }, cuerpo)
      : h('div', { class: 'barra' }, cuerpo);
  }));
}

/** Columnas por día. puntos: [{ etiqueta, valor, titulo }]. Vacío = mensaje discreto. */
export function columnas(puntos, textoVacio = 'Sin movimientos en el período.') {
  if (!puntos.some((p) => p.valor > 0)) return h('p', { class: 'texto-secundario' }, textoVacio);
  const maximo = Math.max(1, ...puntos.map((p) => p.valor));
  return h('div', { class: 'columnas' }, puntos.map((p) => h('div', { class: 'columna', title: p.titulo }, [
    h('div', { class: 'columna-pista' },
      h('div', { class: 'columna-barra', style: `height:${Math.max(p.valor / maximo * 100, p.valor > 0 ? 3 : 0)}%` })),
    h('span', { class: 'columna-etiqueta' }, p.etiqueta),
  ])));
}

/** Recuadro de un indicador: rótulo, número grande y una línea de contexto. */
export function indicador(etiqueta, valor, { detalle, clase = '', alElegir } = {}) {
  const hijos = [
    h('span', { class: 'texto-secundario' }, etiqueta),
    cifra(valor, 'monto-destacado'),
    detalle ? h('span', { class: 'texto-secundario' }, detalle) : null,
  ];
  return alElegir
    ? h('button', { type: 'button', class: `tarjeta tarjeta-resumen indicador-navegable ${clase}`.trim(), onclick: alElegir }, hijos)
    : h('section', { class: `tarjeta tarjeta-resumen ${clase}`.trim() }, hijos);
}

/** Enlace con apariencia de botón para descargar o abrir un documento del servidor. */
export function botonEnlace(texto, href, { variante = '', icono: nombreIcono, descargar } = {}) {
  return h('a', {
    class: `boton ${variante ? `boton-${variante}` : ''}`.trim(),
    href,
    target: descargar ? undefined : '_blank',
    rel: 'noopener',
    download: descargar === true ? '' : descargar,
  }, [nombreIcono ? icono(nombreIcono, 'icono-pequeno') : null, texto]);
}

/** Campo de archivo. `camara` abre directamente la cámara trasera en el teléfono. */
export function campoArchivo(etiqueta, { acepta, camara = false, ayuda } = {}) {
  const input = h('input', {
    type: 'file',
    class: 'campo-archivo-nativo',
    accept: acepta,
    capture: camara ? 'environment' : undefined,
    tabindex: '-1',
  });
  const nombre = h('span', { class: 'campo-archivo-nombre campo-archivo-vacio' }, 'Ningún archivo seleccionado');
  const miniatura = h('img', { class: 'campo-archivo-miniatura', alt: '', hidden: true });
  const elegir = h('button', { type: 'button', class: 'boton campo-archivo-elegir' }, 'Elegir archivo');
  const zona = h('div', { class: 'campo-archivo-zona' }, [elegir, nombre, miniatura]);
  const elemento = campo(etiqueta, input, ayuda);
  input.replaceWith(zona);
  zona.append(input);
  let vistaPrevia;

  const mostrar = () => {
    const archivo = input.files?.[0] ?? null;
    if (vistaPrevia) URL.revokeObjectURL(vistaPrevia);
    vistaPrevia = null;
    if (!archivo) {
      nombre.textContent = 'Ningún archivo seleccionado';
      nombre.classList.add('campo-archivo-vacio');
      miniatura.hidden = true;
      miniatura.removeAttribute('src');
      return;
    }
    nombre.textContent = archivo.name;
    nombre.classList.remove('campo-archivo-vacio');
    if (archivo.type.startsWith('image/')) {
      vistaPrevia = URL.createObjectURL(archivo);
      miniatura.src = vistaPrevia;
      miniatura.alt = archivo.name;
      miniatura.hidden = false;
    } else {
      miniatura.hidden = true;
      miniatura.removeAttribute('src');
    }
  };

  elegir.addEventListener('click', () => input.click());
  input.addEventListener('change', mostrar);
  zona.addEventListener('click', (ev) => {
    if (ev.target === input || ev.target.closest('.campo-archivo-elegir')) return;
    input.click();
  });
  zona.addEventListener('dragover', (ev) => {
    ev.preventDefault();
    zona.classList.add('campo-archivo-sobre');
  });
  zona.addEventListener('dragleave', () => zona.classList.remove('campo-archivo-sobre'));
  zona.addEventListener('drop', (ev) => {
    ev.preventDefault();
    zona.classList.remove('campo-archivo-sobre');
    const lista = ev.dataTransfer?.files;
    if (!lista?.length) return;
    const transferencia = new DataTransfer();
    transferencia.items.add(lista[0]);
    input.files = transferencia.files;
    input.dispatchEvent(new Event('change', { bubbles: true }));
  });

  return {
    elemento,
    control: input,
    archivo: () => input.files?.[0] ?? null,
    vacio: () => !input.files || input.files.length === 0,
  };
}

// ───────────── Modal: una sola estructura para todos ─────────────

/**
 * abrirModal({ titulo, contenido, textoConfirmar, variante, validar, alConfirmar })
 * - contenido: nodos del cuerpo.
 * - validar(): si devuelve false, el botón de confirmar queda deshabilitado.
 * - alConfirmar(ctx): async. Si lanza un error, se muestra en el modal con el mensaje del backend.
 *   ctx = { cerrar, mensaje(nodo), limpiar(), confirmar() }
 * Sin textoConfirmar, el modal es solo de lectura.
 */
export function abrirModal({ titulo, contenido, textoConfirmar, variante = 'principal', validar = () => true, alConfirmar }) {
  const mensajes = h('div', { class: 'modal-mensajes' });
  const botonConfirmar = textoConfirmar ? boton(textoConfirmar, { variante, tipo: 'submit' }) : null;
  const anterior = document.activeElement;

  const cerrar = () => {
    fondo.remove();
    document.removeEventListener('keydown', alTeclear);
    anterior?.focus?.();
  };
  const alTeclear = (ev) => { if (ev.key === 'Escape') cerrar(); };

  let enviando = false;
  const revisar = () => {
    if (botonConfirmar) botonConfirmar.disabled = enviando || !validar();
  };

  const formulario = h('form', { class: 'modal', role: 'dialog', 'aria-modal': 'true', 'aria-label': titulo, novalidate: true }, [
    h('header', { class: 'modal-encabezado' }, [
      h('h2', {}, typeof titulo === 'string' ? tituloVisual(titulo) : titulo),
      boton('', { variante: 'discreto', icono: 'cerrar', onclick: cerrar, etiqueta: 'Cerrar' }),
    ]),
    h('div', { class: 'modal-cuerpo' }, [mensajes, ...[].concat(contenido)]),
    h('footer', { class: 'modal-pie' }, [
      boton(textoConfirmar ? 'Cancelar' : 'Cerrar', { onclick: cerrar }),
      botonConfirmar,
    ]),
  ]);
  const fondo = h('div', { class: 'modal-fondo', onclick: (ev) => { if (ev.target === fondo) cerrar(); } }, formulario);

  const ctx = {
    cerrar,
    limpiar: () => mensajes.replaceChildren(),
    mensaje: (nodo) => mensajes.replaceChildren(nodo),
    confirmar: () => formulario.requestSubmit(),
    revisar,
  };

  formulario.addEventListener('input', revisar);
  formulario.addEventListener('change', revisar);
  formulario.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    if (!alConfirmar || enviando || !validar()) return;
    enviando = true;
    revisar();
    ctx.limpiar();
    try {
      await alConfirmar(ctx);
    } catch (error) {
      ctx.mensaje(avisoError(error));
    } finally {
      enviando = false;
      revisar();
    }
  });

  document.addEventListener('keydown', alTeclear);
  document.body.append(fondo);
  revisar();
  (formulario.querySelector('.campo-archivo-elegir')
    ?? formulario.querySelector('input:not(.campo-archivo-nativo), select, textarea'))?.focus();
  return ctx;
}

// ───────────── Paginación y búsqueda ─────────────

/**
 * Pie de un listado paginado. `paginacion` es lo que envía el backend; `alCambiar(pagina)` se
 * llama con la página pedida. No calcula totales: solo muestra lo que vino.
 */
export function paginador(paginacion, alCambiar) {
  if (!paginacion || paginacion.paginas <= 1) return null;
  const { pagina, paginas, desde, hasta, total } = paginacion;
  return h('nav', { class: 'paginador', 'aria-label': 'Paginación' }, [
    h('span', { class: 'texto-secundario' }, `${desde}-${hasta} de ${total}`),
    h('div', { class: 'paginador-botones' }, [
      boton('Anterior', { variante: 'compacto', deshabilitado: pagina <= 1, onclick: () => alCambiar(pagina - 1) }),
      h('span', { class: 'texto-secundario' }, `Página ${pagina} de ${paginas}`),
      boton('Siguiente', { variante: 'compacto', deshabilitado: pagina >= paginas, onclick: () => alCambiar(pagina + 1) }),
    ]),
  ]);
}

/** Campo de búsqueda con retardo: no consulta en cada tecla. */
export function campoBusqueda(textoAyuda, valorInicial, alBuscar) {
  const input = h('input', { type: 'search', id: `busqueda-${Math.random().toString(36).slice(2, 8)}`, value: valorInicial ?? '', placeholder: textoAyuda, autocomplete: 'off' });
  let espera;
  input.addEventListener('input', () => {
    clearTimeout(espera);
    espera = setTimeout(() => alBuscar(input.value.trim()), 300);
  });
  input.addEventListener('keydown', (ev) => {
    if (ev.key === 'Enter') {
      ev.preventDefault();
      clearTimeout(espera);
      alBuscar(input.value.trim());
    }
  });
  return {
    elemento: h('div', { class: 'campo campo-busqueda' }, [
      h('label', { class: 'oculto-visualmente', for: input.id }, textoAyuda),
      input,
    ]),
    control: input,
    valor: () => input.value.trim(),
  };
}

/**
 * Selector de un registro entre muchos: se escribe y se elige de los resultados.
 * `buscar(texto)` la provee quien llama (las vistas y modales son los que hablan con la API).
 * Cada resultado es { id, nombre, detalle }.
 */
export function selectorBuscable(etiqueta, { buscar, elegido, ayuda, placeholder = 'Escriba para buscar…' } = {}) {
  const input = h('input', { type: 'search', placeholder, autocomplete: 'off' });
  input.id = nuevoId();
  const resultados = h('div', { class: 'resultados', role: 'listbox' });
  const elegidoEl = h('div', { class: 'elegido' });
  let seleccion = elegido ?? null;
  let espera;

  const pintarElegido = () => {
    input.hidden = seleccion !== null;
    resultados.replaceChildren();
    elegidoEl.replaceChildren(seleccion
      ? h('div', { class: 'elegido-fila' }, [
        h('span', { class: 'elegido-nombre' }, seleccion.nombre),
        boton('Cambiar', { variante: 'compacto', onclick: () => { seleccion = null; pintarElegido(); input.focus(); } }),
      ])
      : '');
    input.dispatchEvent(new Event('change', { bubbles: true }));
  };

  const mostrar = (lista) => {
    resultados.replaceChildren(...(lista.length
      ? lista.map((r) => h('button', {
        type: 'button', class: 'resultado', onclick: () => { seleccion = r; pintarElegido(); },
      }, [h('span', {}, r.nombre), r.detalle ? h('span', { class: 'texto-secundario' }, r.detalle) : null]))
      : [h('p', { class: 'texto-secundario' }, 'Sin resultados.')]));
  };

  input.addEventListener('input', () => {
    clearTimeout(espera);
    const texto = input.value.trim();
    if (texto === '') {
      resultados.replaceChildren();
      return;
    }
    espera = setTimeout(async () => {
      try {
        mostrar(await buscar(texto));
      } catch {
        resultados.replaceChildren(h('p', { class: 'texto-secundario' }, 'No se pudo buscar. Intente de nuevo.'));
      }
    }, 300);
  });

  const elemento = h('div', { class: 'campo' }, [
    h('label', { for: input.id }, etiqueta),
    elegidoEl,
    input,
    resultados,
    ayuda ? h('p', { class: 'campo-ayuda' }, ayuda) : null,
  ]);
  pintarElegido();
  return { elemento, control: input, valor: () => seleccion?.id ?? '', elegido: () => seleccion };
}
