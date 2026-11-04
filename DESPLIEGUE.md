# DESPLIEGUE.md — poner el sistema en un servidor

Guía para instalar y mantener el SaaS de cuentas por cobrar en un VPS. Supone que sabe usar una
terminal de Linux; no supone experiencia en administración de servidores.

Todo lo que necesita está en tres archivos del repositorio:

| Archivo | Qué es |
|---|---|
| `docker-compose.produccion.yml` | La pila: PostgreSQL, la aplicación y Caddy (proxy con HTTPS). |
| `Caddyfile` | Qué nombres de dominio se atienden y con qué cabeceras. |
| `.env.produccion.ejemplo` | Plantilla de las contraseñas y los dominios. Se copia a `.env`. |

> **Antes de ponerle datos reales a un cliente, lea §10.** Hay dos limitaciones que no son fallas
> sino decisiones conocidas, y conviene conocerlas antes y no después.

---

## 1. Qué servidor hace falta

Para 10-30 empresas cliente: **2 vCPU, 4 GB de RAM, 80 GB de disco SSD**, Ubuntu 24.04 LTS.
Cualquier VPS de los que cuestan entre 20 y 30 dólares al mes alcanza.

Por qué ese tamaño y no menos:

- **RAM.** El sistema no pagina los listados: `/api/cuentas` y `/api/resumen` cargan la cartera
  completa del usuario en memoria y la calculan en Go (CLAUDE.md §10). Con 20,000 cuentas en una
  empresa, una respuesta de `/api/cuentas` pesa unos 12 MB. Varios usuarios consultando a la vez
  más PostgreSQL con su propia caché: 2 GB queda justo, 4 GB deja margen.
- **CPU.** Dos núcleos: uno para la base y uno para la aplicación. El trabajo pesado son los PDF
  (recibos, estados de cuenta con fotos, informe mensual) y son ráfagas cortas.
- **Disco.** La base crece poco; lo que crece son las **fotos de los recibos**, que viven en disco
  y no en la base. Una foto de celular ronda el medio megabyte: 30 empresas con 50 cobros diarios
  con foto son unos 750 MB al mes. A eso se suman 30 días de respaldos locales. 80 GB dan margen
  para el primer año; vigile `df -h` (§8).

Cuando una sola empresa pase de las 20,000 cuentas, el cuello no será el servidor sino la falta de
paginación: ahí toca paginar los listados, no agrandar la máquina.

---

## 2. DNS

Necesita dos registros apuntando a la IP del servidor:

| Tipo | Nombre | Valor |
|---|---|---|
| A | `cobros.ejemplo.sv` | IP del VPS |
| A | `*.cobros.ejemplo.sv` | IP del VPS |

El comodín hace que `distribuidora-progreso.cobros.ejemplo.sv` resuelva sin tocar el DNS cada vez
que entra un cliente nuevo. Si el servidor tiene IPv6, agregue los dos registros `AAAA` equivalentes.

**El DNS resuelve el comodín, pero el certificado no.** Caddy sin módulos externos valida cada
nombre por HTTP, y para eso el nombre tiene que estar listado en la variable `DOMINIOS`. Un
certificado comodín exigiría validación por DNS, que necesita un Caddy compilado con el módulo del
proveedor de DNS. Por eso dar de alta una empresa incluye agregar su subdominio a `DOMINIOS`
(§5): es editar una variable y recrear un contenedor, no recompilar la aplicación.

Verifique antes de seguir (desde su máquina, no desde el servidor):

```bash
dig +short cobros.ejemplo.sv
dig +short cualquier-cosa.cobros.ejemplo.sv
```

Ambos deben devolver la IP del VPS. Si no, espere a que propague: Caddy no puede emitir
certificados hasta que Let's Encrypt vea esos nombres apuntando aquí.

---

## 3. Preparar el servidor

Como root, una sola vez:

```bash
# Actualizar y poner el reloj en hora (las fechas de negocio dependen de él).
apt update && apt upgrade -y
timedatectl set-timezone UTC

# Docker, del repositorio oficial.
curl -fsSL https://get.docker.com | sh

# Cortafuegos: solo SSH y web. La base de datos NO se publica al host, así que no hay
# puerto 5432 que proteger.
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
ufw --force enable
```

