// vistas/login.js — ingreso. La marca (logo, colores, nombre del producto) se pinta desde
// GET /api/publico/marca: el frontend no sabe nada de ninguna empresa en particular.

import { api } from '../api.js';
import * as estado from '../estado.js';
import { h, boton, campoTexto, avisoError, pintarMarca, restaurarMarca, logoOProducto } from '../componentes.js';

/** Slug inicial: ?empresa= o ?slug= en la URL, luego la última empresa usada en este navegador. */
function slugInicial() {
  const q = new URLSearchParams(location.search);
  return (q.get('empresa') ?? q.get('slug') ?? estado.ultimoSlug()).trim().toLowerCase();
}

export async function render(contenedor, { alEntrar }) {
  const zonaMarca = h('div');
  const mensajes = h('div');
  const slug = campoTexto('Empresa', { valor: slugInicial(), autocompletar: 'organization', ayuda: 'Identificador de su empresa, por ejemplo distribuidora-progreso.' });
  const email = campoTexto('Correo', { tipo: 'email', autocompletar: 'username', requerido: true });
  const clave = campoTexto('Contraseña', { tipo: 'password', autocompletar: 'current-password', requerido: true });
  const entrar = boton('Entrar', { variante: 'principal', tipo: 'submit' });

  let marcaSolicitada = null;
  const cargarMarca = async (valor) => {
    marcaSolicitada = valor;
    try {
      // Sin slug, el backend intenta resolver la empresa por el subdominio.
      const marca = await api.marcaPublica(valor);
      if (marcaSolicitada !== valor) return;
      if (!valor && marca.slug) slug.control.value = marca.slug;
      pintarMarca(marca);
      zonaMarca.replaceChildren(logoOProducto(marca, 'ingreso-marca'));
      pie.textContent = marca?.nombre_producto || '';
    } catch {
      if (marcaSolicitada !== valor) return;
      restaurarMarca();
      zonaMarca.replaceChildren();
      pie.textContent = '';
    }
  };

  let espera;
  slug.control.addEventListener('input', () => {
    clearTimeout(espera);
    espera = setTimeout(() => cargarMarca(slug.valor().toLowerCase()), 400);
  });

  const formulario = h('form', { class: 'tarjeta ingreso-tarjeta', novalidate: true }, [
    zonaMarca,
    h('h1', {}, 'Iniciar sesión'),
    mensajes,
    h('div', { class: 'formulario' }, [slug.elemento, email.elemento, clave.elemento, entrar]),
  ]);

  formulario.addEventListener('submit', async (ev) => {
    ev.preventDefault();
    mensajes.replaceChildren();
    entrar.disabled = true;
    try {
      await api.login(slug.valor().toLowerCase(), email.valor(), clave.control.value);
      await alEntrar();
    } catch (error) {
      mensajes.replaceChildren(avisoError(error));
      entrar.disabled = false;
    }
  });

  const pie = h('p', { class: 'ingreso-pie' });
  contenedor.replaceChildren(h('main', { class: 'ingreso' }, [
    h('div', { class: 'ingreso-banda', 'aria-hidden': 'true' }, [
      h('span', { class: 'ingreso-circulo ingreso-circulo-1' }),
      h('span', { class: 'ingreso-circulo ingreso-circulo-2' }),
      h('span', { class: 'ingreso-circulo ingreso-circulo-3' }),
      h('span', { class: 'ingreso-marca-agua' }, 'solvo'),
    ]),
    h('div', { class: 'ingreso-columna' }, [
      h('div', { class: 'ingreso-presentacion' }, [
        h('img', { class: 'marca-solvo-oscuro', src: '/logo/solvo-horizontal-oscuro.svg', alt: 'Solvo' }),
        h('p', { class: 'ingreso-frase' }, ['Cuentas claras,', h('br'), h('em', {}, 'saldo en cero.')]),
      ]),
      formulario,
      pie,
    ]),
  ]));
  (slug.valor() ? email : slug).control.focus();
  await cargarMarca(slug.valor());
}
