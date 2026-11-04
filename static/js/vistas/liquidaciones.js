// vistas/liquidaciones.js — el corte del cobrador y el historial de entregas.
// Arriba, lo que el usuario tiene en la mano y todavía no ha entregado; abajo, las liquidaciones
// ya cerradas (todas las de la empresa si es administrador, las propias si es cobrador).

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, tabla, vacio, aviso, encabezadoPantalla, barras, dato,
} from '../componentes.js';
import { modalCerrarLiquidacion, modalAprobarLiquidacion, modalRechazarLiquidacion } from './modales_cobranza.js';
import { seccionAdjuntos } from './cuenta.js';

export function claseDiferencia(centavos) {
  if (centavos === 0) return 'diferencia-cuadra';
  return centavos < 0 ? 'diferencia-falta' : 'diferencia-sobra';
}

export function pildoraLiquidacion(l) {
  const clase = { entregada: 'estado-por_vencer', aprobada: 'estado-al_dia', rechazada: 'estado-vencido' }[l.estado] ?? '';
  return h('span', { class: `pildora sv-estado ${clase}` }, formato.estadoLiquidacion(l.estado));
}

function tarjetaCorte(corte, recargar) {
  const hayQueEntregar = corte.total_esperado_centavos > 0 || corte.cantidad_movimientos > 0;
  return tarjeta([
    h('span', { class: 'texto-secundario' }, 'Pendiente de entregar'),
    monto(corte.total_esperado_centavos, 'monto-principal'),
    h('span', { class: 'texto-secundario' },
      `${corte.cantidad_movimientos} cobro(s) de ${corte.cobrador_nombre} al ${formato.fecha(corte.fecha)}`),
    corte.por_metodo.length
      ? barras(corte.por_metodo.map((m) => ({
        etiqueta: formato.metodoPago(m.metodo),
        valor: m.monto_centavos,
        texto: `${formato.dinero(m.monto_centavos)} · ${m.cantidad}`,
      })))
      : null,
    h('div', { class: 'pantalla-acciones' }, [
      boton('Cerrar y entregar', {
        variante: 'principal',
        icono: 'liquidaciones',
        deshabilitado: !hayQueEntregar,
        onclick: () => modalCerrarLiquidacion(corte, recargar),
      }),
    ]),
    hayQueEntregar
      ? null
      : h('p', { class: 'texto-secundario' }, 'No hay dinero pendiente de entregar: todo lo cobrado ya fue liquidado.'),
  ], 'tarjeta-saldo');
}

function detalleCorte(corte) {
  if (!corte.movimientos.length && !corte.anticipos.length) return null;
  const filas = [
    ...corte.movimientos.map((m) => ({
      fecha: m.fecha, numero: m.numero ?? '—', concepto: 'Pago de cuenta',
      metodo: m.metodo, referencia: m.referencia, monto: m.monto_centavos, id: m.id, cuenta: m.cuenta_id,
    })),
    ...corte.anticipos.map((a) => ({
      fecha: a.fecha, numero: a.numero, concepto: `Anticipo de ${a.cliente_nombre}`,
      metodo: a.metodo, referencia: a.referencia, monto: a.monto_centavos, id: a.id, cuenta: null,
    })),
  ];
  return h('section', { class: 'pantalla' }, [
    h('h2', {}, 'Qué incluye el corte'),
    tabla([
      { titulo: 'Fecha', clase: 'columna-fecha', celda: (f) => formato.fecha(f.fecha) },
      { titulo: 'Documento', celda: (f) => f.numero },
      { titulo: 'Concepto', celda: (f) => h('div', {}, [h('div', {}, f.concepto), f.referencia ? h('div', { class: 'texto-secundario' }, `Ref. ${f.referencia}`) : null]) },
      { titulo: 'Forma de pago', celda: (f) => formato.metodoPago(f.metodo) },
      { titulo: 'Monto', clase: 'columna-monto', celda: (f) => monto(f.monto) },
    ], filas, { alElegir: (f) => (f.cuenta ? router.ir('cuentas', f.cuenta) : undefined) }),
  ]);
}

