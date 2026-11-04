-- 002_seed_desarrollo.sql
-- Solo se aplica con ENTORNO=dev (el migrador omite archivos *_desarrollo.sql en otros entornos).
-- Dos empresas para demostrar aislamiento y comportamiento por configuración.
-- Las fechas son relativas a "hoy" en El Salvador al momento de sembrar.
-- Cada bloque de empresa fija app.empresa_id (set_config local a la transacción), igual que la app:
-- el RLS aplica también a esta semilla.
--
-- Credenciales:
--   distribuidora-progreso  admin@progreso.sv     / admin123     (admin)
--   distribuidora-progreso  carlos@progreso.sv    / vendedor123  (vendedor)
--   agroinsumos-valle       admin@agroinsumos.sv  / admin123     (admin)

DO $seed$
DECLARE
    h   DATE := (now() AT TIME ZONE 'America/El_Salvador')::date;   -- "hoy"
    ea  UUID; eb UUID;
    adm UUID; ven UUID; admb UUID;
    c1 UUID; c2 UUID; c3 UUID; c4 UUID; c5 UUID; c6 UUID;
    c7 UUID; c8 UUID; c9 UUID; c10 UUID; c11 UUID; c12 UUID;
    k1 UUID; k2 UUID; k3 UUID; k4 UUID; k5 UUID; k6 UUID; k7 UUID; k8 UUID;
    k9 UUID; k10 UUID; k11 UUID; k12 UUID; k13 UUID; k14 UUID; k15 UUID;
    op UUID;
    b1 UUID; b2 UUID; b3 UUID; b4 UUID;
    bk1 UUID; bk2 UUID; bk3 UUID;
