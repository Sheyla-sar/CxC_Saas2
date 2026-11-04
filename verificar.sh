#!/usr/bin/env bash
# verificar.sh — demostración en vivo y verificación end-to-end de la API.
#
# Requisitos: bash y curl (jq es opcional: si existe, se usa para mostrar el JSON con formato).
# Uso:        ./verificar.sh            (con la API corriendo: go run .)
#             BASE_URL=http://otro:8080 ./verificar.sh
#
# Cada bloque imprime qué regla prueba, la llamada, el código HTTP y la respuesta, y comprueba el
# resultado esperado. Al final muestra el total de comprobaciones y sale con código 1 si alguna falla.
# Se puede ejecutar varias veces: crea sus propios datos en cada corrida.

set -u

BASE="${BASE_URL:-http://localhost:8080}"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
JAR_A="$TMP/admin_a.txt"      # sesión admin de la empresa A
JAR_V="$TMP/vendedor_a.txt"   # sesión vendedor de la empresa A
JAR_B="$TMP/admin_b.txt"      # sesión admin de la empresa B

if [ -t 1 ]; then
  AZUL=$'\e[1;34m'; AMAR=$'\e[1;33m'; VERDE=$'\e[1;32m'; ROJO=$'\e[1;31m'; GRIS=$'\e[0;90m'; NC=$'\e[0m'
else
  AZUL=""; AMAR=""; VERDE=""; ROJO=""; GRIS=""; NC=""
fi

PRUEBAS=0
FALLAS=0
ESTADO=""
RESP=""
UNICO="$(date +%H%M%S)"

seccion() { printf "\n%s══════ %s ══════%s\n" "$AZUL" "$1" "$NC"; }
regla()   { printf "\n%s▶ %s%s\n" "$AMAR" "$1" "$NC"; }

mostrar() {
  local limite="${1:-40}"
  if command -v jq >/dev/null 2>&1 && printf '%s' "$RESP" | jq . >/dev/null 2>&1; then
    local salida total
    salida="$(printf '%s' "$RESP" | jq .)"
    total="$(printf '%s\n' "$salida" | wc -l)"
    printf '%s\n' "$salida" | head -n "$limite" | sed "s/^/    /"
    [ "$total" -gt "$limite" ] && printf "    %s… (%s líneas más)%s\n" "$GRIS" "$((total - limite))" "$NC"
  else
    local max=$((limite * 30))
    if [ "${#RESP}" -gt "$max" ]; then
      printf "    %s%s…%s\n" "${RESP:0:$max}" "$GRIS" "$NC"
    else
      printf "    %s\n" "$RESP"
    fi
  fi
}

# llamar SESION METODO RUTA [CUERPO] [LINEAS_A_MOSTRAR] [HEADER_EXTRA]
llamar() {
  local jar="$1" metodo="$2" ruta="$3" cuerpo="${4:-}" lineas="${5:-40}" extra="${6:-}"
  local args=(-s -o "$TMP/cuerpo" -w '%{http_code}' -X "$metodo" -b "$jar" -c "$jar"
              -H 'Content-Type: application/json')
  # El cuerpo se envía desde un archivo: en Windows, curl recibe los argumentos en la página de
  # códigos ANSI y dañaría los acentos. Así viaja siempre en UTF-8.
  if [ -n "$cuerpo" ]; then
    printf '%s' "$cuerpo" > "$TMP/entrada.json"
    args+=(--data-binary "@$TMP/entrada.json")
  fi
  [ -n "$extra" ] && args+=(-H "$extra")
  printf "  %s%s %s%s\n" "$GRIS" "$metodo" "$ruta" "$NC"
  [ -n "$cuerpo" ] && printf "  %scuerpo: %s%s\n" "$GRIS" "$cuerpo" "$NC"
  [ -n "$extra" ] && printf "  %sheader: %s%s\n" "$GRIS" "$extra" "$NC"
  ESTADO="$(curl "${args[@]}" "$BASE$ruta")"
  RESP="$(cat "$TMP/cuerpo")"
  printf "  → HTTP %s\n" "$ESTADO"
  mostrar "$lineas"
}

