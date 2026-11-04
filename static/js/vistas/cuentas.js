// vistas/cuentas.js — lista de cuentas con filtros por estado (los estados vienen del backend),
// búsqueda por documento o cliente, y paginada: el backend envía una página por vez.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, monto, tabla, vacio, pildoraEstado, marcaRevision, encabezadoPantalla, cargando,
  paginador, campoBusqueda, avisoError, botonEnlace,
} from '../componentes.js';

export async function render(contenedor) {
  const opciones = [estado.filtroTodas(), ...estado.estadosCuenta()];
  const zonaTabla = h('div', {}, cargando());
  const zonaPie = h('div');
  let busqueda = '';

  // El CSV baja exactamente lo que hay en pantalla: el enlace se rehace con el filtro y la
  // búsqueda vigentes en cada carga. Es un enlace, no un fetch: lo descarga el navegador con la
  // cookie de sesión y el archivo nunca pasa por JavaScript.
  const enlaceExcel = botonEnlace('Exportar a Excel', api.urlCarteraCSV(), { icono: 'descargar', descargar: true });

  const filtros = h('div', { class: 'filtros', role: 'group', 'aria-label': 'Filtrar por estado' }, opciones.map((e) =>
    h('button', {
      type: 'button', class: 'filtro', dataset: { estado: e },
      'aria-pressed': String(e === estado.filtroCuentasActual()),
      onclick: () => {
        estado.fijarFiltroCuentas(e);
        filtros.querySelectorAll('.filtro').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.estado === e)));
        cargar(1);
      },
    }, formato.filtroEstado(e))));

  const buscador = campoBusqueda('Buscar por documento o cliente', '', (texto) => {
    busqueda = texto;
    cargar(1);
  });

  async function cargar(pagina) {
    zonaTabla.replaceChildren(cargando());
    zonaPie.replaceChildren();
    enlaceExcel.href = api.urlCarteraCSV({ estado: estado.filtroCuentasActual(), buscar: busqueda });
    try {
      const r = await api.cuentas(estado.filtroCuentasActual(), { pagina, buscar: busqueda });
      zonaTabla.replaceChildren(r.cuentas.length
        ? tabla([
          {
            titulo: 'Cliente',
            celda: (c) => h('div', {}, [
              h('div', {}, c.cliente_nombre),
              c.numero_documento ? h('span', { class: 'texto-secundario' }, c.numero_documento) : null,
              c.requiere_revision ? marcaRevision() : null,
            ]),
          },
          { titulo: 'Monto', clase: 'columna-monto', celda: (c) => monto(c.monto_total_centavos) },
          { titulo: 'Saldo', clase: 'columna-monto', celda: (c) => monto(c.saldo_centavos) },
          {
            titulo: 'Vencimiento',
            clase: 'columna-fecha',
            celda: (c) => h('div', {}, [
              h('div', {}, formato.fecha(c.fecha_vencimiento)),
              h('span', { class: 'texto-secundario' }, formato.vencimiento(c)),
            ]),
          },
          { titulo: 'Estado', celda: (c) => pildoraEstado(c.estado) },
        ], r.cuentas, { alElegir: (c) => router.ir('cuentas', c.id) })
        : vacio(busqueda ? `Ninguna cuenta coincide con “${busqueda}”.` : 'No hay cuentas con este estado.'));
      const pie = paginador(r.paginacion, cargar);
      if (pie) zonaPie.replaceChildren(pie);
    } catch (error) {
      zonaTabla.replaceChildren(avisoError(error));
    }
  }

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Cuentas', [enlaceExcel]),
    buscador.elemento,
    filtros,
    zonaTabla,
    zonaPie,
  ]));
  await cargar(1);
}
