// vistas/ventas.js — ventas en proceso: una columna por etapa. El cambio de etapa se hace desde
// el detalle (modalVenta); al ganar, se abre el formulario de conversión precargado.

import { api } from '../api.js';
import * as formato from '../formato.js';
import { h, boton, monto, encabezadoPantalla } from '../componentes.js';
import { modalVenta, modalCrearOportunidad } from './modales.js';

export async function render(contenedor) {
  const { etapas } = await api.oportunidades();
  const recargar = () => render(contenedor);

  const columnas = etapas.map((g) => h('section', { class: 'columna-etapa', 'aria-label': formato.etapa(g.etapa) }, [
    h('header', { class: 'columna-etapa-encabezado' }, [
      h('h2', {}, `${formato.etapa(g.etapa)} (${g.cantidad})`),
      h('span', { class: 'texto-secundario' }, formato.dinero(g.total_estimado_centavos)),
    ]),
    ...g.oportunidades.map((op) => h('button', { type: 'button', class: 'tarjeta tarjeta-venta', onclick: () => modalVenta(op, recargar) }, [
      h('strong', {}, op.cliente_nombre),
      monto(op.monto_estimado_centavos),
      h('span', { class: 'texto-secundario' }, op.cuenta_id ? 'Convertida en cuenta' : op.vendedor_nombre),
    ])),
  ]));

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Ventas en proceso', [
      boton('Nueva venta', { variante: 'principal', icono: 'agregar', onclick: () => modalCrearOportunidad(recargar) }),
    ]),
    h('div', { class: 'tablero' }, columnas),
  ]));
}