ok()    { PRUEBAS=$((PRUEBAS + 1)); printf "  %s✔ %s%s\n" "$VERDE" "$1" "$NC"; }
falla() { PRUEBAS=$((PRUEBAS + 1)); FALLAS=$((FALLAS + 1)); printf "  %s✘ %s%s\n" "$ROJO" "$1" "$NC"; }

esperar() { # esperar CODIGO_HTTP
  if [ "$ESTADO" = "$1" ]; then ok "HTTP $1 como se esperaba"; else falla "se esperaba HTTP $1 y llegó $ESTADO"; fi
}

contiene() { # contiene TEXTO [DESCRIPCION]
  case "$RESP" in
    *"$1"*) ok "${2:-la respuesta contiene $1}" ;;
    *)      falla "${2:-la respuesta debería contener $1}" ;;
  esac
}

no_contiene() {
  case "$RESP" in
    *"$1"*) falla "${2:-la respuesta no debería contener $1}" ;;
    *)      ok "${2:-la respuesta no contiene $1}" ;;
  esac
}

# Extrae el PRIMER valor de un campo de texto o numérico del JSON (sin depender de jq).
campo()     { printf '%s' "$RESP" | grep -o "\"$1\":\"[^\"]*\"" | head -n1 | cut -d'"' -f4; }
campo_num() { printf '%s' "$RESP" | grep -o "\"$1\":-\{0,1\}[0-9]*" | head -n1 | cut -d: -f2; }

igual() { # igual VALOR ESPERADO DESCRIPCION
  if [ "$1" = "$2" ]; then ok "$3 = $2"; else falla "$3: se esperaba $2 y llegó '$1'"; fi
}

# ─────────────────────────────────────────────────────────────────────────────
printf "%sVerificación de la API de cuentas por cobrar — %s%s\n" "$AZUL" "$BASE" "$NC"
if ! curl -s -o /dev/null "$BASE/api/publico/marca?slug=distribuidora-progreso"; then
  printf "%sNo hay servidor en %s. Ejecute primero: docker compose up -d && go run .%s\n" "$ROJO" "$BASE" "$NC"
  exit 1
fi

# ═════════════════════════════════════════════════════════════════════════════
seccion "1. ENTRADA Y CONFIGURACIÓN (empresa A: Distribuidora El Progreso)"

regla "Marca pública sin sesión: la pantalla de login se pinta con los colores de cada empresa"
llamar "$TMP/anonimo.txt" GET "/api/publico/marca?slug=distribuidora-progreso"
esperar 200; contiene '"color_primario":"#1E3A5F"' "empresa A usa el color por defecto #1E3A5F"
llamar "$TMP/anonimo.txt" GET "/api/publico/marca?slug=agroinsumos-valle"
esperar 200; contiene '"color_primario":"#2E7D32"' "empresa B tiene su propio color #2E7D32, sin tocar código"

regla "Sin sesión no hay acceso a datos (401)"
llamar "$TMP/anonimo.txt" GET "/api/resumen"
esperar 401

regla "Login con contraseña incorrecta: mensaje genérico (401)"
llamar "$TMP/anonimo.txt" POST "/api/login" '{"slug":"distribuidora-progreso","email":"admin@progreso.sv","password":"equivocada"}'
esperar 401; contiene '"codigo":"CREDENCIALES_INVALIDAS"'

regla "Login como admin de la empresa A (cookie HttpOnly)"
llamar "$JAR_A" POST "/api/login" '{"slug":"distribuidora-progreso","email":"admin@progreso.sv","password":"admin123"}'
esperar 200
ADMIN_A_ID="$(campo id)"

