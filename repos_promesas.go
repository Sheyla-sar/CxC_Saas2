package main

// repos_promesas.go — TODO el SQL de las promesas de pago (fase 4).
// Mismas reglas que repos.go: empresaID como PRIMER parámetro y filtro por empresa_id en todo
// query. El alcance del usuario se aplica por el vendedor del cliente (R37/R39).
//
// Sobre la ventana de cumplimiento: una promesa se cumple con los pagos que entran entre el día
// en que se anotó y la fecha prometida, ambos inclusive. Ese día inicial sale de `creado_en`
// convertido a la zona horaria de la empresa (R14), que viaja como parámetro $3: en El Salvador
// una promesa anotada a las 7 de la noche es de ESE día, no del siguiente como diría UTC.
//
// La condición del estado (R80) está escrita aquí en SQL además de en `reglas_promesas.go`, por
// la misma razón que la del saldo y la del estado de una cuenta en `repos_paginacion.go`: es la
// única manera de filtrar por estado y paginar sin traerse todas las promesas. La duplicación es
// deliberada y está acotada a `expresionPagadoSQL` y `condicionEstadoPromesaSQL`; las filas que
// vuelven se re-evalúan con EvaluarPromesa, así que lo que ve el usuario sigue saliendo de las
// reglas. Quien cambie una de las dos fórmulas DEBE cambiarla en los dos lados: la sección 8 de
// `verificar_promesas.sh` compara ambas clasificaciones y falla si dejan de coincidir.

import "fmt"

// expresionPagadoSQL es lo que el cliente pagó dentro de la ventana de la promesa. Solo cuentan
// los movimientos de dinero recibido (`tipo = 'pago'`) no anulados: una condonación o una nota de
// crédito rebajan el saldo pero no son el cliente cumpliendo su palabra.
const expresionPagadoSQL = `COALESCE((
		SELECT SUM(pg.monto_centavos)
		FROM pago pg
		JOIN cuenta cp ON cp.empresa_id = pg.empresa_id AND cp.id = pg.cuenta_id
		WHERE pg.empresa_id = pr.empresa_id AND pg.tipo = 'pago' AND NOT pg.anulado
		  AND cp.cliente_id = pr.cliente_id
		  AND (pr.cuenta_id IS NULL OR pg.cuenta_id = pr.cuenta_id)
		  AND pg.fecha >= (pr.creado_en AT TIME ZONE $3::text)::date
		  AND pg.fecha <= pr.fecha_promesa), 0)`

// expresionAlcanzadaSQL es PromesaAlcanzada escrita para la base: sin monto prometido basta
// cualquier pago; con monto, hay que llegar a él.
const expresionAlcanzadaSQL = `(CASE WHEN pr.monto_centavos IS NULL
		THEN ` + expresionPagadoSQL + ` > 0
		ELSE ` + expresionPagadoSQL + ` >= pr.monto_centavos END)`

const selectPromesa = `
	SELECT pr.id, pr.cliente_id, cl.nombre, cl.telefono, pr.cuenta_id,
	       COALESCE(cu.numero_documento, ''), pr.fecha_promesa, pr.monto_centavos, pr.nota,
	       pr.registrado_por, ur.nombre, pr.anulada, pr.anulada_por, ua.nombre, pr.anulada_en,
	       pr.motivo_anulacion, pr.creado_en,
	       (pr.creado_en AT TIME ZONE $3::text)::date, ` + expresionPagadoSQL + `
	FROM promesa pr
	JOIN cliente cl ON cl.empresa_id = pr.empresa_id AND cl.id = pr.cliente_id
	JOIN usuario ur ON ur.empresa_id = pr.empresa_id AND ur.id = pr.registrado_por
	LEFT JOIN usuario ua ON ua.empresa_id = pr.empresa_id AND ua.id = pr.anulada_por
	LEFT JOIN cuenta cu ON cu.empresa_id = pr.empresa_id AND cu.id = pr.cuenta_id
	WHERE pr.empresa_id = $1 AND ($2::uuid IS NULL OR cl.vendedor_id = $2)`

