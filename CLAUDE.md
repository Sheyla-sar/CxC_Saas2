# CLAUDE.md — SaaS de Cuentas por Cobrar + CRM (distribuidoras de mayoreo, El Salvador)

Este archivo es la fuente de verdad para todas las fases. Antes de cambiar código, leelo completo.
Si una decisión de aquí deja de ser válida, se actualiza este archivo en el mismo cambio.

---------------------------------------------------------------------------------------------------
## 1. Principio rector
---------------------------------------------------------------------------------------------------
Una sola base de código y una sola base de datos sirven a todas las empresas cliente.
Dar de alta una empresa nueva NUNCA requiere tocar código, recompilar ni desplegar.
Todo lo que puede variar entre empresas (marca, colores, parámetros de negocio) vive en
`empresa_marca` y `empresa_config`, y se expone por API. El frontend lee esa configuración y se adapta.

**Regla práctica:** si al implementar algo aparece un valor fijo que una empresa podría querer
distinto, ese valor va a `empresa_config` (con su DEFAULT en la migración), no al código.
Los valores por defecto viven en UN solo lugar: los `DEFAULT` de las columnas en SQL.

---------------------------------------------------------------------------------------------------
## 2. Fases
---------------------------------------------------------------------------------------------------
- **Fase 1: backend.** API JSON (terminada).
- **Fase 2: frontend provisional** en `static/` (ver §11). Se reemplazará la capa visual cuando
  llegue el diseño de Figma, sin tocar la lógica.
- **Fase 3: cobranza en ruta y documentos.** Gestión de usuarios, notas de crédito,
  anticipos, liquidación del cobrador, adjuntos (fotos de recibos y facturas), recibo y estado de
  cuenta en PDF, importación de cartera desde CSV y tablero de indicadores. Backend + frontend.
- **Fase 4 (actual): cobranza en la calle.** Promesa de pago, mensaje de cobro por WhatsApp,
  exportación a Excel y firma del cliente en el recibo. Cada grupo de rutas se registra en su
  propio archivo (`rutas_fase4.go`), para que varias manos puedan trabajar sin pisarse.
- Fuera de alcance todavía: envío automático de WhatsApp/correo, multi sucursal, DTE/facturación
  electrónica, inventario, contabilidad general, autoregistro, recuperación de contraseña,
  facturación del SaaS, tests automatizados.

---------------------------------------------------------------------------------------------------
## 3. Stack
---------------------------------------------------------------------------------------------------
- Go (librería estándar: `net/http`, `database/sql`). Sin frameworks web, sin ORM.
- PostgreSQL 16 con `github.com/jackc/pgx/v5` en modo `stdlib` (driver `"pgx"`).
- SQL plano, sin generadores de queries.
- `golang.org/x/crypto/bcrypt` para contraseñas.
- `github.com/go-pdf/fpdf` para el recibo y el estado de cuenta en PDF (única dependencia de la
  fase 3; no hay motor de plantillas ni generador de reportes).
- `docker-compose.yml` levanta SOLO Postgres (puerto host 5433). La app corre con `go run .`.
- Variables de entorno: `PUERTO` (8080), `DATABASE_URL`, `ENTORNO` (dev), `DOMINIO_BASE` (opcional),
  `RUTA_ARCHIVOS` (carpeta de los adjuntos, por defecto `archivos`).
  Se lee `.env` si existe (sin pisar variables ya definidas). Ver `.env.ejemplo`.

---------------------------------------------------------------------------------------------------
## 4. Estructura de archivos (paquete `main` único)
---------------------------------------------------------------------------------------------------
| Archivo | Responsabilidad |
|---|---|
| `main.go` | Arranque: config → DB → verificaciones de seguridad → migraciones → servidor. Subcomando `crear-empresa`. |
| `config.go` | Variables de entorno y lector de `.env`. |
| `db.go` | Conexión, `establecerEmpresa` (SET LOCAL), verificaciones de rol y de RLS al arrancar. |
| `migrador.go` | Aplica `migraciones/*.sql` embebidas, registra en `schema_migrations`. Archivos `*_desarrollo.sql` solo con `ENTORNO=dev`. |
| `tenant.go` | Contexto por petición (`Tenant`) y **resolución de empresa** (`resolverEmpresa`). |
| `auth.go` | Sesiones en memoria (map + mutex), login, logout. |
| `middleware.go` | Registro/recuperación de pánicos y `conTenant` (sesión → empresa → tx → SET LOCAL → handler → commit). |
| `modelos.go` | Structs de dominio y tipo `Fecha` (DATE sin hora). |
| `repos.go` | TODO el SQL. Cada método recibe `empresaID` como primer parámetro. |
| `repos_paginacion.go` | SQL de los listados paginados: página de cuentas y de clientes, filtro por estado y búsqueda. |
| `reglas.go` | Lógica de negocio pura: sin HTTP, sin SQL. Recibe datos + `ConfigEmpresa`. |
| `handlers.go` | Rutas y traducción HTTP ↔ reglas/repos. |
| `errores.go` | `ErrorApp`, tipos de error y formato JSON de errores. |
| `provision.go` | `go run . crear-empresa ...` — único camino para dar de alta un cliente del SaaS. |
| `verificar.sh` | Verificación end-to-end de las fases 1 y 2 con curl (no requiere jq). |
| **Fase 3** | |
| `repos_cobranza.go` | SQL de usuarios, anticipos, liquidaciones, adjuntos e importación. |
| `repos_tablero.go` | SQL de agregación para el tablero (cobranza por día y por cobrador, fuga, diferencias). |
| `reglas_cobranza.go` | Reglas puras de la fase 3 + antigüedad de saldos y agrupaciones del tablero. |
| `handlers_cobranza.go` | Rutas de usuarios, anticipos, notas de crédito, liquidaciones y adjuntos. |
| `handlers_documentos.go` | Recibo y estado de cuenta en PDF; plantilla e importación de cartera. |
| `handlers_tablero.go` | `GET /api/tablero`: todos los indicadores en una sola petición. |
| `handlers_informe.go` | `GET /api/informe-mensual.pdf`: el informe que recibe el dueño cada mes. |
| `handlers_comprobantes.go` | Los adjuntos convertidos en documento PDF, uno o todos los de un registro. |
| `documentos.go` | Dibujo de los PDF con fpdf (monto en letras, antigüedad, anexo de fotos). |
| `archivos.go` | Guardado en disco de los adjuntos, detección del tipo real y descarga autenticada. |
| `verificar_cobranza.sh` | Verificación end-to-end de la fase 3 con curl. |
| **Fase 4** | |
| `rutas_fase4.go` | Reparte el registro de rutas de la fase 4 en un archivo por grupo. |
| `reglas_promesas.go` | Regla pura que decide si una promesa está pendiente, cumplida o rota. |
| `repos_promesas.go` | SQL de las promesas de pago. |
| `handlers_promesas.go` | Rutas de promesas: anotar, agenda del día, historial y anular. |
| `handlers_exportacion.go` | Mensaje de cobro por WhatsApp y los dos CSV para Excel. |
| `handlers_firma.go` | `POST /api/movimientos/{id}/firma`: la firma que el cliente traza con el dedo. |
| `verificar_promesas.sh` | Verificación end-to-end de las promesas de pago. |
| **Operación** | |
| `Dockerfile` | Imagen en dos etapas; `HEALTHCHECK` contra `/api/salud`; la versión entra por `-ldflags`. |
| `respaldar.sh` | Respaldo de la base (`pg_dump -Fc`) y de los adjuntos; verifica el volcado y rota por días. |
| `restaurar.sh` | Restaura un respaldo; por defecto en una base de PRUEBA, sobre la real exige `CONFIRMAR=SI`. |
| `demostracion.sh` | Levanta DOS instancias (dueño y cobrador) en puertos distintos contra la misma base. |
| `alta_empresa_demo.sh` | Da de alta una TERCERA empresa (Lácteos La Cumbre) con su marca, su cobrador y su cartera, solo por los caminos reales de alta (`crear-empresa`, PATCH de configuración, alta de usuario, importación CSV) y comprueba el aislamiento. Ninguna de sus decisiones vive en código: todo pasa por `empresa_marca`, `empresa_config` y la API. |

