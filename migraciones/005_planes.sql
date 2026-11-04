-- 005_planes.sql — lo que hace falta para vender el servicio en planes.
--
-- El plan NO es una rama en el código: es configuración. Un plan "Ruta" es simplemente una empresa
-- con exigir_liquidacion_cobrador y permitir_adjuntos en true y un límite de usuarios más alto.
-- Cambiar de plan es un PATCH a /api/empresa/config, sin desplegar nada.

ALTER TABLE empresa_config
    ADD COLUMN limite_usuarios INT NOT NULL DEFAULT 0 CHECK (limite_usuarios >= 0);

COMMENT ON COLUMN empresa_config.limite_usuarios IS
    'Máximo de usuarios ACTIVOS que la empresa puede tener. 0 = sin límite.';

-- Nombre del plan contratado. Es informativo (sale en la pantalla de configuración y sirve para
-- saber a quién ofrecerle subir); los límites reales son las columnas de arriba y las banderas
-- que ya existían.
ALTER TABLE empresa
    ADD COLUMN plan TEXT NOT NULL DEFAULT '';

COMMENT ON COLUMN empresa.plan IS
    'Nombre comercial del plan contratado. Informativo: lo que habilita cada plan vive en empresa_config.';
