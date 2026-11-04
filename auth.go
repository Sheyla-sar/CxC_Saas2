package main

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	nombreCookieSesion = "sesion"
	// Duración de plataforma (no de empresa). Reiniciar la app cierra todas las sesiones.
	duracionSesion = 12 * time.Hour

	// Límite de intentos fallidos de ingreso (fuerza bruta). De plataforma, no de empresa.
	ventanaIntentos    = 15 * time.Minute
	maxFallosPorCuenta = 5  // misma empresa + correo
	maxFallosPorOrigen = 30 // misma IP, cualquier cuenta
)

// ─────────────────────────────────────────────────────────────────────────────
// Limitador de intentos de ingreso (en memoria, como las sesiones)
// ─────────────────────────────────────────────────────────────────────────────

type registroFallos struct {
	cantidad int
	desde    time.Time
}

type LimitadorIngreso struct {
	mu     sync.Mutex
	fallos map[string]registroFallos
}

func nuevoLimitadorIngreso() *LimitadorIngreso {
	return &LimitadorIngreso{fallos: map[string]registroFallos{}}
}

func claveCuenta(slug, email string) string { return "cuenta|" + slug + "|" + email }
func claveOrigen(ip string) string          { return "origen|" + ip }

// Bloqueado indica si alguna de las claves superó su límite y cuánto falta para reintentar.
func (l *LimitadorIngreso) Bloqueado(ahora time.Time, cuenta, origen string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var espera time.Duration
	for clave, maximo := range map[string]int{cuenta: maxFallosPorCuenta, origen: maxFallosPorOrigen} {
		r, ok := l.fallos[clave]
		if !ok || ahora.Sub(r.desde) > ventanaIntentos {
			continue
		}
		if r.cantidad >= maximo {
			espera = max(espera, ventanaIntentos-ahora.Sub(r.desde))
		}
	}
	return espera > 0, espera
}

func (l *LimitadorIngreso) RegistrarFallo(ahora time.Time, claves ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, clave := range claves {
		r, ok := l.fallos[clave]
		if !ok || ahora.Sub(r.desde) > ventanaIntentos {
			r = registroFallos{desde: ahora}
		}
		r.cantidad++
		l.fallos[clave] = r
	}
}

func (l *LimitadorIngreso) Limpiar(clave string) {
	l.mu.Lock()
	delete(l.fallos, clave)
	l.mu.Unlock()
}

// Purgar elimina registros vencidos. Se llama periódicamente desde main.
func (l *LimitadorIngreso) Purgar(ahora time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for clave, r := range l.fallos {
		if ahora.Sub(r.desde) > ventanaIntentos {
			delete(l.fallos, clave)
		}
	}
}

func ipDe(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

type Sesion struct {
	Token       string
	UsuarioID   string
	EmpresaID   string
	EmpresaSlug string
	Expira      time.Time
}

// AlmacenSesiones guarda las sesiones en memoria, protegidas por un mutex. Sin JWT.
type AlmacenSesiones struct {
	mu       sync.Mutex
	sesiones map[string]Sesion
}

func nuevoAlmacenSesiones() *AlmacenSesiones {
	return &AlmacenSesiones{sesiones: map[string]Sesion{}}
}

func (a *AlmacenSesiones) Crear(usuarioID, empresaID, slug string, ahora time.Time) (Sesion, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return Sesion{}, err
	}
	s := Sesion{
		Token:       base64.RawURLEncoding.EncodeToString(b),
		UsuarioID:   usuarioID,
		EmpresaID:   empresaID,
		EmpresaSlug: slug,
		Expira:      ahora.Add(duracionSesion),
	}
	a.mu.Lock()
	a.sesiones[s.Token] = s
	a.mu.Unlock()
	return s, nil
}

func (a *AlmacenSesiones) Obtener(token string, ahora time.Time) (Sesion, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sesiones[token]
	if !ok {
		return Sesion{}, false
	}
	if ahora.After(s.Expira) {
		delete(a.sesiones, token)
		return Sesion{}, false
	}
	return s, true
}

func (a *AlmacenSesiones) Eliminar(token string) {
	a.mu.Lock()
	delete(a.sesiones, token)
	a.mu.Unlock()
}

// Limpiar elimina las sesiones vencidas. Se llama periódicamente desde main.
func (a *AlmacenSesiones) Limpiar(ahora time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for t, s := range a.sesiones {
		if ahora.After(s.Expira) {
			delete(a.sesiones, t)
		}
	}
}

func (a *App) sesionDe(r *http.Request) (Sesion, bool) {
	c, err := r.Cookie(nombreCookieSesion)
	if err != nil || c.Value == "" {
		return Sesion{}, false
	}
	return a.sesiones.Obtener(c.Value, time.Now())
}

func (a *App) cookieSesion(valor string, expira time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     nombreCookieSesion,
		Value:    valor,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !a.cfg.EsDesarrollo(),
		Expires:  expira,
	}
	if valor == "" {
		c.MaxAge = -1
	}
	return c
}