---------------------------------------------------------------------------------------------------
## 5. Reglas transversales de arquitectura
---------------------------------------------------------------------------------------------------
**A. Aislamiento multi-tenant en dos capas.**
- Aplicación: toda tabla de negocio lleva `empresa_id`; todo query lleva `WHERE empresa_id = $1`;
  todo método de `Repo` recibe `empresaID` como PRIMER parámetro, sin excepción.
- Base de datos: RLS `ENABLE` + `FORCE` en todas las tablas con `empresa_id`, política
  `aislamiento_empresa` que compara contra `current_setting('app.empresa_id', true)`.
  Cada petición autenticada abre una transacción y ejecuta
  `SELECT set_config('app.empresa_id', $1, true)` (equivalente parametrizable de `SET LOCAL`)
  antes de cualquier query. Sin ese valor la base devuelve cero filas y rechaza inserts.
- Defensa adicional: las FK entre tablas de negocio son COMPUESTAS `(empresa_id, x_id)`, así que es
  imposible (a nivel de esquema) que una cuenta de la empresa A apunte a un cliente de la empresa B.
- La app se conecta con un rol SIN `SUPERUSER` ni `BYPASSRLS` (`cxc_app`), dueño de las tablas; por
  eso se usa `FORCE ROW LEVEL SECURITY`. Al arrancar, la app **se niega a iniciar** si:
  1) el rol es superusuario o tiene BYPASSRLS, o
  2) alguna tabla de `public` con columna `empresa_id` no tiene RLS habilitado y forzado.
  → Toda tabla nueva con `empresa_id` DEBE incluir RLS en su migración o la app no arranca.
- `empresa` y `schema_migrations` no tienen `empresa_id` y no llevan RLS. `empresa` solo contiene
  slug y nombres (necesarios para login y marca pública antes de conocer al usuario).

**B. UUID** en todas las tablas (`gen_random_uuid()` por defecto).

**C. Dinero en centavos** (`BIGINT`). Nunca float. Campos de la API terminan en `_centavos`.

**D. Fechas:** `DATE` para fechas de negocio (tipo Go `Fecha`, JSON `"AAAA-MM-DD"`),
`TIMESTAMPTZ` para auditoría (`creado_en`, `anulado_en`, ...).

**E. Lógica de negocio en `reglas.go`** como funciones puras que reciben datos y la
configuración de la empresa. Los handlers orquestan (cargar → reglas → guardar) y traducen HTTP.
Saldo y estado NUNCA se almacenan: el repo agrega sumas, `reglas.EvaluarCuenta` calcula.

**F. Scoping por rol en los repos.** Los métodos de lectura reciben un `Alcance`
(`Total` para admin, `UsuarioID` para vendedor → `($2::uuid IS NULL OR vendedor_id = $2)`).
Recurso ajeno para un vendedor ⇒ 404 (nunca 403).

**G. Concurrencia.** Operaciones que dependen del saldo (pagos, condonaciones, anulaciones,
ajustes, aprobar/rechazar) bloquean la fila de la cuenta con `SELECT ... FOR UPDATE` antes de
calcular. Conversión de oportunidad bloquea la oportunidad; además `cuenta.oportunidad_id` es UNIQUE.

---------------------------------------------------------------------------------------------------
## 6. Modelo de datos (migraciones/001_esquema.sql)
---------------------------------------------------------------------------------------------------
```
empresa(id, slug UNIQUE, nombre_legal, nombre_comercial, activo, creado_en)
empresa_marca(empresa_id PK FK, logo_url, color_primario='#1E3A5F', color_primario_claro='#2E5C8A',
              nombre_producto, creado_en, actualizado_en)
empresa_config(empresa_id PK FK,
  moneda='USD', simbolo_moneda='$', zona_horaria='America/El_Salvador',
  dias_aviso_previo=7, plazos_credito_permitidos INT[]='{15,30,45,60}',
  permitir_sobrepago=false, exigir_revision_ajuste_monto=true, exigir_motivo_anulacion=true,
  longitud_minima_motivo=10, umbral_comportamiento_bueno=3, umbral_comportamiento_regular=10,
  minimo_cuentas_para_historial=3, cuentas_evaluadas_comportamiento=10,
  exigir_limite_credito=false, permitir_condonaciones=true, creado_en, actualizado_en)
usuario(id, empresa_id, nombre, email, password_hash, rol admin|vendedor, activo, creado_en)
  UNIQUE(empresa_id, email)  -- el mismo correo puede existir en dos empresas
cliente(id, empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos
        (0 = sin límite), vendedor_id, activo, creado_en)
oportunidad(id, empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id,
            cuenta_id NULL, creado_en, actualizado_en)
  etapa: nuevo | contactado | negociacion | ganado | perdido
cuenta(id, empresa_id, cliente_id, oportunidad_id NULL UNIQUE, monto_total_centavos, fecha_emision,
       fecha_vencimiento, vendedor_id, requiere_revision, creado_en)
pago(id, empresa_id, cuenta_id, tipo pago|condonacion, monto_centavos, fecha, nota, registrado_por,
     anulado, anulado_por, anulado_en, motivo_anulacion, creado_en)
  -- "movimiento" en la API. En condonaciones, `nota` guarda el motivo.
ajuste_cuenta(id, empresa_id, cuenta_id, monto_anterior_centavos, monto_nuevo_centavos, motivo,
              solicitado_por, creado_en, estado pendiente|aprobado|rechazado, revisado_por, revisado_en)
reasignacion_cartera(id, empresa_id, vendedor_origen, vendedor_destino, cantidad_clientes,
                     cantidad_oportunidades, cantidad_cuentas, motivo, realizado_por, creado_en)
alerta_generada(id, empresa_id, cuenta_id, tipo por_vencer|vencido, fecha_generada, creado_en)
  UNIQUE(cuenta_id, tipo, fecha_generada)
schema_migrations(version PK, aplicada_en)
```
Agregado por la fase 3 (migraciones/003_cobranza_en_ruta.sql):
```
empresa                 + nit, nrc, direccion, telefono            -- salen impresos en los PDF
empresa_config          + metodos_pago TEXT[]='{efectivo,transferencia,cheque,deposito,tarjeta}',
                          exigir_liquidacion_cobrador=true, permitir_adjuntos=true,
                          tamano_maximo_adjunto_mb=10, prefijo_recibo='REC-',
                          prefijo_nota_credito='NC-', prefijo_anticipo='ANT-', prefijo_liquidacion='LIQ-'
cliente                 + codigo_externo          -- UNIQUE parcial (empresa_id, codigo_externo)
cuenta                  + numero_documento        -- UNIQUE parcial (empresa_id, numero_documento)
pago                    + metodo, referencia, numero, liquidacion_id, anticipo_id
                          tipo ahora: pago | condonacion | nota_credito
correlativo(empresa_id, tipo, siguiente)          -- PK (empresa_id, tipo); numera los documentos
anticipo(id, empresa_id, numero, cliente_id, monto_centavos, fecha, metodo, referencia, nota,
         registrado_por, liquidacion_id, anulado, anulado_por, anulado_en, motivo_anulacion, creado_en)
liquidacion(id, empresa_id, numero, cobrador_id, fecha, estado entregada|aprobada|rechazada,
            total_esperado_centavos, total_entregado_centavos, cantidad_movimientos, nota,
            revisado_por, revisado_en, motivo_rechazo, creado_en)
adjunto(id, empresa_id, entidad, entidad_id, nombre, tipo_mime, tamano_bytes, archivo,
        descripcion, subido_por, creado_en)       -- entidad: cuenta|movimiento|liquidacion|anticipo|cliente
```
Agregado por 007_promesas.sql:
```
promesa(id, empresa_id, cliente_id, cuenta_id NULL, fecha_promesa, monto_centavos NULL, nota,
        registrado_por, anulada, anulada_por, anulada_en, motivo_anulacion, creado_en)
  -- el estado (pendiente|cumplida|rota|anulada) NO se almacena: lo calcula reglas_promesas.go
```
Agregado por 009_firma.sql:
```
adjunto + clase 'comprobante'|'firma'   -- la firma se dibuja DENTRO del recibo, no se lista aparte
```
Agregado por 005_planes.sql (los planes comerciales son configuración, no código):
```
empresa        + plan                 -- nombre comercial del plan; informativo
empresa_config + limite_usuarios=0    -- máximo de usuarios ACTIVOS; 0 = sin límite
```
- Las cuatro tablas nuevas llevan RLS habilitado y forzado (si no, la app no arranca).
- `correlativo` se siembra para las empresas existentes ANTES de activar RLS en la migración:
  mientras corre el migrador no hay `app.empresa_id` y la política rechazaría el INSERT.