regla "Configuración completa: el frontend se adapta a partir de aquí"
llamar "$JAR_A" GET "/api/empresa/config" "" 60
esperar 200
contiene '"plazos_credito_permitidos":[15,30,45,60]' "plazos permitidos por defecto {15,30,45,60}"
contiene '"dias_aviso_previo":7' "aviso previo de 7 días (valor por defecto)"
contiene '"permitir_sobrepago":false' "sobrepago no permitido (valor por defecto)"

regla "Resumen de cartera: totales, cuentas por estado y top 5 deudores"
llamar "$JAR_A" GET "/api/resumen" "" 30
esperar 200; contiene '"cuentas_por_estado"'; contiene '"top_deudores"'

# ═════════════════════════════════════════════════════════════════════════════
seccion "2. CLIENTE, OPORTUNIDAD Y CONVERSIÓN EN CUENTA"

regla "R2: el plazo de crédito debe estar entre los permitidos por la empresa (20 días no lo está)"
llamar "$JAR_A" POST "/api/clientes" "{\"nombre\":\"Abarrotería Prueba $UNICO\",\"plazo_credito_dias\":20}"
esperar 422; contiene '"codigo":"PLAZO_NO_PERMITIDO"'

regla "R1, R4: crear cliente con plazo de 30 días y límite de crédito de \$5,000.00"
llamar "$JAR_A" POST "/api/clientes" "{\"nombre\":\"Abarrotería Prueba $UNICO\",\"contacto\":\"Doña Chepita\",\"telefono\":\"7000-0000\",\"plazo_credito_dias\":30,\"limite_credito_centavos\":500000}"
esperar 201
CLIENTE_ID="$(campo id)"

regla "R6: crear oportunidad de \$3,500.00 en etapa 'nuevo'"
llamar "$JAR_A" POST "/api/oportunidades" "{\"cliente_id\":\"$CLIENTE_ID\",\"monto_estimado_centavos\":350000,\"notas\":\"Pedido de granos básicos\"}"
esperar 201; contiene '"etapa":"nuevo"'
OPORTUNIDAD_ID="$(campo id)"

regla "R6: avanzar a 'negociacion'"
llamar "$JAR_A" PATCH "/api/oportunidades/$OPORTUNIDAD_ID" '{"etapa":"negociacion"}' 5
esperar 200; contiene '"conversion":null' "todavía no hay datos de conversión"

regla "R7: marcar como ganada NO crea la cuenta; devuelve el formulario precargado"
llamar "$JAR_A" PATCH "/api/oportunidades/$OPORTUNIDAD_ID" '{"etapa":"ganado"}'
esperar 200
contiene '"monto_sugerido_centavos":350000' "monto sugerido = monto estimado"
contiene '"plazo_credito_dias":30' "plazo del cliente para calcular el vencimiento"
contiene '"cuenta_id":null' "la oportunidad sigue sin cuenta"

regla "R8, R11: convertir en cuenta (vencimiento = emisión + 30 días)"
llamar "$JAR_A" POST "/api/oportunidades/$OPORTUNIDAD_ID/convertir" '{"monto_total_centavos":350000}'
esperar 201; contiene "\"oportunidad_id\":\"$OPORTUNIDAD_ID\"" "la cuenta queda vinculada a la oportunidad"
contiene '"estado":"al_dia"'
CUENTA_ID="$(campo id)"

regla "R9: una oportunidad solo se convierte una vez"
llamar "$JAR_A" POST "/api/oportunidades/$OPORTUNIDAD_ID/convertir" '{"monto_total_centavos":350000}'
esperar 422; contiene '"codigo":"OPORTUNIDAD_YA_CONVERTIDA"'

