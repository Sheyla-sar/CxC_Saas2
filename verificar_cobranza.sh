#!/usr/bin/env bash
# verificar_cobranza.sh — verificación end-to-end de la FASE 3 (cobranza en ruta y documentos):
# usuarios, notas de crédito, anticipos, liquidación del cobrador, adjuntos, recibo y estado de
# cuenta en PDF, importación de cartera, aislamiento entre empresas y permisos por rol.
#
#   ENTORNO=dev go run .          (en otra terminal)
#   bash verificar_cobranza.sh
#
# Igual que verificar.sh: solo necesita curl. No usa jq ni modifica la base más allá de los
# registros de prueba que crea. Trabaja contra las empresas de demostración del seed.

set -u
API="${API:-http://localhost:8080/api}"
# La carpeta temporal va DENTRO del proyecto, con ruta relativa: en Windows, curl.exe no entiende
# las rutas estilo /tmp de Git Bash y no podría adjuntar los archivos de prueba.
TMP="$(mktemp -d ./.verificar-cobranza.XXXXXX)"
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

idde() { grep -o '"id":"[^"]*"' <<< "$1" | head -1 | cut -d'"' -f4; }
num()  { sed -n "s/.*\"$2\"[[:space:]]*:[[:space:]]*\(-\{0,1\}[0-9]*\).*/\1/p" <<< "$1" | head -1; }

# Los cuerpos JSON se mandan desde archivo: en Windows, curl recibe los acentos de la línea de
# comandos en la página de códigos del sistema y rompería el UTF-8.
jpost()  { printf '%s' "$2" > "$TMP/cuerpo.json"; curl -s -b "$TMP/a.txt" -H 'Content-Type: application/json' -X POST  "$API$1" --data-binary "@$TMP/cuerpo.json"; }
jpatch() { printf '%s' "$2" > "$TMP/cuerpo.json"; curl -s -b "$TMP/a.txt" -H 'Content-Type: application/json' -X PATCH "$API$1" --data-binary "@$TMP/cuerpo.json"; }
jget()   { curl -s -b "$TMP/a.txt" "$API$1"; }
codigo() { curl -s -o /dev/null -w '%{http_code}' -b "$TMP/$1" "$API$2"; }

azul "0. Sesión y configuración de la fase 3"
r=$(curl -s -c "$TMP/a.txt" -X POST "$API/login" \
  -d '{"slug":"distribuidora-progreso","email":"admin@progreso.sv","password":"admin123"}')
check "ingreso del administrador" '"rol":"admin"' "$r"
if [[ "$r" != *'"rol":"admin"'* ]]; then
  printf '\nNo se pudo ingresar. ¿Está corriendo la aplicación con ENTORNO=dev y el seed aplicado?\n'
  exit 1
fi
cfg=$(jget /empresa/config)
check "la configuración trae las formas de pago"       'efectivo' "$cfg"
check "la configuración trae el prefijo de recibo"     '"prefijo_recibo":"REC-"' "$cfg"
check "la configuración trae el tope de los adjuntos"  '"tamano_maximo_adjunto_mb"' "$cfg"
check "la empresa trae sus datos fiscales"             '"nit"' "$cfg"

azul "1. Usuarios (R48-R50)"
correo="cobrador$RANDOM@progreso.sv"
r=$(jpost /usuarios "{\"nombre\":\"Jose Cobrador\",\"email\":\"$correo\",\"rol\":\"vendedor\",\"password\":\"ruta12345\"}")
check "crear un usuario cobrador" '"rol":"vendedor"' "$r"
COB=$(idde "$r")
r=$(jpost /usuarios "{\"nombre\":\"Otro\",\"email\":\"$correo\",\"password\":\"ruta12345\"}")
check "el mismo correo dos veces se rechaza" 'CORREO_YA_USADO' "$r"
r=$(jpost /usuarios '{"nombre":"Corta","email":"corta@progreso.sv","password":"123"}')
check "contraseña corta se rechaza" 'CONTRASENA_CORTA' "$r"
r=$(jpatch "/usuarios/$COB" '{"nombre":"Jose Antonio Cobrador"}')
check "editar el usuario" 'Jose Antonio Cobrador' "$r"
r=$(jpost "/usuarios/$COB/contrasena" '{"password":"nueva12345"}')
sin "el administrador reinicia la contraseña" '"error"' "$r"
ADMIN=$(idde "$(jget /me)")
r=$(jpatch "/usuarios/$ADMIN" '{"activo":false}')
check "la empresa no puede quedarse sin administrador" 'ADMIN' "$r"
r=$(curl -s -c "$TMP/c.txt" -X POST "$API/login" \
  -d "{\"slug\":\"distribuidora-progreso\",\"email\":\"$correo\",\"password\":\"nueva12345\"}")
