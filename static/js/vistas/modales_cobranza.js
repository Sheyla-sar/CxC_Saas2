// vistas/modales_cobranza.js — formularios de la fase 3: notas de crédito, anticipos,
// liquidación del cobrador, usuarios, fotos adjuntas e importación de cartera.
// Misma estructura que modales.js: abrirModal, y los mensajes del backend se muestran tal cual.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import {
  h, aviso, boton, notificar, abrirModal, dato, tabla, monto as montoNodo, barras,
  campoTexto, campoSeleccion, campoFecha, campoMes, campoMonto, campoMotivo, campoArchivo,
  campoFirma, botonEnlace,
} from '../componentes.js';

const montoValido = (campo) => (campo.centavos() ?? 0) > 0;

function resumenCuenta(cuenta) {
  return h('div', { class: 'rejilla-datos' }, [
    dato('Cliente', cuenta.cliente_nombre),
    dato('Saldo pendiente', formato.dinero(cuenta.saldo_centavos)),
    dato('Documento', cuenta.numero_documento || '—'),
  ]);
}

// ───────────── Comprobante de un pago recién registrado ─────────────

/** Se abre apenas se registra un pago: imprimir el recibo numerado o guardarle la foto. */
export function modalPagoRegistrado({ movimiento, cuenta }) {
  const ctx = abrirModal({
    titulo: 'Pago registrado',
    contenido: [
      aviso('exito', h('p', {}, movimiento.numero
        ? `Recibo ${movimiento.numero} por ${formato.dinero(movimiento.monto_centavos)}.`
        : `Pago de ${formato.dinero(movimiento.monto_centavos)} registrado.`)),
      h('div', { class: 'rejilla-datos' }, [
        dato('Cliente', cuenta.cliente_nombre),
        dato('Forma de pago', formato.metodoPago(movimiento.metodo)),
        dato('Saldo pendiente', formato.dinero(cuenta.saldo_centavos)),
      ]),
      h('div', { class: 'pantalla-acciones' }, [
        botonEnlace('Imprimir recibo', api.urlRecibo(movimiento.id), { variante: 'principal', icono: 'documento' }),
        estado.puede('adjuntar')
          ? boton('Que firme el cliente', { icono: 'firma', onclick: () => { ctx.cerrar(); modalFirmarRecibo(movimiento); } })
          : null,
        estado.puede('adjuntar')
          ? boton('Adjuntar foto', { icono: 'foto', onclick: () => { ctx.cerrar(); modalSubirAdjunto('movimiento', movimiento.id); } })
          : null,
      ]),
    ],
  });
}

// ───────────── Firma del cliente en el recibo ─────────────

/**
 * El cobrador le pasa el teléfono al cliente, el cliente firma con el dedo y la firma queda
 * impresa dentro del recibo, en su espacio de firma. Al guardar se ofrece imprimirlo, que es
 * lo que sigue: el recibo ya sale firmado.
 */
export function modalFirmarRecibo(movimiento, alFirmar) {
  const firma = campoFirma('Firma del cliente', {
    ayuda: 'Firme con el dedo dentro del recuadro. Si se equivoca, borre y vuelva a firmar.',
  });

  abrirModal({
    titulo: 'Firma del cliente',
    contenido: [
      aviso('informativo', h('p', {}, 'Pásele el teléfono al cliente para que firme el recibo.')),
      firma.elemento,
    ],
    textoConfirmar: 'Guardar firma',
    validar: () => !firma.vacio(),
    async alConfirmar(ctx) {
      const formulario = new FormData();
      formulario.append('archivo', await firma.png(), 'firma.png');
      await api.guardarFirma(movimiento.id, formulario);
      ctx.cerrar();
      modalReciboFirmado(movimiento);
      alFirmar?.();
    },
  });
}

/** Confirmación con la única acción que sigue: imprimir el recibo, que ya sale firmado. */
function modalReciboFirmado(movimiento) {
  abrirModal({
    titulo: 'Recibo firmado',
    contenido: [
      aviso('exito', h('p', {}, 'El cliente firmó. La firma sale impresa en el recibo.')),
      h('div', { class: 'pantalla-acciones' }, [
        botonEnlace('Imprimir recibo', api.urlRecibo(movimiento.id), { variante: 'principal', icono: 'documento' }),
      ]),
    ],
  });
}

// ───────────── Nota de crédito ─────────────

