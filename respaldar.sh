#!/usr/bin/env bash
# respaldar.sh — saca un respaldo de la base y de los archivos adjuntos.
#
#   ./respaldar.sh                    # deja el respaldo en ./respaldos
#   DESTINO=/mnt/discoB ./respaldar.sh
#
# Pensado para correr desde cron una vez al día:
#   0 2 * * * cd /opt/cxc && ./respaldar.sh >> /var/log/cxc-respaldo.log 2>&1
#
# Un respaldo que nunca se restauró no es un respaldo. Use restaurar.sh contra una base de
# prueba al menos una vez al mes; el procedimiento está en DESPLIEGUE.md.

set -euo pipefail

DESTINO="${DESTINO:-respaldos}"
RETENCION_DIAS="${RETENCION_DIAS:-30}"
CONTENEDOR="${CONTENEDOR:-cxc-postgres}"
BASE="${BASE:-cxc}"
USUARIO_BD="${USUARIO_BD:-postgres}"
RUTA_ARCHIVOS="${RUTA_ARCHIVOS:-archivos}"

marca=$(date +%Y%m%d-%H%M%S)
mkdir -p "$DESTINO"
archivo="$DESTINO/cxc-$marca.dump"
adjuntos="$DESTINO/archivos-$marca.tar.gz"

echo "[$(date '+%F %T')] respaldo iniciado"

# Formato "custom" (-Fc): comprimido y restaurable tabla por tabla con pg_restore.
docker exec "$CONTENEDOR" pg_dump -U "$USUARIO_BD" -d "$BASE" -Fc > "$archivo"
tamano=$(du -h "$archivo" | cut -f1)
echo "  base de datos  → $archivo ($tamano)"

# Los adjuntos viven en disco, no en la base: sin ellos, los recibos pierden sus fotos.
if [[ -d "$RUTA_ARCHIVOS" ]]; then
    tar -czf "$adjuntos" "$RUTA_ARCHIVOS"
    echo "  adjuntos       → $adjuntos ($(du -h "$adjuntos" | cut -f1))"
else
    echo "  adjuntos       → no hay carpeta $RUTA_ARCHIVOS, se omite"
fi

# Comprobación mínima: que el volcado se pueda leer. No reemplaza una restauración de verdad,
# pero detecta el caso más común, que es un archivo truncado.
if ! docker exec -i "$CONTENEDOR" pg_restore --list > /dev/null < "$archivo"; then
    echo "  ERROR: el respaldo no se puede leer. Revíselo antes de confiar en él." >&2
    exit 1
fi
echo "  verificado     → el volcado se lee correctamente"

# Rotación: se borran los respaldos más viejos que la retención configurada.
borrados=$(find "$DESTINO" -maxdepth 1 -name 'cxc-*.dump' -mtime +"$RETENCION_DIAS" -print -delete | wc -l)
find "$DESTINO" -maxdepth 1 -name 'archivos-*.tar.gz' -mtime +"$RETENCION_DIAS" -delete
echo "  rotación       → $borrados respaldo(s) de más de $RETENCION_DIAS días"

echo "[$(date '+%F %T')] respaldo terminado"