// contarPromesasDesde es el mismo universo de filas que selectPromesa, sin los datos que solo
// sirven para mostrar. La zona ($3) la usa la condición del estado, pero no todas las
// condiciones la usan; se nombra igual en el WHERE (nunca es NULL) porque Postgres rechaza una
// consulta a la que se le pasa un parámetro que no aparece en ninguna parte.
const contarPromesasDesde = `
	FROM promesa pr
	JOIN cliente cl ON cl.empresa_id = pr.empresa_id AND cl.id = pr.cliente_id
	WHERE pr.empresa_id = $1 AND ($2::uuid IS NULL OR cl.vendedor_id = $2) AND $3::text IS NOT NULL`

func escanearPromesa(s interface{ Scan(...any) error }) (Promesa, error) {
	var p Promesa
	err := s.Scan(&p.ID, &p.ClienteID, &p.ClienteNombre, &p.ClienteTelefono, &p.CuentaID,
		&p.NumeroDocumento, &p.FechaPromesa, &p.MontoCentavos, &p.Nota, &p.RegistradoPor,
		&p.RegistradoPorNombre, &p.Anulada, &p.AnuladaPor, &p.AnuladaPorNombre, &p.AnuladaEn,
		&p.MotivoAnulacion, &p.CreadoEn, &p.FechaRegistro, &p.PagadoCentavos)
	return p, err
}

func errPromesaNoEncontrada() *ErrorApp {
	return errNoEncontrado("PROMESA_NO_ENCONTRADA", "No encontramos la promesa indicada.")
}

// condicionEstadoPromesaSQL es R80 escrita para la base. El segundo valor dice si la condición
// usa "hoy": `cumplida` y `anulada` no miran la fecha y no deben recibirlo, porque Postgres
// rechaza una consulta a la que se le pasan parámetros de más.
func condicionEstadoPromesaSQL(estado string, posicion int) (string, bool) {
	hoy := fmt.Sprintf("$%d::date", posicion)
	switch estado {
	case PromesaAnulada:
		return " AND pr.anulada", false
	case PromesaCumplida:
		return " AND NOT pr.anulada AND " + expresionAlcanzadaSQL, false
	case PromesaPendiente:
		return " AND NOT pr.anulada AND NOT " + expresionAlcanzadaSQL +
			" AND pr.fecha_promesa >= " + hoy, true
	case PromesaRota:
		return " AND NOT pr.anulada AND NOT " + expresionAlcanzadaSQL +
			" AND pr.fecha_promesa < " + hoy, true
	}
	return "", false
}

// ConsultaPromesas reúne los filtros de un listado de promesas.
type ConsultaPromesas struct {
	Alcance   Alcance
	Zona      string // zona horaria de la empresa: define el inicio de la ventana (R14)
	ClienteID string // vacío = todos los del alcance
	Estado    string // vacío o "todas" = sin filtro
	Hoy       Fecha
	// SoloAgenda deja lo que hay que ir a cobrar hoy: ni anuladas ni cumplidas, con la fecha
	// prometida ya llegada o pasada.
	SoloAgenda bool
	Limite     int // 0 = sin paginar
	Desde      int
}

// filtros arma las condiciones y los parámetros en el orden en que la consulta los usa.
func (q ConsultaPromesas) filtros(empresaID string) (string, []any) {
	parametros := []any{empresaID, q.Alcance.filtro(), q.Zona, filtroOpcional(q.ClienteID)}
	condiciones := " AND ($4::uuid IS NULL OR pr.cliente_id = $4)"
	switch {
	case q.SoloAgenda:
		condiciones += fmt.Sprintf(" AND NOT pr.anulada AND NOT %s AND pr.fecha_promesa <= $%d::date",
			expresionAlcanzadaSQL, len(parametros)+1)
		parametros = append(parametros, q.Hoy)
	case q.Estado != "" && q.Estado != EstadoTodas:
		condicion, usaHoy := condicionEstadoPromesaSQL(q.Estado, len(parametros)+1)
		condiciones += condicion
		if usaHoy {
			parametros = append(parametros, q.Hoy)
		}
	}
	return condiciones, parametros
}

