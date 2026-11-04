// vistas/revisiones.js — ajustes de monto pendientes, con aprobar y rechazar (solo admin).

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import { h, boton, tarjeta, dato, vacio, notificar, encabezadoPantalla } from '../componentes.js';
import { modalConfirmar } from './modales.js';

export async function render(contenedor) {
  const { ajustes } = await api.ajustesPendientes();
  const recargar = () => render(contenedor);

  const resolver = (a, aprobar) => modalConfirmar({
    titulo: aprobar ? 'Aprobar cambio de monto' : 'Rechazar cambio de monto',
    texto: aprobar
      ? `La cuenta de ${a.cliente_nombre} conservará el monto de ${formato.dinero(a.monto_nuevo_centavos)}.`
      : `La cuenta de ${a.cliente_nombre} volverá a su monto anterior de ${formato.dinero(a.monto_anterior_centavos)}.`,
    textoConfirmar: aprobar ? 'Aprobar' : 'Rechazar',
    variante: aprobar ? 'principal' : 'peligro',
    accion: async () => {
      await (aprobar ? api.aprobarAjuste(a.id) : api.rechazarAjuste(a.id));
      estado.invalidar();
      notificar(aprobar ? 'Cambio aprobado.' : 'Cambio rechazado: el monto volvió al anterior.');
      recargar();
    },
  });

  const tarjetas = ajustes.map((a) => tarjeta([
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('h2', {}, h('a', { href: router.enlace('cuentas', a.cuenta_id) }, a.cliente_nombre)),
      h('span', { class: 'texto-secundario' }, formato.fechaHora(a.creado_en)),
    ]),
    h('div', { class: 'rejilla-datos' }, [
      dato('Monto anterior', formato.dinero(a.monto_anterior_centavos)),
      dato('Monto nuevo', formato.dinero(a.monto_nuevo_centavos)),
      dato('Solicitado por', a.solicitado_por_nombre),
    ]),
    dato('Motivo', a.motivo),
    h('div', { class: 'pantalla-acciones' }, [
      boton('Aprobar', { variante: 'principal', onclick: () => resolver(a, true) }),
      boton('Rechazar', { onclick: () => resolver(a, false) }),
    ]),
  ]));

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Revisiones'),
    tarjetas.length ? h('div', { class: 'lista' }, tarjetas) : vacio('No hay cambios de monto pendientes de revisión.'),
  ]));
}
