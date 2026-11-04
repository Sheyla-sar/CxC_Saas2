// vistas/cuenta.js — detalle de una cuenta. El saldo pendiente es el dato más grande de la pantalla.
// Los movimientos anulados siguen visibles, en gris y tachados, con su motivo. Cada pago lleva su
// recibo numerado; la factura y los comprobantes se guardan como fotos de la cuenta.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, dato, tabla, vacio, aviso, pildoraEstado, marcaRevision, enlaceVolver,
  botonEnlace, icono,
} from '../componentes.js';
import { modalPago, modalCondonar, modalEditarMonto, modalAnular } from './modales.js';
import { modalNotaCredito, modalSubirAdjunto, modalEliminarAdjunto } from './modales_cobranza.js';

/**
 * Comprobantes de un registro: cada uno con su título, de qué se trata, quién lo subió y cuándo,
 * y la opción de guardarlo como PDF. Un documento sin autor no sirve para discutir con nadie.
 */
export function seccionAdjuntos(titulo, entidad, entidadId, adjuntos, recargar, { textoVacio } = {}) {
  if (!estado.puede('adjuntar')) return null;
  const acciones = [
    adjuntos.length
      ? botonEnlace('Guardar todo en PDF', api.urlComprobantes(entidad, entidadId), { icono: 'documento' })
      : null,
    boton('Agregar comprobante', { variante: 'principal', icono: 'foto', onclick: () => modalSubirAdjunto(entidad, entidadId, recargar) }),
  ].filter(Boolean);

  return h('section', { class: 'pantalla' }, [
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('h2', {}, titulo),
      h('div', { class: 'pantalla-acciones' }, acciones),
    ]),
    adjuntos.length
      ? h('div', { class: 'galeria' }, adjuntos.map((a) => h('div', { class: 'adjunto' }, [
        h('a', {
          href: api.urlAdjunto(a.id), target: '_blank', rel: 'noopener',
          title: `Ver ${a.titulo || a.nombre}`,
        }, a.tipo_mime.startsWith('image/')
          ? h('img', { src: api.urlAdjunto(a.id), alt: a.titulo || a.nombre, loading: 'lazy' })
          : h('div', { class: 'adjunto-nombre' }, [icono('documento'), ' Ver el PDF'])),
        h('div', { class: 'adjunto-titulo', title: a.nombre }, a.titulo || a.nombre),
        a.descripcion ? h('span', { class: 'texto-secundario' }, a.descripcion) : null,
        h('span', { class: 'texto-secundario' }, `${a.subido_por_nombre} · ${formato.fechaHora(a.creado_en)}`),
        h('div', { class: 'adjunto-pie' }, [
          botonEnlace('PDF', api.urlComprobante(a.id), { variante: 'discreto', icono: 'documento' }),
          estado.esAdmin()
            ? boton('', { variante: 'discreto', icono: 'basura', etiqueta: `Eliminar ${a.titulo || a.nombre}`, onclick: () => modalEliminarAdjunto(a, recargar) })
            : null,
        ]),
      ])))
      : h('p', { class: 'texto-secundario' }, textoVacio ?? 'Todavía no hay comprobantes adjuntos.'),
  ]);
}