El puerto 80 debe quedar abierto aunque todo vaya por HTTPS: es por donde Let's Encrypt comprueba
que el dominio es suyo, cada vez que renueva.

Tenga presente que `ufw` no filtra los puertos que publica Docker: los abre por debajo, en sus
propias reglas de iptables. Aquí no es un problema porque los únicos puertos publicados son 80 y
443, que de todas formas van abiertos. Sí lo sería si alguien agregara un `ports:` a la base de
datos en el compose: quedaría expuesta a internet aunque `ufw` diga lo contrario. No lo haga.

---

## 4. Primer despliegue

```bash
mkdir -p /opt/cxc && cd /opt/cxc
# Copie aquí el repositorio (git clone, scp o rsync desde su máquina).

cp .env.produccion.ejemplo .env
chmod 600 .env
nano .env        # cambie las cinco variables obligatorias
```

Genere cada contraseña con `openssl` y péguela; no invente contraseñas a mano:

```bash
openssl rand -base64 32                                    # POSTGRES_PASSWORD
openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c 40  # CXC_APP_PASSWORD
```

La segunda se filtra a letras y números a propósito: va dentro de una URL de conexión
(`postgres://cxc_app:ESTA@postgres:5432/cxc`) y un `/` o un `+` habría que escaparlos.

Prepare la carpeta de los adjuntos. Dentro del contenedor la aplicación corre como el usuario
10001 (no root), así que la carpeta del host tiene que pertenecerle o no podrá guardar las fotos:

```bash
mkdir -p archivos respaldos
chown -R 10001:10001 archivos
```

Levante la pila:

```bash
docker compose -f docker-compose.produccion.yml up -d --build
```

La primera vez compila la imagen (un par de minutos), crea la base, el rol `cxc_app` y aplica las
migraciones. Revise que arrancó bien:

```bash
docker compose -f docker-compose.produccion.yml ps
docker compose -f docker-compose.produccion.yml logs aplicacion | tail -20
```

En el registro de la aplicación tiene que aparecer, en este orden:

```
migración aplicada: 001_esquema
...
aislamiento verificado: RLS habilitado y forzado en todas las tablas con empresa_id
servidor escuchando en http://localhost:8080 (entorno produccion)
```

Esa línea de "aislamiento verificado" es la comprobación de que ninguna empresa puede ver los
datos de otra. Si no aparece, la aplicación no arrancó: vaya a §9.

Compruebe desde fuera:

```bash
curl -s https://cobros.ejemplo.sv/api/salud
# {"base_de_datos":"ok","estado":"ok","segundos_en_pie":42,"version":"2026.10.17"}
```

Si responde pero sin HTTPS, el certificado todavía no está listo; mire `logs caddy` (§9).

---

## 5. Dar de alta una empresa

El alta es un comando, nunca un cambio de código. El binario dentro de la imagen es
`/aplicacion/cxc`:

```bash
docker compose -f docker-compose.produccion.yml exec aplicacion /aplicacion/cxc crear-empresa \
  --slug distribuidora-progreso \
  --nombre-legal "Distribuidora El Progreso, S.A. de C.V." \
  --nombre-comercial "Distribuidora El Progreso" \
  --admin-nombre "María López" \
  --admin-email "maria@progreso.sv" \
  --admin-password "clave-provisional-larga"
```

El `slug` es el identificador de la empresa: va en el ingreso y es el subdominio. Minúsculas,
números y guiones. No se puede cambiar después sin tocar la base, así que elíjalo bien.

La contraseña queda en el historial de la shell y a la vista de `ps`. Es provisional: la
administradora la cambia al entrar, en Configuración → "Cambiar mi contraseña". Después limpie el historial
con `history -c` o ponga un espacio delante del comando para que ni siquiera se guarde.

Luego agregue el subdominio para que tenga certificado:

```bash
nano .env   # DOMINIOS=cobros.ejemplo.sv distribuidora-progreso.cobros.ejemplo.sv
docker compose -f docker-compose.produccion.yml up -d caddy
```

