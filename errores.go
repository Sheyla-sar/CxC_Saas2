package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

// TipoError clasifica un error de la aplicación. reglas.go solo conoce el tipo; la traducción a
// código HTTP ocurre únicamente en este archivo.
type TipoError int

const (
	ErrEntrada       TipoError = iota + 1 // 400: la petición está mal formada o incompleta
	ErrNoAutenticado                      // 401: no hay sesión válida
	ErrProhibido                          // 403: rol o empresa incorrectos
	ErrNoEncontrado                       // 404: no existe o no pertenece al usuario
	ErrRegla                              // 422: se viola una regla de negocio
	ErrDemasiados                         // 429: demasiados intentos (login)
)

// ErrorApp es un error que se puede mostrar al usuario final. Mensaje en español, legible.
type ErrorApp struct {
	Tipo    TipoError
	Codigo  string
	Mensaje string
	Detalle map[string]any
}

func (e *ErrorApp) Error() string { return e.Codigo + ": " + e.Mensaje }

// con agrega un dato al detalle del error y devuelve el mismo error, para encadenar.
func (e *ErrorApp) con(clave string, valor any) *ErrorApp {
	if e.Detalle == nil {
		e.Detalle = map[string]any{}
	}
	e.Detalle[clave] = valor
	return e
}

func errEntrada(codigo, mensaje string) *ErrorApp {
	return &ErrorApp{Tipo: ErrEntrada, Codigo: codigo, Mensaje: mensaje}
}

func errNoAutenticado(codigo, mensaje string) *ErrorApp {
	return &ErrorApp{Tipo: ErrNoAutenticado, Codigo: codigo, Mensaje: mensaje}
}

func errProhibido(codigo, mensaje string) *ErrorApp {
	return &ErrorApp{Tipo: ErrProhibido, Codigo: codigo, Mensaje: mensaje}
}

func errNoEncontrado(codigo, mensaje string) *ErrorApp {
	return &ErrorApp{Tipo: ErrNoEncontrado, Codigo: codigo, Mensaje: mensaje}
}

func errRegla(codigo, mensaje string) *ErrorApp {
	return &ErrorApp{Tipo: ErrRegla, Codigo: codigo, Mensaje: mensaje}
}

func errSoloAdmin() *ErrorApp {
	return errProhibido("SOLO_ADMIN", "Esta acción solo la puede realizar un administrador.")
}

func estadoHTTP(t TipoError) int {
	switch t {
	case ErrEntrada:
		return http.StatusBadRequest
	case ErrNoAutenticado:
		return http.StatusUnauthorized
	case ErrProhibido:
		return http.StatusForbidden
	case ErrNoEncontrado:
		return http.StatusNotFound
	case ErrRegla:
		return http.StatusUnprocessableEntity
	case ErrDemasiados:
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}

type cuerpoError struct {
	Error detalleError `json:"error"`
}

type detalleError struct {
	Codigo  string         `json:"codigo"`
	Mensaje string         `json:"mensaje"`
	Detalle map[string]any `json:"detalle,omitempty"`
}

// escribirError responde con el formato estándar. Los errores que no son ErrorApp se registran
// en el log y al usuario solo se le muestra un mensaje genérico.
func escribirError(w http.ResponseWriter, err error) {
	var e *ErrorApp
	if !errors.As(err, &e) {
		log.Printf("ERROR interno: %v", err)
		escribirJSON(w, http.StatusInternalServerError, cuerpoError{detalleError{
			Codigo:  "ERROR_INTERNO",
			Mensaje: "Ocurrió un problema inesperado. Intente de nuevo en unos momentos.",
		}})
		return
	}
	escribirJSON(w, estadoHTTP(e.Tipo), cuerpoError{detalleError{e.Codigo, e.Mensaje, e.Detalle}})
}

func escribirJSON(w http.ResponseWriter, estado int, v any) {
	cuerpo, err := json.Marshal(v)
	if err != nil {
		log.Printf("ERROR serializando respuesta: %v", err)
		estado = http.StatusInternalServerError
		cuerpo = []byte(`{"error":{"codigo":"ERROR_INTERNO","mensaje":"Ocurrió un problema inesperado."}}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(estado)
	w.Write(cuerpo)
	w.Write([]byte("\n"))
}
