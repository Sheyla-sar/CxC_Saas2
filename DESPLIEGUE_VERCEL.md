# DESPLIEGUE_VERCEL.md — correr todo el sistema en Vercel

Vercel corre la aplicación completa (API en Go + interfaz) con su **Container Runtime**, a partir de
`Dockerfile.vercel`. No hay que separar frontend y backend: es el mismo binario de siempre.

Como Vercel no tiene estado (una instancia se apaga tras 5 minutos sin tráfico y puede haber varias a la
vez), lo que antes vivía en la memoria o el disco del proceso ahora vive en PostgreSQL:

| Antes | Ahora | Cómo se activa |
|---|---|---|
| Sesiones en memoria | Tabla `sesion` (solo se guarda el SHA-256 del token) | Siempre |
| Límite de intentos de login en memoria | Tabla `intento_ingreso` | Siempre |
| Fotos y PDF en disco | Columna `adjunto.contenido` | `ALMACEN_ARCHIVOS=db` |

Con `ALMACEN_ARCHIVOS=disco` (el valor por defecto) el despliegue en VPS de `DESPLIEGUE.md` sigue igual.

## 1. Base de datos en Neon

1. Cree un proyecto en [neon.com](https://neon.com) (o desde Vercel → Storage → Marketplace → Neon).
2. En el SQL Editor, ejecute `neon_rol.sql` (cambie la contraseña). Crea el rol `cxc_app` y la base `cxc`.
   **No use el rol por defecto de Neon:** la aplicación se niega a arrancar con un rol que ignore Row Level Security.
3. Arme la `DATABASE_URL` con el host **sin** `-pooler`:

   ```
   postgres://cxc_app:SU_CLAVE@ep-xxxx.us-east-2.aws.neon.tech/cxc?sslmode=require
   ```

   Si usa la URL con `-pooler`, la aplicación cambia sola al protocolo simple de pgx y debería funcionar,
   pero es la ruta menos probada.

## 2. Crear la primera empresa (una sola vez, desde su computadora)

```bash
DATABASE_URL='postgres://cxc_app:...@.../cxc?sslmode=require' ENTORNO=produccion \
go run . crear-empresa --slug mi-empresa --nombre-legal "Mi Empresa, S.A. de C.V." \
  --nombre-comercial "Mi Empresa" --admin-nombre "Su Nombre" \
  --admin-email "usted@ejemplo.com" --admin-password "UnaClaveLarga123"
```

Este comando también aplica las migraciones. En producción no se siembran datos de prueba.

## 3. Desplegar en Vercel

1. Suba el repositorio a GitHub (incluya `Dockerfile.vercel`).
2. vercel.com → **Add New → Project** → importe el repositorio.
3. Si Vercel no selecciona **Container** como preset (el repo tiene `go.mod`, y puede detectar Go), cámbielo en
   Settings → General → Framework Preset. `Dockerfile.vercel` está pensado para ganar sobre la detección de Go.
   Con la CLI: `vercel project update --framework container --yes`.
4. En **Settings → Environment Variables** agregue:

   | Variable | Valor |
   |---|---|
   | `PORT` | `8080` (obligatoria: Vercel enruta al puerto 80 si no se define) |
   | `DATABASE_URL` | la URL del paso 1 |
   | `DOMINIO_BASE` | opcional, solo si usa subdominios por empresa |

   `ENTORNO=produccion`, `ALMACEN_ARCHIVOS=db` y `DB_MAX_CONEXIONES=5` ya vienen en `Dockerfile.vercel`.
5. **Deploy**. Después: `https://SU-PROYECTO.vercel.app/api/salud` debe responder `"base_de_datos":"ok"`.
6. Entre con el slug, correo y clave del paso 2.

## 4. Límites que debe conocer

- **Cuerpo de petición y respuesta: 4.5 MB** (límite de Vercel Functions). Una foto de celular de más de 4.5 MB
  se rechaza con 413 antes de llegar a la aplicación, y un PDF o un listado más grande que eso tampoco se entrega.
  El `DESPLIEGUE.md` calcula `/api/cuentas` en ~12 MB con 20,000 cuentas: para empresas de ese tamaño, Vercel no sirve
  sin antes paginar los listados.
- **Almacenamiento:** las fotos ahora ocupan espacio de la base. Neon gratis incluye 0.5 GB (unas 1,000 fotos de
  medio MB). Si va a crecer, plan de pago o un almacén de objetos.
- **Arranque en frío:** tras 5 minutos sin tráfico la instancia se apaga y la siguiente petición espera a que arranque
  (y a que Neon despierte, si estaba suspendido).
- **Container Runtime está en Beta** en Vercel.
- **Subdominios por empresa:** requieren agregar `*.sudominio.com` en Vercel (comodín) y `DOMINIO_BASE`.

## 5. Qué se probó y qué no

Probado localmente (PostgreSQL 16, Go 1.27) con los tres verificadores del repo: **277 comprobaciones, todas
correctas** con `ALMACEN_ARCHIVOS=db`. Además: una sesión creada en una instancia vale en otra instancia y sobrevive a un
reinicio; el límite de intentos se cuenta entre instancias (el 6.º fallo da 429); una foto subida se guarda en la
base, se baja idéntica desde otra instancia, se incrusta en el PDF y otra empresa no puede verla; en modo producción
el servidor escucha en `PORT`, no siembra datos de prueba y marca la cookie como `Secure`.

**No probado:** el build real de `Dockerfile.vercel` en Vercel, la conexión a Neon y el comportamiento del
Container Runtime. Si el primer despliegue falla, mire los logs de build y de ejecución de Vercel.