- El archivo del adjunto vive en disco (`RUTA_ARCHIVOS/<empresa_id>/...`); la columna `archivo`
  nunca se expone en la API.
- Cada tabla de negocio tiene `UNIQUE(empresa_id, id)` para permitir FKs compuestas.
- Índices: todos empiezan por `empresa_id` (p. ej. `pago(empresa_id, cuenta_id)`,
  `cuenta(empresa_id, cliente_id)`, `cuenta(empresa_id, vendedor_id)`).
- FKs con `ON DELETE RESTRICT`. Nada se borra: clientes se desactivan, movimientos se anulan.

Columnas agregadas respecto al documento original (por el principio rector / auditoría):
- `empresa_config.cuentas_evaluadas_comportamiento` (10): la regla 43 fijaba "últimas 10 cuentas";
  es un parámetro que una empresa podría querer distinto.
- `reasignacion_cartera.cantidad_oportunidades`: la regla 41 mueve oportunidades; se registra cuántas.
- `alerta_generada.creado_en`: marca de auditoría de la entrega.

---------------------------------------------------------------------------------------------------
## 7. Reglas de negocio (47). (config) = se lee de empresa_config, nunca constante en código
---------------------------------------------------------------------------------------------------
**Crédito y clientes**
1. Cada cliente tiene su plazo de crédito en días.
2. El plazo debe estar en `plazos_credito_permitidos` (config) → si no, 422 `PLAZO_NO_PERMITIDO`.
3. Los clientes no se eliminan, se desactivan. Con cuentas abiertas (saldo > 0) → 422 `CLIENTE_CON_SALDO`.
4. Límite de crédito por cliente; 0 = sin límite.
5. Al crear cuenta, si hay límite y (saldos abiertos + monto nuevo) > límite:
   `exigir_limite_credito` (config) true → 422 `LIMITE_CREDITO_EXCEDIDO`;
   false → se crea y la respuesta trae `advertencia` con el detalle del exceso.

**Venta y conversión**
6. Etapas: nuevo, contactado, negociacion, ganado, perdido.
7. Pasar a `ganado` NO crea la cuenta: la respuesta trae `conversion` precargada
   (cliente, monto sugerido, fecha de emisión, fecha de vencimiento calculada).
8. La cuenta se crea con `POST /api/oportunidades/{id}/convertir`, que vincula `oportunidad.cuenta_id`.
   Acepta `cliente_id` para emitirla a otro cliente (dentro del alcance del usuario; se aplican plazo,
   límite y estado de ESE cliente). La oportunidad pasa a quedar asociada a ese mismo cliente.
9. Una oportunidad se convierte una sola vez → 422 `OPORTUNIDAD_YA_CONVERTIDA`.
10. Una cuenta puede existir sin oportunidad (`POST /api/cuentas`, venta directa).

**Cuenta por cobrar**
11. Vencimiento = emisión + plazo del cliente; se puede enviar explícito (entonces no se valida el plazo).
12. Saldo = monto total − Σ(pagos y condonaciones NO anulados). Nunca se almacena.
13. Estado (nunca se almacena): `pagado` si saldo ≤ 0; `vencido` si hoy > vencimiento;
    `por_vencer` si faltan ≤ `dias_aviso_previo` (config) días; `al_dia` en otro caso.
14. "Hoy" se evalúa en `zona_horaria` (config), nunca en UTC ni zona del servidor.

**Pagos**
15. Monto > 0 (400 `MONTO_INVALIDO`).
16. Fecha no futura (422 `FECHA_FUTURA`).
17. Pagos parciales permitidos; registrar un pago nunca toca `monto_total_centavos`.
18. `permitir_sobrepago` (config) false y monto > saldo → 422 `SOBREPAGO` con detalle
    `saldo_pendiente_centavos` y `monto_recibido_centavos`. Si true, se acepta y el saldo queda negativo.
19. Todo pago guarda `registrado_por` = usuario autenticado.

**Condonaciones**
20. Movimiento `tipo='condonacion'`: reduce saldo sin representar dinero recibido.
21. Solo admin (403 `SOLO_ADMIN`) y solo si `permitir_condonaciones` (config) (422 `CONDONACIONES_DESHABILITADAS`).
22. Exige motivo con `longitud_minima_motivo` (config) (400 `MOTIVO_INSUFICIENTE`).
23. Nunca supera el saldo pendiente (422 `CONDONACION_EXCEDE_SALDO`). Si no se envía monto, condona el saldo completo.
24. Las condonaciones se excluyen del comportamiento de pago.

**Anulación**
25. Pagos y condonaciones nunca se borran: se anulan (`POST /api/movimientos/{id}/anular`).
26. Motivo obligatorio si `exigir_motivo_anulacion` (config), con `longitud_minima_motivo` (config).
27. Anulado deja de contar para el saldo, pero sigue en el historial con motivo, quién y cuándo.
28. Solo admin. Anular dos veces → 422 `MOVIMIENTO_YA_ANULADO`.

**Ajuste de monto**
29. Se puede editar el monto aunque haya pagos.
30. Exige motivo con longitud mínima (config).
31. El monto nuevo aplica de inmediato; la revisión es control posterior, no bloqueo.
32. Queda `ajuste_cuenta` (anterior, nuevo, motivo, solicitante). Si `exigir_revision_ajuste_monto`
    (config): ajuste `pendiente` y `cuenta.requiere_revision = true`. Si es false: el ajuste se
    registra directamente `aprobado` (revisado_por NULL = aprobación automática).
33. Admin aprueba o rechaza. Rechazar revierte el monto al anterior. `requiere_revision` vuelve a
    false cuando la cuenta ya no tiene ajustes pendientes.
34. Monto nuevo < total aplicado (pagos + condonaciones no anulados) → 422 `MONTO_MENOR_A_PAGADO`.
    También aplica al revertir por rechazo.

**Visibilidad y roles**
35. Dos roles: `admin`, `vendedor`.
36. Admin ve toda su empresa.
37. Vendedor ve solo clientes, oportunidades y cuentas con su `vendedor_id`.
38. Recurso ajeno para vendedor ⇒ 404, no 403.
39. El scoping se resuelve en los repos (`Alcance`), no en los handlers.

**Reasignación de cartera**
40. Admin transfiere la cartera completa de un vendedor a otro usuario activo en una operación.
41. Se mueven: todos sus clientes, oportunidades abiertas (nuevo/contactado/negociacion y ganado
    sin convertir) y cuentas con saldo > 0. Las cuentas pagadas conservan su vendedor (historial).
42. Se registra en `reasignacion_cartera` (origen, destino, cantidades, motivo, quién).

**Comportamiento de pago**
43. Promedio de días de atraso de las últimas `cuentas_evaluadas_comportamiento` (config, 10)
    cuentas pagadas, ordenadas por fecha del último pago.
44. Atraso = fecha del último movimiento tipo `pago` − vencimiento, mínimo 0. Solo cuentan pagos.
    Una cuenta saldada solo con condonaciones no se considera.
45. `bueno` si promedio < `umbral_comportamiento_bueno`; `regular` si ≤ `umbral_comportamiento_regular`;
    `malo` si lo supera; `sin_historial` si hay menos de `minimo_cuentas_para_historial` (config).
    La API devuelve número + clasificación; la interfaz decide cómo mostrarlo.

**Alertas**
46. El backend no envía mensajes. `GET /api/alertas` expone las cuentas `por_vencer` o `vencido` con
    saldo > 0 y si ya se entregaron hoy; `POST /api/alertas/marcar` registra la entrega en
    `alerta_generada` (idempotente por cuenta+tipo+día) para que n8n no repita el aviso.