regla "R8: la cuenta se puede emitir a otro cliente (cliente_id); la venta queda asociada a ese cliente"
llamar "$JAR_A" POST "/api/clientes" "{\"nombre\":\"Sucursal Prueba $UNICO\",\"plazo_credito_dias\":45}" 5
esperar 201
CLIENTE_SUCURSAL="$(campo id)"
llamar "$JAR_A" POST "/api/oportunidades" "{\"cliente_id\":\"$CLIENTE_ID\",\"monto_estimado_centavos\":80000}" 5
esperar 201
OPORTUNIDAD2_ID="$(campo id)"
llamar "$JAR_A" PATCH "/api/oportunidades/$OPORTUNIDAD2_ID" '{"etapa":"ganado"}' 5
esperar 200
llamar "$JAR_A" POST "/api/oportunidades/$OPORTUNIDAD2_ID/convertir" "{\"cliente_id\":\"$CLIENTE_SUCURSAL\"}"
esperar 201
contiene "\"cliente_id\":\"$CLIENTE_SUCURSAL\"" "la cuenta quedó a nombre del otro cliente"
contiene '"dias_para_vencer":45' "vencimiento calculado con el plazo del cliente elegido (45 días)"

# ═════════════════════════════════════════════════════════════════════════════
seccion "3. PAGOS"

regla "R15: el monto debe ser mayor a cero"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":0}' 5
esperar 400

regla "R16: no se aceptan pagos con fecha futura"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":1000,"fecha":"2999-01-01"}' 8
esperar 422; contiene '"codigo":"FECHA_FUTURA"'

regla "R17, R19: abono parcial de \$1,000.00 (queda registrado_por)"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":100000,"nota":"Abono en efectivo"}'
esperar 201
PAGO1_ID="$(campo id)"
contiene "\"registrado_por\":\"$ADMIN_A_ID\"" "registrado_por = usuario autenticado"
contiene '"saldo_centavos":250000' "saldo = 3,500.00 - 1,000.00 = 2,500.00"
contiene '"monto_total_centavos":350000' "el pago no modifica el monto total"

regla "R18: sobrepago rechazado con el saldo exacto en el detalle"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":300000}'
esperar 422
contiene '"codigo":"SOBREPAGO"'
contiene '"saldo_pendiente_centavos":250000' "saldo pendiente exacto: 250000"
contiene '"monto_recibido_centavos":300000' "monto recibido: 300000"

regla "Segundo abono de \$2,450.00: quedan \$50.00 de remanente"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":245000,"nota":"Transferencia"}' 5
esperar 201; contiene '"saldo_centavos":5000' "remanente de 5000 centavos"

# ═════════════════════════════════════════════════════════════════════════════
seccion "4. LÍMITE DE CRÉDITO (empresa A: advertencia, no bloqueo)"

regla "R5: saldo abierto 50.00 + nueva cuenta 5,000.00 > límite 5,000.00 → se crea con advertencia"
llamar "$JAR_A" POST "/api/cuentas" "{\"cliente_id\":\"$CLIENTE_ID\",\"monto_total_centavos\":500000}"
esperar 201
contiene '"advertencia":{"codigo":"LIMITE_CREDITO_EXCEDIDO"' "advertencia con el detalle del exceso"
contiene '"exceso_centavos":5000' "exceso exacto: 5000 centavos"
contiene '"oportunidad_id":null' "R10: cuenta de venta directa, sin oportunidad"
CUENTA2_ID="$(campo id)"

# ═════════════════════════════════════════════════════════════════════════════
seccion "5. CONDONACIÓN Y ANULACIÓN"

regla "R23: no se puede condonar más que el saldo"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/condonar" '{"monto_centavos":9000,"motivo":"Diferencia de precio acordada"}' 10
esperar 422; contiene '"codigo":"CONDONACION_EXCEDE_SALDO"'

regla "R22: la condonación exige motivo con la longitud mínima configurada (10)"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/condonar" '{"motivo":"corto"}' 10
esperar 400; contiene '"codigo":"MOTIVO_INSUFICIENTE"'

