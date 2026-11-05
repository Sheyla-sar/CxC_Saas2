package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

// querier es lo común entre *sql.DB y *sql.Tx.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// maxConexiones acota el pool (0 = 20). En entornos sin estado, como Vercel, cada instancia abre el
// suyo y la plataforma limita los descriptores de archivo, así que ahí conviene 5.
func abrirDB(ctx context.Context, url string, maxConexiones int) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	// pgx prepara y cachea cada sentencia por conexión. Con planes genéricos, Postgres puede seguir
	// usando un plan calculado cuando las tablas eran pequeñas: tras una carga masiva (alta de un
	// cliente con su cartera histórica) /api/resumen llegó a tardar más de 30 s hasta el siguiente
	// autoanálisis. Los planes a medida se recalculan con el tamaño real de las tablas y con los
	// filtros opcionales ($2 IS NULL OR ...) ya resueltos. El costo de planificar es de milisegundos.
	//
	// Un pooler de conexiones en modo transacción (host con "-pooler", como el de Neon) rechaza
	// parámetros de arranque como plan_cache_mode y no admite sentencias preparadas por
	// conexión. Ahí se usa el protocolo simple: cada consulta se planifica con sus valores, que es
	// justo lo que force_custom_plan buscaba.
	if strings.Contains(cfg.Host, "-pooler") {
		cfg.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	} else if _, definido := cfg.RuntimeParams["plan_cache_mode"]; !definido {
		cfg.RuntimeParams["plan_cache_mode"] = "force_custom_plan"
	}
	if maxConexiones <= 0 {
		maxConexiones = 20
	}
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(maxConexiones)
	db.SetMaxIdleConns(min(10, maxConexiones))
	db.SetConnMaxLifetime(30 * time.Minute)

	// Postgres puede estar arrancando todavía (docker compose up -d recién ejecutado).
	var ultimo error
	for intento := 0; intento < 15; intento++ {
		ctxPing, cancelar := context.WithTimeout(ctx, 2*time.Second)
		ultimo = db.PingContext(ctxPing)
		cancelar()
		if ultimo == nil {
			return db, nil
		}
		time.Sleep(time.Second)
	}
	db.Close()
	return nil, fmt.Errorf("no se pudo conectar a PostgreSQL: %w", ultimo)
}

// establecerEmpresa fija app.empresa_id para la transacción actual. Es el equivalente
// parametrizable de SET LOCAL (SET no acepta parámetros $1). Las políticas RLS leen este valor.
func establecerEmpresa(ctx context.Context, tx *sql.Tx, empresaID string) error {
	_, err := tx.ExecContext(ctx, `SELECT set_config('app.empresa_id', $1, true)`, empresaID)
	return err
}

// verificarRolSinPrivilegios impide arrancar con un rol que ignore RLS. Un superusuario o un rol
// con BYPASSRLS vería los datos de todas las empresas aunque las políticas existan.
func verificarRolSinPrivilegios(ctx context.Context, db *sql.DB) error {
	var rol string
	var super, bypass bool
	err := db.QueryRowContext(ctx,
		`SELECT rolname, rolsuper, rolbypassrls FROM pg_roles WHERE rolname = current_user`).
		Scan(&rol, &super, &bypass)
	if err != nil {
		return err
	}
	if super || bypass {
		return fmt.Errorf("el rol %q es superusuario o tiene BYPASSRLS: Row Level Security no lo "+
			"restringiría. Conéctese con un rol sin esos privilegios (ver docker-compose.yml)", rol)
	}
	return nil
}

// verificarRLS impide arrancar si alguna tabla con empresa_id no tiene RLS habilitado Y forzado.
// Así, una migración futura que olvide el RLS se detecta en el primer arranque.
func verificarRLS(ctx context.Context, db *sql.DB) error {
	filas, err := db.QueryContext(ctx, `
		SELECT c.relname
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid AND a.attname = 'empresa_id' AND NOT a.attisdropped
		WHERE n.nspname = 'public' AND c.relkind = 'r'
		  AND NOT (c.relrowsecurity AND c.relforcerowsecurity)
		ORDER BY c.relname`)
	if err != nil {
		return err
	}
	defer filas.Close()
	var sinRLS []string
	for filas.Next() {
		var t string
		if err := filas.Scan(&t); err != nil {
			return err
		}
		sinRLS = append(sinRLS, t)
	}
	if err := filas.Err(); err != nil {
		return err
	}
	if len(sinRLS) > 0 {
		return fmt.Errorf("tablas con empresa_id sin RLS habilitado y forzado: %s", strings.Join(sinRLS, ", "))
	}

	// Prueba en vivo: sin app.empresa_id, la base no debe devolver ningún usuario.
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var visibles int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM usuario`).Scan(&visibles); err != nil {
		return err
	}
	if visibles != 0 {
		return fmt.Errorf("RLS no está filtrando: sin empresa definida se ven %d usuarios", visibles)
	}
	return nil
}

// arregloPG convierte una lista de valores (UUIDs validados) al literal de arreglo de Postgres.
func arregloPG[T any](valores []T) string {
	partes := make([]string, len(valores))
	for i, v := range valores {
		partes[i] = fmt.Sprint(v)
	}
	return "{" + strings.Join(partes, ",") + "}"
}
