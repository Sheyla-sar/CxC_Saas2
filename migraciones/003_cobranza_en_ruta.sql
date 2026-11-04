-- 003_cobranza_en_ruta.sql — Fase 3: cobranza en ruta y documentos.
-- Agrega: método de pago, recibos numerados, notas de crédito, anticipos (saldo a favor),
-- liquidación del cobrador, adjuntos (fotos de recibos y facturas) y número de documento.
-- Toda tabla nueva con empresa_id lleva RLS habilitado y forzado (si no, la app no arranca).

-- ─────────────────────────────────────────────────────────────────────────────
-- Datos de la empresa para los documentos impresos (PDF)
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE empresa
    ADD COLUMN nit       TEXT NOT NULL DEFAULT '',
    ADD COLUMN nrc       TEXT NOT NULL DEFAULT '',
    ADD COLUMN direccion TEXT NOT NULL DEFAULT '',
    ADD COLUMN telefono  TEXT NOT NULL DEFAULT '';

-- ─────────────────────────────────────────────────────────────────────────────
-- Configuración nueva (todo lo que una empresa podría querer distinto)
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE empresa_config
    ADD COLUMN metodos_pago TEXT[] NOT NULL DEFAULT '{efectivo,transferencia,cheque,deposito,tarjeta}'
        CHECK (cardinality(metodos_pago) > 0),
    ADD COLUMN exigir_liquidacion_cobrador BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN permitir_adjuntos           BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN tamano_maximo_adjunto_mb    INT NOT NULL DEFAULT 10
        CHECK (tamano_maximo_adjunto_mb BETWEEN 1 AND 25),
    ADD COLUMN prefijo_recibo        TEXT NOT NULL DEFAULT 'REC-',
    ADD COLUMN prefijo_nota_credito  TEXT NOT NULL DEFAULT 'NC-',
    ADD COLUMN prefijo_anticipo      TEXT NOT NULL DEFAULT 'ANT-',
    ADD COLUMN prefijo_liquidacion   TEXT NOT NULL DEFAULT 'LIQ-';

-- ─────────────────────────────────────────────────────────────────────────────
-- Correlativos por empresa y tipo de documento. Se reservan con UPDATE ... RETURNING
-- (bloqueo de fila), así dos cobradores simultáneos nunca obtienen el mismo número.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE correlativo (
    empresa_id UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    tipo       TEXT NOT NULL CHECK (tipo IN ('recibo', 'nota_credito', 'anticipo', 'liquidacion')),
    siguiente  BIGINT NOT NULL DEFAULT 1 CHECK (siguiente > 0),
    PRIMARY KEY (empresa_id, tipo)
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Número de documento (la factura del cliente) y referencia del sistema de origen,
-- necesarios para importar la cartera existente sin duplicar.
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE cuenta
    ADD COLUMN numero_documento TEXT NOT NULL DEFAULT '';
ALTER TABLE cliente
    ADD COLUMN codigo_externo TEXT NOT NULL DEFAULT '';

-- Únicos solo cuando hay valor: dos cuentas no pueden traer la misma factura.
CREATE UNIQUE INDEX idx_cuenta_documento_unico ON cuenta (empresa_id, numero_documento)
    WHERE numero_documento <> '';
CREATE UNIQUE INDEX idx_cliente_codigo_unico ON cliente (empresa_id, codigo_externo)
    WHERE codigo_externo <> '';

-- ─────────────────────────────────────────────────────────────────────────────
-- Liquidación del cobrador: el corte de caja de quien cobra en ruta.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE liquidacion (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id               UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    numero                   TEXT NOT NULL,
    cobrador_id              UUID NOT NULL,
    fecha                    DATE NOT NULL,
    estado                   TEXT NOT NULL DEFAULT 'entregada'
                             CHECK (estado IN ('entregada', 'aprobada', 'rechazada')),
    total_esperado_centavos  BIGINT NOT NULL CHECK (total_esperado_centavos >= 0),
    total_entregado_centavos BIGINT NOT NULL CHECK (total_entregado_centavos >= 0),
    cantidad_movimientos     INT NOT NULL CHECK (cantidad_movimientos >= 0),
    nota                     TEXT NOT NULL DEFAULT '',
    revisado_por             UUID,
    revisado_en              TIMESTAMPTZ,
    motivo_rechazo           TEXT,
    creado_en                TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    UNIQUE (empresa_id, numero),
    CHECK (estado = 'entregada' OR revisado_en IS NOT NULL),
    FOREIGN KEY (empresa_id, cobrador_id)  REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, revisado_por) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Anticipos: dinero que el cliente entrega sin factura asignada (saldo a favor).
-- Lo aplicado se registra como movimientos con anticipo_id, nunca se descuenta aquí.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE anticipo (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    numero           TEXT NOT NULL,
    cliente_id       UUID NOT NULL,
    monto_centavos   BIGINT NOT NULL CHECK (monto_centavos > 0),
    fecha            DATE NOT NULL,
    metodo           TEXT NOT NULL,
    referencia       TEXT NOT NULL DEFAULT '',
    nota             TEXT NOT NULL DEFAULT '',
    registrado_por   UUID NOT NULL,
    liquidacion_id   UUID,
    anulado          BOOLEAN NOT NULL DEFAULT false,
    anulado_por      UUID,
    anulado_en       TIMESTAMPTZ,
    motivo_anulacion TEXT,
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    UNIQUE (empresa_id, numero),
    CHECK (anulado = (anulado_por IS NOT NULL AND anulado_en IS NOT NULL)),
    FOREIGN KEY (empresa_id, cliente_id)     REFERENCES cliente(empresa_id, id)     ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, registrado_por) REFERENCES usuario(empresa_id, id)     ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, anulado_por)    REFERENCES usuario(empresa_id, id)     ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, liquidacion_id) REFERENCES liquidacion(empresa_id, id) ON DELETE RESTRICT
);

