package main

import (
	"bufio"
	"errors"
	"os"
	"strings"
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
}

// URL que coincide con docker-compose.yml, para que `go run .` funcione sin .env en desarrollo.
const databaseURLDesarrollo = "postgres://cxc_app:cxc_app_dev@localhost:5433/cxc?sslmode=disable"

func cargarConfig() (Config, error) {
	if err := cargarArchivoEnv(".env"); err != nil {
		return Config{}, err
	}
	c := Config{
		Puerto:       valorEnv("PUERTO", "8080"),
		Entorno:      valorEnv("ENTORNO", "dev"),
		DatabaseURL:  strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DominioBase:  strings.ToLower(strings.TrimSpace(os.Getenv("DOMINIO_BASE"))),
		RutaArchivos: valorEnv("RUTA_ARCHIVOS", "archivos"),
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