check "el usuario nuevo ingresa con la contraseña reiniciada" '"rol":"vendedor"' "$r"

azul "2. Número de documento en la cuenta (R51)"
CLI=$(idde "$(jget /clientes)")
doc="FAC-$RANDOM"
r=$(jpost /cuentas "{\"cliente_id\":\"$CLI\",\"monto_total_centavos\":120000,\"numero_documento\":\"$doc\"}")
check "crear la cuenta con su número de documento" "\"numero_documento\":\"$doc\"" "$r"
CTA=$(idde "$r")
r=$(jpost /cuentas "{\"cliente_id\":\"$CLI\",\"monto_total_centavos\":5000,\"numero_documento\":\"$doc\"}")
check "el mismo documento dos veces se rechaza" 'DOCUMENTO' "$r"

azul "3. Pago con forma de pago y recibo numerado (R52-R53)"
r=$(jpost "/cuentas/$CTA/pagos" '{"monto_centavos":30000,"metodo":"efectivo","nota":"Abono en ruta"}')
check "pago en efectivo" '"metodo":"efectivo"' "$r"
check "el pago recibe su número de recibo" '"numero":"REC-' "$r"
MOV=$(idde "$r")
r=$(jpost "/cuentas/$CTA/pagos" '{"monto_centavos":10000,"metodo":"bitcoin"}')
check "una forma de pago no configurada se rechaza" 'METODO' "$r"
r=$(jpost "/cuentas/$CTA/pagos" '{"monto_centavos":20000,"metodo":"transferencia","referencia":"TRX-7788"}')
check "pago con referencia" 'TRX-7788' "$r"

azul "4. Nota de crédito (R54-R56)"
r=$(jpost "/cuentas/$CTA/nota-credito" '{"monto_centavos":10000,"motivo":"Producto devuelto por dano en transporte"}')
check "emitir la nota de crédito" '"tipo":"nota_credito"' "$r"
check "la nota de crédito va numerada" '"numero":"NC-' "$r"
r=$(jget "/cuentas/$CTA")
check "la nota de crédito rebaja el saldo (120000-30000-20000-10000)" '"saldo_centavos":60000' "$r"
r=$(jpost "/cuentas/$CTA/nota-credito" '{"monto_centavos":900000,"motivo":"Supera el saldo a proposito"}')
check "una nota de crédito mayor al saldo se rechaza" 'EXCEDE' "$r"

azul "5. Anticipos (R57-R60)"
r=$(jpost "/clientes/$CLI/anticipos" '{"monto_centavos":40000,"metodo":"efectivo","nota":"Adelanto del pedido"}')
check "registrar el anticipo" '"numero":"ANT-' "$r"
ANT=$(idde "$r")
check "el anticipo nace disponible completo" '"disponible_centavos":40000' "$r"
r=$(jpost "/anticipos/$ANT/aplicar" "{\"cuenta_id\":\"$CTA\",\"monto_centavos\":25000}")
check "aplicar el anticipo a una cuenta" '"metodo":"anticipo"' "$r"
r=$(jget "/anticipos?cliente_id=$CLI")
check "al anticipo le queda el resto disponible" '"disponible_centavos":15000' "$r"
r=$(jget "/cuentas/$CTA")
check "el anticipo aplicado rebaja el saldo" '"saldo_centavos":35000' "$r"
r=$(jpost "/anticipos/$ANT/anular" '{"motivo":"Intento anular un anticipo ya aplicado"}')
check "un anticipo ya aplicado no se anula" 'APLICADO' "$r"
r=$(jpost "/anticipos/$ANT/aplicar" "{\"cuenta_id\":\"$CTA\",\"monto_centavos\":900000}")
check "aplicar más de lo disponible se rechaza" 'DISPONIBLE' "$r"