-- ─────────────────────────────────────────────────────────────────────────────
-- Movimientos: método de pago, recibo numerado, referencia, liquidación y anticipo.
-- Se agrega el tipo 'nota_credito' (devolución documentada), distinto de la condonación.
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE pago
    ADD COLUMN metodo         TEXT NOT NULL DEFAULT 'efectivo',
    ADD COLUMN referencia     TEXT NOT NULL DEFAULT '',
    ADD COLUMN numero         TEXT,
    ADD COLUMN liquidacion_id UUID,
    ADD COLUMN anticipo_id    UUID;

ALTER TABLE pago DROP CONSTRAINT pago_tipo_check;
ALTER TABLE pago ADD CONSTRAINT pago_tipo_check
    CHECK (tipo IN ('pago', 'condonacion', 'nota_credito'));

-- Solo los movimientos de dinero recibido se liquidan.
ALTER TABLE pago ADD CONSTRAINT pago_liquidacion_solo_pagos
    CHECK (liquidacion_id IS NULL OR tipo = 'pago');
ALTER TABLE pago ADD CONSTRAINT pago_anticipo_solo_pagos
    CHECK (anticipo_id IS NULL OR tipo = 'pago');

ALTER TABLE pago
    ADD FOREIGN KEY (empresa_id, liquidacion_id) REFERENCES liquidacion(empresa_id, id) ON DELETE RESTRICT,
    ADD FOREIGN KEY (empresa_id, anticipo_id)    REFERENCES anticipo(empresa_id, id)    ON DELETE RESTRICT;

CREATE UNIQUE INDEX idx_pago_numero_unico ON pago (empresa_id, numero) WHERE numero IS NOT NULL;
CREATE INDEX idx_pago_liquidacion ON pago (empresa_id, liquidacion_id);
CREATE INDEX idx_pago_cobrador ON pago (empresa_id, registrado_por, fecha);
CREATE INDEX idx_anticipo_cliente ON anticipo (empresa_id, cliente_id);
CREATE INDEX idx_liquidacion_cobrador ON liquidacion (empresa_id, cobrador_id, fecha);

-- ─────────────────────────────────────────────────────────────────────────────
-- Adjuntos: fotos de recibos, facturas y comprobantes. El archivo vive en disco;
-- aquí queda su metadato. Se sirven solo con sesión y dentro de la empresa.
-- ─────────────────────────────────────────────────────────────────────────────
CREATE TABLE adjunto (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    entidad      TEXT NOT NULL CHECK (entidad IN ('cuenta', 'movimiento', 'liquidacion', 'anticipo', 'cliente')),
    entidad_id   UUID NOT NULL,
    nombre       TEXT NOT NULL,
    tipo_mime    TEXT NOT NULL CHECK (tipo_mime IN ('image/jpeg', 'image/png', 'application/pdf')),
    tamano_bytes BIGINT NOT NULL CHECK (tamano_bytes > 0),
    archivo      TEXT NOT NULL,
    descripcion  TEXT NOT NULL DEFAULT '',
    subido_por   UUID NOT NULL,
    creado_en    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    FOREIGN KEY (empresa_id, subido_por) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

CREATE INDEX idx_adjunto_entidad ON adjunto (empresa_id, entidad, entidad_id);

-- Correlativos iniciales para las empresas que ya existen.
-- Va ANTES de activar RLS: la migración corre sin app.empresa_id y la política lo rechazaría.
INSERT INTO correlativo (empresa_id, tipo)
SELECT e.id, t
FROM empresa e, unnest(ARRAY['recibo', 'nota_credito', 'anticipo', 'liquidacion']) t
ON CONFLICT DO NOTHING;

-- ─────────────────────────────────────────────────────────────────────────────
-- Row Level Security de las tablas nuevas
-- ─────────────────────────────────────────────────────────────────────────────
DO $rls$
DECLARE
    t TEXT;
BEGIN
    FOREACH t IN ARRAY ARRAY['correlativo', 'liquidacion', 'anticipo', 'adjunto'] LOOP
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