export async function render(contenedor, { id }) {
  const { cuenta, movimientos, ajustes } = await api.cuenta(id);
  let adjuntos = [];
  if (estado.puede('adjuntar')) {
    ({ adjuntos } = await api.adjuntos('cuenta', id));
  }
  const recargar = () => render(contenedor, { id });
  const abierta = cuenta.estado !== 'pagado';

  const acciones = [
    abierta ? boton('Registrar pago', { variante: 'principal', icono: 'pago', onclick: () => modalPago(cuenta, recargar) }) : null,
    abierta && estado.puede('nota_credito')
      ? boton('Nota de crédito', { icono: 'documento', onclick: () => modalNotaCredito(cuenta, recargar) })
      : null,
    abierta && estado.puede('condonar') ? boton('Condonar', { onclick: () => modalCondonar(cuenta, recargar) }) : null,
    boton('Editar monto', { onclick: () => modalEditarMonto(cuenta, recargar) }),
  ];

  const saldo = tarjeta([
    h('span', { class: 'saldo-etiqueta' }, 'Saldo pendiente'),
    monto(cuenta.saldo_centavos, 'monto-principal'),
    h('div', { class: 'sellos' }, [pildoraEstado(cuenta.estado), cuenta.requiere_revision ? marcaRevision() : null]),
    h('span', { class: 'texto-secundario' }, formato.vencimiento(cuenta)),
    h('div', { class: 'pantalla-acciones' }, acciones),
  ], 'tarjeta-saldo');

  const datos = tarjeta(h('div', { class: 'rejilla-datos' }, [
    dato('Documento', cuenta.numero_documento || '—'),
    dato('Monto total', formato.dinero(cuenta.monto_total_centavos)),
    dato('Pagado', formato.dinero(cuenta.total_pagado_centavos)),
    dato('Notas de crédito', formato.dinero(cuenta.total_nota_credito_centavos)),
    dato('Condonado', formato.dinero(cuenta.total_condonado_centavos)),
    dato('Emisión', formato.fecha(cuenta.fecha_emision)),
    dato('Vencimiento', formato.fecha(cuenta.fecha_vencimiento)),
    dato('Vendedor', cuenta.vendedor_nombre),
  ]));

  const historial = movimientos.length
    ? tabla([
      { titulo: 'Fecha', clase: 'columna-fecha', celda: (m) => formato.fecha(m.fecha) },
      {
        titulo: 'Documento',
        celda: (m) => h('div', {}, [
          h('div', {}, m.numero ?? '—'),
          h('span', { class: 'texto-secundario' }, formato.tipoMovimiento(m.tipo)),
        ]),
      },
      {
        titulo: 'Forma de pago',
        celda: (m) => (m.tipo === 'pago'
          ? h('div', {}, [
            h('div', {}, formato.metodoPago(m.metodo)),
            m.referencia ? h('span', { class: 'texto-secundario' }, `Ref. ${m.referencia}`) : null,
          ])
          : '—'),
      },
      {
        titulo: 'Detalle',
        celda: (m) => h('div', {}, [
          m.nota ? h('div', {}, m.nota) : null,
          h('div', { class: 'texto-secundario' }, `Registrado por ${m.registrado_por_nombre}`),
          m.anulado ? h('div', { class: 'movimiento-detalle-anulacion' },
            `Anulado por ${m.anulado_por_nombre} el ${formato.fechaHora(m.anulado_en)}` +
            (m.motivo_anulacion ? `. Motivo: ${m.motivo_anulacion}` : '')) : null,
        ]),
      },
      { titulo: 'Monto', clase: 'columna-monto', celda: (m) => monto(m.monto_centavos) },
      {
        titulo: '',
        celda: (m) => h('div', { class: 'pantalla-acciones' }, [
          m.numero && !m.anulado
            ? botonEnlace('Recibo', api.urlRecibo(m.id), { variante: 'discreto', icono: 'documento' })
            : null,
          !m.anulado && estado.puede('anular')
            ? boton('Anular', { variante: 'discreto', onclick: () => modalAnular(m, recargar) })
            : null,
          m.anulado ? h('span', { class: 'pildora estado-pagado' }, 'Anulado') : null,
        ]),
      },
    ], movimientos, { claseFila: (m) => (m.anulado ? 'movimiento-anulado' : '') })
    : vacio('Todavía no hay pagos registrados.');

  const cambios = ajustes.length
    ? tabla([
      { titulo: 'Fecha', celda: (a) => formato.fechaHora(a.creado_en) },
      { titulo: 'Cambio', celda: (a) => `${formato.dinero(a.monto_anterior_centavos)} → ${formato.dinero(a.monto_nuevo_centavos)}` },
      { titulo: 'Motivo', celda: (a) => h('div', {}, [h('div', {}, a.motivo), h('span', { class: 'texto-secundario' }, `Solicitado por ${a.solicitado_por_nombre}`)]) },
      { titulo: 'Estado', celda: (a) => formato.estadoAjuste(a.estado) },
    ], ajustes)
    : null;

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    h('header', { class: 'pantalla-encabezado' }, [
      enlaceVolver(estado.esAdmin() ? router.enlace('cuentas') : router.enlace('cartera'), 'Volver a cuentas'),
      h('h1', {}, h('a', { href: router.enlace('clientes', cuenta.cliente_id) }, cuenta.cliente_nombre)),
      h('div', { class: 'pantalla-acciones' }, [
        botonEnlace('Estado de cuenta', api.urlEstadoCuenta(cuenta.cliente_id, { detalle: true }), { icono: 'documento' }),
      ]),
      saldo,
    ]),
    cuenta.requiere_revision
      ? aviso('advertencia', h('p', {}, 'El monto de esta cuenta fue editado y está pendiente de revisión.'))
      : null,
    datos,
    h('section', { class: 'pantalla' }, [h('h2', {}, 'Historial de movimientos'), historial]),
    seccionAdjuntos('Factura y comprobantes', 'cuenta', id, adjuntos, recargar,
      { textoVacio: 'Tome una foto de la factura o del comprobante para tenerla aquí.' }),
    cambios ? h('section', { class: 'pantalla' }, [h('h2', {}, 'Cambios de monto'), cambios]) : null,
  ]));
}