function tablaLiquidaciones(liquidaciones, recargar) {
  if (!liquidaciones.length) return vacio('Todavía no hay liquidaciones cerradas.');
  const columnas = [
    { titulo: 'Número', celda: (l) => l.numero },
    { titulo: 'Fecha', clase: 'columna-fecha', celda: (l) => formato.fecha(l.fecha) },
    { titulo: 'Cobrador', celda: (l) => l.cobrador_nombre },
    { titulo: 'Esperado', clase: 'columna-monto', celda: (l) => monto(l.total_esperado_centavos) },
    { titulo: 'Entregado', clase: 'columna-monto', celda: (l) => monto(l.total_entregado_centavos) },
    {
      titulo: 'Diferencia',
      celda: (l) => h('span', { class: claseDiferencia(l.diferencia_centavos) }, formato.diferenciaLiquidacion(l.diferencia_centavos)),
    },
    { titulo: 'Estado', celda: (l) => pildoraLiquidacion(l) },
  ];
  if (estado.puede('aprobar_liquidaciones')) {
    columnas.push({
      titulo: '',
      celda: (l) => (l.estado !== 'entregada' ? null : h('div', { class: 'pantalla-acciones' }, [
        boton('Aprobar', { variante: 'principal', icono: 'aprobar', onclick: () => modalAprobarLiquidacion(l, recargar) }),
        boton('Rechazar', { icono: 'rechazar', onclick: () => modalRechazarLiquidacion(l, recargar) }),
      ])),
    });
  }
  return tabla(columnas, liquidaciones, { alElegir: (l) => router.ir('liquidaciones', l.id) });
}

export async function render(contenedor) {
  const [corte, { liquidaciones }] = await Promise.all([api.corte(), api.liquidaciones()]);
  const recargar = () => render(contenedor);
  const pendientes = liquidaciones.filter((l) => l.estado === 'entregada');

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Liquidación de cobranza'),
    estado.puede('aprobar_liquidaciones') && pendientes.length
      ? aviso('advertencia', h('p', {}, `${pendientes.length} liquidación(es) esperan su aprobación.`))
      : null,
    h('h2', {}, 'Mi corte de hoy'),
    tarjetaCorte(corte, recargar),
    detalleCorte(corte),
    h('section', { class: 'pantalla' }, [
      h('h2', {}, estado.esAdmin() ? 'Liquidaciones de la empresa' : 'Mis liquidaciones'),
      tablaLiquidaciones(liquidaciones, recargar),
    ]),
  ]));
}

// ───────────── Detalle de una liquidación ─────────────

export async function renderDetalle(contenedor, { id }) {
  const { liquidacion: l, movimientos, adjuntos } = await api.liquidacion(id);
  const recargar = () => renderDetalle(contenedor, { id });

  const acciones = l.estado === 'entregada' && estado.puede('aprobar_liquidaciones')
    ? [
      boton('Aprobar', { variante: 'principal', icono: 'aprobar', onclick: () => modalAprobarLiquidacion(l, () => router.ir('liquidaciones')) }),
      boton('Rechazar', { icono: 'rechazar', onclick: () => modalRechazarLiquidacion(l, recargar) }),
    ]
    : [];

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    h('header', { class: 'pantalla-encabezado' }, [
      h('a', { class: 'volver', href: router.enlace('liquidaciones') }, 'Volver a liquidaciones'),
      h('h1', {}, l.numero),
      h('div', { class: 'pantalla-acciones' }, acciones),
      h('span', { class: 'texto-secundario' }, 'Diferencia'),
      h('span', { class: `monto-principal ${claseDiferencia(l.diferencia_centavos)}` }, formato.diferenciaLiquidacion(l.diferencia_centavos)),
      h('div', { class: 'sellos' }, [pildoraLiquidacion(l)]),
    ]),
    tarjeta([
      h('div', { class: 'rejilla-datos' }, [
        dato('Cobrador', l.cobrador_nombre),
        dato('Fecha', formato.fecha(l.fecha)),
        dato('Esperado', formato.dinero(l.total_esperado_centavos)),
        dato('Entregado', formato.dinero(l.total_entregado_centavos)),
        dato('Cobros incluidos', String(l.cantidad_movimientos)),
        dato('Revisado por', l.revisado_por_nombre ?? '—'),
      ]),
      l.nota ? dato('Explicación del cobrador', l.nota) : null,
      l.motivo_rechazo ? aviso('error', h('p', {}, `Rechazada: ${l.motivo_rechazo}`)) : null,
    ]),
    h('section', { class: 'pantalla' }, [
      h('h2', {}, 'Cobros incluidos'),
      movimientos.length
        ? tabla([
          { titulo: 'Fecha', clase: 'columna-fecha', celda: (m) => formato.fecha(m.fecha) },
          { titulo: 'Recibo', celda: (m) => m.numero ?? '—' },
          { titulo: 'Forma de pago', celda: (m) => formato.metodoPago(m.metodo) },
          { titulo: 'Referencia', celda: (m) => m.referencia || '—' },
          { titulo: 'Monto', clase: 'columna-monto', celda: (m) => monto(m.monto_centavos) },
        ], movimientos, { alElegir: (m) => router.ir('cuentas', m.cuenta_id) })
        : vacio('Esta liquidación no tiene cobros asociados.'),
    ]),
    // El cobrador adjunta aquí la foto del depósito o del sobre entregado.
    seccionAdjuntos('Comprobantes de la entrega', 'liquidacion', id, adjuntos ?? [], recargar,
      { textoVacio: 'Tome una foto del depósito o del sobre entregado para dejarla con esta liquidación.' }),
  ]));
}
