package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Valores de ALMACEN_ARCHIVOS: dónde se guardan las fotos y PDF que suben los usuarios.
const (
	almacenEnDisco       = "disco" // carpeta RUTA_ARCHIVOS (VPS con volumen persistente)
	almacenEnBaseDeDatos = "db"    // columna adjunto.contenido (Vercel: el disco es efímero)
)

// Config es la configuración de la PLATAFORMA (no de una empresa). Todo lo que varía por
// empresa vive en empresa_config / empresa_marca, nunca aquí.
type Config struct {
	Puerto      string
	DatabaseURL string
	Entorno     string
	// RutaArchivos es la carpeta donde se guardan las fotos y PDF que suben los usuarios.
	RutaArchivos string
	// DominioBase habilita la resolución por subdominio: <slug>.<DominioBase>. Vacío = deshabilitado.
	DominioBase string
	// AlmacenArchivos decide dónde viven los adjuntos: "disco" (por defecto) o "db".
	AlmacenArchivos string
	// MaxConexionesDB acota el pool hacia PostgreSQL. En entornos sin estado cada instancia abre
	// su propio pool, así que conviene un número chico (5). 0 = valor por defecto (20).
	MaxConexionesDB int
}

// URL que coincide con docker-compose.yml, para que `go run .` funcione sin .env en desarrollo.
const databaseURLDesarrollo = "postgres://cxc_app:cxc_app_dev@localhost:5433/cxc?sslmode=disable"

func cargarConfig() (Config, error) {
	if err := cargarArchivoEnv(".env"); err != nil {
		return Config{}, err
	}
	c := Config{
		// PORT es el nombre que usan Vercel y otras plataformas; PUERTO manda si ambas existen.
		Puerto:       valorEnv("PUERTO", valorEnv("PORT", "8080")),
		Entorno:      valorEnv("ENTORNO", "dev"),
		DatabaseURL:  strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DominioBase:  strings.ToLower(strings.TrimSpace(os.Getenv("DOMINIO_BASE"))),
		RutaArchivos: valorEnv("RUTA_ARCHIVOS", "archivos"),
		AlmacenArchivos: strings.ToLower(valorEnv("ALMACEN_ARCHIVOS", almacenEnDisco)),
	}
	if c.AlmacenArchivos != almacenEnDisco && c.AlmacenArchivos != almacenEnBaseDeDatos {
		return Config{}, fmt.Errorf("ALMACEN_ARCHIVOS debe ser %q o %q, no %q",
			almacenEnDisco, almacenEnBaseDeDatos, c.AlmacenArchivos)
	}
	if v := strings.TrimSpace(os.Getenv("DB_MAX_CONEXIONES")); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 100 {
			return Config{}, fmt.Errorf("DB_MAX_CONEXIONES debe ser un entero entre 1 y 100, no %q", v)
		}
		c.MaxConexionesDB = n
	}
	if c.DatabaseURL == "" {
		if !c.EsDesarrollo() {
			return Config{}, errors.New("DATABASE_URL es obligatoria cuando ENTORNO no es dev")
		}
		c.DatabaseURL = databaseURLDesarrollo
	}
	return c, nil
}

func (c Config) EsDesarrollo() bool { return c.Entorno == "dev" }

func valorEnv(clave, porDefecto string) string {
	if v := strings.TrimSpace(os.Getenv(clave)); v != "" {
		return v
	}
	return porDefecto
}

// cargarArchivoEnv lee un archivo CLAVE=VALOR si existe. No pisa variables ya definidas en el
// entorno, así el entorno real siempre gana sobre el archivo.
func cargarArchivoEnv(ruta string) error {
	f, err := os.Open(ruta)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		linea := strings.TrimSpace(sc.Text())
		if linea == "" || strings.HasPrefix(linea, "#") {
			continue
		}
		clave, valor, ok := strings.Cut(linea, "=")
		if !ok {
			continue
		}
		clave = strings.TrimSpace(strings.TrimPrefix(clave, "export "))
		valor = strings.Trim(strings.TrimSpace(valor), `"'`)
		if _, existe := os.LookupEnv(clave); !existe {
			os.Setenv(clave, valor)
		}
	}
	return sc.Err()
}
