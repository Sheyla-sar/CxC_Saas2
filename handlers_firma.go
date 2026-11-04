package main

// handlers_firma.go — la firma del cliente en el recibo. El cobrador registra el pago, le pasa
// el teléfono al cliente, el cliente firma con el dedo y esa firma sale impresa dentro del
// recibo, en el espacio de firma que el documento ya tiene. Es lo que convierte el recibo en un
// comprobante que se puede mostrar cuando después se discute si se pagó o no.

import (
	"fmt"
	"net/http"
)

// rutasFirma registra las rutas de este grupo.
func (a *App) rutasFirma(mux *http.ServeMux) {
	mux.Handle("POST /api/movimientos/{id}/firma", a.conTenant(hGuardarFirma))
}

// hGuardarFirma recibe el PNG que dibujó el cliente y lo guarda como adjunto del movimiento con
// clase 'firma'. Un movimiento tiene UNA firma: si ya había una, la nueva la reemplaza y la
// anterior se borra con su archivo. Una firma de más en el historial solo confunde a quien
// después tenga que explicar qué firmó el cliente.
func hGuardarFirma(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("MOVIMIENTO_NO_ENCONTRADO", "No encontramos el movimiento indicado."))
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		return 0, nil, errEntrada("ARCHIVO_INVALIDO", "No se pudo leer la firma. Vuelva a firmar.")
	}
	defer r.MultipartForm.RemoveAll()

	archivo, cabecera, err := r.FormFile("archivo")
	if err != nil {
		return 0, nil, errEntrada("ARCHIVO_REQUERIDO", "Pídale al cliente que firme antes de guardar.")
	}
	defer archivo.Close()

	// El acceso al movimiento lo comprueba guardarAdjunto con verificarAcceso (alcance del
	// usuario: para un vendedor, un movimiento ajeno no existe). Se consulta antes la firma
	// anterior para poder reemplazarla.
	anterior, habia, err := t.Repo.FirmaDeMovimiento(t.EmpresaID, id)
	if err != nil {
		return 0, nil, err
	}
	nueva, err := guardarAdjunto(t, cfg, archivo, cabecera, datosAdjunto{
		Entidad: "movimiento", EntidadID: id, Clase: ClaseFirma,
		Descripcion: fmt.Sprintf("Firmado en el teléfono de %s al entregar el recibo.", t.Usuario.Nombre),
	})
	if err != nil {
		return 0, nil, err
	}
	if habia {
		if err := t.Repo.EliminarAdjunto(t.EmpresaID, anterior.ID); err != nil {
			return 0, nil, err
		}
		// El archivo se borra después del DELETE: si la transacción se deshiciera, el metadato
		// volvería y es preferible un archivo de más que una firma que apunta a la nada.
		borrarArchivo(anterior.archivo)
	}
	return http.StatusCreated, map[string]any{
		"adjunto": nueva,
		"mensaje": "Firma guardada. Ya sale impresa en el recibo.",
	}, nil
}