regla "R20, R21: el admin condona el remanente de \$50.00 → la cuenta queda pagada"
llamar "$JAR_A" POST "/api/cuentas/$CUENTA_ID/condonar" '{"motivo":"Remanente por redondeo acordado con la clienta"}'
esperar 201
contiene '"tipo":"condonacion"'; contiene '"estado":"pagado"' "estado = pagado"; contiene '"saldo_centavos":0'

regla "R26: anular exige motivo suficiente"
llamar "$JAR_A" POST "/api/movimientos/$PAGO1_ID/anular" '{"motivo":"error"}' 10
esperar 400; contiene '"codigo":"MOTIVO_INSUFICIENTE"'

regla "R25, R27: anular el primer abono (cheque rebotado): el saldo vuelve a \$1,000.00"
llamar "$JAR_A" POST "/api/movimientos/$PAGO1_ID/anular" '{"motivo":"El cheque fue rechazado por el banco"}'
esperar 200
contiene '"anulado":true'; contiene '"saldo_centavos":100000' "saldo = 100000 al dejar de contar el pago anulado"

regla "R28: no se puede anular dos veces"
llamar "$JAR_A" POST "/api/movimientos/$PAGO1_ID/anular" '{"motivo":"El cheque fue rechazado por el banco"}' 8
esperar 422; contiene '"codigo":"MOVIMIENTO_YA_ANULADO"'

regla "R27: el pago anulado sigue en el historial con motivo, quién y cuándo"
llamar "$JAR_A" GET "/api/cuentas/$CUENTA_ID" "" 80
esperar 200
contiene '"motivo_anulacion":"El cheque fue rechazado por el banco"' "motivo visible en el historial"
contiene '"anulado_por_nombre":"Ana Rivas"' "quién anuló"
contiene '"anulado_en":"' "cuándo se anuló"

# ═════════════════════════════════════════════════════════════════════════════
seccion "6. AJUSTE DE MONTO CON REVISIÓN POSTERIOR"

regla "R34: el monto no puede quedar debajo de lo ya aplicado (2,450.00 pagado + 50.00 condonado)"
llamar "$JAR_A" PATCH "/api/cuentas/$CUENTA_ID/monto" '{"monto_total_centavos":200000,"motivo":"Se devolvieron productos dañados"}' 15
esperar 422; contiene '"codigo":"MONTO_MENOR_A_PAGADO"'; contiene '"total_aplicado_centavos":250000'

regla "R30: editar el monto exige motivo"
llamar "$JAR_A" PATCH "/api/cuentas/$CUENTA_ID/monto" '{"monto_total_centavos":380000}' 8
esperar 400; contiene '"codigo":"MOTIVO_INSUFICIENTE"'

regla "R29, R31, R32: subir el monto a \$3,800.00 aplica de inmediato y marca la cuenta para revisión"
llamar "$JAR_A" PATCH "/api/cuentas/$CUENTA_ID/monto" '{"monto_total_centavos":380000,"motivo":"Se agregaron 3 quintales de frijol al pedido"}'
esperar 200
AJUSTE_ID="$(campo id)"
contiene '"estado":"pendiente"' "ajuste pendiente de revisión"
contiene '"monto_total_centavos":380000' "el nuevo monto ya está vigente"
contiene '"requiere_revision":true' "cuenta marcada para revisión"

regla "La cuenta aparece en la bandeja de ajustes pendientes del admin"
llamar "$JAR_A" GET "/api/ajustes?estado=pendiente" "" 30
esperar 200; contiene "\"id\":\"$AJUSTE_ID\"" "el ajuste está en la lista de pendientes"

regla "R33: el admin aprueba el ajuste → requiere_revision vuelve a false"
llamar "$JAR_A" POST "/api/ajustes/$AJUSTE_ID/aprobar"
esperar 200; contiene '"estado":"aprobado"'; contiene '"requiere_revision":false'

