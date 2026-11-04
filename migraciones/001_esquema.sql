-- 001_esquema.sql
-- Esquema multi-tenant. Reglas: UUID en todo, dinero en centavos (BIGINT), DATE para fechas de
-- negocio, TIMESTAMPTZ para auditoría, FKs compuestas (empresa_id, x_id) y RLS forzado en toda
-- tabla con empresa_id. Los valores por defecto de configuración viven SOLO aquí.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ─────────────────────────────────────────────────────────────────────────────
-- Empresas (sin RLS: no tiene empresa_id; solo slug y nombres)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE empresa (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug             TEXT NOT NULL UNIQUE
                     CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$' AND length(slug) BETWEEN 3 AND 63),
    nombre_legal     TEXT NOT NULL CHECK (length(trim(nombre_legal)) > 0),
    nombre_comercial TEXT NOT NULL CHECK (length(trim(nombre_comercial)) > 0),
    activo           BOOLEAN NOT NULL DEFAULT true,
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE empresa_marca (
    empresa_id           UUID PRIMARY KEY REFERENCES empresa(id) ON DELETE RESTRICT,
    logo_url             TEXT,
    color_primario       TEXT NOT NULL DEFAULT '#1E3A5F' CHECK (color_primario ~ '^#[0-9A-Fa-f]{6}$'),
    color_primario_claro TEXT NOT NULL DEFAULT '#2E5C8A' CHECK (color_primario_claro ~ '^#[0-9A-Fa-f]{6}$'),
    nombre_producto      TEXT NOT NULL CHECK (length(trim(nombre_producto)) > 0),
    creado_en            TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE empresa_config (
    empresa_id                       UUID PRIMARY KEY REFERENCES empresa(id) ON DELETE RESTRICT,
    moneda                           TEXT    NOT NULL DEFAULT 'USD' CHECK (moneda ~ '^[A-Z]{3}$'),
    simbolo_moneda                   TEXT    NOT NULL DEFAULT '$' CHECK (length(simbolo_moneda) BETWEEN 1 AND 5),
    zona_horaria                     TEXT    NOT NULL DEFAULT 'America/El_Salvador',
    dias_aviso_previo                INT     NOT NULL DEFAULT 7 CHECK (dias_aviso_previo >= 0),
    plazos_credito_permitidos        INT[]   NOT NULL DEFAULT '{15,30,45,60}'
                                     CHECK (cardinality(plazos_credito_permitidos) > 0),
    permitir_sobrepago               BOOLEAN NOT NULL DEFAULT false,
    exigir_revision_ajuste_monto     BOOLEAN NOT NULL DEFAULT true,
    exigir_motivo_anulacion          BOOLEAN NOT NULL DEFAULT true,
    longitud_minima_motivo           INT     NOT NULL DEFAULT 10 CHECK (longitud_minima_motivo >= 0),
    umbral_comportamiento_bueno      INT     NOT NULL DEFAULT 3 CHECK (umbral_comportamiento_bueno >= 0),
    umbral_comportamiento_regular    INT     NOT NULL DEFAULT 10,
    minimo_cuentas_para_historial    INT     NOT NULL DEFAULT 3 CHECK (minimo_cuentas_para_historial >= 1),
    cuentas_evaluadas_comportamiento INT     NOT NULL DEFAULT 10,
    exigir_limite_credito            BOOLEAN NOT NULL DEFAULT false,
    permitir_condonaciones           BOOLEAN NOT NULL DEFAULT true,
    creado_en                        TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en                   TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (umbral_comportamiento_bueno <= umbral_comportamiento_regular),
    CHECK (cuentas_evaluadas_comportamiento >= minimo_cuentas_para_historial)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Usuarios
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE usuario (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    nombre        TEXT NOT NULL CHECK (length(trim(nombre)) > 0),
    email         TEXT NOT NULL CHECK (email = lower(email) AND position('@' in email) > 1),
    password_hash TEXT NOT NULL,
    rol           TEXT NOT NULL CHECK (rol IN ('admin', 'vendedor')),
    activo        BOOLEAN NOT NULL DEFAULT true,
    creado_en     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, email),
    UNIQUE (empresa_id, id)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Clientes
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE cliente (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id              UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    nombre                  TEXT NOT NULL CHECK (length(trim(nombre)) > 0),
    contacto                TEXT NOT NULL DEFAULT '',
    telefono                TEXT NOT NULL DEFAULT '',
    plazo_credito_dias      INT NOT NULL CHECK (plazo_credito_dias > 0),
    limite_credito_centavos BIGINT NOT NULL DEFAULT 0 CHECK (limite_credito_centavos >= 0),
    vendedor_id             UUID NOT NULL,
    activo                  BOOLEAN NOT NULL DEFAULT true,
    creado_en               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    FOREIGN KEY (empresa_id, vendedor_id) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Oportunidades (pipeline de venta)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE oportunidad (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id              UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cliente_id              UUID NOT NULL,
    monto_estimado_centavos BIGINT NOT NULL CHECK (monto_estimado_centavos >= 0),
    etapa                   TEXT NOT NULL DEFAULT 'nuevo'
                            CHECK (etapa IN ('nuevo', 'contactado', 'negociacion', 'ganado', 'perdido')),
    notas                   TEXT NOT NULL DEFAULT '',
    vendedor_id             UUID NOT NULL,
    cuenta_id               UUID,
    creado_en               TIMESTAMPTZ NOT NULL DEFAULT now(),
    actualizado_en          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    CHECK (cuenta_id IS NULL OR etapa = 'ganado'),
    FOREIGN KEY (empresa_id, cliente_id)  REFERENCES cliente(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, vendedor_id) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Cuentas por cobrar. Saldo y estado NO se almacenan: se calculan.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE cuenta (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id           UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cliente_id           UUID NOT NULL,
    oportunidad_id       UUID UNIQUE,
    monto_total_centavos BIGINT NOT NULL CHECK (monto_total_centavos > 0),
    fecha_emision        DATE NOT NULL,
    fecha_vencimiento    DATE NOT NULL,
    vendedor_id          UUID NOT NULL,
    requiere_revision    BOOLEAN NOT NULL DEFAULT false,
    creado_en            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    CHECK (fecha_vencimiento >= fecha_emision),
    FOREIGN KEY (empresa_id, cliente_id)     REFERENCES cliente(empresa_id, id)     ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, oportunidad_id) REFERENCES oportunidad(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, vendedor_id)    REFERENCES usuario(empresa_id, id)     ON DELETE RESTRICT
);

ALTER TABLE oportunidad
    ADD FOREIGN KEY (empresa_id, cuenta_id) REFERENCES cuenta(empresa_id, id) ON DELETE RESTRICT;

-- ─────────────────────────────────────────────────────────────────────────────
-- Movimientos: pagos y condonaciones. Nunca se borran, se anulan.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE pago (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cuenta_id        UUID NOT NULL,
    tipo             TEXT NOT NULL DEFAULT 'pago' CHECK (tipo IN ('pago', 'condonacion')),
    monto_centavos   BIGINT NOT NULL CHECK (monto_centavos > 0),
    fecha            DATE NOT NULL,
    nota             TEXT NOT NULL DEFAULT '',
    registrado_por   UUID NOT NULL,
    anulado          BOOLEAN NOT NULL DEFAULT false,
    anulado_por      UUID,
    anulado_en       TIMESTAMPTZ,
    motivo_anulacion TEXT,
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    CHECK (anulado = (anulado_por IS NOT NULL AND anulado_en IS NOT NULL)),
    FOREIGN KEY (empresa_id, cuenta_id)      REFERENCES cuenta(empresa_id, id)  ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, registrado_por) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, anulado_por)    REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Ajustes de monto (control posterior, no bloqueo)
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE ajuste_cuenta (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id              UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cuenta_id               UUID NOT NULL,
    monto_anterior_centavos BIGINT NOT NULL CHECK (monto_anterior_centavos > 0),
    monto_nuevo_centavos    BIGINT NOT NULL CHECK (monto_nuevo_centavos > 0),
    motivo                  TEXT NOT NULL,
    solicitado_por          UUID NOT NULL,
    creado_en               TIMESTAMPTZ NOT NULL DEFAULT now(),
    estado                  TEXT NOT NULL DEFAULT 'pendiente'
                            CHECK (estado IN ('pendiente', 'aprobado', 'rechazado')),
    revisado_por            UUID,
    revisado_en             TIMESTAMPTZ,
    UNIQUE (empresa_id, id),
    CHECK (estado = 'pendiente' OR revisado_en IS NOT NULL),
    FOREIGN KEY (empresa_id, cuenta_id)      REFERENCES cuenta(empresa_id, id)  ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, solicitado_por) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, revisado_por)   REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Reasignación de cartera entre vendedores
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE reasignacion_cartera (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id             UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    vendedor_origen        UUID NOT NULL,
    vendedor_destino       UUID NOT NULL,
    cantidad_clientes      INT NOT NULL CHECK (cantidad_clientes >= 0),
    cantidad_oportunidades INT NOT NULL CHECK (cantidad_oportunidades >= 0),
    cantidad_cuentas       INT NOT NULL CHECK (cantidad_cuentas >= 0),
    motivo                 TEXT NOT NULL,
    realizado_por          UUID NOT NULL,
    creado_en              TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    CHECK (vendedor_origen <> vendedor_destino),
    FOREIGN KEY (empresa_id, vendedor_origen)  REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, vendedor_destino) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, realizado_por)    REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Alertas ya entregadas a un consumidor externo (n8n). El backend no envía nada.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE alerta_generada (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cuenta_id      UUID NOT NULL,
    tipo           TEXT NOT NULL CHECK (tipo IN ('por_vencer', 'vencido')),
    fecha_generada DATE NOT NULL,
    creado_en      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (cuenta_id, tipo, fecha_generada),
    UNIQUE (empresa_id, id),
    FOREIGN KEY (empresa_id, cuenta_id) REFERENCES cuenta(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Índices: todos comienzan por empresa_id
-- ─────────────────────────────────────────────────────────────────────────────
CREATE INDEX idx_usuario_empresa            ON usuario (empresa_id);
CREATE INDEX idx_cliente_empresa_vendedor   ON cliente (empresa_id, vendedor_id);
CREATE INDEX idx_oportunidad_empresa_etapa  ON oportunidad (empresa_id, etapa);
CREATE INDEX idx_oportunidad_empresa_vend   ON oportunidad (empresa_id, vendedor_id);
CREATE INDEX idx_cuenta_empresa_cliente     ON cuenta (empresa_id, cliente_id);
CREATE INDEX idx_cuenta_empresa_vendedor    ON cuenta (empresa_id, vendedor_id);
CREATE INDEX idx_cuenta_empresa_venc        ON cuenta (empresa_id, fecha_vencimiento);
CREATE INDEX idx_pago_empresa_cuenta        ON pago (empresa_id, cuenta_id);
CREATE INDEX idx_ajuste_empresa_estado      ON ajuste_cuenta (empresa_id, estado);
CREATE INDEX idx_ajuste_empresa_cuenta      ON ajuste_cuenta (empresa_id, cuenta_id);
CREATE INDEX idx_reasignacion_empresa       ON reasignacion_cartera (empresa_id);
CREATE INDEX idx_alerta_empresa_fecha       ON alerta_generada (empresa_id, fecha_generada);

-- ─────────────────────────────────────────────────────────────────────────────
-- Row Level Security: red de seguridad de la base de datos.
-- FORCE porque el rol de la aplicación es dueño de las tablas.
-- Sin app.empresa_id definido, la política no deja ver ni escribir ninguna fila.
-- ─────────────────────────────────────────────────────────────────────────────
DO $rls$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'empresa_marca', 'empresa_config', 'usuario', 'cliente', 'oportunidad', 'cuenta',
        'pago', 'ajuste_cuenta', 'reasignacion_cartera', 'alerta_generada'
    ] LOOP
        EXECUTE format('ALTER TABLE %I ENABLE ROW LEVEL SECURITY', t);
        EXECUTE format('ALTER TABLE %I FORCE ROW LEVEL SECURITY', t);
        EXECUTE format(
            'CREATE POLICY aislamiento_empresa ON %I '
            'USING (empresa_id = NULLIF(current_setting(''app.empresa_id'', true), '''')::uuid) '
            'WITH CHECK (empresa_id = NULLIF(current_setting(''app.empresa_id'', true), '''')::uuid)',
            t);
    END LOOP;
END
$rls$;
