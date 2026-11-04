// vistas/alertas.js — cuentas que requieren aviso hoy, ordenadas por urgencia.
// Cada fila lleva al detalle de la cuenta y ofrece mandarle el recordatorio al cliente.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import * as router from '../router.js';
import { h, boton, monto, vacio, pildoraEstado, encabezadoPantalla, aviso, avisoError } from '../componentes.js';

// Urgencia: primero las vencidas (las más antiguas arriba), luego las por vencer (las más próximas arriba).
// Solo ordena usando los días que calcula el backend.
function porUrgencia(a, b) {
  if (a.tipo !== b.tipo) return a.tipo === 'vencido' ? -1 : 1;
  return a.tipo === 'vencido' ? b.dias_vencida - a.dias_vencida : a.dias_para_vencer - b.dias_para_vencer;
}

export async function render(contenedor) {
  const { alertas } = await api.alertas();
  alertas.sort(porUrgencia);

  const zonaAviso = h('div');

  // El texto del mensaje lo redacta el backend (ahí viven el formato del dinero, las etiquetas y
  // la zona horaria): acá solo se abre el enlace. El sistema no manda nada por su cuenta; lo manda
  // la persona desde su propio WhatsApp.
  async function mandarPorWhatsApp(botonFila, alerta) {
    zonaAviso.replaceChildren();
    botonFila.disabled = true;
    try {
      const { url } = await api.mensajeDeCobro(alerta.cliente_id);
      // Si el navegador bloquea la ventana emergente, se ofrece el enlace para tocarlo a mano.
      if (!window.open(url, '_blank')) {
        zonaAviso.replaceChildren(aviso('informativo', [
          h('p', {}, `El navegador bloqueó la ventana de WhatsApp para ${alerta.cliente_nombre}.`),
          h('a', { href: url, target: '_blank', rel: 'noopener' }, 'Abrir el mensaje en WhatsApp'),
        ]));
      }
    } catch (error) {
      // El mensaje del backend ya viene escrito para el usuario (por ejemplo, el cliente sin teléfono).
      zonaAviso.replaceChildren(avisoError(error));
    } finally {
      botonFila.disabled = false;
    }
  }

  const filas = alertas.map((a) => {
    const botonWhatsApp = boton('Mandar por WhatsApp', {
      variante: 'principal',
      icono: 'alertas',
      etiqueta: `Mandar por WhatsApp a ${a.cliente_nombre}`,
      onclick: () => mandarPorWhatsApp(botonWhatsApp, a),
    });
    return h('div', { class: `lista-fila estado-${a.tipo}` }, [
      pildoraEstado(a.tipo),
      h('a', { class: 'lista-fila-principal', href: router.enlace('cuentas', a.cuenta_id) }, [
        h('strong', {}, a.cliente_nombre),
        h('span', { class: 'texto-secundario' }, [a.cliente_contacto, a.cliente_telefono].filter(Boolean).join(' · ')),
        h('span', { class: 'texto-secundario' }, formato.vencimiento({
          estado: a.tipo, dias_vencida: a.dias_vencida, dias_para_vencer: a.dias_para_vencer, fecha_vencimiento: a.fecha_vencimiento,
        })),
      ]),
      a.entregada_hoy ? h('span', { class: 'pildora marca-entregada' }, 'Aviso enviado hoy') : null,
      monto(a.saldo_centavos, 'monto dato-valor'),
      botonWhatsApp,
    ]);
  });

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Alertas'),
    h('p', { class: 'texto-secundario' }, formato.avisoPrevio(estado.config().dias_aviso_previo)),
    zonaAviso,
    filas.length ? h('div', { class: 'lista' }, filas) : vacio('No hay cuentas que requieran aviso hoy.'),
  ]));
}
