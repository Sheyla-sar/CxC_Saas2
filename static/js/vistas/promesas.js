// vistas/promesas.js — la pantalla de las promesas de pago.
// Arriba, la agenda: lo que el cliente dijo que iba a pagar hoy, y lo que prometió para días que
// ya pasaron y sigue sin pagar. Abajo, el historial con filtro por estado, que es donde se ve
// quién promete y cumple y quién promete y no.
//
// El navegador no decide nada: el estado de cada promesa, los días de atraso y lo que el cliente
// abonó dentro de la ventana vienen calculados del backend. Aquí solo se muestran y se agrupan.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, tabla, vacio, cargando, avisoError, encabezadoPantalla,
  paginador, dato,
} from '../componentes.js';
import { modalAnularPromesa, modalCobrarPromesa } from './modales_promesas.js';

const ESTADOS = ['todas', 'pendiente', 'rota', 'cumplida', 'anulada'];

/** Sello de la promesa. Las cuatro tintas significan lo mismo que en el resto del sistema. */
export function pildoraPromesa(p) {
  return h('span', { class: `pildora sv-estado promesa-${p.estado}` }, formato.estadoPromesa(p.estado));
}

/** ¿Puede este usuario anular esta promesa? Lo mismo que exige el backend, para no ofrecer de más. */
function puedeAnular(p) {
  return !p.anulada && (estado.esAdmin() || p.registrado_por === estado.usuario()?.id);
}

/** Una fila de la agenda: el cliente, lo que prometió, para cuándo y el botón de cobrar. */
function fichaAgenda(p, recargar) {
  return h('div', { class: `lista-fila promesa-fila promesa-${p.estado}` }, [
    pildoraPromesa(p),
    h('span', { class: 'lista-fila-principal' }, [
      h('a', { href: router.enlace('clientes', p.cliente_id) }, p.cliente_nombre),
      h('span', { class: 'texto-secundario' }, formato.cuandoPromesa(p)),
      p.numero_documento ? h('span', { class: 'texto-secundario' }, `Factura ${p.numero_documento}`) : null,
      p.nota ? h('span', { class: 'texto-secundario' }, `“${p.nota}”`) : null,
    ]),
    h('span', { class: 'monto dato-valor' }, formato.montoPromesa(p)),
    h('div', { class: 'pantalla-acciones' }, [
      boton('Registrar pago', {
        variante: 'principal', icono: 'pago',
        onclick: () => modalCobrarPromesa(p, recargar),
      }),
      puedeAnular(p) ? boton('Anular', { variante: 'discreto', onclick: () => modalAnularPromesa(p, recargar) }) : null,
    ]),
  ]);
}

function tablaHistorial(promesas, recargar) {
  if (!promesas.length) return vacio('No hay promesas con este estado.');
  const columnas = [
    {
      titulo: 'Cliente',
      clase: 'columna-cliente',
      celda: (p) => h('div', {}, [
        h('div', {}, p.cliente_nombre),
        p.nota ? h('span', { class: 'texto-secundario nota-promesa' }, p.nota) : null,
      ]),
    },
    { titulo: 'Prometió', clase: 'columna-monto', celda: (p) => formato.montoPromesa(p) },
    {
      titulo: 'Para cuándo',
      clase: 'columna-fecha',
      celda: (p) => h('div', {}, [
        h('div', {}, formato.fecha(p.fecha_promesa)),
        h('span', { class: 'texto-secundario' }, formato.cuandoPromesa(p)),
      ]),
    },
    { titulo: 'Abonado', clase: 'columna-monto', celda: (p) => monto(p.pagado_en_ventana_centavos) },
    { titulo: 'La anotó', celda: (p) => p.registrado_por_nombre },
    { titulo: 'Estado', celda: (p) => pildoraPromesa(p) },
    {
      titulo: '',
      celda: (p) => (puedeAnular(p)
        ? boton('Anular', { variante: 'discreto', onclick: () => modalAnularPromesa(p, recargar) })
        : null),
    },
  ];
  return tabla(columnas, promesas, { alElegir: (p) => router.ir('clientes', p.cliente_id) });
}

export async function render(contenedor) {
  const zonaAgenda = h('div', {}, cargando());
  const zonaHistorial = h('div', {}, cargando());
  const zonaPie = h('div');
  let filtro = 'todas';
  let paginaActual = 1;

  const filtros = h('div', { class: 'filtros', role: 'group', 'aria-label': 'Filtrar por estado' }, ESTADOS.map((e) =>
    h('button', {
      type: 'button', class: 'filtro', dataset: { estado: e }, 'aria-pressed': String(e === filtro),
      onclick: () => {
        filtro = e;
        filtros.querySelectorAll('.filtro').forEach((b) => b.setAttribute('aria-pressed', String(b.dataset.estado === e)));
        cargarHistorial(1);
      },
    }, formato.filtroPromesa(e))));

  const recargar = () => { cargarAgenda(); cargarHistorial(paginaActual); };

  async function cargarAgenda() {
    zonaAgenda.replaceChildren(cargando());
    try {
      const r = await api.agendaDePromesas();
      zonaAgenda.replaceChildren(r.promesas.length
        ? h('div', {}, [
          tarjeta([
            h('span', { class: 'texto-secundario' }, 'Prometido y sin cobrar'),
            monto(r.total_prometido_centavos, 'monto-principal'),
            h('div', { class: 'rejilla-datos' }, [
              dato('Visitas de hoy', String(r.promesas.length)),
              dato('Al día', formato.fecha(r.fecha)),
            ]),
          ], 'tarjeta-saldo'),
          h('div', { class: 'lista' }, r.promesas.map((p) => fichaAgenda(p, recargar))),
        ])
        : vacio('No hay promesas pendientes para hoy. Anote una desde la ficha del cliente cuando alguien le diga cuándo va a pagar.'));
    } catch (error) {
      zonaAgenda.replaceChildren(avisoError(error));
    }
  }

  async function cargarHistorial(pagina) {
    paginaActual = pagina;
    zonaHistorial.replaceChildren(cargando());
    zonaPie.replaceChildren();
    try {
      const r = await api.promesas({ estado: filtro, pagina, porPagina: 25 });
      zonaHistorial.replaceChildren(tablaHistorial(r.promesas, recargar));
      const pie = paginador(r.paginacion, cargarHistorial);
      if (pie) zonaPie.replaceChildren(pie);
    } catch (error) {
      zonaHistorial.replaceChildren(avisoError(error));
    }
  }

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Promesas de pago'),
    h('section', { class: 'pantalla' }, [
      h('h2', {}, 'Para cobrar hoy'),
      h('p', { class: 'texto-secundario' },
        'Lo que sus clientes se comprometieron a pagar hoy, y lo que prometieron para días que ya pasaron y sigue sin entrar.'),
      zonaAgenda,
    ]),
    h('section', { class: 'pantalla' }, [
      h('h2', {}, 'Historial de promesas'),
      filtros,
      zonaHistorial,
      zonaPie,
    ]),
  ]));

  await Promise.all([cargarAgenda(), cargarHistorial(1)]);
}