BEGIN
    -- ═════════════════════════════════════════════════════════════════════
    -- EMPRESA A: Distribuidora El Progreso (configuración por defecto)
    -- ═════════════════════════════════════════════════════════════════════
    INSERT INTO empresa (slug, nombre_legal, nombre_comercial)
    VALUES ('distribuidora-progreso', 'Distribuidora El Progreso, S.A. de C.V.', 'Distribuidora El Progreso')
    RETURNING id INTO ea;
    PERFORM set_config('app.empresa_id', ea::text, true);

    INSERT INTO empresa_marca (empresa_id, nombre_producto) VALUES (ea, 'Cobros El Progreso');
    INSERT INTO empresa_config (empresa_id) VALUES (ea);

    INSERT INTO usuario (empresa_id, nombre, email, password_hash, rol)
    VALUES (ea, 'Ana Rivas', 'admin@progreso.sv', crypt('admin123', gen_salt('bf', 10)), 'admin')
    RETURNING id INTO adm;
    INSERT INTO usuario (empresa_id, nombre, email, password_hash, rol)
    VALUES (ea, 'Carlos Martínez', 'carlos@progreso.sv', crypt('vendedor123', gen_salt('bf', 10)), 'vendedor')
    RETURNING id INTO ven;

    -- 12 clientes (tres con límite de crédito)
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Tienda La Bendición', 'Rosa Hernández', '7845-2210', 30, 0, ven) RETURNING id INTO c1;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Abarrotería San José', 'José Alfaro', '7012-3345', 15, 500000, ven) RETURNING id INTO c2;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Mini Súper El Buen Precio', 'Marta Castillo', '7690-1182', 45, 0, ven) RETURNING id INTO c3;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Tienda Doña Tere', 'Teresa Guzmán', '7321-5567', 15, 0, adm) RETURNING id INTO c4;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Abarrotería Los Almendros', 'Luis Portillo', '7456-9921', 30, 800000, ven) RETURNING id INTO c5;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Comercial Hermanos Flores', 'Raúl Flores', '2233-4410', 30, 0, adm) RETURNING id INTO c6;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Tienda El Pulgarcito', 'Carmen Ayala', '7788-0923', 15, 0, ven) RETURNING id INTO c7;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Abarrotería La Esperanza de Soyapango', 'Miguel Rodríguez', '2277-3150', 60, 1500000, adm) RETURNING id INTO c8;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Mini Market Santa Tecla', 'Gloria Menjívar', '7601-4478', 45, 0, ven) RETURNING id INTO c9;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Tienda y Variedades Mi Pueblo', 'Óscar Quintanilla', '7934-6612', 30, 0, adm) RETURNING id INTO c10;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Abarrotería El Sol de Apopa', 'Patricia Mejía', '7120-8834', 60, 0, ven) RETURNING id INTO c11;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (ea, 'Tienda La Ceibita', 'Julio Chávez', '7555-2019', 15, 0, adm) RETURNING id INTO c12;

    -- 15 cuentas. Vencimiento = emisión + plazo del cliente.
    -- Al día (6)
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c1, 125000, h - 10, h + 20, ven) RETURNING id INTO k1;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c3, 348050, h - 5, h + 40, ven) RETURNING id INTO k2;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c4, 56000, h - 3, h + 12, adm) RETURNING id INTO k3;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c6, 412500, h - 8, h + 22, adm) RETURNING id INTO k4;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c8, 780000, h - 20, h + 40, adm) RETURNING id INTO k5;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id, requiere_revision)
    VALUES (ea, c9, 93400, h - 15, h + 30, ven, true) RETURNING id INTO k6;
    -- Por vencer (3): dentro de los 7 días de aviso
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c2, 67500, h - 11, h + 4, ven) RETURNING id INTO k7;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c5, 238000, h - 28, h + 2, ven) RETURNING id INTO k8;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c11, 515000, h - 54, h + 6, ven) RETURNING id INTO k9;
    -- Vencidas (4) con distinta antigüedad
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c7, 45000, h - 20, h - 5, ven) RETURNING id INTO k10;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c10, 186000, h - 48, h - 18, adm) RETURNING id INTO k11;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c12, 72000, h - 55, h - 40, adm) RETURNING id INTO k12;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c1, 310000, h - 105, h - 75, ven) RETURNING id INTO k13;
    -- Pagadas (2)
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c3, 150000, h - 70, h - 25, ven) RETURNING id INTO k14;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (ea, c4, 98000, h - 60, h - 45, adm) RETURNING id INTO k15;

    -- Pagos parciales (6 cuentas), uno anulado y una condonación
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k1, 40000, h - 3, 'Abono en efectivo', ven);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k4, 100000, h - 2, 'Transferencia Banco Agrícola', adm);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k5, 200000, h - 10, 'Cheque #004512', adm);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por,
                      anulado, anulado_por, anulado_en, motivo_anulacion)
    VALUES (ea, k5, 150000, h - 5, 'Cheque #004530', adm,
            true, adm, now() - interval '3 days', 'Cheque rechazado por fondos insuficientes');
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k8, 80000, h - 7, 'Abono en ruta', ven);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k11, 50000, h - 20, 'Abono en efectivo', adm);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k13, 60000, h - 70, 'Abono en efectivo', ven);
    -- Pagada con atraso (5 días)
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k14, 100000, h - 30, 'Transferencia', ven);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k14, 50000, h - 20, 'Cancelación de saldo', ven);
    -- Pagada con condonación de remanente
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k15, 90000, h - 40, 'Pago en efectivo', adm);
    INSERT INTO pago (empresa_id, cuenta_id, tipo, monto_centavos, fecha, nota, registrado_por)
    VALUES (ea, k15, 'condonacion', 8000, h - 35,
            'Remanente condonado por diferencia de precio acordada con la clienta', adm);

    -- Ajuste pendiente de revisión (k6: el vendedor subió el monto)
    INSERT INTO ajuste_cuenta (empresa_id, cuenta_id, monto_anterior_centavos, monto_nuevo_centavos,
                               motivo, solicitado_por, creado_en)
    VALUES (ea, k6, 88400, 93400,
            'Se agregaron 5 fardos de azúcar entregados después de la factura original', ven,
            now() - interval '2 days');

    -- 8 oportunidades en las cinco etapas
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c7, 120000, 'nuevo', 'Interesada en la línea de lácteos', ven);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c12, 60000, 'nuevo', 'Pidió lista de precios de granos básicos', adm);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c2, 250000, 'contactado', 'Visita programada para el jueves', ven);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c10, 180000, 'contactado', 'Quiere surtir bebidas para las fiestas patronales', adm);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c3, 650000, 'negociacion', 'Pide descuento por volumen en aceite y arroz', ven);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c5, 300000, 'negociacion', 'Negociando entrega quincenal', ven);
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c11, 515000, 'ganado', 'Pedido grande de temporada', ven)
    RETURNING id INTO op;
    UPDATE cuenta SET oportunidad_id = op WHERE id = k9;
    UPDATE oportunidad SET cuenta_id = k9 WHERE id = op;
    INSERT INTO oportunidad (empresa_id, cliente_id, monto_estimado_centavos, etapa, notas, vendedor_id)
    VALUES (ea, c6, 90000, 'perdido', 'Eligió otro proveedor por precio', adm);

    -- ═════════════════════════════════════════════════════════════════════
    -- EMPRESA B: Agroinsumos del Valle (configuración distinta)
    -- ═════════════════════════════════════════════════════════════════════
    INSERT INTO empresa (slug, nombre_legal, nombre_comercial)
    VALUES ('agroinsumos-valle', 'Agroinsumos del Valle, S.A. de C.V.', 'Agroinsumos del Valle')
    RETURNING id INTO eb;
    PERFORM set_config('app.empresa_id', eb::text, true);

    INSERT INTO empresa_marca (empresa_id, color_primario, color_primario_claro, nombre_producto)
    VALUES (eb, '#2E7D32', '#4CAF50', 'Cartera Agro');
    INSERT INTO empresa_config (empresa_id, dias_aviso_previo, permitir_sobrepago, exigir_limite_credito)
    VALUES (eb, 3, true, true);

    INSERT INTO usuario (empresa_id, nombre, email, password_hash, rol)
    VALUES (eb, 'Roberto Alvarado', 'admin@agroinsumos.sv', crypt('admin123', gen_salt('bf', 10)), 'admin')
    RETURNING id INTO admb;

    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (eb, 'Agroservicio El Surco', 'Mario Lemus', '2393-1120', 30, 0, admb) RETURNING id INTO b1;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (eb, 'Cooperativa Agrícola Las Brisas', 'Elena Cruz', '2605-7788', 60, 2000000, admb) RETURNING id INTO b2;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (eb, 'Finca Santa Rita', 'Francisco Orellana', '7422-9031', 45, 0, admb) RETURNING id INTO b3;
    INSERT INTO cliente (empresa_id, nombre, contacto, telefono, plazo_credito_dias, limite_credito_centavos, vendedor_id)
    VALUES (eb, 'Agroferretería San Vicente', 'Sonia Pineda', '2396-4450', 30, 500000, admb) RETURNING id INTO b4;

    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (eb, b1, 245000, h - 5, h + 25, admb) RETURNING id INTO bk1;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (eb, b2, 760000, h - 10, h + 50, admb) RETURNING id INTO bk2;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (eb, b3, 132000, h - 43, h + 2, admb);
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    VALUES (eb, b4, 98500, h - 42, h - 12, admb) RETURNING id INTO bk3;
    INSERT INTO cuenta (empresa_id, cliente_id, monto_total_centavos, fecha_emision, fecha_vencimiento, vendedor_id)
    -- Vence en 5 días: con aviso de 3 días está 'al_dia' (en la empresa A, con 7, sería 'por_vencer')
    VALUES (eb, b1, 54000, h - 25, h + 5, admb);

    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (eb, bk2, 300000, h - 3, 'Transferencia', admb);
    INSERT INTO pago (empresa_id, cuenta_id, monto_centavos, fecha, nota, registrado_por)
    VALUES (eb, bk3, 40000, h - 8, 'Abono en efectivo', admb);
END
$seed$;
