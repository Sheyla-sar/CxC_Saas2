package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	nombreCookieSesion = "sesion"
	// Duración de plataforma (no de empresa). Las sesiones viven en PostgreSQL (tabla sesion).
	duracionSesion = 12 * time.Hour

	// Límite de intentos fallidos de ingreso (fuerza bruta). De plataforma, no de empresa.
	ventanaIntentos    = 15 * time.Minute
	maxFallosPorCuenta = 5  // misma empresa + correo
	maxFallosPorOrigen = 30 // misma IP, cualquier cuenta
)

// ─────────────────────────────────────────────────────────────────────────────
// Limitador de intentos de ingreso (en PostgreSQL, tabla intento_ingreso)
// ─────────────────────────────────────────────────────────────────────────────

// Va en la base y no en memoria para que el límite valga aunque la petición caiga en otra
// instancia (entornos sin estado, como Vercel) y sobreviva a un reinicio.

// tiempoMaximoBase acota cada consulta de sesiones e intentos: si la base no responde, el login
// falla pronto en lugar de colgar la petición.
const tiempoMaximoBase = 5 * time.Second

func contextoBase() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), tiempoMaximoBase)
}

type LimitadorIngreso struct {
	db *sql.DB
}

func nuevoLimitadorIngreso(db *sql.DB) *LimitadorIngreso {
	return &LimitadorIngreso{db: db}
}

func claveCuenta(slug, email string) string { return "cuenta|" + slug + "|" + email }
func claveOrigen(ip string) string          { return "origen|" + ip }

// Bloqueado indica si alguna de las claves superó su límite y cuánto falta para reintentar.
// Si la base no responde devuelve false: sin base tampoco hay login que proteger.
func (l *LimitadorIngreso) Bloqueado(ahora time.Time, cuenta, origen string) (bool, time.Duration) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	filas, err := l.db.QueryContext(ctx,
		`SELECT clave, cantidad, desde FROM intento_ingreso WHERE clave IN ($1, $2)`, cuenta, origen)
	if err != nil {
		log.Printf("limitador de ingreso: %v", err)
		return false, 0
	}
	defer filas.Close()
	maximos := map[string]int{cuenta: maxFallosPorCuenta, origen: maxFallosPorOrigen}
	var espera time.Duration
	for filas.Next() {
		var clave string
		var cantidad int
		var desde time.Time
		if err := filas.Scan(&clave, &cantidad, &desde); err != nil {
			log.Printf("limitador de ingreso: %v", err)
			return false, 0
		}
		if ahora.Sub(desde) > ventanaIntentos {
			continue
		}
		if cantidad >= maximos[clave] {
			espera = max(espera, ventanaIntentos-ahora.Sub(desde))
		}
	}
	return espera > 0, espera
}

func (l *LimitadorIngreso) RegistrarFallo(ahora time.Time, claves ...string) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	vencidos := ahora.Add(-ventanaIntentos)
	for _, clave := range claves {
		// Si el registro es de una ventana anterior, empieza de nuevo en 1.
		_, err := l.db.ExecContext(ctx, `
			INSERT INTO intento_ingreso (clave, cantidad, desde) VALUES ($1, 1, $2)
			ON CONFLICT (clave) DO UPDATE SET
				cantidad = CASE WHEN intento_ingreso.desde < $3 THEN 1 ELSE intento_ingreso.cantidad + 1 END,
				desde    = CASE WHEN intento_ingreso.desde < $3 THEN $2 ELSE intento_ingreso.desde END`,
			clave, ahora, vencidos)
		if err != nil {
			log.Printf("limitador de ingreso: %v", err)
		}
	}
}

func (l *LimitadorIngreso) Limpiar(clave string) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	if _, err := l.db.ExecContext(ctx, `DELETE FROM intento_ingreso WHERE clave = $1`, clave); err != nil {
		log.Printf("limitador de ingreso: %v", err)
	}
}

// Purgar elimina registros vencidos. Se llama periódicamente desde main.
func (l *LimitadorIngreso) Purgar(ahora time.Time) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	if _, err := l.db.ExecContext(ctx,
		`DELETE FROM intento_ingreso WHERE desde < $1`, ahora.Add(-ventanaIntentos)); err != nil {
		log.Printf("limitador de ingreso: %v", err)
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

// AlmacenSesiones guarda las sesiones en PostgreSQL (tabla sesion). Sin JWT: el token de la cookie
// es aleatorio y opaco, y en la base solo queda su SHA-256, así que quien lea la tabla no puede
// reconstruir una cookie válida.
type AlmacenSesiones struct {
	db *sql.DB
}

func nuevoAlmacenSesiones(db *sql.DB) *AlmacenSesiones {
	return &AlmacenSesiones{db: db}
}

func hashDeToken(token string) string {
	suma := sha256.Sum256([]byte(token))
	return hex.EncodeToString(suma[:])
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
	ctx, cancelar := contextoBase()
	defer cancelar()
	if _, err := a.db.ExecContext(ctx, `
		INSERT INTO sesion (token_hash, usuario_id, empresa_ref, empresa_slug, expira)
		VALUES ($1, $2, $3, $4, $5)`,
		hashDeToken(s.Token), s.UsuarioID, s.EmpresaID, s.EmpresaSlug, s.Expira); err != nil {
		return Sesion{}, err
	}
	return s, nil
}

func (a *AlmacenSesiones) Obtener(token string, ahora time.Time) (Sesion, bool) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	s := Sesion{Token: token}
	err := a.db.QueryRowContext(ctx, `
		SELECT usuario_id, empresa_ref, empresa_slug, expira
		FROM sesion WHERE token_hash = $1`, hashDeToken(token)).
		Scan(&s.UsuarioID, &s.EmpresaID, &s.EmpresaSlug, &s.Expira)
	if errors.Is(err, sql.ErrNoRows) {
		return Sesion{}, false
	}
	if err != nil {
		log.Printf("sesiones: %v", err)
		return Sesion{}, false
	}
	if ahora.After(s.Expira) {
		a.Eliminar(token)
		return Sesion{}, false
	}
	return s, true
}

func (a *AlmacenSesiones) Eliminar(token string) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	if _, err := a.db.ExecContext(ctx, `DELETE FROM sesion WHERE token_hash = $1`, hashDeToken(token)); err != nil {
		log.Printf("sesiones: %v", err)
	}
}

// Limpiar elimina las sesiones vencidas. Se llama periódicamente desde main.
func (a *AlmacenSesiones) Limpiar(ahora time.Time) {
	ctx, cancelar := contextoBase()
	defer cancelar()
	if _, err := a.db.ExecContext(ctx, `DELETE FROM sesion WHERE expira < $1`, ahora); err != nil {
		log.Printf("sesiones: %v", err)
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