azul "6. Liquidación del cobrador (R61-R65)"
corte=$(jget /liquidaciones/corte)
check "el corte lista lo cobrado sin entregar" '"total_esperado_centavos"' "$corte"
check "el corte se desglosa por forma de pago" '"por_metodo"' "$corte"
TOTAL=$(num "$corte" total_esperado_centavos)
r=$(jpost /liquidaciones "{\"total_entregado_centavos\":$((TOTAL - 500))}")
check "una diferencia sin explicación se rechaza" 'EXPLIC' "$r"
r=$(jpost /liquidaciones "{\"total_entregado_centavos\":$((TOTAL - 500)),\"nota\":\"Faltan 5.00: un cliente pago con un billete falso y lo repongo manana\"}")
check "cierre con faltante explicado" '"numero":"LIQ-' "$r"
LIQ=$(idde "$r")
check "la liquidación registra la diferencia" '"diferencia_centavos":-500' "$r"
r=$(jget /liquidaciones/corte)
check "después de liquidar, el corte queda en cero" '"total_esperado_centavos":0' "$r"
r=$(jpost "/liquidaciones/$LIQ/aprobar" '{}')
check "el administrador aprueba la liquidación" '"estado":"aprobada"' "$r"
r=$(jpost "/liquidaciones/$LIQ/rechazar" '{"motivo":"Ya fue aprobada, no deberia dejarme"}')
check "una liquidación resuelta no se resuelve otra vez" 'RESUELTA' "$r"

azul "7. Adjuntos: fotos de recibos y facturas (R66-R68)"
base64 -d > "$TMP/foto.png" <<'PNG'
iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR42mP4z8AAAAMBAQD3A0FDAAAAAElFTkSuQmCC
PNG
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=movimiento" -F "entidad_id=$MOV" \
  -F "titulo=Recibo del abono" -F "descripcion=Recibo firmado por el cliente" -F "archivo=@$TMP/foto.png;type=image/png")
check "subir la foto al movimiento" '"tipo_mime":"image/png"' "$r"
ADJ=$(idde "$r")
printf 'esto no es una imagen' > "$TMP/falso.png"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=movimiento" -F "entidad_id=$MOV" \
  -F "titulo=Archivo que miente" -F "archivo=@$TMP/falso.png;type=image/png")
check "un archivo que miente sobre su tipo se rechaza" 'TIPO' "$r"
head -c 40 "$TMP/foto.png" > "$TMP/roto.png"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=movimiento" -F "entidad_id=$MOV" \
  -F "titulo=Foto cortada" -F "archivo=@$TMP/roto.png;type=image/png")
check "una foto cortada se rechaza al subirla" 'IMAGEN_ILEGIBLE' "$r"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=movimiento" -F "entidad_id=$MOV"   -F "archivo=@$TMP/foto.png;type=image/png")
check "un comprobante sin título se rechaza" 'TITULO_REQUERIDO' "$r"
r=$(jget "/adjuntos?entidad=movimiento&entidad_id=$MOV")
check "listar los adjuntos del movimiento" "$ADJ" "$r"
check "con sesión, el archivo se descarga" '200' "$(codigo a.txt "/adjuntos/$ADJ/archivo")"
check "sin sesión, el archivo no se descarga" '401' "$(curl -s -o /dev/null -w '%{http_code}' "$API/adjuntos/$ADJ/archivo")"

azul "8. Documentos en PDF (R69-R70)"
c=$(curl -s -o "$TMP/recibo.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/movimientos/$MOV/recibo.pdf")
check "el recibo responde 200" '200' "$c"
check "el recibo es un PDF" '%PDF' "$(head -c 5 "$TMP/recibo.pdf")"
c=$(curl -s -o "$TMP/estado.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/clientes/$CLI/estado-cuenta.pdf?detalle=true&fotos=true")
check "el estado de cuenta responde 200" '200' "$c"
check "el estado de cuenta es un PDF" '%PDF' "$(head -c 5 "$TMP/estado.pdf")"