export function modalNotaCredito(cuenta, alTerminar) {
  const monto = campoMonto('Monto de la nota de crédito', {
    placeholder: formato.montoParaCampo(cuenta.saldo_centavos),
    ayuda: `Déjelo vacío para aplicar todo el saldo (${formato.dinero(cuenta.saldo_centavos)}).`,
  });
  const motivo = campoMotivo('Motivo (devolución, descuento, error de facturación)', estado.motivoMinimo());
  const fecha = campoFecha('Fecha', { ayuda: 'Déjela vacía para usar la fecha de hoy.' });

  abrirModal({
    titulo: 'Emitir nota de crédito',
    contenido: [
      resumenCuenta(cuenta),
      aviso('informativo', h('p', {}, 'La nota de crédito rebaja el saldo con su propio número de documento. ' +
        'No es dinero recibido y no cuenta para la liquidación del cobrador.')),
      monto.elemento, motivo.elemento, fecha.elemento,
    ],
    textoConfirmar: 'Emitir',
    validar: () => motivo.valido() && (monto.vacio() || montoValido(monto)),
    async alConfirmar(ctx) {
      const r = await api.notaCredito(cuenta.id, {
        monto_centavos: monto.vacio() ? undefined : monto.centavos(),
        motivo: motivo.valor(),
        fecha: fecha.valor() || undefined,
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Nota de crédito ${r.movimiento.numero ?? ''} emitida. Saldo pendiente: ${formato.dinero(r.cuenta.saldo_centavos)}`);
      alTerminar?.(r);
    },
  });
}

// ───────────── Anticipos (dinero a favor del cliente) ─────────────

export function modalCrearAnticipo(cliente, alTerminar) {
  const monto = campoMonto('Monto recibido');
  const metodos = estado.metodosPago();
  const metodo = campoSeleccion('Forma de pago', metodos.map((m) => ({ valor: m, texto: formato.metodoPago(m) })), { valor: metodos[0] });
  const referencia = campoTexto('Referencia');
  const fecha = campoFecha('Fecha', { ayuda: 'Déjela vacía para usar la fecha de hoy.' });
  const nota = campoTexto('Nota (opcional)');

  abrirModal({
    titulo: 'Registrar anticipo',
    contenido: [
      aviso('informativo', h('p', {}, `Dinero que ${cliente.nombre} entrega sin factura asignada. ` +
        'Queda como saldo a favor y después se aplica a las cuentas que usted elija.')),
      monto.elemento, metodo.elemento, referencia.elemento, fecha.elemento, nota.elemento,
    ],
    textoConfirmar: 'Registrar anticipo',
    validar: () => montoValido(monto),
    async alConfirmar(ctx) {
      const a = await api.crearAnticipo(cliente.id, {
        monto_centavos: monto.centavos(),
        metodo: metodo.valor(),
        referencia: referencia.valor(),
        fecha: fecha.valor() || undefined,
        nota: nota.valor(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Anticipo ${a.numero} por ${formato.dinero(a.monto_centavos)} registrado.`);
      alTerminar?.(a);
    },
  });
}

export function modalAplicarAnticipo(anticipo, cuentasAbiertas, alTerminar) {
  if (!cuentasAbiertas.length) {
    abrirModal({
      titulo: 'Aplicar anticipo',
      contenido: [aviso('informativo', h('p', {}, 'El cliente no tiene cuentas con saldo pendiente a las que aplicar el anticipo.'))],
    });
    return;
  }
  const cuenta = campoSeleccion('Aplicar a la cuenta',
    cuentasAbiertas.map((c) => ({
      valor: c.id,
      texto: `${c.numero_documento || 'Sin documento'} · vence ${formato.fecha(c.fecha_vencimiento)} · saldo ${formato.dinero(c.saldo_centavos)}`,
    })), { vacia: 'Elija una cuenta…' });
  const monto = campoMonto('Monto a aplicar', {
    ayuda: `Déjelo vacío para aplicar lo que alcance (disponible ${formato.dinero(anticipo.disponible_centavos)}).`,
  });

  abrirModal({
    titulo: `Aplicar anticipo ${anticipo.numero}`,
    contenido: [
      h('div', { class: 'rejilla-datos' }, [
        dato('Anticipo', formato.dinero(anticipo.monto_centavos)),
        dato('Ya aplicado', formato.dinero(anticipo.aplicado_centavos)),
        dato('Disponible', formato.dinero(anticipo.disponible_centavos)),
      ]),
      cuenta.elemento, monto.elemento,
    ],
    textoConfirmar: 'Aplicar',
    validar: () => cuenta.valor() !== '' && (monto.vacio() || montoValido(monto)),
    async alConfirmar(ctx) {
      const r = await api.aplicarAnticipo(anticipo.id, {
        cuenta_id: cuenta.valor(),
        monto_centavos: monto.vacio() ? undefined : monto.centavos(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Anticipo aplicado. Saldo de la cuenta: ${formato.dinero(r.cuenta.saldo_centavos)}`);
      alTerminar?.(r);
    },
  });
}

export function modalAnularAnticipo(anticipo, alTerminar) {
  const motivo = campoMotivo('Motivo de la anulación', estado.motivoMinimo());
  abrirModal({
    titulo: `Anular anticipo ${anticipo.numero}`,
    contenido: [
      aviso('informativo', h('p', {}, 'El anticipo deja de contar como saldo a favor, pero queda en el historial. ' +
        'Un anticipo que ya se aplicó a una cuenta no se puede anular.')),
      motivo.elemento,
    ],
    textoConfirmar: 'Anular',
    variante: 'peligro',
    validar: motivo.valido,
    async alConfirmar(ctx) {
      await api.anularAnticipo(anticipo.id, motivo.valor());
      estado.invalidar();
      ctx.cerrar();
      notificar('Anticipo anulado.');
      alTerminar?.();
    },
  });
}

// ───────────── Liquidación del cobrador ─────────────

export function modalCerrarLiquidacion(corte, alTerminar) {
  const entregado = campoMonto('Dinero que entrega', { centavos: corte.total_esperado_centavos });
  const nota = campoMotivo('Explicación de la diferencia', estado.motivoMinimo(), { obligatorio: false });
  const resumenDiferencia = h('div');

  const actualizar = () => {
    const valor = entregado.centavos();
    if (valor === null) {
      resumenDiferencia.replaceChildren();
      return;
    }
    const diferencia = valor - corte.total_esperado_centavos;
    resumenDiferencia.replaceChildren(diferencia === 0
      ? aviso('exito', h('p', {}, 'El corte cuadra exactamente.'))
      : aviso('advertencia', h('p', {}, `${formato.diferenciaLiquidacion(diferencia)}. Explique por qué antes de cerrar.`)));
  };
  entregado.control.addEventListener('input', actualizar);
  actualizar();

  abrirModal({
    titulo: 'Cerrar y entregar',
    contenido: [
      h('div', { class: 'rejilla-datos' }, [
        dato('Cobrador', corte.cobrador_nombre),
        dato('Fecha', formato.fecha(corte.fecha)),
        dato('Cobros incluidos', String(corte.cantidad_movimientos)),
        dato('Debe entregar', formato.dinero(corte.total_esperado_centavos)),
      ]),
      corte.por_metodo.length
        ? barras(corte.por_metodo.map((m) => ({
          etiqueta: formato.metodoPago(m.metodo),
          valor: m.monto_centavos,
          texto: formato.dinero(m.monto_centavos),
        })))
        : null,
      entregado.elemento,
      resumenDiferencia,
      nota.elemento,
    ],
    textoConfirmar: 'Cerrar liquidación',
    validar: () => entregado.centavos() !== null,
    async alConfirmar(ctx) {
      const r = await api.cerrarLiquidacion({
        total_entregado_centavos: entregado.centavos(),
        nota: nota.valor(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Liquidación ${r.liquidacion.numero} cerrada. ${formato.diferenciaLiquidacion(r.liquidacion.diferencia_centavos)}.`);
      alTerminar?.(r);
    },
  });
}

export function modalAprobarLiquidacion(liquidacion, alTerminar) {
  abrirModal({
    titulo: `Aprobar ${liquidacion.numero}`,
    contenido: [
      h('div', { class: 'rejilla-datos' }, [
        dato('Cobrador', liquidacion.cobrador_nombre),
        dato('Esperado', formato.dinero(liquidacion.total_esperado_centavos)),
        dato('Entregado', formato.dinero(liquidacion.total_entregado_centavos)),
        dato('Diferencia', formato.diferenciaLiquidacion(liquidacion.diferencia_centavos)),
      ]),
      liquidacion.nota ? dato('Explicación del cobrador', liquidacion.nota) : null,
      aviso('informativo', h('p', {}, 'Al aprobar, el dinero queda como recibido y los cobros incluidos dejan de estar pendientes de entrega.')),
    ],
    textoConfirmar: 'Aprobar',
    variante: 'principal',
    async alConfirmar(ctx) {
      await api.aprobarLiquidacion(liquidacion.id);
      estado.invalidar();
      ctx.cerrar();
      notificar(`Liquidación ${liquidacion.numero} aprobada.`);
      alTerminar?.();
    },
  });
}

export function modalRechazarLiquidacion(liquidacion, alTerminar) {
  const motivo = campoMotivo('Motivo del rechazo', estado.motivoMinimo());
  abrirModal({
    titulo: `Rechazar ${liquidacion.numero}`,
    contenido: [
      aviso('advertencia', h('p', {}, 'Los cobros vuelven a quedar pendientes de entrega y aparecerán en el próximo corte del cobrador.')),
      motivo.elemento,
    ],
    textoConfirmar: 'Rechazar',
    variante: 'peligro',
    validar: motivo.valido,
    async alConfirmar(ctx) {
      await api.rechazarLiquidacion(liquidacion.id, motivo.valor());
      estado.invalidar();
      ctx.cerrar();
      notificar(`Liquidación ${liquidacion.numero} rechazada.`);
      alTerminar?.();
    },
  });
}

// ───────────── Usuarios ─────────────

const OPCIONES_ROL = [{ valor: 'vendedor', texto: formato.rol('vendedor') }, { valor: 'admin', texto: formato.rol('admin') }];
const LARGO_MINIMO_PASSWORD = 8;

export function modalCrearUsuario(alTerminar) {
  const nombre = campoTexto('Nombre', { requerido: true });
  const email = campoTexto('Correo', { tipo: 'email', requerido: true, autocompletar: 'off' });
  const rol = campoSeleccion('Rol', OPCIONES_ROL, { valor: 'vendedor' });
  const password = campoTexto('Contraseña inicial', { tipo: 'password', autocompletar: 'new-password' });

  abrirModal({
    titulo: 'Nuevo usuario',
    contenido: [
      nombre.elemento, email.elemento, rol.elemento, password.elemento,
      aviso('informativo', h('p', {}, 'El vendedor ve solo su cartera, cobra y liquida lo suyo. ' +
        'El administrador ve toda la empresa y además aprueba liquidaciones, ajustes y notas de crédito.')),
    ],
    textoConfirmar: 'Crear usuario',
    validar: () => nombre.valor() !== '' && email.valor() !== '' && password.valor().length >= LARGO_MINIMO_PASSWORD,
    async alConfirmar(ctx) {
      const u = await api.crearUsuario({
        nombre: nombre.valor(), email: email.valor(), rol: rol.valor(), password: password.valor(),
      });
      estado.invalidar();
      ctx.cerrar();
      notificar(`Usuario “${u.nombre}” creado.`);
      alTerminar?.(u);
    },
  });
}

export function modalEditarUsuario(usuario, alTerminar) {
  const nombre = campoTexto('Nombre', { valor: usuario.nombre, requerido: true });
  const rol = campoSeleccion('Rol', OPCIONES_ROL, { valor: usuario.rol });
  const activo = campoSeleccion('Situación', [
    { valor: 'si', texto: 'Activo' },
    { valor: 'no', texto: 'Desactivado' },
  ], { valor: usuario.activo ? 'si' : 'no', ayuda: 'Un usuario desactivado no puede ingresar, pero su historial se conserva.' });

  abrirModal({
    titulo: `Editar ${usuario.nombre}`,
    contenido: [dato('Correo', usuario.email), nombre.elemento, rol.elemento, activo.elemento],
    textoConfirmar: 'Guardar',
    validar: () => nombre.valor() !== '',
    async alConfirmar(ctx) {
      await api.actualizarUsuario(usuario.id, {
        nombre: nombre.valor(), rol: rol.valor(), activo: activo.valor() === 'si',
      });
      estado.invalidar();
      ctx.cerrar();
      notificar('Usuario actualizado.');
      alTerminar?.();
    },
  });
}

export function modalReiniciarPassword(usuario, alTerminar) {
  const password = campoTexto('Contraseña nueva', { tipo: 'password', autocompletar: 'new-password' });
  abrirModal({
    titulo: `Contraseña de ${usuario.nombre}`,
    contenido: [
      aviso('informativo', h('p', {}, 'Escriba la contraseña nueva y entréguesela al usuario. Él puede cambiarla después desde Configuración.')),
      password.elemento,
    ],
    textoConfirmar: 'Cambiar contraseña',
    validar: () => password.valor().length >= LARGO_MINIMO_PASSWORD,
    async alConfirmar(ctx) {
      await api.reiniciarPassword(usuario.id, password.valor());
      ctx.cerrar();
      notificar('Contraseña cambiada.');
      alTerminar?.();
    },
  });
}

export function modalCambiarMiPassword() {
  const actual = campoTexto('Contraseña actual', { tipo: 'password', autocompletar: 'current-password' });
  const nueva = campoTexto('Contraseña nueva', { tipo: 'password', autocompletar: 'new-password' });
  const repetir = campoTexto('Repita la contraseña nueva', { tipo: 'password', autocompletar: 'new-password' });
  const avisoIgual = h('div');

  abrirModal({
    titulo: 'Cambiar mi contraseña',
    contenido: [actual.elemento, nueva.elemento, repetir.elemento, avisoIgual],
    textoConfirmar: 'Cambiar',
    validar: () => {
      const coinciden = nueva.valor() === repetir.valor();
      avisoIgual.replaceChildren(!coinciden && repetir.valor() !== ''
        ? aviso('error', h('p', {}, 'Las dos contraseñas nuevas no coinciden.'))
        : '');
      return actual.valor() !== '' && nueva.valor().length >= LARGO_MINIMO_PASSWORD && coinciden;
    },
    async alConfirmar(ctx) {
      await api.cambiarMiPassword(actual.valor(), nueva.valor());
      ctx.cerrar();
      notificar('Su contraseña fue cambiada.');
    },
  });
}

// ───────────── Adjuntos (fotos de recibos y facturas) ─────────────

export function modalSubirAdjunto(entidad, entidadId, alTerminar) {
  const cfg = estado.config();
  const archivo = campoArchivo('Foto o PDF', {
    acepta: 'image/jpeg,image/png,application/pdf',
    camara: true,
    ayuda: `Tome la foto con la cámara del teléfono o elija un archivo. Hasta ${cfg.tamano_maximo_adjunto_mb} MB.`,
  });
  const titulo = campoTexto('Título', {
    requerido: true,
    ayuda: 'Cómo va a reconocerlo después: “Recibo firmado”, “Factura del proveedor”, ' +
      '“Comprobante de depósito”.',
  });
  const descripcion = campoTexto('Descripción (opcional)');

  abrirModal({
    titulo: 'Adjuntar comprobante',
    contenido: [archivo.elemento, titulo.elemento, descripcion.elemento],
    textoConfirmar: 'Subir',
    validar: () => !archivo.vacio() && titulo.valor() !== '',
    async alConfirmar(ctx) {
      const formulario = new FormData();
      formulario.append('entidad', entidad);
      formulario.append('entidad_id', entidadId);
      formulario.append('titulo', titulo.valor());
      formulario.append('descripcion', descripcion.valor());
      formulario.append('archivo', archivo.archivo());
      await api.subirAdjunto(formulario);
      ctx.cerrar();
      notificar('Comprobante adjuntado.');
      alTerminar?.();
    },
  });
}

export function modalEliminarAdjunto(adjunto, alTerminar) {
  abrirModal({
    titulo: 'Eliminar comprobante',
    contenido: [h('p', {}, `Se eliminará “${adjunto.titulo || adjunto.nombre}”. Esta acción no se puede deshacer.`)],
    textoConfirmar: 'Eliminar',
    variante: 'peligro',
    async alConfirmar(ctx) {
      await api.eliminarAdjunto(adjunto.id);
      ctx.cerrar();
      notificar('Comprobante eliminado.');
      alTerminar?.();
    },
  });
}

// ───────────── Importación de cartera ─────────────

function tablaRevision(filas) {
  return tabla([
    { titulo: 'Línea', celda: (f) => String(f.linea) },
    {
      titulo: 'Cliente',
      celda: (f) => h('div', {}, [
        h('div', {}, f.nombre_cliente || '—'),
        h('span', { class: 'texto-secundario' }, f.codigo_cliente + (f.cliente_nuevo ? ' · nuevo' : '')),
      ]),
    },
    { titulo: 'Documento', celda: (f) => f.numero_documento || '—' },
    { titulo: 'Vence', clase: 'columna-fecha', celda: (f) => formato.fecha(f.fecha_vencimiento) },
    { titulo: 'Monto', clase: 'columna-monto', celda: (f) => montoNodo(f.monto_centavos) },
    { titulo: 'Saldo', clase: 'columna-monto', celda: (f) => montoNodo(f.saldo_centavos) },
    {
      titulo: 'Revisión',
      celda: (f) => {
        if (f.errores?.length) return h('div', { class: 'errores-fila' }, f.errores.map((e) => h('span', {}, e)));
        if (f.omitida) return h('span', { class: 'texto-secundario' }, 'Ya cargado: se omite');
        return h('span', { class: 'pildora estado-al_dia' }, 'Lista');
      },
    },
  ], filas, { claseFila: (f) => (f.errores?.length ? 'fila-con-error' : (f.omitida ? 'fila-omitida' : '')) });
}

export function modalImportarCartera(alTerminar) {
  const archivo = campoArchivo('Archivo CSV', { acepta: '.csv,text/csv', ayuda: 'Use la plantilla para no equivocarse con el formato.' });
  const resultado = h('div', { class: 'pantalla' });
  const pie = h('div');
  let revisado = false;

  // El mismo archivo se sube dos veces: primero para revisar y, si el usuario acepta, para aplicar.
  // Así lo que se guarda es exactamente lo que se le mostró.
  const ctx = abrirModal({
    titulo: 'Importar cartera',
    contenido: [archivo.elemento, resultado, pie],
    textoConfirmar: 'Revisar archivo',
    validar: () => !archivo.vacio(),
    async alConfirmar(c) {
      const r = await api.importarCartera(archivo.archivo(), revisado);
      if (r.aplicada) {
        estado.invalidar();
        c.cerrar();
        notificar(`Importación aplicada: ${r.clientes_creados} cliente(s) y ${r.cuentas_creadas} cuenta(s).`);
        alTerminar?.(r);
        return;
      }
      revisado = true;
      resultado.replaceChildren(
        h('div', { class: 'rejilla-datos' }, [
          dato('Filas leídas', String(r.total_filas)),
          dato('Listas para cargar', String(r.filas_validas)),
          dato('Con error', String(r.filas_con_error)),
          dato('Ya cargadas', String(r.filas_omitidas)),
          dato('Clientes nuevos', String(r.clientes_nuevos)),
          dato('Cartera del archivo', formato.dinero(r.total_cartera_centavos)),
        ]),
        r.filas_con_error > 0
          ? aviso('advertencia', h('p', {}, 'Las filas con error no se van a cargar. Puede corregirlas en el archivo y subirlo de nuevo, ' +
            'o continuar y cargar solo las correctas.'))
          : null,
        r.filas.length ? tablaRevision(r.filas) : h('p', { class: 'texto-secundario' }, 'El archivo no tiene filas para cargar.'),
      );
      pie.replaceChildren(r.filas_validas > 0
        ? aviso('informativo', h('p', {}, `Al confirmar se cargarán ${r.filas_validas} fila(s).`))
        : aviso('error', h('p', {}, 'No hay ninguna fila que se pueda cargar. Corrija el archivo y vuelva a subirlo.')));
    },
  });
  return ctx;
}

// ───────────── Informe mensual del dueño ─────────────

/**
 * Elige el mes y abre el informe. `fechaHoy` llega del backend ("AAAA-MM-DD"): el navegador no
 * arma fechas, solo recorta el mes de la que ya recibió.
 */
export function modalInformeMensual(fechaHoy) {
  const mes = campoMes('Mes', {
    valor: String(fechaHoy ?? '').slice(0, 7),
    ayuda: 'El informe compara el mes elegido con el anterior. El mes en curso se informa hasta hoy.',
  });
  const enlace = h('div');

  const actualizar = () => {
    enlace.replaceChildren(mes.valor()
      ? botonEnlace('Abrir el informe', api.urlInformeMensual(mes.valor()), { variante: 'principal', icono: 'documento' })
      : '');
  };
  mes.control.addEventListener('change', actualizar);
  mes.control.addEventListener('input', actualizar);
  actualizar();

  abrirModal({
    titulo: 'Informe mensual',
    contenido: [
      aviso('informativo', h('p', {}, 'Un PDF con la cartera, la cobranza del mes, el desempeño de los cobradores ' +
        'y el saldo que se fue en notas de crédito. Pensado para el dueño, que no entra al sistema todos los días.')),
      mes.elemento,
      enlace,
    ],
  });
}

