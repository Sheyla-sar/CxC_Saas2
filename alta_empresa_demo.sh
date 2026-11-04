#!/usr/bin/env bash
# alta_empresa_demo.sh — da de alta una TERCERA empresa en la misma base y la deja usable, para
# mostrar el aislamiento multi-tenant con datos propios y una configuración distinta.
#
#   docker compose up -d
#   ENTORNO=dev go run .            (en otra terminal)
#   bash alta_empresa_demo.sh
#
# Hace exactamente lo que haría el alta de un cliente real del SaaS, sin tocar código ni
# recompilar (§1 de CLAUDE.md):
#
#   1. `go run . crear-empresa` crea la empresa, su marca, su configuración y su administrador.
#   2. El administrador entra a la API y ajusta SU marca y SU configuración (PATCH).
#   3. Crea a su cobrador desde la propia aplicación (R48).
#   4. Importa su cartera desde un CSV con la plantilla del sistema (R71-R74), en dos pasadas.
#   5. El cobrador registra cobros del día y anota una promesa, para que el tablero tenga vida.
#
# Es idempotente en lo importante: si la empresa ya existe, avisa y sigue con los pasos 2 a 5, y
# reimportar el mismo CSV no duplica clientes ni cuentas (R73). Lo que sí se repite en cada corrida
# son los dos cobros y la promesa del paso 5. Para empezar de cero:
# `docker compose down -v && docker compose up -d` y volver a correrlo.

set -u
API="${API:-http://localhost:8080/api}"
SLUG="${SLUG:-lacteos-la-cumbre}"
ADMIN_EMAIL="admin@lacumbre.sv"
ADMIN_PASS="admin123"
COBRADOR_EMAIL="oscar@lacumbre.sv"
COBRADOR_PASS="cobrador123"

# La carpeta temporal va DENTRO del proyecto y con ruta relativa: en Windows, curl.exe no entiende
# las rutas estilo /tmp de Git Bash.
TMP="$(mktemp -d ./.alta-empresa.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

azul() { printf '\n\033[1;34m%s\033[0m\n' "$1"; }
ok()   { printf '  \033[0;32m✔\033[0m %s\n' "$1"; }
mal()  { printf '  \033[0;31m✘\033[0m %s\n       %s\n' "$1" "${2:0:240}"; }
# paso <descripción> <texto esperado en la respuesta> <respuesta>
paso() { if [[ "$3" == *"$2"* ]]; then ok "$1"; else mal "$1" "$3"; FALLO=1; fi; }
FALLO=0

