// vistas/cliente.js — ficha del cliente: saldo, límite y disponible, anticipos a favor, cuentas
// abiertas, historial y comportamiento de pago como una línea de texto con color (sin gráficas).

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, boton, tarjeta, monto, dato, tabla, vacio, pildoraEstado, marcaRevision, enlaceVolver, botonEnlace,
} from '../componentes.js';
import { modalCrearAnticipo, modalAplicarAnticipo, modalAnularAnticipo } from './modales_cobranza.js';
import { modalAnotarPromesa, modalAnularPromesa } from './modales_promesas.js';
import { pildoraPromesa } from './promesas.js';

function tablaCuentas(cuentas, textoVacio) {
  if (!cuentas.length) return vacio(textoVacio);
  return tabla([
    { titulo: 'Documento', celda: (c) => c.numero_documento || '—' },
    { titulo: 'Emisión', clase: 'columna-fecha', celda: (c) => formato.fecha(c.fecha_emision) },
    { titulo: 'Vencimiento', celda: (c) => h('div', {}, [h('div', {}, formato.fecha(c.fecha_vencimiento)), h('span', { class: 'texto-secundario' }, formato.vencimiento(c))]) },
    { titulo: 'Monto', clase: 'columna-monto', celda: (c) => monto(c.monto_total_centavos) },
    { titulo: 'Saldo', clase: 'columna-monto', celda: (c) => monto(c.saldo_centavos) },
    { titulo: 'Estado', celda: (c) => h('div', { class: 'sellos' }, [pildoraEstado(c.estado), c.requiere_revision ? marcaRevision() : null]) },
  ], cuentas, { alElegir: (c) => router.ir('cuentas', c.id) });
}

function seccionAnticipos(cliente, anticipos, abiertas, recargar) {
  const vivos = anticipos.filter((a) => !a.anulado);
  const aFavor = vivos.reduce((suma, a) => suma + a.disponible_centavos, 0);
  return h('section', { class: 'pantalla' }, [
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('div', {}, [
        h('h2', {}, 'Anticipos'),
        h('span', { class: 'texto-secundario' }, aFavor > 0
          ? `${formato.dinero(aFavor)} a favor del cliente, sin aplicar a ninguna cuenta.`
          : 'Dinero entregado sin factura asignada.'),
      ]),
      boton('Registrar anticipo', { icono: 'anticipo', onclick: () => modalCrearAnticipo(cliente, recargar) }),
    ]),
    anticipos.length
      ? tabla([
        { titulo: 'Número', celda: (a) => a.numero },
        { titulo: 'Fecha', clase: 'columna-fecha', celda: (a) => formato.fecha(a.fecha) },
        { titulo: 'Forma de pago', celda: (a) => h('div', {}, [h('div', {}, formato.metodoPago(a.metodo)), a.referencia ? h('span', { class: 'texto-secundario' }, `Ref. ${a.referencia}`) : null]) },
        { titulo: 'Monto', clase: 'columna-monto', celda: (a) => monto(a.monto_centavos) },
        { titulo: 'Disponible', clase: 'columna-monto', celda: (a) => monto(a.disponible_centavos) },
        {
          titulo: '',
          celda: (a) => (a.anulado
            ? h('span', { class: 'pildora estado-pagado' }, 'Anulado')
            : h('div', { class: 'pantalla-acciones' }, [
              a.disponible_centavos > 0
                ? boton('Aplicar', { variante: 'principal', onclick: () => modalAplicarAnticipo(a, abiertas, recargar) })
                : h('span', { class: 'pildora estado-pagado' }, 'Aplicado'),
              a.aplicado_centavos === 0 && estado.esAdmin()
                ? boton('Anular', { variante: 'discreto', onclick: () => modalAnularAnticipo(a, recargar) })
                : null,
            ])),
        },
      ], anticipos, { claseFila: (a) => (a.anulado ? 'movimiento-anulado' : '') })
      : h('p', { class: 'texto-secundario' }, 'Este cliente no tiene anticipos registrados.'),
  ]);
}

