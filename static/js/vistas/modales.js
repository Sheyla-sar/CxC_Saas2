// vistas/modales.js — los formularios en modal que se abren desde varias pantallas.
// Todos usan abrirModal (misma estructura) y muestran los mensajes del backend tal cual.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import {
  h, aviso, avisoError, boton, notificar, abrirModal, dato,
  campoTexto, campoSeleccion, campoFecha, campoMonto, campoMotivo, pildoraEstado, selectorBuscable,
} from '../componentes.js';
import { modalPagoRegistrado } from './modales_cobranza.js';

const montoValido = (campo) => (campo.centavos() ?? 0) > 0;

// Los listados vienen paginados, así que los selectores de cliente buscan en el backend en vez
// de traerse la cartera entera. Cada resultado conserva los datos del cliente (plazo, límite).
function buscadorDeClientes(etiqueta, elegido, ayuda) {
  return selectorBuscable(etiqueta, {
    elegido,
    ayuda,
    placeholder: 'Escriba el nombre, el código o el teléfono…',
    buscar: async (texto) => {
      const { clientes } = await api.clientes({ buscar: texto, porPagina: 8 });
      return clientes.map((c) => ({
        ...c,
        detalle: [c.codigo_externo, c.telefono, `plazo ${formato.plazo(c.plazo_credito_dias)}`].filter(Boolean).join(' · '),
      }));
    },
  });
}

/** Detalle del exceso de límite de crédito (error bloqueante o advertencia). */
function detalleExceso(detalle) {
  if (detalle?.exceso_centavos === undefined) return null;
  return h('p', {}, `Exceso sobre el límite: ${formato.dinero(detalle.exceso_centavos)} ` +
    `(límite ${formato.dinero(detalle.limite_credito_centavos)}, deuda abierta ${formato.dinero(detalle.saldo_abierto_centavos)}).`);
}

function resumenCuenta(cuenta) {
  return h('div', { class: 'rejilla-datos' }, [
    dato('Cliente', cuenta.cliente_nombre),
    dato('Saldo pendiente', formato.dinero(cuenta.saldo_centavos)),
    dato('Estado', pildoraEstado(cuenta.estado)),
  ]);
}

// ───────────── Registrar pago ─────────────

export function modalPago(cuenta, alTerminar) {
  const monto = campoMonto('Monto recibido');
  const metodos = estado.metodosPago();
  const metodo = campoSeleccion('Forma de pago', metodos.map((m) => ({ valor: m, texto: formato.metodoPago(m) })), { valor: metodos[0] });
  const referencia = campoTexto('Referencia', { ayuda: 'Número de transferencia, cheque o depósito.' });
  const fecha = campoFecha('Fecha del pago', { ayuda: 'Déjela vacía para registrarlo con fecha de hoy.' });
  const nota = campoTexto('Nota (opcional)');

  // La referencia solo se pide cuando el pago no es en efectivo.
  const ajustarReferencia = () => { referencia.elemento.hidden = metodo.valor() === 'efectivo'; };
  metodo.control.addEventListener('change', ajustarReferencia);
  ajustarReferencia();

  abrirModal({
    titulo: 'Registrar pago',
    contenido: [resumenCuenta(cuenta), monto.elemento, metodo.elemento, referencia.elemento, fecha.elemento, nota.elemento],
    textoConfirmar: 'Registrar pago',
    validar: () => montoValido(monto),
    async alConfirmar(ctx) {
      try {
        const r = await api.registrarPago(cuenta.id, {
          monto_centavos: monto.centavos(),
          fecha: fecha.valor() || undefined,
          metodo: metodo.valor(),
          referencia: referencia.elemento.hidden ? '' : referencia.valor(),
          nota: nota.valor(),
        });
        estado.invalidar();
        ctx.cerrar();
        alTerminar?.(r);
        // El comprobante se ofrece enseguida: recibo numerado y foto del respaldo.
        modalPagoRegistrado(r);
      } catch (error) {
        if (error.codigo !== 'SOBREPAGO') throw error;
        // Regla: ofrecer abonar exactamente el saldo disponible con un clic.
        const saldo = error.detalle.saldo_pendiente_centavos;
        const acciones = saldo > 0
          ? [boton(`Abonar ${formato.dinero(saldo)}`, { variante: 'principal', onclick: () => { monto.fijar(saldo); ctx.confirmar(); } })]
          : [];
        ctx.mensaje(avisoError(error, [h('div', { class: 'aviso-acciones' }, acciones)]));
      }
    },
  });
}

