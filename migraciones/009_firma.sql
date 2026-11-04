-- 009_firma.sql — la firma del cliente en el recibo.
--
-- El cobrador registra el pago, le pasa el teléfono al cliente y el cliente firma con el dedo.
-- Esa firma se guarda como un adjunto más (archivo en disco, metadato acá), pero NO es un
-- comprobante: no se lista en la galería de comprobantes del registro, no se anexa como página
-- suelta al final de un PDF y no entra en el paquete de comprobantes. Se dibuja DENTRO del
-- recibo, en el espacio de firma que el recibo ya tiene impreso, que es donde una firma
-- significa algo. Por eso el adjunto necesita decir de qué clase es: sin esta columna habría
-- que adivinarlo por el título, y un título se puede escribir distinto.
--
-- Queda 'comprobante' como DEFAULT para que todo lo ya subido siga siendo lo que era.

ALTER TABLE adjunto
    ADD COLUMN clase TEXT NOT NULL DEFAULT 'comprobante'
        CHECK (clase IN ('comprobante', 'firma'));

COMMENT ON COLUMN adjunto.clase IS
    'comprobante = foto o PDF que se archiva y se imprime como documento; '
    'firma = la firma que el cliente trazó en el teléfono, que se dibuja dentro del recibo.';

-- Buscar la firma de un movimiento es la consulta caliente de esta función: pasa una vez al
-- registrar el pago (para reemplazar la anterior) y otra al imprimir el recibo.
CREATE INDEX IF NOT EXISTS idx_adjunto_firma
    ON adjunto (empresa_id, entidad, entidad_id)
    WHERE clase = 'firma';
