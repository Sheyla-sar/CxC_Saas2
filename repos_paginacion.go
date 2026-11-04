package main

// repos_paginacion.go — listados paginados. Traen UNA página de la base en vez de toda la cartera.
//
// Para poder filtrar y paginar en SQL hay que saber el saldo y el estado de cada cuenta dentro de
// la base. Las fórmulas siguen siendo las de reglas.go (R12 y R13): aquí están escritas en SQL una
// sola vez, en `expresionSaldoSQL` y `condicionEstadoSQL`. La sección 13 de `verificar_cobranza.sh`
// comprueba que SQL y Go clasifiquen exactamente igual: si una fórmula cambia en reglas.go hay que
// cambiarla también aquí, y esa comprobación lo detecta si se olvida.
//
// Las filas que se devuelven se vuelven a evaluar con EvaluarCuenta: lo que ve el usuario siempre
// sale de reglas.go, nunca de la base.

import (
	"fmt"
	"strings"
)

// expresionSaldoSQL es R12: monto total − pagos − condonaciones − notas de crédito no anulados.
const expresionSaldoSQL = `(c.monto_total_centavos - COALESCE(SUM(p.monto_centavos)
	FILTER (WHERE p.tipo IN ('pago', 'condonacion', 'nota_credito') AND NOT p.anulado), 0))`

// condicionEstadoSQL es R13 escrita para un HAVING. Devuelve también los parámetros que la
// condición necesita, en orden: cada estado usa distintos (pagado no mira la fecha; vencido solo
// mira hoy), y Postgres rechaza una consulta con parámetros de más.
func condicionEstadoSQL(estado string, hoy Fecha, diasAviso, posicion int) (string, []any) {
	p := func(n int) string { return fmt.Sprintf("$%d", posicion+n) }
	switch estado {
	case EstadoPagado:
		return fmt.Sprintf(" HAVING %s <= 0", expresionSaldoSQL), nil
	case EstadoVencido:
		return fmt.Sprintf(" HAVING %s > 0 AND c.fecha_vencimiento < %s::date",
			expresionSaldoSQL, p(0)), []any{hoy}
	case EstadoPorVencer:
		return fmt.Sprintf(" HAVING %s > 0 AND c.fecha_vencimiento >= %s::date AND c.fecha_vencimiento - %s::date <= %s::int",
			expresionSaldoSQL, p(0), p(0), p(1)), []any{hoy, diasAviso}
	case EstadoAlDia:
		return fmt.Sprintf(" HAVING %s > 0 AND c.fecha_vencimiento >= %s::date AND c.fecha_vencimiento - %s::date > %s::int",
			expresionSaldoSQL, p(0), p(0), p(1)), []any{hoy, diasAviso}
	}
	return "", nil
}

// ConsultaCuentas reúne los filtros de un listado de cuentas.
type ConsultaCuentas struct {
	Alcance   Alcance
	ClienteID string // vacío = todos
	Estado    string // vacío o "todas" = sin filtro
	Buscar    string // número de documento o nombre del cliente
	Hoy       Fecha
	DiasAviso int
	Limite    int
	Desde     int // desplazamiento (OFFSET)
}

// condicionBusqueda arma el filtro de texto. Se busca por número de documento y por nombre del
// cliente, que es como la gente busca "la factura de la tienda de la esquina".
func condicionBusqueda(buscar string, posicion int) (string, any) {
	if strings.TrimSpace(buscar) == "" {
		return "", nil
	}
	cond := fmt.Sprintf(" AND ($%d::text IS NULL OR c.numero_documento ILIKE $%d OR cl.nombre ILIKE $%d)",
		posicion, posicion, posicion)
	return cond, "%" + strings.TrimSpace(buscar) + "%"
}