// ─────────────────────────────────────────────────────────────────────────────
// Handlers de autenticación
// ─────────────────────────────────────────────────────────────────────────────

type respuestaSesion struct {
	Usuario Usuario        `json:"usuario"`
	Empresa empresaPublica `json:"empresa"`
}

type empresaPublica struct {
	ID              string `json:"id"`
	Slug            string `json:"slug"`
	NombreComercial string `json:"nombre_comercial"`
}

func errCredenciales() *ErrorApp {
	return errNoAutenticado("CREDENCIALES_INVALIDAS", "El correo, la contraseña o la empresa no son correctos.")
}

// login: email + contraseña + slug de empresa. El mensaje de error es el mismo exista o no la
// empresa o el correo, y siempre se ejecuta una comparación bcrypt para no revelar nada por tiempo.
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Slug     string `json:"slug"`
	}
	if err := leerJSON(r, &in); err != nil {
		escribirError(w, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(in.Email))
	slug := strings.ToLower(strings.TrimSpace(in.Slug))
	solicitado := slugSolicitado(r, a.cfg.DominioBase)
	if slug == "" {
		slug = solicitado
	}
	if email == "" || in.Password == "" || slug == "" {
		escribirError(w, errEntrada("DATOS_INCOMPLETOS", "Ingrese su correo, su contraseña y la empresa."))
		return
	}
	if solicitado != "" && solicitado != slug {
		escribirError(w, errProhibido("EMPRESA_NO_COINCIDE", "La empresa indicada no coincide con la dirección desde la que ingresa."))
		return
	}

	// Se revisa ANTES de bcrypt: un ataque bloqueado tampoco consume CPU.
	ahora := time.Now()
	cuenta, origen := claveCuenta(slug, email), claveOrigen(ipDe(r))
	if bloqueado, espera := a.intentos.Bloqueado(ahora, cuenta, origen); bloqueado {
		minutos := int(espera.Minutes()) + 1
		w.Header().Set("Retry-After", fmt.Sprint(int(espera.Seconds())+1))
		escribirError(w, (&ErrorApp{Tipo: ErrDemasiados, Codigo: "DEMASIADOS_INTENTOS",
			Mensaje: fmt.Sprintf("Hubo demasiados intentos fallidos. Espere %d minuto(s) antes de intentar de nuevo.", minutos)}).
			con("reintentar_en_segundos", int(espera.Seconds())+1))
		return
	}

	usuario, empresa, err := a.verificarCredenciales(r, slug, email, in.Password)
	var credenciales *ErrorApp
	if errors.As(err, &credenciales) && credenciales.Codigo == "CREDENCIALES_INVALIDAS" {
		a.intentos.RegistrarFallo(ahora, cuenta, origen)
	}
	if err != nil {
		escribirError(w, err)
		return
	}
	a.intentos.Limpiar(cuenta)
	s, err := a.sesiones.Crear(usuario.ID, empresa.ID, empresa.Slug, time.Now())
	if err != nil {
		escribirError(w, err)
		return
	}
	http.SetCookie(w, a.cookieSesion(s.Token, s.Expira))
	escribirJSON(w, http.StatusOK, respuestaSesion{usuario, empresaPublica{empresa.ID, empresa.Slug, empresa.NombreComercial}})
}

func (a *App) verificarCredenciales(r *http.Request, slug, email, password string) (Usuario, Empresa, error) {
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return Usuario{}, Empresa{}, err
	}
	defer tx.Rollback()

	empresa, err := buscarEmpresaActivaPorSlug(ctx, tx, slug)
	var e *ErrorApp
	if errors.As(err, &e) {
		bcrypt.CompareHashAndPassword(a.hashFicticio, []byte(password))
		return Usuario{}, Empresa{}, errCredenciales()
	} else if err != nil {
		return Usuario{}, Empresa{}, err
	}
	if err := establecerEmpresa(ctx, tx, empresa.ID); err != nil {
		return Usuario{}, Empresa{}, err
	}
	repo := &Repo{ctx: ctx, q: tx}
	usuario, err := repo.BuscarUsuarioPorEmail(empresa.ID, email)
	if errors.As(err, &e) || (err == nil && !usuario.Activo) {
		bcrypt.CompareHashAndPassword(a.hashFicticio, []byte(password))
		return Usuario{}, Empresa{}, errCredenciales()
	} else if err != nil {
		return Usuario{}, Empresa{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(usuario.PasswordHash), []byte(password)) != nil {
		return Usuario{}, Empresa{}, errCredenciales()
	}
	return usuario, empresa, tx.Commit()
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(nombreCookieSesion); err == nil {
		a.sesiones.Eliminar(c.Value)
	}
	http.SetCookie(w, a.cookieSesion("", time.Unix(0, 0)))
	escribirJSON(w, http.StatusOK, map[string]string{"mensaje": "Sesión cerrada."})
}