// ListarPromesas devuelve las promesas que cumplen el filtro y, cuando se pide una página, el
// total que hay detrás. Con Limite = 0 no pagina (la agenda y la ficha del cliente son listas
// cortas por naturaleza: lo que un cobrador tiene que visitar en un día).
func (r *Repo) ListarPromesas(empresaID string, q ConsultaPromesas) ([]Promesa, int, error) {
	condiciones, parametros := q.filtros(empresaID)

	total := 0
	if q.Limite > 0 {
		if err := r.q.QueryRowContext(r.ctx,
			`SELECT COUNT(*)`+contarPromesasDesde+condiciones, parametros...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}

	// El historial se lee de lo más reciente hacia atrás; la agenda, de lo más atrasado hacia hoy,
	// porque lo primero que hay que ir a ver es a quien ya le pasó la fecha.
	orden := ` ORDER BY pr.fecha_promesa DESC, pr.creado_en DESC`
	if q.SoloAgenda {
		orden = ` ORDER BY pr.fecha_promesa, pr.creado_en`
	}
	consulta := selectPromesa + condiciones + orden
	if q.Limite > 0 {
		consulta += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, len(parametros)+1, len(parametros)+2)
		parametros = append(parametros, q.Limite, q.Desde)
	}

	filas, err := r.q.QueryContext(r.ctx, consulta, parametros...)
	if err != nil {
		return nil, 0, err
	}
	defer filas.Close()
	res := []Promesa{}
	for filas.Next() {
		p, err := escanearPromesa(filas)
		if err != nil {
			return nil, 0, err
		}
		res = append(res, p)
	}
	if err := filas.Err(); err != nil {
		return nil, 0, err
	}
	if q.Limite == 0 {
		total = len(res)
	}
	return res, total, nil
}

// ObtenerPromesa trae una promesa dentro del alcance del usuario. Fuera de su cartera, 404 (R38).
func (r *Repo) ObtenerPromesa(empresaID string, alc Alcance, zona, id string) (Promesa, error) {
	p, err := escanearPromesa(r.q.QueryRowContext(r.ctx,
		selectPromesa+` AND pr.id = $4`, empresaID, alc.filtro(), zona, id))
	return p, noEncontrado(err, errPromesaNoEncontrada())
}

// BloquearPromesa serializa las anulaciones: dos anulaciones simultáneas no pueden pasar ambas.
func (r *Repo) BloquearPromesa(empresaID, id string) error {
	var x string
	err := r.q.QueryRowContext(r.ctx,
		`SELECT id FROM promesa WHERE empresa_id = $1 AND id = $2 FOR UPDATE`, empresaID, id).Scan(&x)
	return noEncontrado(err, errPromesaNoEncontrada())
}

// NuevaPromesa son los datos con los que nace una promesa.
type NuevaPromesa struct {
	ClienteID     string
	CuentaID      *string
	FechaPromesa  Fecha
	MontoCentavos *int64
	Nota          string
	RegistradoPor string
}

func (r *Repo) CrearPromesa(empresaID string, p NuevaPromesa) (string, error) {
	var id string
	err := r.q.QueryRowContext(r.ctx, `
		INSERT INTO promesa (empresa_id, cliente_id, cuenta_id, fecha_promesa, monto_centavos,
		                     nota, registrado_por)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		empresaID, p.ClienteID, textoONil(p.CuentaID), p.FechaPromesa, p.MontoCentavos,
		p.Nota, p.RegistradoPor).Scan(&id)
	return id, err
}

// AnularPromesa no borra nada: deja la promesa en el historial con quién la anuló y por qué.
func (r *Repo) AnularPromesa(empresaID, id, usuarioID, motivo string) error {
	var motivoDB any
	if motivo != "" {
		motivoDB = motivo
	}
	_, err := r.q.ExecContext(r.ctx, `
		UPDATE promesa SET anulada = true, anulada_por = $3, anulada_en = now(), motivo_anulacion = $4
		WHERE empresa_id = $1 AND id = $2 AND NOT anulada`, empresaID, id, usuarioID, motivoDB)
	return err
}