Recrear el contenedor de Caddy corta el tráfico un par de segundos y pide el certificado nuevo.
Los certificados ya emitidos están en un volumen y no se vuelven a pedir.

Esto no es imprescindible: la empresa también funciona entrando por el dominio base, porque la
empresa se resuelve por la sesión. El subdominio es presentación y una segunda verificación.

---

## 6. Actualizar a una versión nueva, y volver atrás

**Antes de actualizar, saque un respaldo** (§7). Es el único camino de vuelta si una migración
cambia el esquema.

```bash
cd /opt/cxc
./respaldar.sh                 # primero esto
git pull                       # o copie los archivos nuevos
nano .env                      # suba VERSION, por ejemplo a 2026.11.03
docker compose -f docker-compose.produccion.yml up -d --build
curl -s https://cobros.ejemplo.sv/api/salud   # debe devolver la VERSION nueva
```

La actualización tarda unos segundos y **cierra todas las sesiones abiertas** (§10): hágala
temprano o tarde, no en media jornada de cobranza.

### Volver atrás

Cada despliegue deja su imagen etiquetada con la versión. Para volver a la anterior no se
recompila nada:

```bash
docker image ls cxc                 # ver qué versiones hay en el servidor
nano .env                           # VERSION=2026.10.17 (la anterior)
docker compose -f docker-compose.produccion.yml up -d --no-build aplicacion
```

**Ojo con las migraciones.** El migrador solo avanza: no deshace. Si la versión nueva aplicó una
migración, volver al binario anterior lo deja corriendo contra un esquema más nuevo. Eso suele
funcionar cuando la migración solo agregó columnas o tablas, y no funciona si cambió o quitó algo.
Si al volver atrás la aplicación falla, el camino correcto es restaurar el respaldo que sacó antes
de actualizar:

```bash
docker compose -f docker-compose.produccion.yml stop aplicacion
BASE_DESTINO=cxc CONFIRMAR=SI ./restaurar.sh respaldos/cxc-AAAAMMDD-HHMMSS.dump
docker compose -f docker-compose.produccion.yml up -d aplicacion
```

---

## 7. Respaldos

`respaldar.sh` saca el volcado de la base (`pg_dump -Fc`), empaqueta la carpeta `archivos` con las
fotos, comprueba que el volcado se puede leer y borra lo que pase de 30 días.

Deje una línea en el cron de root:

```bash
crontab -e
```

```cron
0 2 * * * cd /opt/cxc && ./respaldar.sh >> /var/log/cxc-respaldo.log 2>&1
```

- **Dónde quedan:** en `/opt/cxc/respaldos`, como `cxc-AAAAMMDD-HHMMSS.dump` y
  `archivos-AAAAMMDD-HHMMSS.tar.gz`.
- **Cuánto se conservan:** 30 días. Para cambiarlo, `RETENCION_DIAS=60 ./respaldar.sh` (o en la
  línea del cron).
- **Copia fuera del servidor:** los respaldos están en el mismo disco que la base, así que no
  sirven de nada si se pierde el servidor. Agregue una segunda línea que los copie a otra máquina
  o a un bucket, por ejemplo `rsync -az /opt/cxc/respaldos/ respaldo@otra-maquina:/cxc/`.

### Probar la restauración una vez al mes

Un respaldo que nunca se restauró no es un respaldo. `restaurar.sh` restaura, por defecto, sobre
una base de prueba, así que esto no toca los datos reales:

```bash
cd /opt/cxc
./restaurar.sh respaldos/cxc-AAAAMMDD-HHMMSS.dump
```

El script imprime cuántas empresas, clientes, cuentas y pagos quedaron en la base restaurada, y
cuántos dueños distintos tienen las tablas: **tiene que decir 1**, porque todas deben pertenecer a
`cxc_app`. Si dice otra cosa, el aislamiento por RLS no se aplicaría sobre la base restaurada.

Ponga el día en el calendario. Es cinco minutos al mes y es lo único que distingue un respaldo de
un archivo grande.

---

## 8. Monitoreo

### `/api/salud`

No requiere sesión y no revela nada del negocio:

```bash
curl -s https://cobros.ejemplo.sv/api/salud
```

| Respuesta | Qué significa |
|---|---|
| `200` con `"estado":"ok"` | La aplicación responde y habla con la base. Todo bien. |
| `503` con `"base_de_datos":"sin_respuesta"` | La aplicación está viva pero **PostgreSQL no contesta**. Nadie puede trabajar. Vaya a §9. |
| Sin respuesta / 502 | No llegó a la aplicación: o el contenedor está caído, o es Caddy. |

Dos campos más que conviene mirar: `version` dice qué está corriendo (útil después de actualizar)
y `segundos_en_pie` se reinicia en cero cada vez que el contenedor se reinicia; si lo encuentra
bajo sin que usted haya desplegado, algo está haciendo caer la aplicación.

### Chequeo externo gratuito

Monitorear desde el propio servidor no sirve: si el servidor se cae, el monitoreo también. Use un
servicio externo gratuito (UptimeRobot, Better Stack o similar), con un monitor HTTP(s):

- URL: `https://cobros.ejemplo.sv/api/salud`
- Intervalo: 5 minutos
- Alerta: cuando el código no sea 200, al correo y al teléfono de quien atiende

El mismo servicio avisa si el certificado está por vencer, que es la otra cosa que puede dejar el
sitio inaccesible sin que nadie se dé cuenta.

### Revisión manual, una vez por semana

```bash
docker compose -f docker-compose.produccion.yml ps     # los tres contenedores "running"/"healthy"
df -h /                                                 # espacio libre: actúe bajo el 20%
du -sh /opt/cxc/archivos /opt/cxc/respaldos             # qué es lo que está creciendo
tail -5 /var/log/cxc-respaldo.log                       # que el respaldo de anoche corrió
```

---

## 9. Si algo no arranca

Primero, siempre:

```bash
docker compose -f docker-compose.produccion.yml logs --tail=50 aplicacion
docker compose -f docker-compose.produccion.yml logs --tail=50 postgres
docker compose -f docker-compose.produccion.yml logs --tail=50 caddy
```

### `el rol "X" es superusuario o tiene BYPASSRLS`

**Esto es intencional y no se desactiva.** El aislamiento entre empresas se apoya en Row Level
Security, y un superusuario se lo salta: vería la cartera de todas las empresas a la vez. La
aplicación prefiere no arrancar.

Causa real: `DATABASE_URL` está apuntando al usuario `postgres` en vez de a `cxc_app`. Corrija el
`.env` y recree el contenedor con `up -d aplicacion`.

### `tablas con empresa_id sin RLS habilitado y forzado: <tabla>`

También intencional. Alguien agregó una tabla con `empresa_id` en una migración y olvidó el
`ENABLE ROW LEVEL SECURITY` / `FORCE ROW LEVEL SECURITY` y su política. La solución es arreglar la
migración (vea cómo lo hacen las tablas de `migraciones/003_cobranza_en_ruta.sql`), nunca quitar la
verificación: sin eso esa tabla sería visible entre empresas.

### `RLS no está filtrando: sin empresa definida se ven N usuarios`

La prueba en vivo falló: las tablas tienen RLS pero no pertenecen al rol de la aplicación, o la
política no está. Pasa típicamente tras restaurar un respaldo con `--no-owner`. Restaure de nuevo
con `restaurar.sh`, que conserva los dueños.

### `pg_isready` nunca pasa / `role "cxc_app" does not exist`

El script que crea el rol y la base corre **una sola vez**, cuando el volumen de datos está vacío.
Si cambió `CXC_APP_PASSWORD` después del primer arranque, el rol sigue con la contraseña vieja.

Para cambiarla sobre una base que ya tiene datos, hágalo en la base y en el `.env`:

```bash
docker exec -it cxc-postgres psql -U postgres -c "ALTER ROLE cxc_app PASSWORD 'la-nueva';"
nano .env    # la misma en CXC_APP_PASSWORD
docker compose -f docker-compose.produccion.yml up -d aplicacion
```