// Promesas de pago del cliente: lo que prometió, cuándo y si cumplió. El estado de cada una lo
// calcula el backend; aquí solo se muestra.
function seccionPromesas(cliente, abiertas, promesas, recargar) {
  return h('section', { class: 'pantalla' }, [
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('div', {}, [
        h('h2', {}, 'Promesas de pago'),
        h('span', { class: 'texto-secundario' }, 'Lo que el cliente se comprometió a pagar, y si cumplió.'),
      ]),
      boton('Anotar promesa', { icono: 'documento', onclick: () => modalAnotarPromesa(cliente, abiertas, recargar) }),
    ]),
    promesas.length
      ? tabla([
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
        { titulo: 'Resultado', celda: (p) => pildoraPromesa(p) },
        {
          titulo: '',
          celda: (p) => (!p.anulada && (estado.esAdmin() || p.registrado_por === estado.usuario()?.id)
            ? boton('Anular', { variante: 'discreto', onclick: () => modalAnularPromesa(p, recargar) })
            : null),
        },
      ], promesas, { claseFila: (p) => (p.anulada ? 'movimiento-anulado' : '') })
      : h('p', { class: 'texto-secundario' }, 'Este cliente no tiene promesas registradas.'),
  ]);
}

export async function render(contenedor, { id }) {
  const r = await api.cliente(id);
  const { cliente, credito, comportamiento_pago: comportamiento } = r;
  const { anticipos } = await api.anticipos(id);
  // Las promesas se piden ya filtradas por cliente: traerse 200 para quedarse con tres es
  // trabajo que la base hace mejor.
  const { promesas: susPromesas } = await api.promesas({ clienteId: id, porPagina: 50 });
  const recargar = () => render(contenedor, { id });
  const abiertas = r.cuentas.filter((c) => c.estado !== 'pagado');
  const pagadas = r.cuentas.filter((c) => c.estado === 'pagado');

  const resumen = tarjeta([
    h('span', { class: 'saldo-etiqueta' }, 'Saldo pendiente'),
    monto(r.saldo_centavos, 'monto-principal'),
    h('p', { class: `comportamiento comportamiento-${comportamiento.clasificacion}` }, formato.comportamiento(comportamiento)),
    h('div', { class: 'rejilla-datos' }, [
      dato('Límite de crédito', credito.sin_limite ? 'Sin límite' : formato.dinero(credito.limite_credito_centavos)),
      dato('Disponible', credito.sin_limite ? 'Sin límite' : formato.dinero(credito.disponible_centavos)),
      dato('Saldo vencido', formato.dinero(r.saldo_vencido_centavos)),
      dato('Plazo de crédito', formato.plazo(cliente.plazo_credito_dias)),
    ]),
  ], 'tarjeta-saldo');

  const contacto = tarjeta(h('div', { class: 'rejilla-datos' }, [
    dato('Código', cliente.codigo_externo || '—'),
    dato('Contacto', cliente.contacto || '—'),
    dato('Teléfono', cliente.telefono || '—'),
    dato('Vendedor', cliente.vendedor_nombre),
    dato('Situación', cliente.activo ? 'Activo' : 'Desactivado'),
  ]));

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    h('header', { class: 'pantalla-encabezado' }, [
      enlaceVolver(estado.esAdmin() ? router.enlace('clientes') : router.enlace('cartera'), 'Volver a clientes'),
      h('h1', {}, cliente.nombre),
      h('div', { class: 'pantalla-acciones' }, [
        botonEnlace('Estado de cuenta', api.urlEstadoCuenta(id, { detalle: true }), { variante: 'principal', icono: 'documento' }),
        estado.puede('adjuntar')
          ? botonEnlace('Con fotos', api.urlEstadoCuenta(id, { detalle: true, fotos: true }), { icono: 'foto' })
          : null,
      ]),
      resumen,
    ]),
    contacto,
    h('section', { class: 'pantalla' }, [h('h2', {}, 'Cuentas abiertas'), tablaCuentas(abiertas, 'No tiene cuentas abiertas.')]),
    seccionAnticipos(cliente, anticipos, abiertas, recargar),
    seccionPromesas(cliente, abiertas, susPromesas, recargar),
    h('section', { class: 'pantalla' }, [h('h2', {}, 'Historial de cuentas pagadas'), tablaCuentas(pagadas, 'Todavía no tiene cuentas pagadas.')]),
  ]));
}