**Multi-empresa**
47. Toda consulta confinada a la empresa del usuario (app + BD). El mismo correo puede existir en
    dos empresas. Alta de empresas solo por `go run . crear-empresa`.

---------------------------------------------------------------------------------------------------
### Reglas de la fase 3 (48-75)
---------------------------------------------------------------------------------------------------
**Usuarios**
48. El admin crea y edita usuarios desde la app. Correo único por empresa (422 `CORREO_YA_USADO`);
    contraseña de 8 a 72 caracteres (400 `CONTRASENA_CORTA` / `CONTRASENA_LARGA`). El correo no se
    puede cambiar después (422 `CORREO_NO_EDITABLE`): es la identidad del usuario.
49. Los usuarios no se borran: se desactivan. Un usuario desactivado no ingresa, pero su historial
    (quién cobró, quién anuló) se conserva.
50. La empresa siempre conserva al menos un administrador activo: desactivar o degradar al último
    → 422 `ULTIMO_ADMIN`. El admin reinicia contraseñas ajenas; cada quien cambia la propia
    verificando la actual.

**Documentos y formas de pago**
51. La cuenta puede llevar `numero_documento` (la factura). Si se repite dentro de la empresa →
    422 `DOCUMENTO_DUPLICADO`. Vacío se permite y no choca (índice único parcial).
52. Todo pago lleva forma de pago; debe estar en `metodos_pago` (config) → 422 `METODO_NO_PERMITIDO`.
    Vacío = la primera configurada. `referencia` guarda el número de transferencia, cheque o depósito.
53. Cada documento se numera con su correlativo por empresa y tipo (`correlativo`), con el prefijo
    de `empresa_config`: recibo (pagos), nota de crédito, anticipo y liquidación. El número se
    reserva dentro de la misma transacción que crea el documento: no hay saltos ni repetidos.

**Notas de crédito**
54. Movimiento `tipo='nota_credito'`: rebaja el saldo como la condonación, pero es un documento
    comercial (devolución, descuento posterior, error de facturación), no un perdón de deuda.
    Se informa aparte en `total_nota_credito_centavos`.
55. Solo admin, exige motivo con `longitud_minima_motivo` (config) y nunca supera el saldo
    (422 `NOTA_CREDITO_EXCEDE_SALDO`). Sin monto, aplica el saldo completo.
56. No es dinero recibido: no entra en la liquidación del cobrador ni en el comportamiento de pago.

**Anticipos**
57. Dinero que el cliente entrega sin factura asignada. Se numera, lleva forma de pago y queda
    como saldo a favor: `disponible = monto − aplicado`.
58. Aplicar un anticipo crea un movimiento `pago` con `metodo='anticipo'` y `anticipo_id`, y la
    referencia apunta al número del anticipo. Nunca supera lo disponible
    (422 `ANTICIPO_EXCEDE_DISPONIBLE`) ni se aplica a un cliente distinto
    (422 `ANTICIPO_DE_OTRO_CLIENTE`).
59. Un anticipo con aplicaciones no se anula (422 `ANTICIPO_YA_APLICADO`); anularlo dos veces
    tampoco (422 `ANTICIPO_YA_ANULADO`). Solo admin.
60. Orden de bloqueo para evitar interbloqueos: anticipo → cuenta.

**Liquidación del cobrador**
61. El corte reúne los pagos y anticipos del cobrador que todavía no están en ninguna liquidación,
    con fecha hasta hoy, y los desglosa por forma de pago. Las aplicaciones de anticipo no entran:
    ahí no hay dinero nuevo.
62. Al cerrar, el cobrador declara cuánto entrega. Si no coincide con lo esperado, debe explicarlo
    (400 `EXPLICACION_REQUERIDA`). Sin cobros pendientes no hay nada que cerrar
    (422 `SIN_COBROS_PENDIENTES`).
63. El corte bloquea los pagos (`FOR UPDATE`) antes de calcular: dos cierres simultáneos no pueden
    incluir el mismo cobro.
64. El admin aprueba o rechaza. Rechazar libera los cobros: vuelven a aparecer en el próximo corte.
    Una liquidación resuelta no se resuelve otra vez (422 `LIQUIDACION_YA_RESUELTA`).
65. `exigir_liquidacion_cobrador` (config) decide si la empresa usa este flujo; con false, la
    interfaz no muestra el apartado (el backend sigue respondiendo).

**Adjuntos**
66. Se adjuntan fotos (JPG/PNG) y PDF a una cuenta, movimiento, liquidación, anticipo o cliente,
    si `permitir_adjuntos` (config). El tope por archivo es `tamano_maximo_adjunto_mb` (config).
67. El tipo se decide por el CONTENIDO del archivo, no por su nombre ni por lo que declare el
    navegador (422 `TIPO_ARCHIVO_INVALIDO`). Además, una imagen debe poder decodificarse completa
    (400 `IMAGEN_ILEGIBLE`): una subida cortada conserva la cabecera válida y solo se delata al
    leer los datos.
68. El archivo se sirve solo a una sesión de la misma empresa; para otra empresa es 404.

**Documentos impresos**
69. Recibo (`/api/movimientos/{id}/recibo.pdf`): número, datos fiscales de la empresa, monto en
    letras, saldo anterior y nuevo, forma de pago y espacio de firmas. Anexa las fotos del
    movimiento.
70. Estado de cuenta (`/api/clientes/{id}/estado-cuenta.pdf`): antigüedad de saldos, documentos
    pendientes, anticipos a favor y comportamiento de pago; con `detalle=true` agrega los
    movimientos y con `fotos=true` anexa las fotos de las cuentas pendientes. Una foto ilegible se
    omite: nunca impide imprimir el documento.

**Importación de cartera**
71. La plantilla CSV se descarga de la propia API. Las columnas se leen por nombre, no por orden;
    falta una → 400 `COLUMNA_FALTANTE`.
72. Siempre hay dos pasadas: revisar (`aplicar` ausente) y aplicar (`aplicar=true`). La revisión
    informa fila por fila qué se creará y qué tiene error, sin detenerse en el primer problema.
73. El cliente se identifica por `codigo_externo` y el documento por `numero_documento`: un código
    ya existente reutiliza el cliente y un documento ya cargado se omite. Reimportar el mismo
    archivo no duplica nada.
74. Lo ya abonado entra como un pago histórico con la fecha de emisión, para que el saldo quede
    igual al del control que traía la empresa.

**Tablero**
75. `GET /api/tablero` devuelve en una sola petición: antigüedad de saldos, proyección de cobro a
    7/15/30 días, cobranza de hoy/semana/mes/mes anterior, serie diaria de 30 días (completa, con
    los días sin cobranza en cero), saldo que se fue en notas de crédito y condonaciones, y
    —solo para admin— el desempeño por cobrador y cuántas liquidaciones esperan aprobación.
    El vendedor recibe lo mismo acotado a su cartera, sin el apartado de cobradores.

**Informe mensual y planes**
76. `GET /api/informe-mensual.pdf[?mes=AAAA-MM]` arma el informe del dueño: cobrado del mes contra
    el anterior, cartera de hoy, antigüedad, cómo entró el dinero, cobradores, cuentas emitidas,
    notas de crédito y condonaciones con quién las autorizó, y los clientes que más deben.
    Sin `mes`, el mes en curso (hasta hoy). Un mes futuro → 422 `MES_FUTURO`; mal escrito → 400
    `MES_INVALIDO`. Los "puntos de atención" son HECHOS derivados (porcentaje vencido,
    concentración, saldos de más de 90 días, faltantes de liquidación), nunca recomendaciones.
    La actividad es del mes; la cartera es la foto de hoy, y así se rotula: reconstruir la cartera
    de un mes cerrado exigiría un historial que no se guarda.
77. `empresa_config.limite_usuarios` (0 = sin límite) es el máximo de usuarios ACTIVOS del plan
    contratado. Se verifica al crear y al reactivar, no al editar → 422 `LIMITE_USUARIOS`.
    `empresa.plan` guarda el nombre comercial del plan y es informativo: lo que cada plan habilita
    son las banderas de `empresa_config`. Cambiar de plan es un PATCH, nunca un despliegue.

