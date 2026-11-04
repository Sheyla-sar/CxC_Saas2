-- 007_promesas.sql — Fase 4: promesa de pago.
--
-- Hasta aquí el sistema registraba lo que YA pasó. Esto registra lo que VA a pasar: el cobrador
-- visita, no cobra, y anota "me paga el viernes". Con eso se arma la agenda del día siguiente y,
-- sobre todo, queda constancia de quién promete y no cumple, que es el mejor predictor de mora
-- que una distribuidora puede tener sin comprar nada.
--
-- El ESTADO de la promesa (pendiente, cumplida, rota) NO se guarda, igual que el saldo y el
-- estado de una cuenta: se calcula con los pagos del cliente dentro de la ventana que va del día
-- en que se anotó la promesa a la fecha prometida. Guardarlo obligaría a un proceso que lo
-- actualice cada noche y a confiar en que ese proceso corrió.

CREATE TABLE promesa (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       UUID NOT NULL REFERENCES empresa(id) ON DELETE RESTRICT,
    cliente_id       UUID NOT NULL,
    -- Opcional: la factura concreta que el cliente dijo que iba a pagar. Sin ella, la promesa es
    -- del cliente y cuenta cualquier pago suyo dentro de la ventana.
    cuenta_id        UUID,
    fecha_promesa    DATE NOT NULL,
    -- Opcional: cuánto prometió. Sin monto, basta con que pague algo para darla por cumplida.
    monto_centavos   BIGINT CHECK (monto_centavos IS NULL OR monto_centavos > 0),
    nota             TEXT NOT NULL DEFAULT '',
    registrado_por   UUID NOT NULL,
    anulada          BOOLEAN NOT NULL DEFAULT false,
    anulada_por      UUID,
    anulada_en       TIMESTAMPTZ,
    motivo_anulacion TEXT,
    creado_en        TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, id),
    CHECK (anulada = (anulada_por IS NOT NULL AND anulada_en IS NOT NULL)),
    FOREIGN KEY (empresa_id, cliente_id)     REFERENCES cliente(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, cuenta_id)      REFERENCES cuenta(empresa_id, id)  ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, registrado_por) REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT,
    FOREIGN KEY (empresa_id, anulada_por)    REFERENCES usuario(empresa_id, id) ON DELETE RESTRICT
);

-- Todos los índices empiezan por empresa_id. El de la agenda es el que se consulta cada mañana.
CREATE INDEX idx_promesa_cliente ON promesa (empresa_id, cliente_id, fecha_promesa);
CREATE INDEX idx_promesa_cuenta  ON promesa (empresa_id, cuenta_id);
CREATE INDEX idx_promesa_agenda  ON promesa (empresa_id, fecha_promesa) WHERE NOT anulada;

-- ─────────────────────────────────────────────────────────────────────────────
-- Row Level Security: sin esto la aplicación se niega a arrancar (§5.A de CLAUDE.md).
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE promesa ENABLE ROW LEVEL SECURITY;
ALTER TABLE promesa FORCE ROW LEVEL SECURITY;
CREATE POLICY aislamiento_empresa ON promesa
    USING (empresa_id = NULLIF(current_setting('app.empresa_id', true), '')::uuid)
    WITH CHECK (empresa_id = NULLIF(current_setting('app.empresa_id', true), '')::uuid);

COMMENT ON TABLE promesa IS
    'Compromiso de pago que el cliente le da al cobrador. El estado (pendiente/cumplida/rota) no se almacena: lo calcula reglas_promesas.go con los pagos de la ventana.';