azul "9. Importación de cartera (R71-R74)"
curl -s -b "$TMP/a.txt" "$API/importacion/plantilla.csv" -o "$TMP/plantilla.csv"
check "la plantilla trae los encabezados" 'codigo_cliente' "$(head -1 "$TMP/plantilla.csv")"
S=$RANDOM
cabecera='codigo_cliente,nombre_cliente,contacto,telefono,plazo_dias,limite_credito,numero_documento,fecha_emision,fecha_vencimiento,monto,abonado'
{
  echo "$cabecera"
  echo "C$S-1,Tienda La Bendicion,Marta Perez,7777-1111,30,5000.00,DOC$S-1,2026-09-01,2026-10-01,450.00,100.00"
  echo "C$S-1,Tienda La Bendicion,Marta Perez,7777-1111,30,5000.00,DOC$S-2,2026-09-15,2026-10-15,300.00,0"
  echo "C$S-2,Super El Ahorro,Luis Gomez,7777-2222,99,0,DOC$S-3,2026-09-20,,200.00,0"
  echo "C$S-3,Fila sin monto,,,30,0,DOC$S-4,2026-09-20,,,0"
} > "$TMP/malo.csv"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/importacion/cartera" -F "archivo=@$TMP/malo.csv")
check "la revisión marca el plazo no permitido" 'plazo' "$r"
check "la revisión marca la fila sin monto"     'monto' "$r"
check "la revisión no guarda nada todavía"      '"aplicada":false' "$r"
{
  echo "$cabecera"
  echo "C$S-1,Tienda La Bendicion,Marta Perez,7777-1111,30,5000.00,DOC$S-1,2026-09-01,2026-10-01,450.00,100.00"
  echo "C$S-1,Tienda La Bendicion,Marta Perez,7777-1111,30,5000.00,DOC$S-2,2026-09-15,2026-10-15,300.00,0"
  echo "C$S-2,Super El Ahorro,Luis Gomez,7777-2222,30,0,DOC$S-3,2026-09-20,,200.00,0"
} > "$TMP/bueno.csv"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/importacion/cartera" -F "archivo=@$TMP/bueno.csv" -F "aplicar=true")
check "la importación se aplica"            '"aplicada":true' "$r"
check "crea los dos clientes del archivo"   '"clientes_creados":2' "$r"
check "crea las tres cuentas del archivo"   '"cuentas_creadas":3' "$r"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/importacion/cartera" -F "archivo=@$TMP/bueno.csv")
check "volver a importar avisa que ya existe" "DOC$S-1" "$r"

azul "10. Tablero (R75)"
t=$(jget /tablero)
check "el tablero trae la antigüedad de saldos"   '"antiguedad"' "$t"
check "el tablero trae la proyección de cobro"    '"proximos_vencimientos"' "$t"
check "el tablero trae la serie de cobranza"      '"serie_cobranza"' "$t"
check "el tablero trae el desempeño por cobrador" '"cobradores"' "$t"

azul "10.b Informe mensual del dueño (R76)"
c=$(curl -s -o "$TMP/informe.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/informe-mensual.pdf")
check "el informe del mes en curso responde 200" '200' "$c"
check "el informe es un PDF" '%PDF' "$(head -c 5 "$TMP/informe.pdf")"
c=$(curl -s -o "$TMP/informe-ant.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/informe-mensual.pdf?mes=2026-09")
check "se puede pedir un mes anterior" '200' "$c"
r=$(curl -s -b "$TMP/a.txt" "$API/informe-mensual.pdf?mes=2099-01")
check "un mes futuro se rechaza" 'MES_FUTURO' "$r"
r=$(curl -s -b "$TMP/a.txt" "$API/informe-mensual.pdf?mes=septiembre")
check "un mes mal escrito se rechaza" 'MES_INVALIDO' "$r"

azul "11. Aislamiento entre empresas (R47 sobre lo nuevo)"
curl -s -c "$TMP/b.txt" -X POST "$API/login" \
  -d '{"slug":"agroinsumos-valle","email":"admin@agroinsumos.sv","password":"admin123"}' > /dev/null
check "la empresa B no descarga el adjunto de A"    '404' "$(codigo b.txt "/adjuntos/$ADJ/archivo")"
check "la empresa B no ve la liquidación de A"      '404' "$(codigo b.txt "/liquidaciones/$LIQ")"
check "la empresa B no imprime el recibo de A"      '404' "$(codigo b.txt "/movimientos/$MOV/recibo.pdf")"
sin "la empresa B no ve el anticipo de A" "$ANT" "$(curl -s -b "$TMP/b.txt" "$API/anticipos")"

azul "12. Permisos por rol"
r=$(curl -s -b "$TMP/c.txt" "$API/usuarios")
check "el vendedor no lista usuarios" 'SOLO_ADMIN' "$r"
printf '%s' '{"monto_centavos":1000,"motivo":"No deberia poder hacer esto"}' > "$TMP/cuerpo.json"
r=$(curl -s -b "$TMP/c.txt" -H 'Content-Type: application/json' -X POST "$API/cuentas/$CTA/nota-credito" --data-binary "@$TMP/cuerpo.json")
check "el vendedor no emite notas de crédito" 'SOLO_ADMIN' "$r"
r=$(curl -s -b "$TMP/c.txt" "$API/liquidaciones/corte")
check "el vendedor sí ve su propio corte" '"total_esperado_centavos"' "$r"