---------------------------------------------------------------------------------------------------
### Paginación y búsqueda (78)
---------------------------------------------------------------------------------------------------
78. `GET /api/cuentas` y `GET /api/clientes` devuelven UNA página: `?pagina=` (1 por defecto) y
    `?por_pagina=` (50 por defecto, máximo 200 — `PorPaginaPorDefecto` y `PorPaginaMaximo` en
    `reglas.go`, límites de plataforma, no de empresa). Un valor que no sea entero mayor que cero,
    o un tamaño por encima del máximo → 400 `PARAMETRO_INVALIDO`. La respuesta agrega
    `paginacion: {pagina, por_pagina, total, paginas, desde, hasta, hay_mas}`.
    `?buscar=` filtra: en cuentas por número de documento o nombre del cliente; en clientes por
    nombre, código externo o teléfono. `GET /api/clientes?con_cuentas=true` agrega a cada cliente
    `cuentas_abiertas_lista` con la versión REDUCIDA de la cuenta (`CuentaEnCartera`): es lo que
    usa la pantalla del cobrador; mandar la cuenta completa multiplicaba por nueve la respuesta.
    Para filtrar por estado y paginar dentro de la base, las fórmulas del saldo (R12) y del estado
    (R13) están escritas TAMBIÉN en SQL, en `repos_paginacion.go` (`expresionSaldoSQL` y
    `condicionEstadoSQL`). Es una duplicación deliberada y acotada a esas dos expresiones: las
    filas que vuelven se re-evalúan con `EvaluarCuenta`, así que lo que ve el usuario sigue
    saliendo de `reglas.go`. Quien cambie una de las dos fórmulas DEBE cambiarla en los dos lados;
    la sección 13 de `verificar_cobranza.sh` compara el conteo por estado de `/api/resumen`
    (calculado en Go) contra el `total` de `/api/cuentas?estado=…` (calculado en SQL) y falla si
    dejan de coincidir.

---------------------------------------------------------------------------------------------------
### Comprobantes como documento (79)
---------------------------------------------------------------------------------------------------
79. Un adjunto no es un archivo suelto: es un documento con `titulo` (obligatorio, hasta 120
    caracteres → 400 `TITULO_REQUERIDO`), `descripcion`, quién lo subió y cuándo. Esos cuatro datos
    se imprimen junto a la foto en todo PDF donde aparezca. El título es obligatorio a propósito:
    un archivo llamado `IMG_20261017_142233.jpg` no le dice nada a nadie dentro de seis meses, que
    es justo cuando hay que buscarlo para discutir un saldo.
    `GET /api/adjuntos/{id}/documento.pdf` entrega UN comprobante como PDF; si el adjunto ya era un
    PDF se devuelve tal cual, porque volver a envolverlo solo degradaría el original.
    `GET /api/comprobantes.pdf?entidad=&entidad_id=` entrega TODOS los de un registro en un solo
    documento: una hoja índice con quién subió cada uno, y después una página por foto.
    fpdf no puede incrustar PDF ajenos, así que los adjuntos que ya venían en PDF se listan en el
    índice y se entregan aparte; el documento lo dice en lugar de omitirlos en silencio.
    El acceso es el del registro al que cuelgan: para otra empresa, 404.

---------------------------------------------------------------------------------------------------
### Cobranza en la calle (80-86)
---------------------------------------------------------------------------------------------------
80. **Mensaje de cobro por WhatsApp.** `GET /api/clientes/{id}/mensaje-cobro` devuelve
    `{telefono, texto, url}`. El sistema NO envía nada: arma el mensaje y el enlace `wa.me`, y lo
    manda la persona desde su propio WhatsApp. No se usa la API de WhatsApp ni se contrata un
    intermediario. El texto lo redacta el BACKEND —ahí viven el formato del dinero, las etiquetas
    y la zona horaria—; el navegador solo abre el enlace. Lleva saludo, nombre comercial de la
    empresa, saldo total, el detalle de los documentos vencidos (número, vencimiento, días de
    atraso y saldo) y un cierre cordial; de usted, sin emojis y sin signos de admiración. Sin saldo
    vencido es un estado de cuenta cordial, no un reclamo. La lista se recorta a 12 documentos y el
    resto se resume en una línea. El teléfono se normaliza a solo dígitos y, si viene sin código de
    país, se le antepone el de El Salvador (503, constante en `handlers_exportacion.go`: el día que
    se venda fuera del país se muda a `empresa_config`). Cliente sin teléfono → 422
    `CLIENTE_SIN_TELEFONO`. El cliente se busca con el alcance del usuario (ajeno ⇒ 404), pero sus
    cuentas se leen completas, igual que el estado de cuenta (R70): el saldo que se le informa al
    cliente es el que él debe.
81. **Exportación a Excel.** `GET /api/cuentas.csv?estado=&buscar=` entrega la cartera con los
    mismos filtros, alcance y búsqueda que `GET /api/cuentas` (R78), y `GET /api/antiguedad.csv`
    una fila por cliente con su saldo repartido en los tramos de `AntiguedadSaldos` y el total de
    `SaldosPorCliente` — las mismas funciones del tablero y del estado de cuenta: el CSV nunca
    cuenta distinto que el resto del sistema. Los dos archivos empiezan con BOM UTF-8
    (los bytes EF BB BF): sin él, Excel en Windows abre los acentos rotos y el cliente concluye que
    el sistema está mal. Separador coma, decimales con punto, montos como número plano sin símbolo
    (con la moneda en su propia columna) para que Excel los sume, y fechas en `AAAA-MM-DD`. El CSV
    de cartera informa Pagado, Condonado y Nota de crédito por separado para que
    Monto − aplicado = Saldo cuadre en la hoja (R12). Más de 20 000 filas → 422
    `EXPORTACION_DEMASIADO_GRANDE`: lo que se necesita ahí es un filtro, no un archivo más grande.
82. **Firma del cliente en el recibo.** `POST /api/movimientos/{id}/firma` recibe el PNG que el
    cliente trazó con el dedo y lo guarda como adjunto de `clase='firma'` (solo PNG, hasta 2 MB; el
    título lo pone el sistema —"Firma del cliente"— porque quien firma no va a escribirlo). Un
    movimiento tiene UNA firma: volver a firmar reemplaza la anterior y borra su archivo. La firma
    se dibuja DENTRO del recibo, sobre la línea de "Recibido por el cliente", a escala y sin
    deformarla; no se anexa como página, no entra en el paquete de comprobantes, no aparece en el
    listado de adjuntos y `/api/adjuntos/{id}/documento.pdf` de una firma responde 404: una firma
    sin su recibo encima no comprueba nada. Una firma ilegible se omite y el recibo se imprime
    igual (mismo aislamiento con `recover` que las demás imágenes, §10.b).

**Promesa de pago**
83. El cobrador anota lo que el cliente se comprometió a pagar: `POST /api/clientes/{id}/promesas`
    con la fecha prometida y, opcionalmente, el monto, la factura y lo que dijo. La fecha no puede
    ser anterior a hoy (422 `FECHA_PASADA`); la cuenta indicada debe ser del mismo cliente
    (422 `CUENTA_DE_OTRO_CLIENTE`). El cliente debe estar activo y dentro del alcance del usuario.
84. El estado de una promesa NUNCA se almacena: se calcula, igual que el saldo y el estado de una
    cuenta. `cumplida` si entre el día en que se anotó y la fecha prometida, ambos inclusive, el
    cliente pagó al menos lo prometido (sin monto prometido basta cualquier pago); `pendiente` si
    todavía no llega la fecha y no alcanzó; `rota` si la fecha ya pasó y no alcanzó; `anulada` no
    cuenta para nada. Solo suman los movimientos `tipo='pago'` no anulados: una condonación o una
    nota de crédito rebajan el saldo, pero no son el cliente cumpliendo su palabra. Si la promesa
    apunta a una cuenta, solo cuentan los pagos de esa cuenta. "Hoy" se evalúa en `zona_horaria`
    (config), igual que R14, y el día en que se anotó también.