Si el servidor es nuevo y **no hay datos que perder**, lo más limpio es empezar de cero:
`docker compose -f docker-compose.produccion.yml down -v` borra el volumen y el próximo arranque
vuelve a crear el rol. Con datos reales, jamás.

### `DATABASE_URL es obligatoria cuando ENTORNO no es dev`

El contenedor arrancó sin esa variable: casi siempre es que `.env` no está en la misma carpeta
desde la que ejecutó `docker compose`.

### `no se pudo conectar a PostgreSQL`

La aplicación reintenta 15 veces antes de rendirse. Si llegó hasta aquí, Postgres está caído o no
terminó de recuperarse: `logs postgres` dirá por qué (lo más común, disco lleno).

### No hay HTTPS / el navegador avisa que el certificado no es válido

```bash
docker compose -f docker-compose.produccion.yml logs caddy | grep -i -E "error|certificate"
```

Las tres causas, por frecuencia:

1. El nombre no está en `DOMINIOS`. Agréguelo y `up -d caddy`.
2. El DNS no apunta al servidor todavía, o el puerto 80 está cerrado (§2, §3).
3. Let's Encrypt lo limitó por haber reintentado demasiado. Tiene topes por hora y por semana;
   corrija la causa y espere. Si llega a eso, no borre el volumen `caddy-datos`: ahí están los
   certificados que ya tiene.

### La aplicación no puede guardar las fotos

Permisos de la carpeta del host: `chown -R 10001:10001 /opt/cxc/archivos` (§4).

---

## 10. Dos advertencias que hay que conocer

> ### Una sola instancia de la aplicación. No dos.
>
> Las sesiones viven en la memoria del proceso (`auth.go`: un mapa con un mutex), no en la base ni
> en Redis. Dos consecuencias que no son negociables hoy:
>
> - **No se puede escalar a dos réplicas.** Con dos contenedores, cada petición caería en uno u
>   otro y el usuario quedaría fuera de sesión la mitad de las veces. No agregue `replicas:` al
>   compose. Si un día hace falta más capacidad, primero hay que mover las sesiones fuera del
>   proceso.
> - **Cada reinicio cierra todas las sesiones.** Actualizar, reiniciar el contenedor o reiniciar el
>   servidor obliga a todos los usuarios a volver a entrar. No se pierde ningún dato: solo tienen
>   que ingresar de nuevo. Por eso las actualizaciones van fuera del horario de cobranza.

> ### Antes del primer cliente real: respaldo probado.
>
> No basta con que el cron esté puesto. Antes de que una empresa cargue su cartera de verdad,
> ejecute el ciclo completo una vez: `./respaldar.sh`, después `./restaurar.sh <el dump>` y revise
> que los conteos que imprime son los que espera (§7). Un respaldo que nunca se restauró no es un
> respaldo, y estos datos son las cuentas por cobrar de alguien.

---

## 11. Qué no está resuelto todavía

Esto se despliega y funciona, pero hay piezas de la operación del negocio que todavía son manuales
o no existen. Conviene tenerlas a la vista antes de prometerlas:

- **Cobro de la suscripción.** El sistema no factura ni cobra a las empresas cliente. No hay
  pasarela de pago ni control de morosidad del SaaS. Se cobra por fuera, y la única palanca
  automática es `empresa_config.limite_usuarios` (y desactivar la empresa en la base). El plan
  contratado se guarda en `empresa.plan` y es informativo: lo que cada plan habilita son las
  banderas de `empresa_config`.
- **Consola web para dar de alta empresas.** El alta es por línea de comandos en el servidor (§5),
  más editar `DOMINIOS`. No hay autoregistro ni pantalla de administración del SaaS.
- **Certificado comodín.** Cada subdominio nuevo se agrega a mano a `DOMINIOS` (§2).
- **Recuperación de contraseña por correo.** Si un administrador pierde la suya, otro
  administrador se la reinicia desde la aplicación. Si se pierde la del último administrador, hay
  que intervenir la base.
- **Envío de avisos.** El sistema expone las alertas por API (`GET /api/alertas`) pero no manda
  WhatsApp ni correo; eso lo hace una automatización externa.
- **Paginación.** Los listados traen la cartera completa (§1).