azul "13. Paginación y búsqueda (R78)"
# La base filtra y pagina; reglas.go sigue decidiendo el saldo y el estado. Esta sección comprueba
# que las dos formas de contar coincidan: si alguien cambia una fórmula en un solo lado, falla.
resumen=$(jget /resumen)
for e in al_dia por_vencer vencido pagado; do
  enGo=$(sed -n "s/.*\"estado\":\"$e\",\"cantidad\":\([0-9]*\).*/\1/p" <<< "$resumen" | head -1)
  enSQL=$(num "$(jget "/cuentas?estado=$e&por_pagina=1")" total)
  check "SQL y Go cuentan igual las cuentas en estado $e" "$enGo" "$enSQL"
done
r=$(jget "/cuentas?por_pagina=2&pagina=1")
check "la primera página informa la paginación" '"pagina":1,"por_pagina":2' "$r"
primera=$(idde "$r")
r=$(jget "/cuentas?por_pagina=2&pagina=2")
sin "la segunda página trae cuentas distintas" "$primera" "$r"
check "la segunda página se identifica" '"pagina":2' "$r"
r=$(jget "/cuentas?buscar=zzz-no-existe")
check "una búsqueda sin resultados devuelve cero" '"total":0' "$r"
r=$(jget "/clientes?por_pagina=3")
check "los clientes también vienen paginados" '"por_pagina":3' "$r"
r=$(jget "/clientes?buscar=Abarrot")
check "se puede buscar un cliente por nombre" '"clientes":[{' "$r"
r=$(jget "/clientes?con_cuentas=true&por_pagina=2")
check "la cartera trae las cuentas abiertas de cada cliente" 'cuentas_abiertas_lista' "$r"
r=$(jget "/cuentas?pagina=0")
check "una página inválida se rechaza" 'PARAMETRO_INVALIDO' "$r"
r=$(jget "/cuentas?por_pagina=5000")
check "un tamaño de página excesivo se rechaza" 'PARAMETRO_INVALIDO' "$r"

azul "14. Comprobantes como documento (R79)"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=cuenta" -F "entidad_id=$CTA" \
  -F "titulo=Recibo firmado por el cliente" -F "descripcion=Abono entregado en la tienda" \
  -F "archivo=@$TMP/foto.png;type=image/png")
check "el adjunto guarda su título" '"titulo":"Recibo firmado por el cliente"' "$r"
check "el adjunto guarda quién lo subió" '"subido_por_nombre"' "$r"
COMP=$(idde "$r")
c=$(curl -s -o "$TMP/comp.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/adjuntos/$COMP/documento.pdf")
check "un comprobante se puede guardar como PDF" '200' "$c"
check "ese comprobante es un PDF" '%PDF' "$(head -c 5 "$TMP/comp.pdf")"
c=$(curl -s -o "$TMP/comps.pdf" -w '%{http_code}' -b "$TMP/a.txt" "$API/comprobantes.pdf?entidad=cuenta&entidad_id=$CTA")
check "todos los comprobantes de la cuenta en un PDF" '200' "$c"
check "ese paquete es un PDF" '%PDF' "$(head -c 5 "$TMP/comps.pdf")"
r=$(curl -s -b "$TMP/a.txt" -X POST "$API/adjuntos" -F "entidad=cuenta" -F "entidad_id=$CTA" \
  -F "titulo=$(printf 'x%.0s' $(seq 1 200))" -F "archivo=@$TMP/foto.png;type=image/png")
check "un título demasiado largo se rechaza" 'TEXTO_DEMASIADO_LARGO' "$r"
check "la empresa B no baja el comprobante de A en PDF" '404' "$(codigo b.txt "/adjuntos/$COMP/documento.pdf")"
check "la empresa B no arma el paquete de A" '404' "$(codigo b.txt "/comprobantes.pdf?entidad=cuenta&entidad_id=$CTA")"

printf '\n\033[1;34m══════ RESULTADO ══════\033[0m\n'
if [[ $FALLA -eq 0 ]]; then
  printf '\033[0;32m✔ las %d comprobaciones de la fase 3 pasaron.\033[0m\n' "$OK"
else
  printf '\033[0;31m✘ %d de %d comprobaciones fallaron.\033[0m\n' "$FALLA" "$((OK + FALLA))"
fi
exit $((FALLA > 0))
