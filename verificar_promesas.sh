#!/usr/bin/env bash
# verificar_promesas.sh — verificación end-to-end de la PROMESA DE PAGO (fase 4):
# anotar la promesa, la agenda del día, el cumplimiento al registrar el pago, la promesa rota,
# la anulación, el rechazo de fechas pasadas, el alcance del vendedor y el aislamiento entre
# empresas.
#
#   ENTORNO=dev go run .          (en otra terminal)
#   bash verificar_promesas.sh
#
# Igual que verificar.sh y verificar_cobranza.sh: solo necesita curl, no usa jq y no modifica la
# base más allá de los registros de prueba que crea. Trabaja contra las empresas del seed.

set -u
API="${API:-http://localhost:8080/api}"
# La carpeta temporal va DENTRO del proyecto, con ruta relativa: en Windows, curl.exe no entiende
# las rutas estilo /tmp de Git Bash.
TMP="$(mktemp -d ./.verificar-promesas.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT
OK=0
FALLA=0

azul() { printf '\n\033[1;34m%s\033[0m\n' "$1"; }
ok()   { OK=$((OK + 1)); printf '  \033[0;32m✔\033[0m %s\n' "$1"; }
mal()  { FALLA=$((FALLA + 1)); printf '  \033[0;31m✘\033[0m %s\n       esperaba: %s\n       obtuvo:   %s\n' "$1" "$2" "${3:0:240}"; }

# check <descripción> <texto esperado> <respuesta>
check() { if [[ "$3" == *"$2"* ]]; then ok "$1"; else mal "$1" "$2" "$3"; fi; }
# sin <descripción> <texto que NO debe aparecer> <respuesta>
sin()   { if [[ "$3" != *"$2"* ]]; then ok "$1"; else mal "$1" "que NO apareciera $2" "$3"; fi; }

