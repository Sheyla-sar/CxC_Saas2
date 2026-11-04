-- 007_promesas_desarrollo.sql — solo con ENTORNO=dev (el migrador omite *_desarrollo.sql).
--
-- Siembra promesas ROTAS, que son las únicas que no se pueden fabricar desde la API: anotar una
-- promesa con fecha pasada se rechaza (FECHA_PASADA), así que una promesa incumplida solo aparece
-- cuando el tiempo pasa. Sin estas filas, ni la demostración ni `verificar_promesas.sh` podrían
-- mostrar el dato por el que existe la pantalla: quién promete y no cumple.
--
-- La ventana de cumplimiento de cada una es [creado_en, fecha_promesa] = [ayer, ayer], y el monto
-- prometido es mayor que cualquier abono de ese día en la semilla: quedan rotas y así se quedan.
-- Igual que las demás semillas, el bloque fija app.empresa_id: el RLS también aplica aquí.

DO $seed$
DECLARE
    ea  UUID;
    adm UUID;
    ven UUID;
    cv  UUID;  -- cliente del vendedor
    ca  UUID;  -- cliente del administrador
    h   DATE := (now() AT TIME ZONE 'America/El_Salvador')::date;
BEGIN
    SELECT id INTO ea FROM empresa WHERE slug = 'distribuidora-progreso';
    IF ea IS NULL THEN
        RETURN; -- base sin datos de demostración
    END IF;
    PERFORM set_config('app.empresa_id', ea::text, true);

    SELECT id INTO adm FROM usuario WHERE empresa_id = ea AND rol = 'admin'    LIMIT 1;
    SELECT id INTO ven FROM usuario WHERE empresa_id = ea AND rol = 'vendedor' LIMIT 1;
    SELECT id INTO cv  FROM cliente WHERE empresa_id = ea AND nombre = 'Tienda La Bendición' LIMIT 1;
    SELECT id INTO ca  FROM cliente WHERE empresa_id = ea AND nombre = 'Tienda Doña Tere'    LIMIT 1;

    -- Cartera del vendedor: la promesa que el cobrador escuchó en la puerta y no se cumplió.
    IF cv IS NOT NULL AND ven IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM promesa WHERE empresa_id = ea AND cliente_id = cv) THEN
        INSERT INTO promesa (empresa_id, cliente_id, fecha_promesa, monto_centavos, nota,
                             registrado_por, creado_en)
        VALUES (ea, cv, h - 1, 150000,
                'Dijo que el esposo llegaba con el dinero de la venta del día.',
                ven, now() - interval '1 day');
    END IF;

    -- Cartera del administrador: sirve para comprobar que el vendedor NO la ve (R37/R38).
    IF ca IS NOT NULL AND adm IS NOT NULL
       AND NOT EXISTS (SELECT 1 FROM promesa WHERE empresa_id = ea AND cliente_id = ca) THEN
        INSERT INTO promesa (empresa_id, cliente_id, fecha_promesa, monto_centavos, nota,
                             registrado_por, creado_en)
        VALUES (ea, ca, h - 1, 120000,
                'Prometió pasar a dejar el abono a la oficina; no llegó.',
                adm, now() - interval '1 day');
    END IF;
END
$seed$;
