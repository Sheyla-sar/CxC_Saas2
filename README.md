# Cuentas por Cobrar + CRM para distribuidoras

SaaS multi-empresa para distribuidoras de mayoreo en El Salvador: control de cuentas por cobrar,
pagos, condonaciones, ajustes con revisión, un pipeline simple de ventas y **cobranza en ruta**
(recibo numerado con la firma del cliente, liquidación del cobrador, fotos de los comprobantes,
promesas de pago, mensaje de cobro por WhatsApp, estado de cuenta en PDF y exportación a Excel).

API JSON en Go + PostgreSQL, con una interfaz web sin dependencias servida por el mismo binario.

Las reglas de negocio, el modelo de datos y las decisiones de arquitectura están en
[`CLAUDE.md`](CLAUDE.md), que es la referencia para las siguientes fases.
La puesta en producción está en [`DESPLIEGUE.md`](DESPLIEGUE.md).

## Requisitos

- **Go 1.27** o superior.
- **Docker** con Docker Compose (solo levanta PostgreSQL; la aplicación corre fuera del contenedor).
- **bash** y **curl** para los verificadores. En Windows vienen con Git Bash.

No hace falta Node, ni npm, ni ningún gestor de paquetes del frontend: la interfaz es HTML, CSS y
JavaScript sin dependencias, servido por el mismo binario de Go.

## Cómo levantarlo

```bash
git clone https://github.com/Jechev1/SaaS_CxC.git
cd SaaS_CxC

docker compose up -d --wait     # PostgreSQL 16 en localhost:5433
go run .                        # migra, siembra datos de prueba e imprime las credenciales
```

Abra **http://localhost:8080** e ingrese con una de las credenciales de la tabla de abajo.

El puerto de la base es el **5433**, no el 5432, para no chocar con un PostgreSQL que ya esté
instalado en la máquina. Si 5433 también está ocupado, cambie el mapeo en `docker-compose.yml` y
ajuste `DATABASE_URL`.

### Ver las dos pantallas a la vez

El producto tiene dos vistas muy distintas: la del dueño en la computadora y la del cobrador en el
teléfono. Para verlas lado a lado:

```bash
bash demostracion.sh            # dueño en :8080, cobrador en :8081
```

Levanta dos instancias contra la misma base. Van en puertos distintos a propósito: la sesión vive
en la memoria del proceso y la cookie es por origen, así que con un solo puerto entrar como
cobrador cerraría la sesión del dueño. Angoste la ventana del cobrador a unos 400 px y la interfaz
se acomoda sola a tamaño de teléfono. Lo que registra el cobrador le aparece al dueño al recargar.

### Comprobar que todo funciona

Con la aplicación corriendo, en otra terminal:

```bash
bash verificar.sh               # cuentas por cobrar, clientes y ventas   (129 comprobaciones)
bash verificar_cobranza.sh      # cobranza en ruta y documentos           ( 94 comprobaciones)
bash verificar_promesas.sh      # promesas de pago                        ( 54 comprobaciones)
```

Recorren el producto de punta a punta contra la base real: crean clientes, facturan, cobran,
anulan, liquidan, importan cartera, generan PDF y comprueban que una empresa no pueda ver los datos
de otra. Los tres deben terminar sin fallas.

### Volver a los datos de prueba originales

```bash
docker compose down -v && docker compose up -d --wait && go run .
```

Borra la base y la vuelve a sembrar. Los verificadores dejan datos de prueba regados; conviene
hacer esto antes de una demostración.

Al arrancar en `ENTORNO=dev` la aplicación:

1. Se niega a iniciar si el rol de base de datos es superusuario o tiene `BYPASSRLS`.
2. Aplica las migraciones pendientes (`migraciones/*.sql`, registradas en `schema_migrations`).
3. Comprueba que toda tabla con `empresa_id` tenga Row Level Security habilitado y forzado.
4. Imprime las credenciales de las dos empresas de prueba:

| Empresa | slug | Usuario | Contraseña | Rol |
|---|---|---|---|---|
| Distribuidora El Progreso | `distribuidora-progreso` | admin@progreso.sv | admin123 | admin |
| Distribuidora El Progreso | `distribuidora-progreso` | carlos@progreso.sv | vendedor123 | vendedor |
| Agroinsumos del Valle | `agroinsumos-valle` | admin@agroinsumos.sv | admin123 | admin |

Una tercera empresa, **Lácteos La Cumbre**, no viene en el seed: se da de alta con
`bash alta_empresa_demo.sh` (ver [Alta de una empresa nueva](#alta-de-una-empresa-nueva)), que es
el camino real de alta de un cliente del SaaS. Sus credenciales:

| Empresa | slug | Usuario | Contraseña | Rol |
|---|---|---|---|---|
| Lácteos La Cumbre | `lacteos-la-cumbre` | admin@lacumbre.sv | admin123 | admin |
| Lácteos La Cumbre | `lacteos-la-cumbre` | oscar@lacumbre.sv | cobrador123 | vendedor (cobrador) |

Las tres empresas comparten código y base de datos, pero tienen configuración distinta
(Agroinsumos: otro color, aviso previo de 3 días, sobrepago permitido y límite de crédito exigido.
La Cumbre: marca morada, aviso previo de 5 días, plazos de 8/15/30 días, sin condonaciones, tres
formas de pago, recibos con prefijo `RC-` y un plan con tope de cinco usuarios activos).

Para volver a los datos de prueba originales: `docker compose down -v && docker compose up -d`.

## Configuración

Variables de entorno (ver [`.env.ejemplo`](.env.ejemplo); se lee `.env` si existe):

| Variable | Por defecto | Uso |
|---|---|---|
| `PUERTO` | `8080` | Puerto HTTP |
| `DATABASE_URL` | la de docker-compose (solo en dev) | Conexión con un rol **sin** superusuario |
| `ENTORNO` | `dev` | `dev` siembra datos e imprime credenciales |
| `DOMINIO_BASE` | vacío | Habilita resolución por subdominio `<slug>.<DOMINIO_BASE>` |
| `RUTA_ARCHIVOS` | `archivos` | Carpeta donde se guardan las fotos y PDF que suben los usuarios |

Todo lo que cambia entre empresas (colores, moneda, plazos, umbrales, políticas) **no** está aquí:
vive en `empresa_marca` y `empresa_config` y se cambia con `PATCH /api/empresa/config`.

Para desplegar, el [`Dockerfile`](Dockerfile) compila el binario y publica una imagen mínima con
los archivos estáticos. Su `HEALTHCHECK` consulta `GET /api/salud` y la versión se inyecta al
compilar: `docker build --build-arg VERSION=1.2.3 -t cxc:1.2.3 .`

## Alta de una empresa nueva

Sin tocar código, recompilar ni desplegar:

```bash
go run . crear-empresa --slug abarroteria-lopez \
  --nombre-legal "Abarrotería López, S.A. de C.V." --nombre-comercial "Abarrotería López" \
  --admin-nombre "María López" --admin-email "maria@abarroterialopez.sv" --admin-password "********"
```

Crea la empresa, su marca y su configuración con los valores por defecto, y el administrador inicial.
La empresa puede iniciar sesión de inmediato, aunque el servidor esté corriendo.

Para ver el alta completa de un cliente del SaaS —y de paso el aislamiento multi-empresa con datos
de verdad— está `alta_empresa_demo.sh`, que da de alta a **Lácteos La Cumbre** sobre la misma base
que las dos empresas del seed:

```bash
docker compose up -d
ENTORNO=dev go run .            # en otra terminal
bash alta_empresa_demo.sh
```

Pasos que ejecuta, todos por el camino que usaría la empresa de verdad:

1. `crear-empresa` da de alta la empresa, su marca, su configuración y su administradora.
2. La administradora entra a la API y ajusta **su** marca y **sus** reglas con un `PATCH`.
3. Crea a su cobrador desde la propia aplicación.
4. Importa su cartera (9 clientes, 14 facturas) desde el CSV de la plantilla, en dos pasadas:
   revisar y aplicar. Reimportar el mismo archivo no duplica nada.
5. El cobrador registra cobros del día y anota una promesa, para que el tablero tenga datos.
6. Comprueba el aislamiento: los clientes de La Cumbre no existen para El Progreso, y pedir uno
   ajeno responde 404.

Volver a correrlo no duplica clientes ni cuentas; sí agrega dos cobros y una promesa cada vez.
Para empezar de cero: `docker compose down -v && docker compose up -d`.

## Aislamiento entre empresas

Dos capas independientes:

- **Aplicación:** cada método del repositorio recibe `empresa_id` como primer parámetro y cada query
  filtra por él. El vendedor además solo ve lo suyo (un recurso ajeno responde 404).
- **Base de datos:** cada petición abre una transacción y fija `app.empresa_id`; las políticas RLS
  (forzadas, con un rol sin privilegios) no devuelven filas de otra empresa aunque falte un `WHERE`.
  Las claves foráneas son compuestas `(empresa_id, id)`, así que una fila de una empresa tampoco
  puede apuntar a datos de otra.

`verificar.sh` termina con esa prueba: el admin de Agroinsumos intenta leer una cuenta de
El Progreso por su ID y recibe 404.

## API

Todas las respuestas son JSON. Los montos van en centavos (`*_centavos`) y las fechas como `AAAA-MM-DD`.
Los errores tienen la forma:

```json
{"error": {"codigo": "SOBREPAGO", "mensaje": "El pago de $3,000.00 es mayor que el saldo pendiente de $2,500.00. ...",
           "detalle": {"saldo_pendiente_centavos": 250000, "monto_recibido_centavos": 300000}}}
```

`400` entrada inválida · `401` sin sesión · `403` rol o empresa incorrecta · `404` no existe o no es tuyo ·
`422` regla de negocio · `500` error interno.

Los listados de cuentas y de clientes vienen paginados: `?pagina=` (1 por defecto) y `?por_pagina=`
(50 por defecto, máximo 200; fuera de rango responde `400 PARAMETRO_INVALIDO`). Cada respuesta trae
un bloque `paginacion` con `pagina`, `por_pagina`, `total`, `paginas`, `desde`, `hasta` y `hay_mas`.

| Método | Ruta | Notas |
|---|---|---|
| GET | `/api/salud` | Sin sesión. `{estado, base_de_datos, version, segundos_en_pie}`; 503 si la base no responde |
| GET | `/api/publico/marca?slug=` | Sin sesión. Logo, colores y nombre del producto |
| POST | `/api/login` | `{slug, email, password}` → cookie `sesion` HttpOnly |
| POST | `/api/logout` | |
| GET | `/api/me` | Usuario y empresa de la sesión |
| GET / PATCH | `/api/empresa/config` | Marca y configuración completa. PATCH solo admin |
| GET | `/api/usuarios` | Solo admin (para elegir destino de reasignación) |
| GET | `/api/resumen?top=5` | Totales, cuentas por estado, top deudores |
| GET / POST | `/api/clientes` | `?incluir_inactivos=true` · `?buscar=` nombre, código externo o teléfono · `?con_cuentas=true` agrega `cuentas_abiertas_lista` · `?pagina=&por_pagina=` |
| GET / PATCH | `/api/clientes/{id}` | Detalle con saldo, crédito disponible, comportamiento de pago y cuentas |
| POST | `/api/clientes/{id}/desactivar` | Rechaza si tiene saldo pendiente |
| GET / POST | `/api/oportunidades` | GET agrupa por etapa |
| PATCH | `/api/oportunidades/{id}` | Al pasar a `ganado` devuelve el formulario de conversión precargado |
| POST | `/api/oportunidades/{id}/convertir` | `{cliente_id?, monto_total_centavos?, fecha_emision?, fecha_vencimiento?}`. Crea la cuenta y la vincula (una sola vez) |
| GET / POST | `/api/cuentas` | `?estado=todas\|al_dia\|por_vencer\|vencido\|pagado` · `?buscar=` número de documento o cliente · `?pagina=&por_pagina=` |
| GET | `/api/cuentas/{id}` | Con movimientos (incluidos los anulados) y ajustes |
| PATCH | `/api/cuentas/{id}/monto` | `{monto_total_centavos, motivo}` |
| POST | `/api/cuentas/{id}/pagos` | `{monto_centavos, fecha?, metodo?, referencia?, nota?}`. Devuelve el recibo numerado |
| POST | `/api/cuentas/{id}/condonar` | Solo admin. `{monto_centavos?, motivo}` |
| POST | `/api/movimientos/{id}/anular` | Solo admin. `{motivo}` |
| GET | `/api/ajustes?estado=pendiente` | Solo admin |
| POST | `/api/ajustes/{id}/aprobar` · `/rechazar` | Solo admin. Rechazar revierte el monto |
| POST | `/api/vendedores/{id}/reasignar-cartera` | Solo admin. `{vendedor_destino_id, motivo}` |
| GET | `/api/alertas?solo_pendientes=true` | Cuentas por vencer o vencidas que requieren aviso hoy |
| POST | `/api/alertas/marcar` | `{cuenta_ids: [...]}` registra el aviso entregado (sin duplicar en el día) |

### Cobranza en ruta y documentos (fase 3)

| Método | Ruta | Notas |
|---|---|---|
| GET | `/api/tablero` | Indicadores: antigüedad, proyección de cobro, cobranza, cobradores |
| POST / PATCH | `/api/usuarios` · `/api/usuarios/{id}` | Solo admin. Alta y edición de la gente de la empresa |
| POST | `/api/usuarios/{id}/contrasena` · `/api/me/contrasena` | Reinicio por el admin · cambio propio |
| POST | `/api/cuentas/{id}/nota-credito` | Solo admin. Devolución o descuento posterior, con su número |
| GET / POST | `/api/anticipos` · `/api/clientes/{id}/anticipos` | Dinero a favor del cliente sin factura asignada |
| POST | `/api/anticipos/{id}/aplicar` · `/anular` | Aplicar a una cuenta · anular (si no se aplicó) |
| GET | `/api/liquidaciones/corte` | Lo cobrado que el cobrador todavía no entrega, por forma de pago |
| POST | `/api/liquidaciones` | Cierra el corte. Si no cuadra, exige explicación |
| POST | `/api/liquidaciones/{id}/aprobar` · `/rechazar` | Solo admin. Rechazar devuelve los cobros al corte |
| GET / POST / DELETE | `/api/adjuntos` | Fotos y PDF de recibos, facturas y comprobantes, con título y descripción |
| GET | `/api/adjuntos/{id}/documento.pdf` | Un comprobante como PDF, con su título, descripción y quién lo subió |
| GET | `/api/comprobantes.pdf` | `?entidad=&entidad_id=`. Todos los comprobantes de un registro en un PDF |
| GET | `/api/movimientos/{id}/recibo.pdf` | Recibo con monto en letras y las fotos del pago |
| GET | `/api/clientes/{id}/estado-cuenta.pdf` | `?detalle=true&fotos=true` |
| GET | `/api/informe-mensual.pdf` | `?mes=AAAA-MM`. El informe que recibe el dueño cada mes |
| GET / POST | `/api/importacion/plantilla.csv` · `/api/importacion/cartera` | Solo admin. Revisa y luego aplica |

### Integración con n8n (alertas)

El backend no envía mensajes. Un flujo externo puede, una vez al día:

1. `POST /api/login` con un usuario admin (guardar la cookie).
2. `GET /api/alertas?solo_pendientes=true` → cada alerta trae cliente, teléfono, saldo y días.
3. Enviar el WhatsApp o correo.
4. `POST /api/alertas/marcar` con los `cuenta_ids` enviados. Una segunda ejecución el mismo día ya no los devuelve.

## Estructura

```
main.go          arranque, verificaciones de seguridad y subcomando crear-empresa
config.go        variables de entorno
db.go            conexión, SET LOCAL de la empresa, verificaciones de rol y RLS
migrador.go      migraciones embebidas
tenant.go        contexto por petición y resolución de empresa
auth.go          sesiones en memoria y login
middleware.go    transacción por petición, registro y recuperación
modelos.go       tipos de dominio
repos.go         todo el SQL
reglas.go        reglas de negocio puras
handlers.go      rutas HTTP
errores.go       formato de errores
provision.go     alta de empresas

repos_cobranza.go      SQL de la fase 3 (usuarios, anticipos, liquidaciones, adjuntos, importación)
repos_tablero.go       agregaciones del tablero
reglas_cobranza.go     reglas de la fase 3, antigüedad de saldos y agrupaciones
handlers_cobranza.go   rutas de la fase 3
handlers_documentos.go PDF e importación de cartera
handlers_tablero.go    GET /api/tablero
documentos.go          dibujo de los PDF
archivos.go            adjuntos en disco
repos_paginacion.go    listados paginados: página, búsqueda y filtro por estado en SQL

static/          interfaz web (ver abajo)
handlers_informe.go    informe mensual del dueño en PDF

migraciones/     001_esquema · 002_seed_desarrollo · 003_cobranza_en_ruta ·
                 004_seed_cobranza_desarrollo · 005_planes
verificar.sh     demostración end-to-end de las fases 1 y 2
verificar_cobranza.sh  demostración end-to-end de la fase 3
verificar_promesas.sh  demostración end-to-end de las promesas de pago
alta_empresa_demo.sh   alta de una tercera empresa con su cartera (demostración multi-empresa)
demostracion.sh        dueño en :8080 y cobrador en :8081 contra la misma base

Dockerfile       imagen de la aplicación (HEALTHCHECK contra /api/salud)
respaldar.sh     respaldo de la base y de los adjuntos
restaurar.sh     restauración de un respaldo (por defecto sobre una base de prueba)
```

## Interfaz web

Con el servidor corriendo, abra **http://localhost:8080**. Para abrir directamente la pantalla de
ingreso de una empresa: `http://localhost:8080/?empresa=agroinsumos-valle`.

HTML, CSS y JavaScript sin dependencias en `static/`, servido por el mismo binario de Go. La marca,
los colores, la moneda y las opciones visibles vienen de la configuración de cada empresa por API.
Los detalles de arquitectura están en `CLAUDE.md` §11.

Qué verá según el rol:

- **Administración:** Resumen (tablero con antigüedad de saldos, proyección de cobro, cobranza de
  los últimos 30 días, desempeño de los cobradores y saldo que se fue en notas de crédito),
  Ventas en proceso, Cuentas, Clientes, Liquidación, Alertas, Revisiones y Configuración
  (usuarios e importación de cartera).
- **Vendedor o cobrador:** Mi cartera, Mi corte (lo que cobró y todavía no entrega) y Alertas.
  Al registrar un pago puede imprimir el recibo numerado y tomarle una foto al comprobante.

## Respaldos

```bash
./respaldar.sh                                    # deja el volcado y los adjuntos en ./respaldos
DESTINO=/mnt/discoB RETENCION_DIAS=60 ./respaldar.sh

./restaurar.sh respaldos/cxc-20261015-020000.dump            # restaura en la base cxc_prueba
BASE_DESTINO=cxc CONFIRMAR=SI ./restaurar.sh respaldos/x.dump  # sobre la base REAL
```

`respaldar.sh` saca el volcado de la base con `pg_dump -Fc`, comprime la carpeta de adjuntos en un
`tar.gz`, comprueba que el volcado se pueda leer y borra los respaldos de más de `RETENCION_DIAS`
(30 por defecto). Está pensado para correr desde cron una vez al día.

`restaurar.sh` restaura sobre una base de **prueba** salvo que se escriba `CONFIRMAR=SI` a mano.
La base restaurada se crea con dueño `cxc_app` y se restaura conservando el dueño de las tablas:
si quedaran a nombre del superusuario, la aplicación no arrancaría, porque el aislamiento depende
de `FORCE ROW LEVEL SECURITY` sobre tablas propias.

Un respaldo que nunca se restauró no es un respaldo. Pruebe la restauración contra una base de
prueba y levante la aplicación apuntando a ella: si arranca, valida el RLS y responde, el respaldo
sirve.

## Fuera de alcance todavía

Envío real de mensajes, multi sucursal, facturación electrónica (DTE), inventario, contabilidad,
autoregistro, recuperación de contraseña, facturación del SaaS y tests automatizados.