85. `GET /api/promesas/agenda` es la agenda del día: las promesas no anuladas y no cumplidas cuya
    fecha ya llegó o ya pasó, de la cartera del usuario, de la más atrasada a la de hoy.
    `GET /api/promesas` devuelve una página del historial (R78) con `?estado=` y `?cliente_id=`.
86. Una promesa no se borra: se anula (`POST /api/promesas/{id}/anular`) con motivo de
    `longitud_minima_motivo` (config) cuando `exigir_motivo_anulacion`. Anularla dos veces → 422
    `PROMESA_YA_ANULADA`. Solo puede anularla quien la anotó o un administrador (403
    `PROMESA_DE_OTRO_USUARIO`); para un vendedor, la promesa de un cliente ajeno es 404 (R38).

---------------------------------------------------------------------------------------------------
## 8. Resolución de tenant y autenticación
---------------------------------------------------------------------------------------------------
- Login: `{email, password, slug}` (si falta `slug`, se toma de `X-Empresa-Slug`/subdominio).
  bcrypt. Mensaje de error genérico (no revela si existe la empresa o el correo).
- Sesión: token aleatorio de 32 bytes en `map` en memoria con `sync.Mutex`, cookie `sesion`
  HttpOnly, SameSite=Lax, Secure fuera de dev, duración 12 h. Sin JWT. Reiniciar la app cierra sesiones.
- `resolverEmpresa` (tenant.go) es el ÚNICO lugar que decide la empresa: hoy la fuente es la sesión;
  `X-Empresa-Slug` o el subdominio (`<slug>.<DOMINIO_BASE>`) solo pueden confirmarla, si difieren → 403
  `EMPRESA_NO_COINCIDE`. Para migrar a subdominio como fuente principal se cambia solo esa función.
- `conTenant` (middleware.go): sesión → `resolverEmpresa` → `BEGIN` → `set_config(app.empresa_id)` →
  recarga usuario (activo, rol actual, empresa activa) → handler → `COMMIT` → recién ahí escribe
  la respuesta. Si el handler devuelve error, `ROLLBACK`.

---------------------------------------------------------------------------------------------------
## 9. API
---------------------------------------------------------------------------------------------------
```
GET    /api/salud                      sin sesión: {estado, base_de_datos, version, segundos_en_pie}
GET    /api/publico/marca?slug=        sin sesión: logo, colores, nombre del producto
POST   /api/login | POST /api/logout | GET /api/me
GET    /api/empresa/config             {empresa, marca, config}
PATCH  /api/empresa/config             admin. {marca:{...parcial}, config:{...parcial}}
GET    /api/usuarios                   admin. Lista usuarios (necesario para elegir destino de reasignación)
GET    /api/resumen?top=5              totales, cuentas por estado, top deudores (vendedor: su cartera)
GET    /api/clientes[?incluir_inactivos=true&buscar=&con_cuentas=true&pagina=&por_pagina=]
                                       | POST /api/clientes
GET    /api/clientes/{id}              datos, saldo, cuentas, comportamiento, límite y disponible
PATCH  /api/clientes/{id}              (limite_credito y vendedor_id: solo admin)
POST   /api/clientes/{id}/desactivar
GET    /api/oportunidades              agrupadas por etapa | POST /api/oportunidades
PATCH  /api/oportunidades/{id}         si queda en 'ganado' responde `conversion` precargada
POST   /api/oportunidades/{id}/convertir  {cliente_id?, monto_total_centavos?, fecha_emision?, fecha_vencimiento?}
GET    /api/cuentas?estado=todas|al_dia|por_vencer|vencido|pagado[&buscar=&pagina=&por_pagina=]
                                       | POST /api/cuentas
GET    /api/cuentas/{id}               cuenta + movimientos (incluye anulados) + ajustes
PATCH  /api/cuentas/{id}/monto         {monto_total_centavos, motivo}
POST   /api/cuentas/{id}/pagos         {monto_centavos, fecha?, nota?}
POST   /api/cuentas/{id}/condonar      admin. {monto_centavos?, motivo, fecha?}
POST   /api/movimientos/{id}/anular    admin. {motivo}
GET    /api/ajustes?estado=pendiente|aprobado|rechazado|todos    admin
POST   /api/ajustes/{id}/aprobar | /rechazar                      admin
POST   /api/vendedores/{id}/reasignar-cartera                     admin. {vendedor_destino_id, motivo}
GET    /api/alertas[?solo_pendientes=true] | POST /api/alertas/marcar {cuenta_ids:[...]}

  ── Fase 3 ──
GET    /api/tablero                    indicadores del tablero (ver R75)
POST   /api/usuarios                   admin. {nombre, email, rol?, activo?, password}
PATCH  /api/usuarios/{id}              admin. {nombre?, rol?, activo?}
POST   /api/usuarios/{id}/contrasena   admin. {password}
POST   /api/me/contrasena              {password_actual, password_nueva}
GET    /api/anticipos[?cliente_id=&solo_disponibles=true]
POST   /api/clientes/{id}/anticipos    {monto_centavos, metodo?, referencia?, fecha?, nota?}
POST   /api/anticipos/{id}/aplicar     {cuenta_id, monto_centavos?}
POST   /api/anticipos/{id}/anular      admin. {motivo}
POST   /api/cuentas/{id}/nota-credito  admin. {monto_centavos?, motivo, fecha?}
GET    /api/liquidaciones/corte[?cobrador_id=]   lo cobrado y no entregado (admin puede ver a otro)
POST   /api/liquidaciones              {total_entregado_centavos, nota?}
GET    /api/liquidaciones[?estado=]  | GET /api/liquidaciones/{id}
POST   /api/liquidaciones/{id}/aprobar | /rechazar {motivo}        admin
GET    /api/adjuntos?entidad=&entidad_id=
POST   /api/adjuntos                   multipart: entidad, entidad_id, titulo?, descripcion?, archivo
DELETE /api/adjuntos/{id}              admin
GET    /api/adjuntos/{id}/archivo      el archivo, solo con sesión de la misma empresa
GET    /api/adjuntos/{id}/documento.pdf          un comprobante como PDF (R79)
GET    /api/comprobantes.pdf?entidad=&entidad_id=  todos los comprobantes de un registro en un PDF
GET    /api/movimientos/{id}/recibo.pdf
GET    /api/clientes/{id}/estado-cuenta.pdf[?detalle=true&fotos=true]
GET    /api/informe-mensual.pdf[?mes=AAAA-MM]    informe del dueño (R76)
POST   /api/clientes/{id}/promesas               {fecha_promesa, monto_centavos?, cuenta_id?, nota?}
GET    /api/promesas[?estado=&cliente_id=&pagina=&por_pagina=]
GET    /api/promesas/agenda                      lo prometido para hoy o antes, sin cumplir (R85)
POST   /api/promesas/{id}/anular                 {motivo}
GET    /api/clientes/{id}/mensaje-cobro          {telefono, texto, url} para WhatsApp (R80)
GET    /api/cuentas.csv[?estado=&buscar=]        la cartera para Excel (R81)
GET    /api/antiguedad.csv                       antigüedad por cliente para Excel (R81)
POST   /api/movimientos/{id}/firma               multipart: archivo (PNG). La firma del cliente (R82)
GET    /api/importacion/plantilla.csv  admin
POST   /api/importacion/cartera        admin. multipart: archivo, aplicar=true para confirmar
```
Las rutas que entregan archivos (PDF, CSV, adjuntos) responden con el binario; el resto, JSON.
`POST /api/cuentas` acepta `numero_documento` y `POST|PATCH /api/clientes` acepta `codigo_externo`.
`POST /api/cuentas/{id}/pagos` acepta `metodo` y `referencia`, y la respuesta trae el recibo numerado.
`GET /api/cuentas` y `GET /api/clientes` vienen paginados (R78): `pagina`, `por_pagina` y `buscar`;
la respuesta agrega `paginacion: {pagina, por_pagina, total, paginas, desde, hasta, hay_mas}`.
`GET /api/salud` no lleva sesión ni toca datos de ninguna empresa: 200 si la base responde, 503 si
no. La consultan el `HEALTHCHECK` del contenedor y el monitoreo externo. `version` la inyecta el
compilador (`-ldflags "-X main.version=..."`); por defecto es `dev`.
Errores: `{"error":{"codigo","mensaje","detalle"?}}` — 400 entrada inválida, 401 sin sesión,
403 rol/empresa, 404 no existe o no es tuyo, 422 regla de negocio, 500 interno.
Mensajes en español para usuario final. Montos en mensajes con `simbolo_moneda` de la empresa.