// ListarCuentasPagina devuelve una página de cuentas y el total que cumple el filtro.
//
// Los parámetros se arman en orden y solo se incluyen los que la consulta realmente usa: Postgres
// rechaza una consulta a la que se le pasan parámetros de más.
func (r *Repo) ListarCuentasPagina(empresaID string, q ConsultaCuentas) ([]Cuenta, int, error) {
	parametros := []any{empresaID, q.Alcance.filtro(), filtroOpcional(q.ClienteID)}

	having := ""
	if q.Estado != "" && q.Estado != EstadoTodas {
		condicion, extra := condicionEstadoSQL(q.Estado, q.Hoy, q.DiasAviso, len(parametros)+1)
		having = condicion
		parametros = append(parametros, extra...)
	}
	busqueda, valor := condicionBusqueda(q.Buscar, len(parametros)+1)
	if valor != nil {
		parametros = append(parametros, valor)
	}

	total, err := r.contarCuentas(busqueda, having, parametros)
	if err != nil {
		return nil, 0, err
	}
	consulta := selectCuenta + ` AND ($3::uuid IS NULL OR c.cliente_id = $3)` + busqueda + agruparCuenta +
		having + fmt.Sprintf(` ORDER BY c.fecha_vencimiento, c.creado_en LIMIT $%d OFFSET $%d`,
		len(parametros)+1, len(parametros)+2)
	filas, err := r.q.QueryContext(r.ctx, consulta, append(parametros, q.Limite, q.Desde)...)
	if err != nil {
		return nil, 0, err
	}
	defer filas.Close()
	res := []Cuenta{}
	for filas.Next() {
		c, err := escanearCuenta(filas)
		if err != nil {
			return nil, 0, err
		}
		res = append(res, c)
	}
	return res, total, filas.Err()
}

// contarCuentas cuenta las cuentas que cumplen el mismo filtro, para saber cuántas páginas hay.
func (r *Repo) contarCuentas(busqueda, having string, parametros []any) (int, error) {
	interna := `
		SELECT c.id
		FROM cuenta c
		JOIN cliente cl ON cl.empresa_id = c.empresa_id AND cl.id = c.cliente_id
		LEFT JOIN pago p ON p.empresa_id = c.empresa_id AND p.cuenta_id = c.id
		WHERE c.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)
		  AND ($3::uuid IS NULL OR c.cliente_id = $3)` + busqueda + ` GROUP BY c.id` + having
	var n int
	err := r.q.QueryRowContext(r.ctx, `SELECT COUNT(*) FROM (`+interna+`) x`, parametros...).Scan(&n)
	return n, err
}

// ListarClientesPagina devuelve una página de clientes y el total. El saldo de cada uno NO se
// calcula aquí: el handler trae las cuentas de esos clientes y usa reglas.go.
func (r *Repo) ListarClientesPagina(empresaID string, alc Alcance, incluirInactivos bool, buscar string, limite, desde int) ([]Cliente, int, error) {
	parametros := []any{empresaID, alc.filtro(), incluirInactivos}
	filtroTexto := ""
	if b := strings.TrimSpace(buscar); b != "" {
		filtroTexto = " AND (c.nombre ILIKE $4 OR c.codigo_externo ILIKE $4 OR c.telefono ILIKE $4)"
		parametros = append(parametros, "%"+b+"%")
	}
	condiciones := ` AND (c.activo OR $3)` + filtroTexto

	var total int
	err := r.q.QueryRowContext(r.ctx, `
		SELECT COUNT(*) FROM cliente c
		WHERE c.empresa_id = $1 AND ($2::uuid IS NULL OR c.vendedor_id = $2)`+condiciones,
		parametros...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	consulta := selectCliente + condiciones +
		fmt.Sprintf(` ORDER BY c.nombre LIMIT $%d OFFSET $%d`, len(parametros)+1, len(parametros)+2)
	filas, err := r.q.QueryContext(r.ctx, consulta, append(parametros, limite, desde)...)
	if err != nil {
		return nil, 0, err
	}
	defer filas.Close()
	res := []Cliente{}
	for filas.Next() {
		c, err := escanearCliente(filas)
		if err != nil {
			return nil, 0, err
		}
		res = append(res, c)
	}
	return res, total, filas.Err()
}

// CuentasDeClientes trae las cuentas de un puñado de clientes (los de una página). Es lo que
// permite mostrar el saldo de cada cliente sin leer la cartera entera.
func (r *Repo) CuentasDeClientes(empresaID string, alc Alcance, clienteIDs []string) ([]Cuenta, error) {
	if len(clienteIDs) == 0 {
		return []Cuenta{}, nil
	}
	consulta := selectCuenta + ` AND c.cliente_id = ANY($3::uuid[])` + agruparCuenta +
		` ORDER BY c.fecha_vencimiento, c.creado_en`
	filas, err := r.q.QueryContext(r.ctx, consulta, empresaID, alc.filtro(), arregloPG(clienteIDs))
	if err != nil {
		return nil, err
	}
	defer filas.Close()
	res := []Cuenta{}
	for filas.Next() {
		c, err := escanearCuenta(filas)
		if err != nil {
			return nil, err
		}
		res = append(res, c)
	}
	return res, filas.Err()
}

// filtroOpcional convierte "" en NULL, para los parámetros `($n::uuid IS NULL OR ...)`.
func filtroOpcional(valor string) any {
	if strings.TrimSpace(valor) == "" {
		return nil
	}
	return valor
}
