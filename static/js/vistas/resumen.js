// vistas/resumen.js — el tablero del dueño: cuánto hay por cobrar, cuánto se atrasó, cuánto
// entró y cuánto debería entrar. Todas las cifras vienen calculadas del backend
// (GET /api/resumen y GET /api/tablero); aquí solo se dibujan.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, tabla, vacio, barras, columnas, indicador, aviso,
} from '../componentes.js';
import { modalInformeMensual } from './modales_cobranza.js';

// Color de cada tramo de antigüedad: mientras más viejo el saldo, más grave.
const CLASE_TRAMO = ['relleno-al-dia', 'relleno-por-vencer', 'relleno-por-vencer', 'relleno-vencido', 'relleno-vencido'];

function tarjetaAntiguedad(tramos) {
  const total = tramos.reduce((suma, t) => suma + t.saldo_centavos, 0);
  return tarjeta([
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('h2', {}, 'Antigüedad de saldos'),
      h('span', { class: 'texto-secundario' }, `${formato.dinero(total)} por cobrar`),
    ]),
    total === 0
      ? vacio('No hay saldos pendientes.')
      : barras(tramos.map((t, i) => ({
        etiqueta: t.etiqueta,
        valor: t.saldo_centavos,
        clase: CLASE_TRAMO[i],
        texto: `${formato.dinero(t.saldo_centavos)} · ${t.cantidad}`,
      }))),
    h('p', { class: 'texto-secundario' }, 'Días transcurridos desde el vencimiento. Las cuentas por vencer todavía están en plazo.'),
  ]);
}

function tarjetaCobranza(serie) {
  // El eje marca una fecha por semana: con más, las etiquetas se enciman en el teléfono.
  const cada = 7;
  const puntos = serie.map((p, i) => ({
    valor: p.monto_centavos,
    etiqueta: (serie.length - 1 - i) % cada === 0 ? formato.fechaCorta(p.fecha) : '',
    titulo: `${formato.fecha(p.fecha)}: ${formato.dinero(p.monto_centavos)} en ${p.cantidad} pago(s)`,
  }));
  return tarjeta([
    h('h2', {}, 'Cobranza de los últimos 30 días'),
    columnas(puntos, 'Todavía no hay pagos registrados en el período.'),
  ]);
}

function tarjetaCobradores(cobradores) {
  const conDatos = cobradores.filter((c) => c.cobrado_mes_centavos > 0 || c.cartera_centavos > 0);
  if (!conDatos.length) return null;
  return h('section', { class: 'pantalla' }, [
    h('h2', {}, 'Cobradores'),
    tabla([
      { titulo: 'Cobrador', celda: (c) => c.nombre },
      { titulo: 'Cobrado del mes', clase: 'columna-monto', celda: (c) => monto(c.cobrado_mes_centavos) },
      { titulo: 'Cartera', clase: 'columna-monto', celda: (c) => monto(c.cartera_centavos) },
      {
        titulo: 'Vencido',
        clase: 'columna-monto',
        celda: (c) => h('div', {}, [
          monto(c.vencido_centavos),
          h('div', { class: 'texto-secundario' }, c.cartera_centavos > 0 ? formato.porcentaje(c.porcentaje_mora) : '—'),
        ]),
      },
      { titulo: 'Clientes', clase: 'columna-monto', celda: (c) => String(c.clientes) },
      {
        titulo: 'Liquidaciones del mes',
        celda: (c) => (c.tiene_liquidaciones
          ? h('span', { class: claseDiferencia(c.diferencia_liquidaciones_centavos) }, formato.diferenciaLiquidacion(c.diferencia_liquidaciones_centavos))
          : h('span', { class: 'texto-secundario' }, 'Sin liquidar')),
      },
    ], conDatos),
    h('p', { class: 'texto-secundario' }, 'El cobrado del mes es el dinero que cada quien recibió. La cartera es la que tiene asignada.'),
  ]);
}

function claseDiferencia(centavos) {
  if (centavos === 0) return 'diferencia-cuadra';
  return centavos < 0 ? 'diferencia-falta' : 'diferencia-sobra';
}