Catálogo de códigos 422: PLAZO_NO_PERMITIDO, CLIENTE_CON_SALDO, CLIENTE_INACTIVO,
LIMITE_CREDITO_EXCEDIDO, OPORTUNIDAD_YA_CONVERTIDA, OPORTUNIDAD_NO_GANADA, FECHA_FUTURA, SOBREPAGO,
CONDONACIONES_DESHABILITADAS, CONDONACION_EXCEDE_SALDO, CUENTA_SIN_SALDO, MOVIMIENTO_YA_ANULADO,
MONTO_MENOR_A_PAGADO, AJUSTE_YA_RESUELTO, AJUSTE_SUPERADO, USUARIO_INACTIVO.
Fase 3: CORREO_YA_USADO, CORREO_NO_EDITABLE, ULTIMO_ADMIN, DOCUMENTO_DUPLICADO, METODO_NO_PERMITIDO,
NOTA_CREDITO_EXCEDE_SALDO, ANTICIPO_EXCEDE_DISPONIBLE, ANTICIPO_DE_OTRO_CLIENTE, ANTICIPO_YA_APLICADO,
ANTICIPO_YA_ANULADO, ANTICIPO_ANULADO, ANTICIPO_SIN_SALDO, SIN_COBROS_PENDIENTES,
LIQUIDACION_YA_RESUELTA, TIPO_ARCHIVO_INVALIDO, ARCHIVO_DEMASIADO_GRANDE, ADJUNTOS_DESHABILITADOS,
ARCHIVO_CON_ERRORES, LIMITE_USUARIOS, MES_FUTURO, CLIENTE_SIN_TELEFONO,
EXPORTACION_DEMASIADO_GRANDE, FECHA_PASADA, CUENTA_DE_OTRO_CLIENTE, PROMESA_YA_ANULADA.
403 propios de la fase 4: PROMESA_DE_OTRO_USUARIO. 404: PROMESA_NO_ENCONTRADA. Códigos 400 propios: CONTRASENA_CORTA,
CONTRASENA_LARGA, EXPLICACION_REQUERIDA, IMAGEN_ILEGIBLE, COLUMNA_FALTANTE, CSV_INVALIDO,
MES_INVALIDO, PARAMETRO_INVALIDO, TITULO_REQUERIDO, FECHA_REQUERIDA.

---------------------------------------------------------------------------------------------------
## 10. Decisiones de la fase 1
---------------------------------------------------------------------------------------------------
- Paginado lo que crece con la cartera: `/api/cuentas` y `/api/clientes` traen una página desde la
  base (R78). `/api/resumen`, `/api/tablero` y el informe mensual siguen cargando toda la cartera
  del alcance y calculando en Go con `reglas.go`: ese es el próximo techo conocido, y cuando el
  volumen lo exija hay que agregarlos en SQL manteniendo las fórmulas de `reglas.go`.
- `Fecha` = medianoche UTC sin hora; diferencias de días exactas. "Hoy" siempre por `HoyEn(zona, ahora)`.
- Rechazar un ajuste solo es posible si es el último aplicado a la cuenta (el monto actual es su
  monto nuevo); si hubo otro ajuste después → 422 `AJUSTE_SUPERADO` (resolver primero el más reciente).
- Oportunidad convertida no puede salir de `ganado` (422 `OPORTUNIDAD_YA_CONVERTIDA`).
- Vendedor: al crear clientes/cuentas/oportunidades el `vendedor_id` es siempre él mismo; no puede
  cambiar `limite_credito_centavos` ni `vendedor_id` de un cliente (403 `SOLO_ADMIN`).
- Detalle de cliente para vendedor: lista solo sus cuentas, pero comportamiento y crédito disponible
  se calculan con todas las cuentas del cliente (son propiedades del cliente).
- Seed (`002_seed_desarrollo.sql`) usa fechas relativas a "hoy" en El Salvador al momento de sembrar;
  contraseñas con `crypt()` de pgcrypto (bcrypt `$2a$`, compatible con Go).
- El nombre del producto por defecto al provisionar es el nombre comercial.
- Duración de sesión (12 h) es de plataforma, no de empresa.
- Límites de PLATAFORMA (reglas.go, no por empresa): montos hasta $1,000,000,000.00 (`MontoMaximoCentavos`,
  evita desbordes de int64 y de los enteros exactos de JavaScript); fechas de negocio entre 1900-01-01 y
  2199-12-31; textos: motivo 500, notas/nota 1000, contacto 120, teléfono 40, nombre 200. El carácter
  nulo (\u0000) se rechaza al leer el JSON (Postgres no lo admite en texto).
- La conexión fuerza `plan_cache_mode=force_custom_plan`: con planes genéricos cacheados por pgx, tras una
  carga masiva /api/resumen llegó a tardar más de 30 s (timeout) hasta el siguiente autoanálisis.
- Escala medida con 5,000 cuentas y 200 clientes en una empresa: /api/cuentas 36 KB en 37 ms y
  /api/clientes 23 KB en 40 ms; la pantalla del cobrador (clientes con `con_cuentas=true` y
  `por_pagina=25`) pasó de 916 KB a 207 KB. /api/resumen 126 ms y /api/tablero 111 ms: esos dos no
  están paginados y crecen con la cartera.
- Límite de intentos de login: 5 fallos por cuenta y 30 por IP en 15 minutos → 429 DEMASIADOS_INTENTOS
  (`LimitadorIngreso` en auth.go, en memoria, purgado periódico desde main.go).

---------------------------------------------------------------------------------------------------
## 10.b Decisiones de la fase 3
---------------------------------------------------------------------------------------------------
- Saldo = monto − pagos − condonaciones − notas de crédito. La nota de crédito se agrega en el MISMO
  query que las otras dos (`selectCuenta`) y se resta en `CalcularSaldo`: es el único lugar donde se
  calcula el saldo de una cuenta, y por eso cualquier tipo nuevo de movimiento debe sumarse ahí.
- `totalAplicado` (regla 34) incluye las notas de crédito: no se puede bajar el monto de la cuenta
  por debajo de lo ya aplicado entre pagos, condonaciones y notas.
- Los adjuntos se guardan en disco, no en la base: una foto de 5 MB en una columna `bytea` multiplica
  el tamaño de los respaldos y hace lentas las consultas. En la base queda solo el metadato.
- Al subir una imagen se decodifica COMPLETA, no solo su cabecera. Antes de esto, un archivo cortado
  pasaba la validación y después hacía fallar el PDF donde se anexaba.
- fpdf entra en pánico (no devuelve error) con algunas imágenes dañadas, así que `registrarImagen`
  aísla cada foto con `recover`: un adjunto ilegible se omite y el documento se imprime igual.
- La serie diaria del tablero la completa el backend con los días en cero. El navegador nunca
  construye fechas (regla D de §5 y §11).
- El tablero atribuye la cobranza de dos maneras distintas y ambas se muestran etiquetadas: la serie
  y los totales van por la cartera (`cuenta.vendedor_id`), y el cuadro de cobradores por quien
  recibió el dinero (`pago.registrado_por`), que es lo que después tiene que liquidar.
- Los cortes de la proyección (7/15/30 días) son de presentación y viven en `handlers_tablero.go`,
  no en `empresa_config`: no cambian el resultado de ninguna regla de negocio.
- La importación usa dos pasadas con el mismo archivo (revisar y aplicar) en lugar de guardar la
  revisión en el servidor: lo que se aplica es exactamente lo que se mostró, sin estado intermedio.