texto() { sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" <<< "$1" | head -1; }
ids()   { grep -o '"id":"[^"]*"' <<< "$1" | cut -d'"' -f4; }
# idPorCorreo <json de /usuarios> <correo> — el orden del listado es por nombre, así que no se
# puede tomar "el último": se busca el objeto que lleva ese correo.
idPorCorreo() { grep -o '{[^{}]*}' <<< "$1" | grep -F "\"email\":\"$2\"" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4; }

# Los cuerpos JSON se mandan desde archivo: en Windows, curl recibe los acentos de la línea de
# comandos en la página de códigos del sistema y rompería el UTF-8.
envia() { printf '%s' "$4" > "$TMP/cuerpo.json"; curl -s -b "$TMP/$1" -H 'Content-Type: application/json' -X "$2" "$API$3" --data-binary "@$TMP/cuerpo.json"; }
jget()  { curl -s -b "$TMP/$1" "$API$2"; }

if ! curl -s -f "$API/salud" > /dev/null; then
  printf 'La aplicación no responde en %s.\n  docker compose up -d\n  ENTORNO=dev go run .\n' "$API"
  exit 1
fi

# ─────────────────────────────────────────────────────────────────────────────
azul "1. Alta de la empresa (go run . crear-empresa)"
# ─────────────────────────────────────────────────────────────────────────────
salida=$(go run . crear-empresa \
  --slug "$SLUG" \
  --nombre-legal "Distribuidora de Lácteos La Cumbre, S.A. de C.V." \
  --nombre-comercial "Lácteos La Cumbre" \
  --admin-nombre "Silvia Menjívar" \
  --admin-email "$ADMIN_EMAIL" \
  --admin-password "$ADMIN_PASS" 2>&1)
if [[ "$salida" == *"Empresa creada correctamente"* ]]; then
  ok "empresa creada sin tocar código ni recompilar"
elif [[ "$salida" == *"ya existe una empresa con el slug"* ]]; then
  ok "la empresa ya existía; se continúa con su configuración y su cartera"
else
  mal "no se pudo crear la empresa" "$salida"
  exit 1
fi

# ─────────────────────────────────────────────────────────────────────────────
azul "2. Su administrador entra y ajusta SU marca y SU configuración"
# ─────────────────────────────────────────────────────────────────────────────
printf '{"slug":"%s","email":"%s","password":"%s"}' "$SLUG" "$ADMIN_EMAIL" "$ADMIN_PASS" > "$TMP/login.json"
r=$(curl -s -c "$TMP/a.txt" -X POST "$API/login" --data-binary "@$TMP/login.json")
paso "ingreso del administrador" '"rol":"admin"' "$r"
[[ "$FALLO" == 1 ]] && exit 1

# Marca propia (el color solo vive en la banda del encabezado, §11) y parámetros de negocio
# distintos a los de las otras dos empresas: otro aviso previo, otros plazos, sin condonaciones
# y un tope de usuarios del plan contratado (R77).
r=$(envia a.txt PATCH /empresa/config '{
  "marca":  {"nombre_producto":"Cobros La Cumbre","color_primario":"#6A1B9A","color_primario_claro":"#8E44AD"},
  "config": {"dias_aviso_previo":5,"plazos_credito_permitidos":[8,15,30],
             "permitir_condonaciones":false,"metodos_pago":["efectivo","transferencia","cheque"],
             "prefijo_recibo":"RC-","limite_usuarios":5,"umbral_comportamiento_bueno":2}
}')
paso "marca propia (morado, otro nombre de producto)" '"Cobros La Cumbre"' "$r"
paso "aviso previo de 5 días"                          '"dias_aviso_previo":5' "$r"
paso "sin condonaciones (la interfaz oculta el botón)" '"permitir_condonaciones":false' "$r"
paso "plan con tope de 5 usuarios activos"             '"limite_usuarios":5' "$r"

# ─────────────────────────────────────────────────────────────────────────────
azul "3. Crea a su cobrador desde la aplicación (R48)"
# ─────────────────────────────────────────────────────────────────────────────
r=$(envia a.txt POST /usuarios "$(printf '{"nombre":"Óscar Rivera","email":"%s","rol":"vendedor","password":"%s"}' "$COBRADOR_EMAIL" "$COBRADOR_PASS")")
if [[ "$r" == *'"email":"'"$COBRADOR_EMAIL"'"'* ]]; then
  ok "cobrador creado"
elif [[ "$r" == *CORREO_YA_USADO* ]]; then
  ok "el cobrador ya existía"
else
  mal "no se pudo crear al cobrador" "$r"
fi
USUARIOS=$(jget a.txt /usuarios)
COBRADOR=$(idPorCorreo "$USUARIOS" "$COBRADOR_EMAIL")
ADMIN_ID=$(idPorCorreo "$USUARIOS" "$ADMIN_EMAIL")
printf '{"slug":"%s","email":"%s","password":"%s"}' "$SLUG" "$COBRADOR_EMAIL" "$COBRADOR_PASS" > "$TMP/login.json"
r=$(curl -s -c "$TMP/v.txt" -X POST "$API/login" --data-binary "@$TMP/login.json")
paso "ingreso del cobrador" '"rol":"vendedor"' "$r"

# ─────────────────────────────────────────────────────────────────────────────
azul "4. Importa su cartera desde el CSV de la plantilla (R71-R74)"
# ─────────────────────────────────────────────────────────────────────────────
# "Hoy" lo dice el backend, en la zona horaria de la empresa (R14): el script nunca lo calcula
# por su cuenta. Las fechas del archivo se derivan de ese día para que haya cuentas vencidas,
# por vencer y al día.
HOY=$(texto "$(jget a.txt /resumen)" fecha)
[[ -z "$HOY" ]] && { mal "no se pudo leer la fecha de la empresa" "$(jget a.txt /resumen)"; exit 1; }
d() { date -d "$HOY $1 days" +%F; }

curl -s -b "$TMP/a.txt" "$API/importacion/plantilla.csv" -o "$TMP/plantilla.csv"
paso "la plantilla se descarga de la propia API" 'codigo_cliente' "$(head -1 "$TMP/plantilla.csv")"

{
  head -1 "$TMP/plantilla.csv"
  # Nueve clientes de una distribuidora de lácteos, con catorce documentos repartidos en el tiempo.
  echo "LC-001,Súper Mercado La Despensa,Gloria Mejía,7823-4410,30,8000.00,FAC-10021,$(d -58),$(d -28),1840.00,400.00"
  echo "LC-001,Súper Mercado La Despensa,Gloria Mejía,7823-4410,30,8000.00,FAC-10087,$(d -12),$(d 18),965.50,0"
  echo "LC-002,Tienda y Pupusería Mi Rancho,Dora Alvarenga,7114-8832,15,0,FAC-10034,$(d -47),$(d -32),312.75,0"
  echo "LC-002,Tienda y Pupusería Mi Rancho,Dora Alvarenga,7114-8832,15,0,FAC-10095,$(d -9),$(d 6),188.00,0"
  echo "LC-003,Cafetería El Buen Café,Nelson Zelaya,2501-7723,8,1500.00,FAC-10052,$(d -20),$(d -12),540.25,140.25"
  echo "LC-004,Panadería Santa Eduviges,Blanca Rivas,7345-9012,15,2500.00,FAC-10061,$(d -24),$(d -9),780.00,0"
  echo "LC-004,Panadería Santa Eduviges,Blanca Rivas,7345-9012,15,2500.00,FAC-10102,$(d -4),$(d 11),455.00,0"
  echo "LC-005,Comedor Doña Chenta,Vicenta Guardado,7760-3321,8,0,FAC-10070,$(d -16),$(d -8),221.50,0"
  echo "LC-006,Minimarket Los Ángeles,Jorge Interiano,2277-5590,30,6000.00,FAC-10075,$(d -35),$(d -5),2310.00,810.00"
  echo "LC-006,Minimarket Los Ángeles,Jorge Interiano,2277-5590,30,6000.00,FAC-10110,$(d -2),$(d 28),1125.75,0"
  echo "LC-007,Tienda El Jicarito,Ernesto Alfaro,7988-1204,15,0,FAC-10081,$(d -31),$(d -16),398.40,0"
  echo "LC-008,Restaurante Sabor Cuscatleco,Patricia Melgar,2234-6677,30,4000.00,FAC-10093,$(d -19),$(d 11),1670.00,670.00"
  echo "LC-009,Abarrotería La Chapina,Hilda Ramírez,7452-8890,8,0,FAC-10105,$(d -6),$(d 2),276.80,0"
  echo "LC-009,Abarrotería La Chapina,Hilda Ramírez,7452-8890,8,0,FAC-10112,$(d -1),$(d 7),143.20,0"
} > "$TMP/cartera.csv"

r=$(curl -s -b "$TMP/a.txt" -X POST "$API/importacion/cartera" -F "archivo=@$TMP/cartera.csv")
paso "primera pasada: revisa y no guarda nada" '"aplicada":false' "$r"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/importacion/cartera" -F "archivo=@$TMP/cartera.csv" -F "aplicar=true")
if [[ "$r" == *'"aplicada":true'* ]]; then
  ok "segunda pasada: cartera cargada ($(sed -n 's/.*"clientes_creados":\([0-9]*\).*/\1/p' <<< "$r") clientes, $(sed -n 's/.*"cuentas_creadas":\([0-9]*\).*/\1/p' <<< "$r") cuentas)"
else
  mal "no se pudo aplicar la importación" "$r"
fi

# La cartera importada queda a nombre de quien importa; se le pasa al cobrador para que la vea en
# su pantalla (R40-R42). Es la misma operación que usaría la empresa cuando cambia de cobrador.
if [[ -n "$COBRADOR" && -n "$ADMIN_ID" && "$COBRADOR" != "$ADMIN_ID" ]]; then
  r=$(envia a.txt POST "/vendedores/$ADMIN_ID/reasignar-cartera" "$(printf '{"vendedor_destino_id":"%s","motivo":"Asignación inicial de la ruta al cobrador"}' "$COBRADOR")")
  paso "cartera asignada a la ruta del cobrador" 'cantidad_clientes' "$r"
fi

# ─────────────────────────────────────────────────────────────────────────────
azul "5. El cobrador trabaja la ruta: dos cobros y una promesa"
# ─────────────────────────────────────────────────────────────────────────────
vencidas=$(ids "$(jget v.txt '/cuentas?estado=vencido&por_pagina=5')")
n=0
for cuenta in $vencidas; do
  n=$((n + 1)); [[ $n -gt 2 ]] && break
  monto=$([[ $n == 1 ]] && echo 15000 || echo 22500)
  r=$(envia v.txt POST "/cuentas/$cuenta/pagos" "$(printf '{"monto_centavos":%s,"metodo":"efectivo","nota":"Abono recibido en la ruta"}' "$monto")")
  paso "cobro $n registrado con su recibo numerado (RC-)" '"numero":"RC-' "$r"
done

cliente=$(ids "$(jget v.txt '/clientes?por_pagina=1&con_cuentas=true')" | head -1)
if [[ -n "$cliente" ]]; then
  r=$(envia v.txt POST "/clientes/$cliente/promesas" "$(printf '{"fecha_promesa":"%s","monto_centavos":30000,"nota":"Dijo que pasa el camión el viernes y paga completo"}' "$(d 3)")")
  paso "promesa de pago anotada" '"fecha_promesa"' "$r"
fi

# ─────────────────────────────────────────────────────────────────────────────
azul "6. Aislamiento: cada empresa solo ve lo suyo (R47)"
# ─────────────────────────────────────────────────────────────────────────────
printf '{"slug":"distribuidora-progreso","email":"admin@progreso.sv","password":"admin123"}' > "$TMP/login.json"
curl -s -c "$TMP/p.txt" -X POST "$API/login" --data-binary "@$TMP/login.json" > /dev/null
suyo=$(jget a.txt '/clientes?por_pagina=200')
ajeno=$(jget p.txt '/clientes?por_pagina=200')
if [[ "$suyo" == *"La Despensa"* && "$ajeno" != *"La Despensa"* ]]; then
  ok "los clientes de Lácteos La Cumbre no existen para Distribuidora El Progreso"
else
  mal "el aislamiento entre empresas no se comportó como se esperaba" "$(head -c 200 <<< "$ajeno")"
  FALLO=1
fi
if [[ -n "$cliente" ]]; then
  c=$(curl -s -o /dev/null -w '%{http_code}' -b "$TMP/p.txt" "$API/clientes/$cliente")
  [[ "$c" == 404 ]] && ok "un cliente ajeno responde 404, no 403 (R38)" || { mal "se esperaba 404 al pedir un cliente ajeno" "obtuvo $c"; FALLO=1; }
fi

# ─────────────────────────────────────────────────────────────────────────────
printf '\n\033[1;34m%s\033[0m\n' "Credenciales de la empresa nueva (solo para la demostración en dev)"
printf '
  Empresa      Lácteos La Cumbre        slug: %s
  Dueña        %-26s contraseña: %s
  Cobrador     %-26s contraseña: %s

  Entre en http://localhost:8080 (o :8081), elija esos datos y compare con las otras dos empresas:
  mismo binario, misma base, otra marca, otras reglas y ningún dato en común.
' "$SLUG" "$ADMIN_EMAIL" "$ADMIN_PASS" "$COBRADOR_EMAIL" "$COBRADOR_PASS"

[[ "$FALLO" == 0 ]] || { printf '\nAlgún paso no quedó como se esperaba; revise lo marcado arriba.\n'; exit 1; }
exit 0
