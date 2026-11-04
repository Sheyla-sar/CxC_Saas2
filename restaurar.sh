#!/usr/bin/env bash
# restaurar.sh — restaura un respaldo. Por defecto lo hace sobre una base de PRUEBA, para que
# verificar que el respaldo sirve no ponga en riesgo la base real.
#
#   ./restaurar.sh respaldos/cxc-20261015-020000.dump              # restaura en la base cxc_prueba
#   BASE_DESTINO=cxc CONFIRMAR=SI ./restaurar.sh respaldos/x.dump  # restaura sobre la base REAL
#
# Restaurar sobre la base real BORRA lo que haya en ella. Por eso exige CONFIRMAR=SI escrito a
# mano: no hay forma de hacerlo por accidente.

set -euo pipefail

archivo="${1:-}"
BASE_DESTINO="${BASE_DESTINO:-cxc_prueba}"
CONTENEDOR="${CONTENEDOR:-cxc-postgres}"
USUARIO_BD="${USUARIO_BD:-postgres}"
# Rol con el que se conecta la aplicación. La base restaurada tiene que ser SUYA: así el esquema
# público le pertenece y puede crear y, sobre todo, el RLS forzado sigue aplicándose sobre sus
# propias tablas. Si la base quedara a nombre del superusuario, la aplicación no arrancaría.
ROL_APP="${ROL_APP:-cxc_app}"

if [[ -z "$archivo" || ! -f "$archivo" ]]; then
    echo "Uso: ./restaurar.sh <archivo.dump>" >&2
    echo "Respaldos disponibles:" >&2
    ls -1t respaldos/*.dump 2>/dev/null | head -5 >&2 || echo "  (ninguno)" >&2
    exit 2
fi

if [[ "$BASE_DESTINO" != *prueba* && "${CONFIRMAR:-}" != "SI" ]]; then
    echo "Va a restaurar sobre la base \"$BASE_DESTINO\", que NO es de prueba." >&2
    echo "Todo lo que haya en ella se pierde. Si está seguro:" >&2
    echo "  BASE_DESTINO=$BASE_DESTINO CONFIRMAR=SI $0 $archivo" >&2
    exit 2
fi

echo "[$(date '+%F %T')] restaurando $archivo en la base $BASE_DESTINO"

docker exec "$CONTENEDOR" psql -U "$USUARIO_BD" -d postgres -c "DROP DATABASE IF EXISTS $BASE_DESTINO;" > /dev/null
docker exec "$CONTENEDOR" psql -U "$USUARIO_BD" -d postgres -c "CREATE DATABASE $BASE_DESTINO OWNER $ROL_APP;" > /dev/null
# Se restaura CONSERVANDO el dueño de cada tabla. Con --no-owner las tablas quedarían a nombre
# del superusuario y la aplicación, que se conecta con un rol sin privilegios, no podría usarlas:
# el aislamiento por RLS depende de que las tablas sean suyas (FORCE ROW LEVEL SECURITY).
docker exec -i "$CONTENEDOR" pg_restore -U "$USUARIO_BD" -d "$BASE_DESTINO" < "$archivo"

# Si el rol de la aplicación no existe (restauración en otro servidor), las tablas no tendrían
# dueño válido y pg_restore ya habría fallado; este aviso explica qué hacer.
duenos=$(docker exec "$CONTENEDOR" psql -U "$USUARIO_BD" -d "$BASE_DESTINO" -t -A -c     "SELECT COUNT(DISTINCT tableowner) FROM pg_tables WHERE schemaname='public';")
echo "  dueños de las tablas: $duenos (debe ser 1: el rol de la aplicación)"

echo "  restaurado. Conteo de la base restaurada:"
docker exec "$CONTENEDOR" psql -U "$USUARIO_BD" -d "$BASE_DESTINO" -t -c "
    SELECT '    empresas: ' || (SELECT COUNT(*) FROM empresa)
        || ', clientes: ' || (SELECT COUNT(*) FROM cliente)
        || ', cuentas: '  || (SELECT COUNT(*) FROM cuenta)
        || ', pagos: '    || (SELECT COUNT(*) FROM pago);"

echo "[$(date '+%F %T')] listo"
echo
echo "Si restauró en una base de prueba, apunte la aplicación a ella para revisarla:"
echo "  DATABASE_URL=\"postgres://cxc_app:...@localhost:5433/$BASE_DESTINO?sslmode=disable\" ./cxc"
