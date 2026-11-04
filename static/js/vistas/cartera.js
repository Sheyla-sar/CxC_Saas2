// vistas/cartera.js — "Mi cartera" del vendedor: sus clientes con saldo y el estado de cada cuenta
// abierta. Registrar un pago toma dos clics: "Registrar pago" y confirmar en el modal.
// El backend envía una página de clientes con sus cuentas abiertas ya adentro (?con_cuentas=true).

import { api } from '../api.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, vacio, pildoraEstado, marcaRevision, encabezadoPantalla, cargando,
  paginador, campoBusqueda, avisoError,
} from '../componentes.js';
import { modalCobrarACliente } from './modales.js';
import { modalAnotarPromesa } from './modales_promesas.js';

export async function render(contenedor) {
  const zonaLista = h('div', {}, cargando());
  const zonaPie = h('div');
  let busqueda = '';

  const buscador = campoBusqueda('Buscar un cliente', '', (texto) => {
    busqueda = texto;
    cargar(1);
  });

  function fichaCliente(cl, recargar) {
    const abiertas = cl.cuentas_abiertas_lista ?? [];
    return tarjeta([
      h('div', { class: 'tarjeta-seccion-titulo' }, [
        h('div', {}, [
          h('h2', {}, h('a', { href: router.enlace('clientes', cl.id) }, cl.nombre)),
          h('span', { class: 'texto-secundario' }, [cl.contacto, cl.telefono].filter(Boolean).join(' · ')),
        ]),
        monto(cl.saldo_centavos, 'monto-destacado'),
      ]),
      // Una sola acción por cliente: en la puerta se cobra al cliente, no a una factura. Si debe
      // varias, el modal pregunta a cuál se le aplica el pago.
      abiertas.length
        ? h('div', { class: 'lista' }, abiertas.map((c) => h('a', {
          class: `lista-fila estado-${c.estado}`, href: router.enlace('cuentas', c.id),
        }, [
          pildoraEstado(c.estado),
          h('span', { class: 'lista-fila-principal' }, [
            h('span', {}, formato.vencimiento(c)),
            c.numero_documento ? h('span', { class: 'texto-secundario' }, c.numero_documento) : null,
            c.requiere_revision ? marcaRevision() : null,
          ]),
          monto(c.saldo_centavos, 'monto dato-valor'),
        ])))
        : h('p', { class: 'texto-secundario' }, 'Sin cuentas pendientes.'),
      // Una sola acción PRINCIPAL por ficha (cobrar). Anotar la promesa es secundaria: es lo
      // que queda cuando el cliente no paga, y el cobrador la usa sin salir de la cartera.
      abiertas.length
        ? h('div', { class: 'pantalla-acciones' }, [
          boton('Registrar pago', {
            variante: 'principal', icono: 'pago',
            onclick: () => modalCobrarACliente(cl, abiertas, recargar),
          }),
          boton('Anotar promesa', {
            icono: 'documento',
            onclick: () => modalAnotarPromesa(cl, abiertas, recargar),
          }),
        ])
        : null,
    ]);
  }

  async function cargar(pagina) {
    zonaLista.replaceChildren(cargando());
    zonaPie.replaceChildren();
    try {
      const r = await api.clientes({ pagina, porPagina: 25, buscar: busqueda, conCuentas: true });
      const recargar = () => cargar(pagina);
      const conSaldo = r.clientes.filter((cl) => (cl.cuentas_abiertas_lista ?? []).length > 0);
      const sinSaldo = r.clientes.filter((cl) => (cl.cuentas_abiertas_lista ?? []).length === 0);

      zonaLista.replaceChildren(h('div', { class: 'pantalla' }, [
        conSaldo.length
          ? h('div', { class: 'lista' }, conSaldo.map((cl) => fichaCliente(cl, recargar)))
          : vacio(busqueda ? `Ningún cliente coincide con “${busqueda}”.` : 'No tiene clientes con saldo pendiente.'),
        sinSaldo.length
          ? h('section', { class: 'pantalla' }, [
            h('h2', {}, 'Clientes sin saldo pendiente'),
            h('div', { class: 'lista' }, sinSaldo.map((cl) => h('a', { class: 'lista-fila', href: router.enlace('clientes', cl.id) }, [
              h('span', { class: 'lista-fila-principal' }, cl.nombre),
              h('span', { class: 'texto-secundario' }, 'Sin saldo pendiente'),
            ]))),
          ])
          : null,
      ]));
      const pie = paginador(r.paginacion, cargar);
      if (pie) zonaPie.replaceChildren(pie);
    } catch (error) {
      zonaLista.replaceChildren(avisoError(error));
    }
  }

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Mi cartera'),
    buscador.elemento,
    zonaLista,
    zonaPie,
  ]));
  await cargar(1);
}
