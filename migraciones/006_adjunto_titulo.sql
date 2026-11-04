-- 006_adjunto_titulo.sql — el adjunto deja de ser "un archivo suelto" y pasa a ser un documento.
--
-- Antes solo guardaba el nombre del archivo que venía del teléfono ("IMG_20261017_142233.jpg"),
-- que no le dice nada a nadie dentro de seis meses. Con el título, quien archiva escribe de qué
-- se trata ("Recibo firmado por doña Marta"), y eso es lo que sale impreso en el PDF junto con
-- la descripción, quién lo subió y cuándo.

ALTER TABLE adjunto
    ADD COLUMN titulo TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN adjunto.titulo IS
    'Cómo se llama el documento para la gente. Vacío = se muestra el nombre del archivo.';
