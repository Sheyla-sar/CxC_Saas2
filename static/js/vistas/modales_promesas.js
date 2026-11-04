// vistas/modales_promesas.js — formularios de la promesa de pago (fase 4): anotar lo que el
// cliente se comprometió a pagar y anular una promesa mal anotada.
// Misma estructura que los demás modales: abrirModal, y los mensajes del backend se muestran tal
// cual. Aquí no se calcula ningún estado: el backend dice si la promesa está pendiente, cumplida
// o rota, y esta pantalla solo lo muestra.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import {
  h, aviso, avisoError, notificar, abrirModal, dato,
  campoTexto, campoSeleccion, campoFecha, campoMonto, campoMotivo,
} from '../componentes.js';
import { modalCobrarACliente, modalPago } from './modales.js';

/**
 * Anotar una promesa de pago.
 * `cuentasAbiertas` es opcional: si el cliente debe varias facturas, el cobrador puede decir a
 * cuál se refirió. Sin elegir ninguna, la promesa es del cliente y la cumple cualquier abono suyo.
 */
export function modalAnotarPromesa(cliente, cuentasAbiertas, alTerminar) {
  const abiertas = cuentasAbiertas ?? [];
  const fecha = campoFecha('¿Qué día va a pagar?', {
    ayuda: 'El día que le dijo el cliente. No puede ser anterior a hoy.',
  });
  const monto = campoMonto('¿Cuánto prometió? (opcional)', {
    ayuda: 'Déjelo vacío si no dijo un monto: entonces cualquier abono cumple la promesa.',
  });
  const cuenta = abiertas.length
    ? campoSeleccion('¿Por cuál factura? (opcional)', abiertas.map((c) => ({
      valor: c.id,
      texto: `${c.numero_documento || 'Sin número de documento'} · ${formato.dinero(c.saldo_centavos)}`,
    })), { vacia: 'Cualquier factura del cliente' })
    : null;
  const nota = campoTexto('Qué dijo el cliente (opcional)');

  abrirModal({
    titulo: `Anotar promesa de ${cliente.nombre}`,
    contenido: [
      aviso('informativo', h('p', {}, 'Queda registrado lo que el cliente se comprometió a pagar. ' +
        'El día prometido aparecerá en su agenda y, si no cumple, quedará constancia.')),
      fecha.elemento,
      monto.elemento,
      cuenta ? cuenta.elemento : null,
      nota.elemento,
    ],
    textoConfirmar: 'Anotar promesa',
    validar: () => fecha.valor() !== '' && (monto.vacio() || (monto.centavos() ?? 0) > 0),
    async alConfirmar(ctx) {
      const p = await api.crearPromesa(cliente.id, {
        fecha_promesa: fecha.valor(),
        monto_centavos: monto.vacio() ? undefined : monto.centavos(),
        cuenta_id: cuenta?.valor() || undefined,
        nota: nota.valor(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(p.monto_centavos
        ? `Promesa anotada: ${formato.dinero(p.monto_centavos)} para el ${formato.fecha(p.fecha_promesa)}.`
        : `Promesa anotada para el ${formato.fecha(p.fecha_promesa)}.`);
      alTerminar?.(p);
    },
  });
}

/** Anular una promesa mal anotada. No se borra: queda en el historial con su motivo. */
export function modalAnularPromesa(promesa, alTerminar) {
  const motivo = campoMotivo('Motivo de la anulación', estado.motivoMinimo());
  abrirModal({
    titulo: `Anular la promesa de ${promesa.cliente_nombre}`,
    contenido: [
      h('div', { class: 'rejilla-datos' }, [
        dato('Prometió', promesa.monto_centavos ? formato.dinero(promesa.monto_centavos) : 'Sin monto'),
        dato('Para el', formato.fecha(promesa.fecha_promesa)),
        dato('La anotó', promesa.registrado_por_nombre),
      ]),
      aviso('informativo', h('p', {}, 'La promesa deja de contar para la agenda y para el ' +
        'historial de cumplimiento, pero no se borra: queda con el motivo y con quién la anuló.')),
      motivo.elemento,
    ],
    textoConfirmar: 'Anular',
    variante: 'peligro',
    validar: motivo.valido,
    async alConfirmar(ctx) {
      await api.anularPromesa(promesa.id, motivo.valor());
      estado.invalidar();
      ctx.cerrar();
      notificar('Promesa anulada.');
      alTerminar?.();
    },
  });
}

/**
 * Cobrar desde la agenda: el cobrador llegó a la puerta y el cliente va a pagar.
 * Se piden al backend las cuentas abiertas del cliente (la agenda manda la promesa, no la
 * cartera). Si la promesa era por una factura concreta, se abre directamente esa.
 */
export async function modalCobrarPromesa(promesa, alTerminar) {
  try {
    const r = await api.cliente(promesa.cliente_id);
    const abiertas = r.cuentas.filter((c) => c.estado !== 'pagado');
    const laPrometida = abiertas.find((c) => c.id === promesa.cuenta_id);
    if (laPrometida) {
      modalPago(laPrometida, alTerminar);
      return;
    }
    modalCobrarACliente({ ...r.cliente, saldo_centavos: r.saldo_centavos }, abiertas, alTerminar);
  } catch (error) {
    // Se entra por un botón, no por una navegación: el error se muestra donde el usuario está.
    abrirModal({ titulo: `Cobrar a ${promesa.cliente_nombre}`, contenido: [avisoError(error)] });
  }
}
