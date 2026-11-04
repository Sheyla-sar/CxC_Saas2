// router.js — navegación por hash (#/cuentas/123). No sabe nada de pantallas ni de permisos:
// solo traduce el hash a { nombre, parametros } y avisa cuando cambia.

const rutas = [];

/** Registra un patrón como "cuentas/:id" bajo un nombre de ruta. */
export function definir(nombre, patron) {
  const partes = patron.split('/').filter(Boolean);
  rutas.push({ nombre, partes });
}

export function actual() {
  const ruta = location.hash.replace(/^#\/?/, '').split('?')[0];
  const segmentos = ruta.split('/').filter(Boolean).map(decodeURIComponent);
  for (const r of rutas) {
    if (r.partes.length !== segmentos.length) continue;
    const parametros = {};
    const coincide = r.partes.every((parte, i) => {
      if (parte.startsWith(':')) {
        parametros[parte.slice(1)] = segmentos[i];
        return true;
      }
      return parte === segmentos[i];
    });
    if (coincide) return { nombre: r.nombre, parametros };
  }
  return { nombre: null, parametros: {} };
}

export function enlace(...segmentos) {
  return `#/${segmentos.map((s) => encodeURIComponent(s)).join('/')}`;
}

export function ir(...segmentos) {
  const destino = enlace(...segmentos);
  if (location.hash === destino) {
    window.dispatchEvent(new HashChangeEvent('hashchange'));
  } else {
    location.hash = destino;
  }
}

export function alCambiar(fn) {
  window.addEventListener('hashchange', fn);
}
