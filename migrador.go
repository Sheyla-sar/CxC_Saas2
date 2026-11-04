package main

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migraciones/*.sql
var migracionesFS embed.FS

// Clave arbitraria para pg_advisory_xact_lock: evita que dos procesos migren a la vez.
const claveBloqueoMigraciones = 20260921

// migrar aplica en orden las migraciones pendientes, cada una en su propia transacción, y las
// registra en schema_migrations. Los archivos *_desarrollo.sql solo se aplican en desarrollo.
func migrar(ctx context.Context, db *sql.DB, esDesarrollo bool) ([]string, error) {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version     TEXT PRIMARY KEY,
			aplicada_en TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return nil, fmt.Errorf("creando schema_migrations: %w", err)
	}

	archivos, err := fs.Glob(migracionesFS, "migraciones/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(archivos)

	var aplicadas []string
	for _, archivo := range archivos {
		version := strings.TrimSuffix(strings.TrimPrefix(archivo, "migraciones/"), ".sql")
		if strings.HasSuffix(version, "_desarrollo") && !esDesarrollo {
			continue
		}
		aplicada, err := aplicarMigracion(ctx, db, archivo, version)
		if err != nil {
			return aplicadas, fmt.Errorf("migración %s: %w", version, err)
		}
		if aplicada {
			aplicadas = append(aplicadas, version)
		}
	}
	return aplicadas, nil
}

func aplicarMigracion(ctx context.Context, db *sql.DB, archivo, version string) (bool, error) {
	contenido, err := migracionesFS.ReadFile(archivo)
	if err != nil {
		return false, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, claveBloqueoMigraciones); err != nil {
		return false, err
	}
	var existe bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&existe); err != nil {
		return false, err
	}
	if existe {
		return false, nil
	}
	// Sin argumentos, pgx usa el protocolo simple: admite varios comandos en un solo Exec.
	if _, err := tx.ExecContext(ctx, string(contenido)); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func migracionAplicada(ctx context.Context, db *sql.DB, version string) (bool, error) {
	var existe bool
	err := db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&existe)
	return existe, err
}
