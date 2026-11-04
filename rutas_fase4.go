package main

// rutas_fase4.go — el reparto de la fase 4. Cada grupo de rutas se registra en SU archivo
// (`handlers_promesas.go`, `handlers_exportacion.go`, `handlers_firma.go`) para que varias
// manos puedan trabajar a la vez sin tocar el mismo registro de rutas.

import "net/http"

// rutasFase4 las llama rutas() en handlers.go.
func (a *App) rutasFase4(mux *http.ServeMux) {
	a.rutasPromesas(mux)
	a.rutasExportacion(mux)
	a.rutasFirma(mux)
}
