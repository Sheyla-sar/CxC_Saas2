package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// apiFunc es la forma de todo handler autenticado: recibe el Tenant ya resuelto y devuelve
// (estado HTTP, cuerpo) o un error. No escribe en el ResponseWriter: así la respuesta se envía
// solo DESPUÉS de confirmar la transacción.
type apiFunc func(r *http.Request, t *Tenant) (int, any, error)

const tiempoMaximoPeticion = 30 * time.Second

// conTenant: sesión → resolverEmpresa → BEGIN → set_config('app.empresa_id') → recarga del usuario
// → handler → COMMIT → respuesta. Cualquier error hace ROLLBACK.
// abrirTenant resuelve sesión y empresa, abre la transacción y fija app.empresa_id. Devuelve el
// Tenant, la transacción y la petición con el Tenant en su context. Si algo falla, ya escribió
// la respuesta de error y devuelve ok = false.
func (a *App) abrirTenant(w http.ResponseWriter, r *http.Request) (*Tenant, *sql.Tx, *http.Request, func(), bool) {
	sesion, ok := a.sesionDe(r)
	if !ok {
		escribirError(w, errNoAutenticado("NO_AUTENTICADO", "Su sesión no es válida o expiró. Inicie sesión nuevamente."))
		return nil, nil, r, func() {}, false
	}
	empresaID, err := resolverEmpresa(r, sesion, a.cfg.DominioBase)
	if err != nil {
		escribirError(w, err)
		return nil, nil, r, func() {}, false
	}

	ctx, cancelar := context.WithTimeout(r.Context(), tiempoMaximoPeticion)
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		cancelar()
		escribirError(w, err)
		return nil, nil, r, func() {}, false
	}
	cerrar := func() {
		tx.Rollback() // sin efecto si ya hubo Commit
		cancelar()
	}
	if err := establecerEmpresa(ctx, tx, empresaID); err != nil {
		escribirError(w, err)
		cerrar()
		return nil, nil, r, func() {}, false
	}
	repo := &Repo{ctx: ctx, q: tx}

	// El rol y el estado se leen de la base en cada petición: si un admin desactiva a un
	// usuario o cambia su rol, el efecto es inmediato aunque la sesión siga en memoria.
	usuario, err := repo.ObtenerUsuario(empresaID, sesion.UsuarioID)
	var e *ErrorApp
	if errors.As(err, &e) || (err == nil && !usuario.Activo) {
		a.sesiones.Eliminar(sesion.Token)
		escribirError(w, errNoAutenticado("SESION_INVALIDA", "Su usuario ya no tiene acceso. Inicie sesión nuevamente."))
		cerrar()
		return nil, nil, r, func() {}, false
	} else if err != nil {
		escribirError(w, err)
		cerrar()
		return nil, nil, r, func() {}, false
	}

	t := &Tenant{ctx: ctx, Repo: repo, EmpresaID: empresaID, EmpresaSlug: sesion.EmpresaSlug,
		Usuario: usuario, Ahora: time.Now()}
	return t, tx, r.WithContext(context.WithValue(ctx, claveTenant{}, t)), cerrar, true
}

func (a *App) conTenant(h apiFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t, tx, r, cerrar, ok := a.abrirTenant(w, r)
		if !ok {
			return
		}
		defer cerrar()

		estado, cuerpo, err := h(r, t)
		if err != nil {
			escribirError(w, err)
			return
		}
		if err := tx.Commit(); err != nil {
			escribirError(w, err)
			return
		}
		// Los documentos (PDF y CSV) se escriben tal cual; el resto va como JSON.
		switch v := cuerpo.(type) {
		case archivoPDF:
			escribirPDF(w, v.Nombre, v.Contenido)
		case archivoCSV:
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", v.Nombre))
			w.Write(v.Contenido)
		default:
			escribirJSON(w, estado, cuerpo)
		}
	})
}

// conTenantDirecto es para respuestas que NO son JSON y que el handler escribe por su cuenta
// (la descarga de un archivo adjunto). Resuelve la sesión y el tenant igual que conTenant, pero
// la transacción es de solo lectura: se deshace al terminar.
func (a *App) conTenantDirecto(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _, r, cerrar, ok := a.abrirTenant(w, r)
		if !ok {
			return
		}
		defer cerrar() // solo lectura: la transacción se deshace al terminar
		h(w, r)
	})
}

// soloAdmin envuelve un handler que solo puede usar el rol admin (403 para vendedor).
func soloAdmin(h apiFunc) apiFunc {
	return func(r *http.Request, t *Tenant) (int, any, error) {
		if err := t.ExigirAdmin(); err != nil {
			return 0, nil, err
		}
		return h(r, t)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Registro de peticiones y recuperación de pánicos
// ─────────────────────────────────────────────────────────────────────────────

type registroRespuesta struct {
	http.ResponseWriter
	estado int
}

func (rr *registroRespuesta) WriteHeader(estado int) {
	rr.estado = estado
	rr.ResponseWriter.WriteHeader(estado)
}

func conRegistro(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		rr := &registroRespuesta{ResponseWriter: w, estado: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				log.Printf("PÁNICO en %s %s: %v\n%s", r.Method, r.URL.Path, p, debug.Stack())
				escribirError(rr, errors.New("pánico en handler"))
			}
			log.Printf("%-6s %-45s %d %s", r.Method, r.URL.RequestURI(), rr.estado, time.Since(inicio).Round(time.Millisecond))
		}()
		// Las subidas de fotos y del CSV de cartera necesitan más que el límite normal de 1 MB.
		limite := int64(1 << 20)
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			limite = tamanoMaximoSubida
		}
		r.Body = http.MaxBytesReader(w, r.Body, limite)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(rr, r)
	})
}
