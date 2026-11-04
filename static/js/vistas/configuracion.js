// vistas/configuracion.js — lo que un administrador necesita para poner la empresa a trabajar:
// dar de alta a su gente e ingresar la cartera que hoy tiene en papel o en Excel.

import { api } from '../api.js';
import * as estado from '../estado.js';
import * as formato from '../formato.js';
import {
  h, boton, tarjeta, tabla, vacio, aviso, encabezadoPantalla, botonEnlace, dato,
} from '../componentes.js';
import {
  modalCrearUsuario, modalEditarUsuario, modalReiniciarPassword, modalCambiarMiPassword, modalImportarCartera,
} from './modales_cobranza.js';

function tablaUsuarios(usuarios, recargar) {
  return tabla([
    {
      titulo: 'Nombre',
      celda: (u) => h('div', {}, [
        h('div', {}, u.nombre),
        h('span', { class: 'texto-secundario' }, u.email),
      ]),
    },
    { titulo: 'Rol', celda: (u) => formato.rol(u.rol) },
    {
      titulo: 'Situación',
      celda: (u) => h('span', { class: `pildora ${u.activo ? 'estado-al_dia' : 'estado-pagado'}` }, u.activo ? 'Activo' : 'Desactivado'),
    },
    {
      titulo: '',
      celda: (u) => h('div', { class: 'pantalla-acciones' }, [
        boton('Editar', { onclick: () => modalEditarUsuario(u, recargar) }),
        boton('Cambiar contraseña', { onclick: () => modalReiniciarPassword(u, recargar) }),
      ]),
    },
  ], usuarios);
}

export async function render(contenedor) {
  const { usuarios } = await api.usuarios();
  const recargar = () => render(contenedor);
  const yo = estado.usuario();

  const seccionUsuarios = h('section', { class: 'pantalla' }, [
    h('div', { class: 'tarjeta-seccion-titulo' }, [
      h('h2', {}, 'Usuarios'),
      boton('Nuevo usuario', { variante: 'principal', icono: 'agregar', onclick: () => modalCrearUsuario(recargar) }),
    ]),
    usuarios.length ? tablaUsuarios(usuarios, recargar) : vacio('Todavía no hay usuarios.'),
    h('p', { class: 'texto-secundario' },
      'Los usuarios no se borran: se desactivan. Un usuario desactivado no puede ingresar, pero su historial se conserva. ' +
      'La empresa siempre debe conservar al menos un administrador activo.'),
  ]);

  const seccionImportar = h('section', { class: 'pantalla' }, [
    h('h2', {}, 'Importar cartera'),
    tarjeta([
      h('p', {}, 'Suba la cartera que hoy lleva en Excel o en papel. Primero se revisa el archivo y se le muestra ' +
        'fila por fila qué se va a crear y qué tiene errores; nada se guarda hasta que usted confirme.'),
      h('div', { class: 'pantalla-acciones' }, [
        botonEnlace('Descargar plantilla', api.urlPlantillaImportacion(), { icono: 'descargar', descargar: 'plantilla-cartera.csv' }),
        boton('Subir archivo', { variante: 'principal', icono: 'importar', onclick: () => modalImportarCartera(recargar) }),
      ]),
      h('div', { class: 'rejilla-datos' }, [
        dato('Clientes', 'Se identifican por su código. Si el código ya existe, se usa ese cliente en vez de duplicarlo.'),
        dato('Documentos', 'Se identifican por su número. Un documento ya cargado se omite, no se duplica.'),
        dato('Abonos', 'Lo ya abonado entra como pago histórico para que el saldo quede igual al de su control actual.'),
      ]),
    ]),
  ]);

  const seccionCuenta = h('section', { class: 'pantalla' }, [
    h('h2', {}, 'Mi cuenta'),
    tarjeta([
      h('div', { class: 'rejilla-datos' }, [
        dato('Nombre', yo.nombre),
        dato('Correo', yo.email),
        dato('Rol', formato.rol(yo.rol)),
        dato('Empresa', estado.empresa().nombre_comercial),
      ]),
      h('div', { class: 'pantalla-acciones' }, [
        boton('Cambiar mi contraseña', { onclick: () => modalCambiarMiPassword() }),
      ]),
    ]),
  ]);

  contenedor.replaceChildren(h('div', { class: 'pantalla' }, [
    encabezadoPantalla('Configuración'),
    estado.puede('gestionar_usuarios') ? seccionUsuarios : aviso('informativo', h('p', {}, 'Solo un administrador puede gestionar usuarios.')),
    estado.puede('importar_cartera') ? seccionImportar : null,
    seccionCuenta,
  ]));
}
