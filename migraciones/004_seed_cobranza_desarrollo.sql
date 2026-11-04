-- 004_seed_cobranza_desarrollo.sql — solo con ENTORNO=dev.
-- Completa los datos fiscales de las empresas de demostración (salen impresos en el recibo y en el
-- estado de cuenta) y agrega un anticipo de ejemplo para probar el saldo a favor.

DO $seed$
DECLARE
    ea   UUID;
    adm  UUID;
    cli  UUID;
    h    DATE := (now() AT TIME ZONE 'America/El_Salvador')::date;
BEGIN
    UPDATE empresa SET nit = '0614-120518-102-3', nrc = '245871-8',
                       direccion = 'Km 10 1/2 Carretera a Santa Ana, San Salvador',
                       telefono = '2273-4500'
    WHERE slug = 'distribuidora-progreso';

    UPDATE empresa SET nit = '1217-030719-101-5', nrc = '318204-2',
                       direccion = 'Cantón El Niño, San Vicente',
                       telefono = '2393-1800'
    WHERE slug = 'agroinsumos-valle';

    SELECT id INTO ea FROM empresa WHERE slug = 'distribuidora-progreso';
    IF ea IS NULL THEN
        RETURN; -- base sin datos de demostración
    END IF;
    PERFORM set_config('app.empresa_id', ea::text, true);

    SELECT id INTO adm FROM usuario WHERE empresa_id = ea AND rol = 'admin' LIMIT 1;
    SELECT id INTO cli FROM cliente WHERE empresa_id = ea AND nombre = 'Abarrotería Los Almendros' LIMIT 1;

    -- Anticipo con saldo a favor: el cliente dejó dinero sin factura asignada.
    IF cli IS NOT NULL AND NOT EXISTS (SELECT 1 FROM anticipo WHERE empresa_id = ea) THEN
        INSERT INTO anticipo (empresa_id, numero, cliente_id, monto_centavos, fecha, metodo,
                              referencia, nota, registrado_por)
        VALUES (ea, 'ANT-000001', cli, 50000, h - 2, 'transferencia', 'TRX-99120',
                'Abono adelantado para el pedido de la próxima semana', adm);
        UPDATE correlativo SET siguiente = 2 WHERE empresa_id = ea AND tipo = 'anticipo';
    END IF;
END
$seed$;
