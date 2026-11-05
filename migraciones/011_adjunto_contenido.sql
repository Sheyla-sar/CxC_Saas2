-- 011_adjunto_contenido.sql
-- Permite guardar el contenido de un adjunto (foto o PDF) en la propia base de datos.
--
-- Se usa cuando ALMACEN_ARCHIVOS=db (Vercel: el disco de la función es efímero). Con el modo
-- por defecto (ALMACEN_ARCHIVOS=disco) esta columna queda en NULL y el archivo sigue en disco.
-- La columna archivo conserva su NOT NULL: en modo db se guarda vacía.
--
-- adjunto ya tiene Row Level Security, así que el contenido hereda el aislamiento por empresa.

ALTER TABLE adjunto ADD COLUMN contenido BYTEA;
