#!/usr/bin/env bash
# demostracion.sh — levanta DOS instancias de la aplicación contra la MISMA base de datos, para
# poder tener abiertas al mismo tiempo la pantalla del dueño y la del cobrador y ver cómo lo que
# hace uno le aparece al otro.
#
#   bash demostracion.sh          # 8080 = dueño, 8081 = cobrador
#   bash demostracion.sh 9000     # 9000 y 9001
#
# Por qué dos instancias y no dos pestañas: la sesión vive en la memoria del proceso y la cookie
# es por origen, así que dos pestañas del mismo puerto comparten la sesión y una cierra a la otra.
# Dos puertos son dos orígenes: dos sesiones independientes, cada una con su usuario.
#
# Las dos escriben en la misma base: un cobro registrado en la del cobrador se ve, recargando, en
# la del dueño. Eso es justamente lo que se quiere mostrar.

set -uo pipefail

PUERTO_DUENO="${1:-8080}"
PUERTO_COBRADOR=$((PUERTO_DUENO + 1))
REGISTROS="${REGISTROS:-registros}"

cd "$(dirname "$0")"
mkdir -p "$REGISTROS"

echo "Compilando…"
if ! go build -o cxc.exe .; then
    echo "No compila. Corregí eso antes de la demostración." >&2
    exit 1
fi

# Si quedaron instancias de una demostración anterior, se cierran: dos procesos en el mismo puerto
# fallan de una forma difícil de diagnosticar en medio de una reunión.
taskkill //F //IM cxc.exe > /dev/null 2>&1
sleep 2

esperar() { # esperar <puerto> <nombre>
    for _ in $(seq 1 40); do
        if curl -s -o /dev/null "http://localhost:$1/api/salud"; then
            echo "  $2 listo en http://localhost:$1"
            return 0
        fi
        sleep 1
    done
    echo "  $2 NO respondió en el puerto $1. Mirá $REGISTROS/$2.log" >&2
    return 1
}

# La primera instancia aplica las migraciones; la segunda arranca después, cuando ya no hay nada
# que migrar. Arrancarlas a la vez haría que las dos intenten migrar al mismo tiempo.
PUERTO="$PUERTO_DUENO" ./cxc.exe > "$REGISTROS/dueno.log" 2>&1 &
esperar "$PUERTO_DUENO" dueno || exit 1

PUERTO="$PUERTO_COBRADOR" ./cxc.exe > "$REGISTROS/cobrador.log" 2>&1 &
esperar "$PUERTO_COBRADOR" cobrador || exit 1

cat <<FIN

──────────────────────────────────────────────────────────────────────
  Dos ventanas, lado a lado:

  DUEÑO     http://localhost:$PUERTO_DUENO
            admin@progreso.sv / admin123      (empresa: distribuidora-progreso)

  COBRADOR  http://localhost:$PUERTO_COBRADOR
            carlos@progreso.sv / vendedor123  (empresa: distribuidora-progreso)

  Para la ventana del cobrador, angostá el navegador a ~400 px de ancho:
  la aplicación se acomoda sola a tamaño de teléfono.

  Las dos trabajan sobre la misma base. Lo que registra el cobrador le
  aparece al dueño al recargar.
──────────────────────────────────────────────────────────────────────
FIN