function tarjetaFuga(fuga) {
  const total = fuga.condonado_centavos + fuga.nota_credito_centavos;
  if (total === 0) return null;
  return tarjeta([
    h('h2', {}, 'Saldo que se fue sin cobrarse este mes'),
    h('div', { class: 'rejilla-datos' }, [
      h('div', { class: 'dato' }, [
        h('span', { class: 'dato-etiqueta' }, 'Notas de crédito'),
        h('span', { class: 'dato-valor monto' }, formato.dinero(fuga.nota_credito_centavos)),
      ]),
      h('div', { class: 'dato' }, [
        h('span', { class: 'dato-etiqueta' }, 'Condonaciones'),
        h('span', { class: 'dato-valor monto' }, formato.dinero(fuga.condonado_centavos)),
      ]),
    ]),
    h('p', { class: 'texto-secundario' }, 'Devoluciones, descuentos posteriores y deuda perdonada. No entró dinero por este monto.'),
  ]);
}

export async function render(contenedor) {
  const [r, t] = await Promise.all([api.resumen(), api.tablero()]);
  const cfg = estado.config();
  const semana = t.proximos_vencimientos.find((p) => p.dias === 7);
  const mes = t.proximos_vencimientos.find((p) => p.dias === 30);
  const variacion = t.cobranza.mes_anterior_centavos;

  const banda = h('header', { class: 'pantalla-encabezado' }, [
    h('h1', {}, ['Total por ', h('em', {}, 'cobrar')]),
    h('div', { class: 'pantalla-acciones' }, [
      boton('Informe mensual', { variante: 'principal', icono: 'documento', onclick: () => modalInformeMensual(r.fecha) }),
    ]),
    monto(r.total_por_cobrar_centavos, 'monto-principal'),
    h('div', { class: 'banda-cifras' }, [
      h('button', {
        type: 'button', class: 'banda-cifra',
        onclick: () => { estado.fijarFiltroCuentas('vencido'); router.ir('cuentas'); },
      }, [
        h('span', { class: 'sv-etiqueta' }, 'Vencido'),
        monto(r.total_vencido_centavos, 'monto-destacado'),
        h('span', { class: 'banda-nota' }, 'Ver cuentas vencidas'),
      ]),
      h('div', { class: 'banda-cifra' }, [
        h('span', { class: 'sv-etiqueta' }, 'Al día'),
        monto(r.total_al_dia_centavos, 'monto-destacado'),
        h('span', { class: 'banda-nota' }, `Incluye ${formato.dinero(r.total_por_vencer_centavos)} por vencer. ${formato.avisoPrevio(cfg.dias_aviso_previo)}`),
      ]),
    ]),
  ]);

  const segundaFila = h('div', { class: 'rejilla-indicadores' }, [
    indicador('Cobrado hoy', formato.dinero(t.cobranza.hoy_centavos), {
      detalle: `${formato.dinero(t.cobranza.semana_centavos)} en los últimos 7 días`,
    }),
    indicador('Cobrado este mes', formato.dinero(t.cobranza.mes_centavos), {
      detalle: `Mes anterior completo: ${formato.dinero(variacion)}`,
    }),
    indicador('Entra esta semana', formato.dinero(semana?.saldo_centavos ?? 0), {
      detalle: `${semana?.cantidad ?? 0} cuenta(s) vencen en 7 días`,
    }),
    indicador('Entra este mes', formato.dinero(mes?.saldo_centavos ?? 0), {
      detalle: `${mes?.cantidad ?? 0} cuenta(s) vencen en 30 días`,
    }),
  ]);

  const deudores = r.top_deudores.length
    ? tabla([
      { titulo: 'Cliente', celda: (d) => d.cliente_nombre },
      { titulo: 'Cuentas abiertas', celda: (d) => String(d.cuentas_abiertas) },
      { titulo: 'Vencido', clase: 'columna-monto', celda: (d) => monto(d.saldo_vencido_centavos) },
      { titulo: 'Saldo', clase: 'columna-monto', celda: (d) => monto(d.saldo_centavos) },
    ], r.top_deudores, { alElegir: (d) => router.ir('clientes', d.cliente_id) })
    : vacio('No hay saldos pendientes.');

  const pendientes = t.liquidaciones_pendientes ?? 0;

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    banda,
    pendientes > 0
      ? aviso('advertencia', h('p', {}, `Hay ${pendientes} liquidación(es) de cobrador esperando su aprobación.`),
        [h('a', { class: 'boton', href: router.enlace('liquidaciones') }, 'Revisarlas')])
      : null,
    segundaFila,
    tarjetaAntiguedad(t.antiguedad),
    tarjetaCobranza(t.serie_cobranza),
    t.cobradores ? tarjetaCobradores(t.cobradores) : null,
    tarjetaFuga(t.fuga_mes),
    h('section', { class: 'pantalla' }, [h('h2', {}, 'Clientes que más deben'), deudores]),
  ]));
}
