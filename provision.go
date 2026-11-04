package main

// provision.go — alta de empresas nuevas. Es el ÚNICO camino para dar de alta un cliente del SaaS:
// no requiere tocar código, recompilar ni desplegar.
//
//   go run . crear-empresa --slug abarroteria-lopez --nombre-legal "..." --nombre-comercial "..." \
//       --admin-nombre "..." --admin-email "..." --admin-password "..."

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

var (
	patronSlug  = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	patronEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

type datosProvision struct {
	Slug, NombreLegal, NombreComercial, AdminNombre, AdminEmail, AdminPassword string
}

func ejecutarCrearEmpresa(args []string) int {
	fs := flag.NewFlagSet("crear-empresa", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var d datosProvision
	fs.StringVar(&d.Slug, "slug", "", "identificador de la empresa (minúsculas, números y guiones)")
	fs.StringVar(&d.NombreLegal, "nombre-legal", "", "razón social")
	fs.StringVar(&d.NombreComercial, "nombre-comercial", "", "nombre comercial")
	fs.StringVar(&d.AdminNombre, "admin-nombre", "", "nombre del administrador inicial")
	fs.StringVar(&d.AdminEmail, "admin-email", "", "correo del administrador inicial")
	fs.StringVar(&d.AdminPassword, "admin-password", "", "contraseña del administrador inicial (mínimo 8 caracteres)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		usoCrearEmpresa(fs)
		return 2
	}
	d.Slug = strings.ToLower(strings.TrimSpace(d.Slug))
	d.NombreLegal = strings.TrimSpace(d.NombreLegal)
	d.NombreComercial = strings.TrimSpace(d.NombreComercial)
	d.AdminNombre = strings.TrimSpace(d.AdminNombre)
	d.AdminEmail = strings.ToLower(strings.TrimSpace(d.AdminEmail))

	if err := validarProvision(d); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		usoCrearEmpresa(fs)
		return 2
	}

	cfg, err := cargarConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error de configuración: %v\n", err)
		return 1
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelar()
	db, err := abrirDB(ctx, cfg.DatabaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer db.Close()
	if err := verificarRolSinPrivilegios(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if _, err := migrar(ctx, db, cfg.EsDesarrollo()); err != nil {
		fmt.Fprintf(os.Stderr, "Error aplicando migraciones: %v\n", err)
		return 1
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(d.AdminPassword), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	defer tx.Rollback()
	repo := &Repo{ctx: ctx, q: tx}

	empresaID, err := repo.CrearEmpresa(d.Slug, d.NombreLegal, d.NombreComercial)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			fmt.Fprintf(os.Stderr, "Error: ya existe una empresa con el slug %q.\n", d.Slug)
			return 1
		}
		fmt.Fprintf(os.Stderr, "Error creando la empresa: %v\n", err)
		return 1
	}
	// Desde aquí, igual que en la app: todo bajo RLS con la empresa recién creada.
	if err := establecerEmpresa(ctx, tx, empresaID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if err := repo.CrearMarcaYConfig(empresaID, d.NombreComercial); err != nil {
		fmt.Fprintf(os.Stderr, "Error creando marca y configuración: %v\n", err)
		return 1
	}
	if err := repo.CrearCorrelativos(empresaID); err != nil {
		fmt.Fprintf(os.Stderr, "Error creando los correlativos de documentos: %v\n", err)
		return 1
	}
	admin, err := repo.CrearUsuario(empresaID, d.AdminNombre, d.AdminEmail, string(hash), RolAdmin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creando el administrador: %v\n", err)
		return 1
	}
	marca, err := repo.ObtenerMarca(empresaID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	conf, err := repo.ObtenerConfig(empresaID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	if err := tx.Commit(); err != nil {
		fmt.Fprintf(os.Stderr, "Error confirmando el alta: %v\n", err)
		return 1
	}

	fmt.Println("Empresa creada correctamente.")
	fmt.Println()
	fmt.Printf("  Empresa          %s\n", d.NombreComercial)
	fmt.Printf("  Razón social     %s\n", d.NombreLegal)
	fmt.Printf("  Slug             %s\n", d.Slug)
	fmt.Printf("  ID               %s\n", empresaID)
	fmt.Println()
	fmt.Printf("  Marca            %s  (colores %s / %s)\n", marca.NombreProducto, marca.ColorPrimario, marca.ColorPrimarioClaro)
	fmt.Printf("  Configuración    %s %s, zona %s, aviso %d días, plazos %v\n",
		conf.Moneda, conf.SimboloMoneda, conf.ZonaHoraria, conf.DiasAvisoPrevio, conf.PlazosCreditoPermitidos)
	fmt.Println()
	fmt.Printf("  Administrador    %s <%s>\n", admin.Nombre, admin.Email)
	fmt.Printf("  ID admin         %s\n", admin.ID)
	fmt.Println()
	fmt.Printf("  Login: POST /api/login {\"slug\":%q,\"email\":%q,\"password\":\"***\"}\n", d.Slug, d.AdminEmail)
	return 0
}

func validarProvision(d datosProvision) error {
	switch {
	case d.Slug == "" || d.NombreLegal == "" || d.NombreComercial == "" || d.AdminNombre == "" ||
		d.AdminEmail == "" || d.AdminPassword == "":
		return errors.New("todos los parámetros son obligatorios")
	case !patronSlug.MatchString(d.Slug) || len(d.Slug) < 3 || len(d.Slug) > 63:
		return fmt.Errorf("slug %q inválido: use de 3 a 63 caracteres en minúsculas, números y guiones (ej. abarroteria-lopez)", d.Slug)
	case !patronEmail.MatchString(d.AdminEmail):
		return fmt.Errorf("correo %q inválido", d.AdminEmail)
	case utf8.RuneCountInString(d.AdminPassword) < 8:
		return errors.New("la contraseña del administrador debe tener al menos 8 caracteres")
	case len(d.AdminPassword) > 72:
		return errors.New("la contraseña del administrador no puede superar 72 bytes")
	}
	return nil
}

func usoCrearEmpresa(fs *flag.FlagSet) {
	fmt.Fprintln(os.Stderr, `Uso:
  go run . crear-empresa --slug abarroteria-lopez --nombre-legal "Abarrotería López, S.A. de C.V." \
      --nombre-comercial "Abarrotería López" --admin-nombre "María López" \
      --admin-email "maria@abarroterialopez.sv" --admin-password "********"

Parámetros:`)
	fs.SetOutput(os.Stderr)
	fs.PrintDefaults()
}