regla "R33: un segundo ajuste rechazado revierte el monto al anterior"
llamar "$JAR_A" PATCH "/api/cuentas/$CUENTA_ID/monto" '{"monto_total_centavos":420000,"motivo":"Error de digitación del vendedor"}' 5
esperar 200
AJUSTE2_ID="$(campo id)"
llamar "$JAR_A" POST "/api/ajustes/$AJUSTE2_ID/rechazar"
esperar 200; contiene '"estado":"rechazado"'; contiene '"monto_total_centavos":380000' "monto revertido a 380000"

# ═════════════════════════════════════════════════════════════════════════════
seccion "7. ROLES: VISIBILIDAD DEL VENDEDOR"

regla "Login como vendedor de la empresa A"
llamar "$JAR_V" POST "/api/login" '{"slug":"distribuidora-progreso","email":"carlos@progreso.sv","password":"vendedor123"}' 5
esperar 200
VENDEDOR_ID="$(campo id)"

regla "R38: el vendedor pide una cuenta que no es suya → 404 (no se le confirma que exista)"
llamar "$JAR_V" GET "/api/cuentas/$CUENTA_ID" "" 8
esperar 404

regla "R21, R28: condonar y anular son solo para admin → 403"
llamar "$JAR_V" POST "/api/cuentas/$CUENTA_ID/condonar" '{"motivo":"Intento del vendedor"}' 8
esperar 403; contiene '"codigo":"SOLO_ADMIN"'

regla "R37: el resumen del vendedor solo incluye su cartera"
llamar "$JAR_V" GET "/api/resumen" "" 12
esperar 200; no_contiene '"ajustes_pendientes_revision"' "el vendedor no ve la bandeja de ajustes"

# ═════════════════════════════════════════════════════════════════════════════
seccion "8. REASIGNACIÓN DE CARTERA"

regla "R40–R42: transferir la cartera del vendedor al admin (las cuentas pagadas conservan su vendedor)"
llamar "$JAR_A" POST "/api/vendedores/$VENDEDOR_ID/reasignar-cartera" "{\"vendedor_destino_id\":\"$ADMIN_A_ID\",\"motivo\":\"El vendedor cambió de ruta de distribución\"}"
esperar 200; contiene '"cantidad_clientes"'; contiene '"realizado_por"'

regla "Después de reasignar, el vendedor ya no ve cuentas con saldo"
llamar "$JAR_V" GET "/api/cuentas?estado=vencido" "" 8
esperar 200; contiene '"cuentas":[]' "sin cuentas vencidas en su cartera"

# ═════════════════════════════════════════════════════════════════════════════
seccion "9. ALERTAS PARA UN CONSUMIDOR EXTERNO (n8n)"

regla "R46: cuentas que requieren aviso hoy (por vencer o vencidas con saldo)"
llamar "$JAR_A" GET "/api/alertas?solo_pendientes=true" "" 30
esperar 200
ALERTA_CUENTA="$(campo cuenta_id)"
if [ -n "$ALERTA_CUENTA" ]; then
  regla "R46: registrar la entrega del aviso"
  llamar "$JAR_A" POST "/api/alertas/marcar" "{\"cuenta_ids\":[\"$ALERTA_CUENTA\"]}" 10
  esperar 200; contiene "\"registradas\":[\"$ALERTA_CUENTA\"]" "aviso registrado"

  regla "R46: marcarla otra vez el mismo día no duplica el aviso"
  llamar "$JAR_A" POST "/api/alertas/marcar" "{\"cuenta_ids\":[\"$ALERTA_CUENTA\"]}" 10
  esperar 200; contiene "\"ya_registradas\":[\"$ALERTA_CUENTA\"]" "reconocida como ya entregada"
else
  printf "  %s(todas las alertas de hoy ya fueron marcadas en corridas anteriores)%s\n" "$GRIS" "$NC"
fi

# ═════════════════════════════════════════════════════════════════════════════
seccion "10. AISLAMIENTO ENTRE EMPRESAS"

