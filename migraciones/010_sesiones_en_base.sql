-- 010_sesiones_en_base.sql
-- Sesiones y límite de intentos de ingreso en PostgreSQL, no en la memoria del proceso.
--
-- Por qué: en un entorno sin estado (Vercel, o cualquier plataforma que reinicia o duplica
-- instancias) la memoria de un proceso no sirve: la siguiente petición puede caer en otra
-- instancia y el usuario quedaría fuera. Con las sesiones en la base, cualquier instancia
-- reconoce al usuario y un reinicio ya no cierra las sesiones.
--
-- Estas dos tablas NO llevan columna empresa_id y NO usan Row Level Security, a propósito:
-- se consultan ANTES de saber a qué empresa pertenece la petición (la sesión es justamente lo que
-- lo dice). La seguridad está en que solo se guarda el SHA-256 del token: quien lea la tabla
-- no puede reconstruir una cookie válida.

CREATE TABLE sesion (
    token_hash   TEXT PRIMARY KEY,
    usuario_id   UUID NOT NULL,
    empresa_ref  UUID NOT NULL,
    empresa_slug TEXT NOT NULL,
    expira       TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_sesion_expira ON sesion (expira);

CREATE TABLE intento_ingreso (
    clave    TEXT PRIMARY KEY,
    cantidad INT NOT NULL CHECK (cantidad > 0),
    desde    TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_intento_ingreso_desde ON intento_ingreso (desde);
