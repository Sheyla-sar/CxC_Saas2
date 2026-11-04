package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // zonas horarias embebidas: no depender del sistema operativo (R14)

	"golang.org/x/crypto/bcrypt"
)

// App reúne las dependencias compartidas del servidor.
type App struct {
	db       *sql.DB
	cfg      Config
	sesiones *AlmacenSesiones
	intentos *LimitadorIngreso
	// hashFicticio se compara cuando el usuario no existe, para que el login tarde lo mismo.
	hashFicticio []byte
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "crear-empresa":
			os.Exit(ejecutarCrearEmpresa(os.Args[2:]))
		default:
			fmt.Fprintf(os.Stderr, "Comando desconocido %q. Comandos disponibles:\n"+
				"  go run .                  inicia el servidor\n"+
				"  go run . crear-empresa    da de alta una empresa nueva (ver --help)\n", os.Args[1])
			os.Exit(2)
		}
	}
	if err := servir(); err != nil {
		log.Fatalf("ERROR: %v", err)
	}
}

func servir() error {
	cfg, err := cargarConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()

	db, err := abrirDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := verificarRolSinPrivilegios(ctx, db); err != nil {
		return err
	}
	aplicadas, err := migrar(ctx, db, cfg.EsDesarrollo())
	if err != nil {
		return err
	}
	for _, v := range aplicadas {
		log.Printf("migración aplicada: %s", v)
	}
	if err := verificarRLS(ctx, db); err != nil {
		return err
	}
	log.Printf("aislamiento verificado: RLS habilitado y forzado en todas las tablas con empresa_id")

	if cfg.EsDesarrollo() {
		if err := imprimirCredencialesDesarrollo(ctx, db); err != nil {
			return err
		}
	}

	if err := configurarArchivos(cfg.RutaArchivos); err != nil {
		return fmt.Errorf("no se pudo preparar la carpeta de archivos adjuntos: %w", err)
	}

	hashFicticio, err := bcrypt.GenerateFromPassword([]byte("contraseña-ficticia"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	app := &App{db: db, cfg: cfg, sesiones: nuevoAlmacenSesiones(), intentos: nuevoLimitadorIngreso(), hashFicticio: hashFicticio}

	go func() {
		for range time.Tick(10 * time.Minute) {
			app.sesiones.Limpiar(time.Now())
			app.intentos.Purgar(time.Now())
		}
	}()

	srv := &http.Server{
		Addr:              ":" + cfg.Puerto,
		Handler:           app.sitio(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errServidor := make(chan error, 1)
	go func() {
		log.Printf("servidor escuchando en http://localhost:%s (entorno %s)", cfg.Puerto, cfg.Entorno)
		errServidor <- srv.ListenAndServe()
	}()

	senales := make(chan os.Signal, 1)
	signal.Notify(senales, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errServidor:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case s := <-senales:
		log.Printf("señal %v recibida, cerrando...", s)
		ctxCierre, cancelar := context.WithTimeout(ctx, 10*time.Second)
		defer cancelar()
		return srv.Shutdown(ctxCierre)
	}
	return nil
}

// sitio une la API y el frontend provisional en el mismo origen (sin CORS ni segundo servidor):
// /api/ va a rutas() sin cambios; todo lo demás se sirve desde el directorio static/.
func (a *App) sitio() http.Handler {
	raiz := http.NewServeMux()
	raiz.Handle("/api/", a.rutas())
	raiz.Handle("/", archivosEstaticos("static"))
	return raiz
}

// archivosEstaticos sirve static/ desde disco: cambiar el diseño no requiere recompilar.
func archivosEstaticos(dir string) http.Handler {
	// Windows puede registrar .js como text/plain y el navegador rechazaría los módulos ES6.
	mime.AddExtensionType(".js", "text/javascript; charset=utf-8")
	mime.AddExtensionType(".css", "text/css; charset=utf-8")
	archivos := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r) // sin listados de directorio
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		archivos.ServeHTTP(w, r)
	})
}

// Contraseñas de los usuarios que crea 002_seed_desarrollo.sql (solo desarrollo).
var contrasenasSemilla = map[string]string{
	"admin@progreso.sv":    "admin123",
	"carlos@progreso.sv":   "vendedor123",
	"admin@agroinsumos.sv": "admin123",
}

// imprimirCredencialesDesarrollo lista los usuarios de las empresas semilla, leyéndolos de la base
// empresa por empresa (con app.empresa_id fijado, igual que la app) y comprobando la contraseña.
func imprimirCredencialesDesarrollo(ctx context.Context, db *sql.DB) error {
	sembrada, err := migracionAplicada(ctx, db, "002_seed_desarrollo")
	if err != nil || !sembrada {
		return err
	}
	var b strings.Builder
	b.WriteString("\n══════════════════════ CREDENCIALES DE DESARROLLO ══════════════════════\n")
	for _, slug := range []string{"distribuidora-progreso", "agroinsumos-valle"} {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		empresa, err := buscarEmpresaActivaPorSlug(ctx, tx, slug)
		if err != nil {
			tx.Rollback()
			continue
		}
		if err := establecerEmpresa(ctx, tx, empresa.ID); err != nil {
			tx.Rollback()
			return err
		}
		usuarios, err := (&Repo{ctx: ctx, q: tx}).ListarUsuarios(empresa.ID)
		tx.Rollback()
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "\n  %s   (slug: %s)\n", empresa.NombreComercial, empresa.Slug)
		for _, u := range usuarios {
			clave, ok := contrasenasSemilla[u.Email]
			if !ok || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(clave)) != nil {
				clave = "(cambiada)"
			}
			fmt.Fprintf(&b, "    %-9s %-24s contraseña: %-12s %s\n", u.Rol, u.Email, clave, u.Nombre)
		}
	}
	b.WriteString("\n  Ejemplo: curl -c cookies.txt -X POST localhost:8080/api/login \\\n")
	b.WriteString("    -d '{\"slug\":\"distribuidora-progreso\",\"email\":\"admin@progreso.sv\",\"password\":\"admin123\"}'\n")
	b.WriteString("════════════════════════════════════════════════════════════════════════\n")
	fmt.Println(b.String())
	return nil
}