- `ValidarConfig` también valida los campos de la fase 3. Sin eso, un PATCH podía dejar a la empresa
  sin formas de pago (ningún cobro se podría registrar) o con un tope de adjunto imposible.
  Las formas de pago son identificadores normalizados (minúsculas, sin espacios): la etiqueta
  visible la ponen `formato.js` y `EtiquetaMetodoPago`. `anticipo` está reservado para el sistema.
- Los planes comerciales son configuración, no código (el documento comercial se mantiene fuera
  del repositorio). El único límite que
  el sistema aplica es el de usuarios activos; el de cartera es comercial y no bloquea nada: a un
  cliente que crece no se le traba el sistema, se le llama.
- La fórmula del saldo (R12) y la del estado (R13) están escritas dos veces: en `reglas.go` y en
  SQL en `repos_paginacion.go`. Es la única manera de filtrar por estado y paginar sin traer la
  cartera entera. La duplicación es deliberada y acotada a esas dos expresiones; las filas que
  vuelven se re-evalúan con `EvaluarCuenta` y la sección 13 de `verificar_cobranza.sh` falla si
  los dos lados dejan de clasificar igual. Cambiar una fórmula en un solo lado es la trampa más
  fácil de este proyecto: por eso esa comprobación existe.
- El listado de clientes con `con_cuentas=true` manda `CuentaEnCartera`, no la cuenta completa:
  esa pantalla dibuja diez campos y mandar todo multiplicaba por nueve el peso de la respuesta.
- La fórmula del estado de una promesa (R84) está escrita dos veces, en `reglas_promesas.go` y en
  SQL en `repos_promesas.go`, por la misma razón que la del saldo: es la única manera de filtrar
  por estado y paginar en la base. La sección 8 de `verificar_promesas.sh` compara las dos formas
  de contar y falla si dejan de coincidir.
- Las empresas de demostración del seed son dos; la tercera (Lácteos La Cumbre) NO se siembra por
  migración, se da de alta con `alta_empresa_demo.sh`. Sembrarla sería la trampa cómoda: demostraría
  el aislamiento pero no que dar de alta un cliente no toca código. El script usa el subcomando
  `crear-empresa` y después solo la API, con la sesión de esa empresa, igual que lo haría el cliente.
- La base restaurada se crea con dueño `cxc_app` y se restaura CONSERVANDO el dueño de las tablas
  (sin `--no-owner`). Si las tablas quedan a nombre del superusuario la aplicación no arranca: el
  aislamiento depende de `FORCE ROW LEVEL SECURITY` sobre tablas propias. La restauración se probó
  de verdad: se levantó la aplicación contra la base restaurada y arranca, valida el RLS y responde.

---------------------------------------------------------------------------------------------------
## 11. Frontend provisional (Fase 2)
---------------------------------------------------------------------------------------------------
Dirección visual: **"Libreta"** — la estructura del libro de cuentas y del talonario, ejecutada con
limpieza moderna: papel crema, hojas con filete cálido, tablas como renglones contables, sellos de
hule para los estados, títulos en serif del sistema y cero sombras. De la dirección "Campo" se tomó
la legibilidad: cuerpo de 17px en teléfono, botones de 48px (56px el principal), filas de 72px y una
barra sólida de color a la izquierda de cada fila diciendo el estado. Las cuatro tintas tienen
significado fijo (azul = acción, ladrillo = vencido, oliva = al día, ámbar = por vencer).

HTML + CSS + JavaScript vanilla con módulos ES6. Sin frameworks, sin npm, sin build, sin CDN.
`main.go` (`sitio()`) sirve `static/` en la raíz y `/api/` sigue en `rutas()`: mismo origen, sin CORS.
Los archivos se leen de disco: cambiar el diseño no requiere recompilar.

Capas (la regla central: reemplazar el diseño = cambiar CSS y marcado, nunca la lógica):
| Archivo | Responsabilidad |
|---|---|
| `js/api.js` | ÚNICO archivo que conoce rutas de la API. Lanza `ErrorApi {estado, codigo, mensaje, detalle}`. |
| `js/estado.js` | Sesión, configuración, permisos de interfaz (`puede()`, `secciones()`), caché en memoria. |
| `js/formato.js` | ÚNICO lugar que convierte centavos ↔ texto; fechas y etiquetas. |
| `js/router.js` | Navegación por hash, sin conocer pantallas ni permisos. |
| `js/componentes.js` | Piezas de DOM reutilizables, iconos Lucide inline, `abrirModal` (estructura única). |
| `js/vistas/*.js` | Una vista por pantalla + `modales.js`. Nunca hacen fetch directo. |
| `js/vistas/modales_cobranza.js` | Formularios de la fase 3 (notas de crédito, anticipos, liquidación, usuarios, fotos, importación). |
| `js/vistas/liquidaciones.js` | Corte del cobrador, historial y detalle de una liquidación. |
| `js/vistas/configuracion.js` | Usuarios, importación de cartera y cambio de la propia contraseña. |
| `css/base.css` / `css/componentes.css` | Tokens en `:root`, layout y componentes. Clases por función. |

Reglas:
- La marca viene del backend: `/api/publico/marca` en el ingreso, `/api/empresa/config` al entrar;
  `pintarMarca` escribe `--color-primario` y `--color-primario-claro` en `:root`. Prohibido fijar en
  código colores de marca, símbolo de moneda, plazos o nombre del producto.
- La configuración gobierna la interfaz: `permitir_condonaciones` (botón condonar),
  `exigir_revision_ajuste_monto` (menú Revisiones), `plazos_credito_permitidos` (selector),
  `simbolo_moneda` (todo monto), `exigir_limite_credito` y `dias_aviso_previo` (textos).
- El navegador NUNCA calcula saldos ni estados; solo muestra, agrupa u ordena lo que envía la API.
  Tampoco calcula el vencimiento: en la conversión solo envía `fecha_vencimiento` si el usuario la editó.
- Mensajes de error del backend se muestran tal cual. SOBREPAGO ofrece "Abonar <saldo>" con un clic.
  `advertencia` en una respuesta = operación hecha (aviso amarillo), no error.
- Menú: admin → Resumen, Ventas en proceso, Cuentas, Clientes, Liquidación, Alertas
  (+ Revisiones si aplica), Configuración; vendedor → Mi cartera, Mi corte, Alertas.
  El apartado de liquidación aparece solo si `exigir_liquidacion_cobrador`, y el mismo nombre de
  ruta cambia de rótulo según el rol (el cobrador entrega SU corte).
  Ocultar opciones es cortesía; la seguridad está en el backend.
- Fase 3 en la interfaz: el Resumen es el tablero (indicadores, antigüedad de saldos, cobranza de
  30 días, cobradores y fuga del mes), dibujado con `barras`, `columnas` e `indicador` de
  `componentes.js` — SVG y CSS, sin librerías de gráficos. Ninguna de esas funciones suma ni
  clasifica: reciben lo que ya calculó el backend.
- Los PDF y el CSV se abren como enlaces (`botonEnlace`) a la propia API con la cookie de sesión:
  no se descargan por fetch ni se reconstruyen en el navegador.
- La foto se toma con `campoArchivo({ camara: true })` (`capture="environment"`), que en el teléfono
  abre la cámara trasera directamente.
- Layout único responsivo: menú lateral ≥768px, barra inferior fija debajo. Grids con `minmax(0, 1fr)`.

Decisiones de lenguaje y de interacción (fijadas por el cliente, no se cambian sin consultarlo):
- **Se trata al usuario de USTED**, en toda la aplicación y en los mensajes del backend. Nada de
  voseo ni de tuteo, aunque la maqueta de origen lo usara.
- **Una sola acción principal por ficha.** En la cartera del cobrador el botón de registrar pago va
  en la ficha del CLIENTE, no en cada factura: en la puerta se le cobra al cliente, y si debe
  varias facturas el modal pregunta a cuál se le aplica (`modalCobrarACliente`). Antes había un
  botón por factura y un cliente con cinco documentos llenaba la pantalla.
- Ningún icono va solo: todos llevan su palabra. Hay usuarios con poca alfabetización digital.
- El color de marca de la empresa vive solo en la banda del encabezado. El área de datos usa las
  tintas del sistema, para que una marca estridente no vuelva ilegible la cartera ni le quite su
  significado al rojo de vencido.