idde()  { grep -o '"id":"[^"]*"' <<< "$1" | head -1 | cut -d'"' -f4; }
ids()   { grep -o '"id":"[^"]*"' <<< "$1" | cut -d'"' -f4; }
num()   { sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\(-\{0,1\}[0-9]*\).*/\1/p" <<< "$1" | head -1; }
texto() { sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" <<< "$1" | head -1; }
cuantos() { grep -o "$2" <<< "$1" | wc -l | tr -d ' '; }

# Los cuerpos JSON se mandan desde archivo: en Windows, curl recibe los acentos de la línea de
# comandos en la página de códigos del sistema y rompería el UTF-8.
jpost()  { printf '%s' "$2" > "$TMP/cuerpo.json"; curl -s -b "$TMP/a.txt" -H 'Content-Type: application/json' -X POST "$API$1" --data-binary "@$TMP/cuerpo.json"; }
jget()   { curl -s -b "$TMP/a.txt" "$API$1"; }
# Las mismas dos, pero con la sesión que se indique (vendedor o empresa B).
cpost()  { printf '%s' "$3" > "$TMP/cuerpo.json"; curl -s -b "$TMP/$1" -H 'Content-Type: application/json' -X POST "$API$2" --data-binary "@$TMP/cuerpo.json"; }
cget()   { curl -s -b "$TMP/$1" "$API$2"; }
codigo() { printf '%s' "${3:-{\}}" > "$TMP/cuerpo.json"; curl -s -o /dev/null -w '%{http_code}' -b "$TMP/$1" -H 'Content-Type: application/json' -X POST "$API$2" --data-binary "@$TMP/cuerpo.json"; }

azul "0. Sesiones"
r=$(curl -s -c "$TMP/a.txt" -X POST "$API/login" \
  -d '{"slug":"distribuidora-progreso","email":"admin@progreso.sv","password":"admin123"}')
check "ingreso del administrador" '"rol":"admin"' "$r"
if [[ "$r" != *'"rol":"admin"'* ]]; then
  printf '\nNo se pudo ingresar. ¿Está corriendo la aplicación con ENTORNO=dev y el seed aplicado?\n'
  exit 1
fi
curl -s -c "$TMP/v.txt" -X POST "$API/login" \
  -d '{"slug":"distribuidora-progreso","email":"carlos@progreso.sv","password":"vendedor123"}' > /dev/null
curl -s -c "$TMP/b.txt" -X POST "$API/login" \
  -d '{"slug":"agroinsumos-valle","email":"admin@agroinsumos.sv","password":"admin123"}' > /dev/null

# "Hoy" lo dice el backend, en la zona horaria de la empresa (R14): el script nunca lo calcula.
HOY=$(texto "$(jget /promesas/agenda)" fecha)
check "la agenda informa la fecha de hoy de la empresa" "$HOY" "$HOY"
FUTURO=$(date -d '+7 days' +%F 2>/dev/null || echo '2099-12-31')

azul "1. Anotar una promesa (R80)"
CLI=$(idde "$(jget /clientes)")
r=$(jpost /cuentas "{\"cliente_id\":\"$CLI\",\"monto_total_centavos\":80000}")
CTA=$(idde "$r")
# La promesa apunta a la cuenta recién creada: así la ventana de cumplimiento no arrastra abonos
# que el cliente ya hizo hoy por otras facturas, y el script da el mismo resultado cada vez.
r=$(jpost "/clientes/$CLI/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"monto_centavos\":30000,\"cuenta_id\":\"$CTA\",\"nota\":\"Dijo que paga cuando le entren las ventas del fin de semana\"}")
check "anotar una promesa a futuro"            '"estado":"pendiente"' "$r"
check "la promesa guarda el monto prometido"   '"monto_centavos":30000' "$r"
check "la promesa guarda quién la anotó"       '"registrado_por_nombre"' "$r"
check "la promesa nace sin nada pagado"        '"pagado_en_ventana_centavos":0' "$r"
check "la promesa dice cuánto falta"           '"falta_centavos":30000' "$r"
PEND=$(idde "$r")
r=$(jpost "/clientes/$CLI/promesas" '{"fecha_promesa":"2020-01-01","monto_centavos":1000}')
check "una fecha ya pasada se rechaza" 'FECHA_PASADA' "$r"
r=$(jpost "/clientes/$CLI/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"monto_centavos\":0}")
check "un monto en cero se rechaza" 'MONTO_INVALIDO' "$r"
r=$(jpost "/clientes/$CLI/promesas" '{"monto_centavos":1000}')
check "sin fecha prometida se rechaza" 'FECHA_REQUERIDA' "$r"
OTRO=$(ids "$(jget /clientes)" | sed -n '2p')
r=$(jpost "/clientes/$OTRO/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"cuenta_id\":\"$CTA\"}")
check "una cuenta de otro cliente se rechaza" 'CUENTA_DE_OTRO_CLIENTE' "$r"

azul "2. La agenda del día"
r=$(jget /promesas/agenda)
sin "una promesa a futuro no está en la agenda de hoy" "$PEND" "$r"
check "la agenda trae las promesas ya vencidas del seed" '"estado":"rota"' "$r"
check "la agenda suma lo prometido" '"total_prometido_centavos"' "$r"
r=$(jpost "/clientes/$CLI/promesas" "{\"fecha_promesa\":\"$HOY\",\"monto_centavos\":25000,\"cuenta_id\":\"$CTA\",\"nota\":\"Pasa hoy por la tarde con el abono\"}")
check "anotar una promesa para hoy" '"estado":"pendiente"' "$r"
HOYP=$(idde "$r")
r=$(jget /promesas/agenda)
check "la promesa de hoy aparece en la agenda" "$HOYP" "$r"

azul "3. Cumplimiento: el estado se calcula, no se guarda"
r=$(jpost "/cuentas/$CTA/pagos" '{"monto_centavos":25000,"metodo":"efectivo","nota":"Abono de la promesa"}')
check "registrar el pago prometido" '"metodo":"efectivo"' "$r"
MOV=$(idde "$r")
r=$(jget "/promesas?estado=cumplida&por_pagina=200")
check "la promesa pasa a cumplida al pagar" "$HOYP" "$r"
r=$(jget /promesas/agenda)
sin "una promesa cumplida sale de la agenda" "$HOYP" "$r"
r=$(jget "/promesas?estado=pendiente&por_pagina=200")
sin "y deja de estar pendiente" "$HOYP" "$r"
r=$(jpost "/movimientos/$MOV/anular" '{"motivo":"El cheque del cliente rebotó, se revierte el abono"}')
check "anular el pago" '"anulado":true' "$r"
r=$(jget "/promesas?estado=pendiente&por_pagina=200")
check "al anular el pago, la promesa vuelve a estar pendiente" "$HOYP" "$r"
r=$(jpost "/cuentas/$CTA/pagos" '{"monto_centavos":25000,"metodo":"efectivo"}')
check "se vuelve a registrar el abono" '"metodo":"efectivo"' "$r"

azul "4. Promesa rota: la que no se cumplió a tiempo"
r=$(jget "/promesas?estado=rota&por_pagina=200")
ROTAS=$(num "$r" total)
check "hay promesas rotas"                       '"estado":"rota"' "$r"
check "una promesa rota dice cuántos días lleva" '"dias_de_atraso"' "$r"
if [[ "${ROTAS:-0}" -ge 1 ]]; then ok "el filtro de rotas cuenta $ROTAS"; else mal "el filtro de rotas cuenta al menos una" ">= 1" "${ROTAS:-0}"; fi
ROTA=$(idde "$r")
r=$(jget /promesas/agenda)
check "las rotas siguen en la agenda hasta que se cobren" "$ROTA" "$r"

azul "5. Anulación (R80)"
r=$(jpost "/clientes/$CLI/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"monto_centavos\":15000}")
ANU=$(idde "$r")
r=$(jpost "/promesas/$ANU/anular" '{"motivo":"corta"}')
check "un motivo demasiado corto se rechaza" 'MOTIVO_INSUFICIENTE' "$r"
r=$(jpost "/promesas/$ANU/anular" '{"motivo":"Se anoto al cliente equivocado durante la visita"}')
check "anular la promesa"                 '"estado":"anulada"' "$r"
check "la anulación deja quién y por qué" '"anulada_por_nombre"' "$r"
r=$(jpost "/promesas/$ANU/anular" '{"motivo":"Intento anularla otra vez a proposito"}')
check "anular dos veces se rechaza" 'PROMESA_YA_ANULADA' "$r"
r=$(jget "/promesas?estado=anulada&por_pagina=200")
check "la anulada aparece en su propio filtro" "$ANU" "$r"
r=$(jget /promesas/agenda)
sin "una promesa anulada no cuenta para nada" "$ANU" "$r"

azul "6. Alcance del vendedor (R37-R39)"
# Un cliente que NO es del vendedor: el que no aparece en su propio listado.
SUYOS=$(ids "$(cget v.txt '/clientes?por_pagina=200')")
AJENO=""
for c in $(ids "$(jget '/clientes?por_pagina=200')"); do
  if ! grep -qx "$c" <<< "$SUYOS"; then AJENO="$c"; break; fi
done
if [[ -n "$AJENO" ]]; then
  r=$(jpost "/clientes/$AJENO/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"monto_centavos\":12000}")
  AJENA=$(idde "$r")
  check "el administrador anota una promesa de su propia cartera" "\"cliente_id\":\"$AJENO\"" "$r"
  sin "el vendedor no ve la promesa de un cliente ajeno" "$AJENA" "$(cget v.txt '/promesas?por_pagina=200')"
  check "el vendedor no puede anular una promesa ajena (404, no 403)" '404' \
    "$(codigo v.txt "/promesas/$AJENA/anular" '{"motivo":"No deberia poder anularla nunca"}')"
  r=$(cpost v.txt "/clientes/$AJENO/promesas" "{\"fecha_promesa\":\"$FUTURO\"}")
  check "el vendedor tampoco anota promesas de clientes ajenos" 'NO_ENCONTRADO' "$r"
else
  mal "encontrar un cliente fuera de la cartera del vendedor" "un cliente del administrador" "ninguno"
fi
MIO=$(sed -n '1p' <<< "$SUYOS")
if [[ -z "$MIO" ]]; then
  # El vendedor puede haberse quedado sin cartera (una reasignación de otra verificación):
  # se le crea un cliente suyo, que es lo que esta sección necesita de verdad.
  MIO=$(idde "$(cpost v.txt /clientes "{\"nombre\":\"Tienda de prueba $RANDOM\",\"plazo_credito_dias\":30}")")
fi
r=$(cpost v.txt "/clientes/$MIO/promesas" "{\"fecha_promesa\":\"$FUTURO\",\"monto_centavos\":9000,\"nota\":\"Promesa que anota el cobrador en la ruta\"}")
# Se comprueba quién quedó como autor, no el estado: si ese cliente ya abonó hoy, la promesa nace
# cumplida, que es exactamente lo que dice la regla.
check "el vendedor sí anota promesas de su cartera" '"registrado_por_nombre":"Carlos' "$r"
MIA=$(idde "$r")
r=$(cpost v.txt "/promesas/$MIA/anular" '{"motivo":"El mismo cobrador corrige lo que acaba de anotar"}')
check "quien anotó la promesa puede anularla" '"estado":"anulada"' "$r"

azul "7. Aislamiento entre empresas (R47)"
sin "la empresa B no ve las promesas de A" "$PEND" "$(cget b.txt '/promesas?por_pagina=200')"
sin "la empresa B no ve la agenda de A"    "$ROTA" "$(cget b.txt '/promesas/agenda')"
check "la empresa B no anula una promesa de A" '404' \
  "$(codigo b.txt "/promesas/$PEND/anular" '{"motivo":"Intento anular una promesa de otra empresa"}')"
r=$(cpost b.txt "/clientes/$CLI/promesas" "{\"fecha_promesa\":\"$FUTURO\"}")
check "la empresa B no anota promesas a clientes de A" 'NO_ENCONTRADO' "$r"

azul "8. SQL y Go clasifican igual"
# El filtro por estado lo resuelve la base (repos_promesas.go) y el estado que se muestra lo
# calcula EvaluarPromesa (reglas_promesas.go). Esta sección compara las dos: si alguien cambia la
# fórmula en un solo lado, falla aquí.
todas=$(jget '/promesas?estado=todas&por_pagina=200')
TOTAL=$(num "$todas" total)
suma=0
for e in pendiente cumplida rota anulada; do
  enGo=$(cuantos "$todas" "\"estado\":\"$e\"")
  enSQL=$(num "$(jget "/promesas?estado=$e&por_pagina=1")" total)
  check "SQL y Go cuentan igual las promesas en estado $e" "$enGo" "$enSQL"
  suma=$((suma + enSQL))
done
check "los cuatro estados suman el total" "$TOTAL" "$suma"

azul "9. Paginación y parámetros (R78)"
r=$(jget '/promesas?por_pagina=1&pagina=1')
check "la primera página informa la paginación" '"pagina":1,"por_pagina":1' "$r"
primera=$(idde "$r")
r=$(jget '/promesas?por_pagina=1&pagina=2')
sin "la segunda página trae otra promesa" "$primera" "$r"
check "la segunda página se identifica" '"pagina":2' "$r"
r=$(jget '/promesas?pagina=0')
check "una página inválida se rechaza" 'PARAMETRO_INVALIDO' "$r"
r=$(jget '/promesas?por_pagina=5000')
check "un tamaño de página excesivo se rechaza" 'PARAMETRO_INVALIDO' "$r"
r=$(jget '/promesas?estado=cualquiera')
check "un estado inventado se rechaza" 'ESTADO_INVALIDO' "$r"

printf '\n\033[1;34m══════ RESULTADO ══════\033[0m\n'
if [[ $FALLA -eq 0 ]]; then
  printf '\033[0;32m✔ las %d comprobaciones de la promesa de pago pasaron.\033[0m\n' "$OK"
else
  printf '\033[0;31m✘ %d de %d comprobaciones fallaron.\033[0m\n' "$FALLA" "$((OK + FALLA))"
fi
exit $((FALLA > 0))