regla "Login como admin de la empresa B (Agroinsumos del Valle)"
llamar "$JAR_B" POST "/api/login" '{"slug":"agroinsumos-valle","email":"admin@agroinsumos.sv","password":"admin123"}' 5
esperar 200

regla "R47: la empresa B intenta leer por ID una cuenta de la empresa A → 404"
llamar "$JAR_B" GET "/api/cuentas/$CUENTA_ID" "" 8
esperar 404; contiene '"codigo":"CUENTA_NO_ENCONTRADA"'

regla "R47: tampoco puede registrarle pagos ni leer el cliente de la empresa A → 404"
llamar "$JAR_B" POST "/api/cuentas/$CUENTA_ID/pagos" '{"monto_centavos":100}' 5
esperar 404
llamar "$JAR_B" GET "/api/clientes/$CLIENTE_ID" "" 5
esperar 404

regla "R47: la lista de cuentas de B no contiene nada de A"
llamar "$JAR_B" GET "/api/cuentas" "" 5
esperar 200; no_contiene "$CUENTA_ID" "la cuenta de A no aparece"; no_contiene "Progreso" "ningún dato de A"

regla "Tenant: una sesión de A con header X-Empresa-Slug de B → 403"
llamar "$JAR_A" GET "/api/resumen" "" 5 "X-Empresa-Slug: agroinsumos-valle"
esperar 403; contiene '"codigo":"EMPRESA_NO_COINCIDE"'

# ═════════════════════════════════════════════════════════════════════════════
seccion "11. MISMA BASE DE CÓDIGO, DISTINTA CONFIGURACIÓN (empresa B)"

regla "Configuración de B: aviso previo 3 días, sobrepago permitido, límite de crédito exigido"
llamar "$JAR_B" GET "/api/empresa/config" "" 5
esperar 200
contiene '"dias_aviso_previo":3'; contiene '"permitir_sobrepago":true'; contiene '"exigir_limite_credito":true'

regla "R13 con config: en B una cuenta que vence en 5 días está 'al_dia' (en A, con 7 días, sería 'por_vencer')"
llamar "$JAR_B" GET "/api/cuentas?estado=al_dia" "" 5
esperar 200; contiene '"dias_para_vencer":5,"dias_vencida":0' "cuenta a 5 días de vencer clasificada al_dia"

regla "R18 con config: en B el sobrepago se acepta y el saldo queda negativo"
llamar "$JAR_B" POST "/api/clientes" "{\"nombre\":\"Agroservicio Prueba $UNICO\",\"plazo_credito_dias\":30,\"limite_credito_centavos\":300000}" 5
esperar 201
CLIENTE_B="$(campo id)"
llamar "$JAR_B" POST "/api/cuentas" "{\"cliente_id\":\"$CLIENTE_B\",\"monto_total_centavos\":200000}" 5
esperar 201
CUENTA_B="$(campo id)"
llamar "$JAR_B" POST "/api/cuentas/$CUENTA_B/pagos" '{"monto_centavos":250000,"nota":"Pagó de más, queda saldo a favor"}' 50
esperar 201; contiene '"saldo_centavos":-50000' "saldo a favor de 500.00"

regla "R5 con config: en B exceder el límite de crédito se RECHAZA (en A solo advertía)"
llamar "$JAR_B" POST "/api/cuentas" "{\"cliente_id\":\"$CLIENTE_B\",\"monto_total_centavos\":350000}"
esperar 422; contiene '"codigo":"LIMITE_CREDITO_EXCEDIDO"'

# ═════════════════════════════════════════════════════════════════════════════
seccion "RESULTADO"
if [ "$FALLAS" -eq 0 ]; then
  printf "%s✔ %d comprobaciones, todas correctas.%s\n" "$VERDE" "$PRUEBAS" "$NC"
  exit 0
fi
printf "%s✘ %d de %d comprobaciones fallaron.%s\n" "$ROJO" "$FALLAS" "$PRUEBAS" "$NC"
exit 1
