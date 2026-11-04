// vistas/clientes.js — lista de clientes con su saldo, con búsqueda y paginada.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, monto, tabla, vacio, encabezadoPantalla, cargando, paginador, campoBusqueda, avisoError,
  botonEnlace,
} from '../componentes.js';
import { modalCrearCliente, modalReasignarCartera } from './modales.js';

export async function render(contenedor) {
  const zonaTabla = h('div', {}, cargando());
  const zonaPie = h('div');
  let busqueda = '';
  const recargar = () => cargar(1);

  // Los dos archivos que pide el contador. Son enlaces a la propia API con la cookie de sesión:
  // el navegador los descarga y el CSV nunca se arma en JavaScript.
  const acciones = [
    botonEnlace('Exportar a Excel', api.urlCarteraCSV(), { icono: 'descargar', descargar: true }),
    botonEnlace('Antigüedad de saldos', api.urlAntiguedadCSV(), { icono: 'documento', descargar: true }),
    estado.puede('reasignar_cartera') ? boton('Reasignar cartera', { icono: 'reasignar', onclick: () => modalReasignarCartera(recargar) }) : null,
    boton('Nuevo cliente', { variante: 'principal', icono: 'agregar', onclick: () => modalCrearCliente((c) => router.ir('clientes', c.id)) }),
  ].filter(Boolean);

  const buscador = campoBusqueda('Buscar por nombre, código o teléfono', '', (texto) => {
    busqueda = texto;
    cargar(1);
  });

  async function cargar(pagina) {
    zonaTabla.replaceChildren(cargando());
    zonaPie.replaceChildren();
    try {
      const r = await api.clientes({ pagina, buscar: busqueda });
      zonaTabla.replaceChildren(r.clientes.length
        ? tabla([
          {
            titulo: 'Cliente',
            celda: (c) => h('div', {}, [
              h('div', {}, c.nombre),
              h('span', { class: 'texto-secundario' }, [c.codigo_externo, c.contacto, c.telefono].filter(Boolean).join(' · ')),
            ]),
          },
          { titulo: 'Plazo', celda: (c) => formato.plazo(c.plazo_credito_dias) },
          { titulo: 'Vendedor', celda: (c) => c.vendedor_nombre },
          { titulo: 'Cuentas vencidas', clase: 'columna-monto', celda: (c) => String(c.cuentas_vencidas) },
          { titulo: 'Saldo', clase: 'columna-monto', celda: (c) => monto(c.saldo_centavos) },
        ], r.clientes, { alElegir: (c) => router.ir('clientes', c.id) })
        : vacio(busqueda ? `Ningún cliente coincide con “${busqueda}”.` : 'Todavía no hay clientes.'));
      const pie = paginador(r.paginacion, cargar);
      if (pie) zonaPie.replaceChildren(pie);
    } catch (error) {
      zonaTabla.replaceChildren(avisoError(error));
    }
  }

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Clientes', acciones),
    buscador.elemento,
    zonaTabla,
    zonaPie,
  ]));
  await cargar(1);
}