// ───────────── Condonar ─────────────

export function modalCondonar(cuenta, alTerminar) {
  const monto = campoMonto('Monto a condonar', {
    placeholder: formato.montoParaCampo(cuenta.saldo_centavos),
    ayuda: `Déjelo vacío para condonar todo el saldo (${formato.dinero(cuenta.saldo_centavos)}).`,
  });
  const motivo = campoMotivo('Motivo de la condonación', estado.motivoMinimo());

  abrirModal({
    titulo: 'Condonar saldo',
    contenido: [
      resumenCuenta(cuenta),
      aviso('informativo', h('p', {}, 'La condonación reduce el saldo sin registrar dinero recibido.')),
      monto.elemento,
      motivo.elemento,
    ],
    textoConfirmar: 'Condonar',
    variante: 'peligro',
    validar: () => motivo.valido() && (monto.vacio() || montoValido(monto)),
    async alConfirmar(ctx) {
      const r = await api.condonar(cuenta.id, {
        monto_centavos: monto.vacio() ? undefined : monto.centavos(),
        motivo: motivo.valor(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Condonación registrada. Saldo pendiente: ${formato.dinero(r.cuenta.saldo_centavos)}`);
      alTerminar?.(r);
    },
  });
}

// ───────────── Editar monto ─────────────

export function modalEditarMonto(cuenta, alTerminar) {
  const monto = campoMonto('Nuevo monto total', {
    centavos: cuenta.monto_total_centavos,
    ayuda: `Monto actual: ${formato.dinero(cuenta.monto_total_centavos)}. Pagado: ${formato.dinero(cuenta.total_pagado_centavos)}. ` +
      `Condonado: ${formato.dinero(cuenta.total_condonado_centavos)}.`,
  });
  const motivo = campoMotivo('Motivo del cambio', estado.motivoMinimo());
  const revision = estado.config().exigir_revision_ajuste_monto
    ? aviso('informativo', h('p', {}, 'El nuevo monto aplica de inmediato y queda pendiente de revisión por un administrador.'))
    : null;

  abrirModal({
    titulo: 'Editar monto de la cuenta',
    contenido: [resumenCuenta(cuenta), revision, monto.elemento, motivo.elemento],
    textoConfirmar: 'Guardar monto',
    validar: () => motivo.valido() && montoValido(monto),
    async alConfirmar(ctx) {
      const r = await api.editarMonto(cuenta.id, { monto_total_centavos: monto.centavos(), motivo: motivo.valor() });
      estado.invalidar();
      ctx.cerrar();
      notificar(r.ajuste.estado === 'pendiente'
        ? `Monto actualizado a ${formato.dinero(r.cuenta.monto_total_centavos)}. Quedó pendiente de revisión.`
        : `Monto actualizado a ${formato.dinero(r.cuenta.monto_total_centavos)}.`);
      alTerminar?.(r);
    },
  });
}

// ───────────── Anular movimiento ─────────────

export function modalAnular(movimiento, alTerminar) {
  const obligatorio = estado.config().exigir_motivo_anulacion;
  const motivo = campoMotivo('Motivo de la anulación', estado.motivoMinimo(), { obligatorio });

  abrirModal({
    titulo: `Anular ${formato.tipoMovimiento(movimiento.tipo).toLowerCase()}`,
    contenido: [
      h('div', { class: 'rejilla-datos' }, [
        dato('Tipo', formato.tipoMovimiento(movimiento.tipo)),
        dato('Monto', formato.dinero(movimiento.monto_centavos)),
        dato('Fecha', formato.fecha(movimiento.fecha)),
      ]),
      aviso('informativo', h('p', {}, 'El movimiento no se borra: queda en el historial marcado como anulado y deja de contar para el saldo.')),
      motivo.elemento,
    ],
    textoConfirmar: 'Anular',
    variante: 'peligro',
    validar: motivo.valido,
    async alConfirmar(ctx) {
      const r = await api.anularMovimiento(movimiento.id, motivo.valor());
      estado.invalidar();
      ctx.cerrar();
      notificar(`Movimiento anulado. Saldo pendiente: ${formato.dinero(r.cuenta.saldo_centavos)}`);
      alTerminar?.(r);
    },
  });
}

// ───────────── Crear cliente ─────────────

export function modalCrearCliente(alTerminar) {
  const cfg = estado.config();
  const nombre = campoTexto('Nombre del negocio', { requerido: true });
  const contacto = campoTexto('Persona de contacto');
  const telefono = campoTexto('Teléfono', { tipo: 'tel' });
  const plazo = campoSeleccion('Plazo de crédito',
    cfg.plazos_credito_permitidos.map((d) => ({ valor: String(d), texto: formato.plazo(d) })));
  const limite = estado.puede('asignar_limite_credito')
    ? campoMonto('Límite de crédito (opcional)', { ayuda: 'Déjelo vacío si el cliente no tiene límite.' })
    : null;

  abrirModal({
    titulo: 'Nuevo cliente',
    contenido: [nombre.elemento, contacto.elemento, telefono.elemento, plazo.elemento, limite?.elemento],
    textoConfirmar: 'Crear cliente',
    validar: () => nombre.valor() !== '' && (!limite || limite.vacio() || limite.centavos() !== null),
    async alConfirmar(ctx) {
      const cliente = await api.crearCliente({
        nombre: nombre.valor(),
        contacto: contacto.valor(),
        telefono: telefono.valor(),
        plazo_credito_dias: Number(plazo.valor()),
        limite_credito_centavos: limite && !limite.vacio() ? limite.centavos() : undefined,
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Cliente “${cliente.nombre}” creado.`);
      alTerminar?.(cliente);
    },
  });
}

// ───────────── Crear venta en proceso ─────────────

export async function modalCrearOportunidad(alTerminar) {
  const cliente = buscadorDeClientes('Cliente');
  const monto = campoMonto('Monto estimado');
  const notas = campoTexto('Notas');

  abrirModal({
    titulo: 'Nueva venta en proceso',
    contenido: [cliente.elemento, monto.elemento, notas.elemento],
    textoConfirmar: 'Crear',
    validar: () => cliente.valor() !== '' && monto.centavos() !== null,
    async alConfirmar(ctx) {
      await api.crearOportunidad({ cliente_id: cliente.valor(), monto_estimado_centavos: monto.centavos(), notas: notas.valor() });
      estado.invalidar();
      ctx.cerrar();
      notificar('Venta en proceso creada.');
      alTerminar?.();
    },
  });
}

// ───────────── Detalle de una venta en proceso y cambio de etapa ─────────────

export function modalVenta(op, alTerminar) {
  const contenido = [
    h('div', { class: 'rejilla-datos' }, [
      dato('Cliente', op.cliente_nombre),
      dato('Monto estimado', formato.dinero(op.monto_estimado_centavos)),
      dato('Vendedor', op.vendedor_nombre),
      dato('Etapa actual', formato.etapa(op.etapa)),
    ]),
    op.notas ? dato('Notas', op.notas) : null,
  ];

  let ctx;
  const cambiar = async (etapa) => {
    ctx.limpiar();
    try {
      const r = await api.cambiarEtapa(op.id, etapa);
      estado.invalidar();
      ctx.cerrar();
      if (r.conversion) {
        // Ganada: NO se crea la cuenta; se abre el formulario precargado y editable.
        modalConversion(r.conversion, alTerminar);
      } else {
        notificar(`Venta movida a “${formato.etapa(r.oportunidad.etapa)}”.`);
      }
      alTerminar?.();
    } catch (error) {
      ctx.mensaje(avisoError(error));
    }
  };

  if (op.cuenta_id) {
    contenido.push(aviso('informativo', h('p', {}, 'Esta venta ya se convirtió en una cuenta por cobrar.'),
      [h('a', { class: 'boton', href: router.enlace('cuentas', op.cuenta_id), onclick: () => ctx.cerrar() }, 'Ver la cuenta')]));
  } else {
    contenido.push(h('div', { class: 'campo' }, [
      h('span', { class: 'dato-etiqueta' }, 'Mover a la etapa'),
      h('div', { class: 'selector-etapas' }, estado.etapas().map((e) =>
        h('button', { type: 'button', class: 'filtro', 'aria-pressed': String(e === op.etapa), onclick: () => cambiar(e) }, formato.etapa(e)))),
    ]));
    if (op.etapa === 'ganado') {
      contenido.push(aviso('advertencia', h('p', {}, 'Esta venta está ganada pero todavía no tiene cuenta por cobrar.'),
        [boton('Crear la cuenta', { variante: 'principal', onclick: () => cambiar('ganado') })]));
    }
  }

  ctx = abrirModal({ titulo: 'Venta en proceso', contenido });
}

// ───────────── Conversión de venta ganada en cuenta ─────────────

export async function modalConversion(precarga, alTerminar) {
  const cfg = estado.config();

  // El cliente de la venta viene preseleccionado; se puede cambiar buscando otro.
  const original = {
    id: precarga.cliente_id, nombre: precarga.cliente_nombre,
    plazo_credito_dias: precarga.plazo_credito_dias, limite_credito_centavos: precarga.limite_credito_centavos,
  };
  const cliente = buscadorDeClientes('Cliente', original,
    'Puede emitir la cuenta a otro cliente; la venta quedará asociada a ese cliente.');
  const monto = campoMonto('Monto de la cuenta', { centavos: precarga.monto_sugerido_centavos });
  const emision = campoFecha('Fecha de emisión', { valor: precarga.fecha_emision });
  const vencimiento = campoFecha('Fecha de vencimiento', { valor: precarga.fecha_vencimiento, ayuda: ' ' });
  const ayudaVencimiento = vencimiento.elemento.querySelector('.campo-ayuda');
  const limite = h('div');

  // Solo se envía el vencimiento si el usuario lo cambió: el cálculo lo hace el backend con el
  // plazo del cliente elegido. El navegador nunca calcula fechas.
  let vencimientoEditado = false;
  vencimiento.control.addEventListener('input', () => { vencimientoEditado = true; });

  const actualizar = () => {
    const elegido = cliente.elegido() ?? original;
    const comoPrecarga = elegido.id === original.id && emision.valor() === precarga.fecha_emision;
    if (!vencimientoEditado) vencimiento.control.value = comoPrecarga ? precarga.fecha_vencimiento : '';
    ayudaVencimiento.textContent = vencimientoEditado
      ? 'Fecha elegida manualmente.'
      : `${comoPrecarga ? 'Calculada' : 'Se calculará al guardar'} con el plazo de ${elegido.nombre} ` +
        `(${formato.plazo(elegido.plazo_credito_dias)}). También puede elegirla a mano.`;
    limite.replaceChildren(elegido.limite_credito_centavos > 0
      ? aviso('informativo', h('p', {}, `Límite de crédito de ${elegido.nombre}: ${formato.dinero(elegido.limite_credito_centavos)}. ` +
        (cfg.exigir_limite_credito
          ? 'Si la venta lo supera, no se podrá registrar.'
          : 'Si la venta lo supera, se registrará con una advertencia.')))
      : '');
  };
  cliente.control.addEventListener('change', actualizar);
  emision.control.addEventListener('change', actualizar);
  vencimiento.control.addEventListener('change', actualizar);
  actualizar();

  abrirModal({
    titulo: 'Crear cuenta por cobrar',
    contenido: [limite, cliente.elemento, monto.elemento, emision.elemento, vencimiento.elemento],
    textoConfirmar: 'Crear cuenta',
    validar: () => montoValido(monto) && emision.valor() !== '',
    async alConfirmar(ctx) {
      let r;
      try {
        r = await api.convertirOportunidad(precarga.oportunidad_id, {
          cliente_id: cliente.valor() !== original.id ? cliente.valor() : undefined,
          monto_total_centavos: monto.centavos(),
          fecha_emision: emision.valor(),
          fecha_vencimiento: vencimientoEditado && vencimiento.valor() ? vencimiento.valor() : undefined,
        });
      } catch (error) {
        if (error.codigo !== 'LIMITE_CREDITO_EXCEDIDO') throw error;
        ctx.mensaje(avisoError(error, [detalleExceso(error.detalle)]));
        return;
      }
      estado.invalidar();
      ctx.cerrar();
      alTerminar?.();
      const verCuenta = () => router.ir('cuentas', r.cuenta.id);
      if (r.advertencia) {
        // La operación SÍ se completó: aviso amarillo, no error.
        const resultado = abrirModal({
          titulo: 'Cuenta creada con advertencia',
          contenido: [aviso('advertencia', [h('p', {}, r.advertencia.mensaje), detalleExceso(r.advertencia)],
            [boton('Ver la cuenta', { variante: 'principal', onclick: () => { resultado.cerrar(); verCuenta(); } })])],
        });
      } else {
        notificar('Cuenta por cobrar creada.');
        verCuenta();
      }
    },
  });
}

// ───────────── Reasignar cartera ─────────────

export async function modalReasignarCartera(alTerminar) {
  const { usuarios } = await estado.enCache('usuarios', api.usuarios);
  const opcion = (u) => ({ valor: u.id, texto: `${u.nombre} (${formato.rol(u.rol)})` });
  const origen = campoSeleccion('Transferir la cartera de', usuarios.map(opcion), { vacia: 'Elija quién entrega…' });
  const destino = campoSeleccion('A', usuarios.filter((u) => u.activo).map(opcion), { vacia: 'Elija quién recibe…' });
  const motivo = campoMotivo('Motivo', estado.motivoMinimo());

  abrirModal({
    titulo: 'Reasignar cartera',
    contenido: [
      aviso('informativo', h('p', {}, 'Se transfieren todos los clientes, las ventas en proceso abiertas y las cuentas con saldo. ' +
        'Las cuentas ya pagadas conservan su vendedor original.')),
      origen.elemento, destino.elemento, motivo.elemento,
    ],
    textoConfirmar: 'Reasignar',
    validar: () => origen.valor() !== '' && destino.valor() !== '' && origen.valor() !== destino.valor() && motivo.valido(),
    async alConfirmar(ctx) {
      const r = await api.reasignarCartera(origen.valor(), destino.valor(), motivo.valor());
      estado.invalidar();
      ctx.cerrar();
      notificar(r.mensaje);
      alTerminar?.(r);
    },
  });
}

// ───────────── Confirmación simple (aprobar / rechazar ajustes) ─────────────

export function modalConfirmar({ titulo, texto, textoConfirmar, variante, accion }) {
  abrirModal({
    titulo,
    contenido: [h('p', {}, texto)],
    textoConfirmar,
    variante,
    async alConfirmar(ctx) {
      await accion();
      ctx.cerrar();
    },
  });
}

// ───────────── Cobrar a un cliente (una sola puerta de entrada en la calle) ─────────────

/**
 * El cobrador toca "Registrar pago" en la ficha del cliente, no en cada factura. Si el cliente
 * debe una sola, se va directo a registrar el pago; si debe varias, primero elige a cuál se lo
 * aplica, que es la pregunta que de todos modos le hace al cliente en la puerta.
 */
export function modalCobrarACliente(cliente, cuentasAbiertas, alTerminar) {
  const abiertas = cuentasAbiertas ?? [];
  if (abiertas.length === 0) {
    abrirModal({
      titulo: `Cobrar a ${cliente.nombre}`,
      contenido: [aviso('informativo', h('p', {}, 'Este cliente no tiene facturas pendientes.'))],
    });
    return;
  }
  if (abiertas.length === 1) {
    modalPago(abiertas[0], alTerminar);
    return;
  }

  const ctx = abrirModal({
    titulo: `Cobrar a ${cliente.nombre}`,
    contenido: [
      h('p', {}, `Debe ${formato.dinero(cliente.saldo_centavos)} en ${abiertas.length} facturas. ` +
        '¿A cuál le aplica el pago?'),
      h('div', { class: 'lista' }, abiertas.map((c) => h('button', {
        type: 'button',
        class: `lista-fila lista-fila-elegible estado-${c.estado}`,
        onclick: () => { ctx.cerrar(); modalPago(c, alTerminar); },
      }, [
        h('span', { class: 'lista-fila-principal' }, [
          h('span', {}, c.numero_documento || 'Sin número de documento'),
          h('span', { class: 'texto-secundario' }, formato.vencimiento(c)),
        ]),
        h('span', { class: 'monto dato-valor' }, formato.dinero(c.saldo_centavos)),
      ]))),
    ],
  });
}
