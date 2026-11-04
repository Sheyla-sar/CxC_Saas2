package main

// handlers_cobranza.go — endpoints de la fase 3: usuarios, anticipos, notas de crédito,
// liquidación del cobrador y adjuntos. Igual que handlers.go: traducen HTTP, deciden con
// reglas_cobranza.go y guardan con repos_cobranza.go.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ─────────────────────────────────────────────────────────────────────────────
// Usuarios
// ─────────────────────────────────────────────────────────────────────────────

type entradaUsuario struct {
	Nombre   *string `json:"nombre"`
	Email    *string `json:"email"`
	Rol      *string `json:"rol"`
	Activo   *bool   `json:"activo"`
	Password *string `json:"password"`
}

func hCrearUsuario(r *http.Request, t *Tenant) (int, any, error) {
	var in entradaUsuario
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	d, err := ValidarDatosUsuario(DatosUsuario{
		Nombre: valor(in.Nombre), Email: valor(in.Email),
		Rol: valorO(in.Rol, RolVendedor), Activo: valorO(in.Activo, true),
	})
	if err != nil {
		return 0, nil, err
	}
	if d.Email == "" {
		return 0, nil, errEntrada("CORREO_REQUERIDO", "Escriba el correo con el que ingresará el usuario.")
	}
	clave := valor(in.Password)
	if err := ValidarPassword(clave); err != nil {
		return 0, nil, err
	}
	if d.Activo {
		if err := verificarCupoDeUsuarios(t); err != nil {
			return 0, nil, err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(clave), bcrypt.DefaultCost)
	if err != nil {
		return 0, nil, err
	}
	u, err := t.Repo.CrearUsuario(t.EmpresaID, d.Nombre, d.Email, string(hash), d.Rol)
	if err != nil {
		if esCorreoDuplicado(err) {
			return 0, nil, errRegla("CORREO_YA_USADO", "Ya existe un usuario con ese correo en su empresa.")
		}
		return 0, nil, err
	}
	if !d.Activo {
		if err := t.Repo.ActualizarUsuario(t.EmpresaID, u.ID, d.Nombre, d.Rol, false); err != nil {
			return 0, nil, err
		}
		u.Activo = false
	}
	return http.StatusCreated, u, nil
}

// verificarCupoDeUsuarios aplica el límite de usuarios activos del plan contratado (R77).
func verificarCupoDeUsuarios(t *Tenant) error {
	cfg, err := t.Config()
	if err != nil {
		return err
	}
	if cfg.LimiteUsuarios <= 0 {
		return nil
	}
	activos, err := t.Repo.ContarUsuariosActivos(t.EmpresaID)
	if err != nil {
		return err
	}
	return ValidarAltaUsuario(activos, cfg)
}

func hActualizarUsuario(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errUsuarioNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in entradaUsuario
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.Email != nil {
		return 0, nil, errEntrada("CORREO_NO_EDITABLE",
			"El correo no se puede cambiar: es con el que el usuario ingresa. Cree otro usuario si hace falta.")
	}
	objetivo, err := t.Repo.ObtenerUsuario(t.EmpresaID, id)
	if err != nil {
		return 0, nil, err
	}
	d, err := ValidarDatosUsuario(DatosUsuario{
		Nombre: valorO(in.Nombre, objetivo.Nombre), Email: objetivo.Email,
		Rol: valorO(in.Rol, objetivo.Rol), Activo: valorO(in.Activo, objetivo.Activo),
	})
	if err != nil {
		return 0, nil, err
	}
	restantes, err := t.Repo.ContarAdminsActivos(t.EmpresaID, objetivo.ID)
	if err != nil {
		return 0, nil, err
	}
	if err := ValidarCambioDeRol(objetivo, d, restantes); err != nil {
		return 0, nil, err
	}
	if d.Activo && !objetivo.Activo {
		if err := verificarCupoDeUsuarios(t); err != nil {
			return 0, nil, err
		}
	}
	if err := t.Repo.ActualizarUsuario(t.EmpresaID, id, d.Nombre, d.Rol, d.Activo); err != nil {
		return 0, nil, err
	}
	u, err := t.Repo.ObtenerUsuario(t.EmpresaID, id)
	return http.StatusOK, u, err
}

// hReiniciarPassword: un admin le asigna una contraseña nueva a cualquier usuario de su empresa.
func hReiniciarPassword(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errUsuarioNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if _, err := t.Repo.ObtenerUsuario(t.EmpresaID, id); err != nil {
		return 0, nil, err
	}
	if err := ValidarPassword(in.Password); err != nil {
		return 0, nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.CambiarPassword(t.EmpresaID, id, string(hash)); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]string{"mensaje": "Contraseña actualizada."}, nil
}

// hCambiarMiPassword: el propio usuario cambia su contraseña indicando la actual.
func hCambiarMiPassword(r *http.Request, t *Tenant) (int, any, error) {
	var in struct {
		PasswordActual string `json:"password_actual"`
		PasswordNueva  string `json:"password_nueva"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(t.Usuario.PasswordHash), []byte(in.PasswordActual)) != nil {
		return 0, nil, errNoAutenticado("CONTRASENA_INCORRECTA", "La contraseña actual no es correcta.")
	}
	if err := ValidarPassword(in.PasswordNueva); err != nil {
		return 0, nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.PasswordNueva), bcrypt.DefaultCost)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.CambiarPassword(t.EmpresaID, t.Usuario.ID, string(hash)); err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]string{"mensaje": "Contraseña actualizada."}, nil
}

func esCorreoDuplicado(err error) bool {
	return err != nil && strings.Contains(err.Error(), "usuario_empresa_id_email_key")
}

// ─────────────────────────────────────────────────────────────────────────────
// Notas de crédito
// ─────────────────────────────────────────────────────────────────────────────

func hNotaCredito(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errCuentaNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		MontoCentavos *int64 `json:"monto_centavos"`
		Motivo        string `json:"motivo"`
		Fecha         *Fecha `json:"fecha"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	c, cfg, hoy, err := bloquearYEvaluar(t, id)
	if err != nil {
		return 0, nil, err
	}
	fecha := valorO(in.Fecha, hoy)
	monto, motivo, err := ValidarNotaCredito(in.MontoCentavos, in.Motivo, fecha, hoy, c.SaldoCentavos, cfg)
	if err != nil {
		return 0, nil, err
	}
	numero, err := t.Repo.ReservarNumero(t.EmpresaID, "nota_credito", cfg.PrefijoNotaCredito)
	if err != nil {
		return 0, nil, err
	}
	movID, err := t.Repo.CrearMovimiento(t.EmpresaID, NuevoMovimiento{
		CuentaID: id, Tipo: TipoNotaCredito, Numero: numero, MontoCentavos: monto, Fecha: fecha,
		Metodo: TipoNotaCredito, Nota: motivo, RegistradoPor: t.Usuario.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	return respuestaDeMovimiento(t, movID, id, http.StatusCreated)
}

// ─────────────────────────────────────────────────────────────────────────────
// Anticipos
// ─────────────────────────────────────────────────────────────────────────────

func anticiposEvaluados(t *Tenant, clienteID string) ([]Anticipo, error) {
	lista, err := t.Repo.ListarAnticipos(t.EmpresaID, t.Alcance(), clienteID)
	if err != nil {
		return nil, err
	}
	for i := range lista {
		EvaluarAnticipo(&lista[i])
	}
	return lista, nil
}

func hListarAnticipos(r *http.Request, t *Tenant) (int, any, error) {
	clienteID := strings.TrimSpace(r.URL.Query().Get("cliente_id"))
	if clienteID != "" {
		var err error
		if clienteID, err = validarID(clienteID, "cliente_id"); err != nil {
			return 0, nil, err
		}
	}
	lista, err := anticiposEvaluados(t, clienteID)
	if err != nil {
		return 0, nil, err
	}
	if r.URL.Query().Get("solo_disponibles") == "true" {
		con := []Anticipo{}
		for _, a := range lista {
			if a.DisponibleCentavos > 0 {
				con = append(con, a)
			}
		}
		lista = con
	}
	return http.StatusOK, map[string]any{"anticipos": lista}, nil
}

func hCrearAnticipo(r *http.Request, t *Tenant) (int, any, error) {
	clienteID, err := idDeRuta(r, errClienteNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		MontoCentavos *int64 `json:"monto_centavos"`
		Fecha         *Fecha `json:"fecha"`
		Metodo        string `json:"metodo"`
		Referencia    string `json:"referencia"`
		Nota          string `json:"nota"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.MontoCentavos == nil {
		return 0, nil, errEntrada("MONTO_REQUERIDO", "Indique el monto del anticipo en centavos.")
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	cl, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), clienteID)
	if err != nil {
		return 0, nil, err
	}
	if !cl.Activo {
		return 0, nil, errRegla("CLIENTE_INACTIVO", "El cliente está desactivado.")
	}
	fecha := valorO(in.Fecha, hoy)
	metodo, err := ValidarAnticipo(*in.MontoCentavos, fecha, hoy, in.Metodo, cfg)
	if err != nil {
		return 0, nil, err
	}
	referencia, err := ValidarTexto(in.Referencia, "referencia", LargoMaximoCodigo)
	if err != nil {
		return 0, nil, err
	}
	nota, err := ValidarTexto(in.Nota, "nota", LargoMaximoNota)
	if err != nil {
		return 0, nil, err
	}
	numero, err := t.Repo.ReservarNumero(t.EmpresaID, "anticipo", cfg.PrefijoAnticipo)
	if err != nil {
		return 0, nil, err
	}
	id, err := t.Repo.CrearAnticipo(t.EmpresaID, numero, clienteID, *in.MontoCentavos, fecha,
		metodo, referencia, nota, t.Usuario.ID)
	if err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAnticipo(t.EmpresaID, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	EvaluarAnticipo(&a)
	return http.StatusCreated, a, nil
}

// hAplicarAnticipo abona a una cuenta con dinero que el cliente ya había entregado.
func hAplicarAnticipo(r *http.Request, t *Tenant) (int, any, error) {
	anticipoID, err := idDeRuta(r, errAnticipoNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		CuentaID      string `json:"cuenta_id"`
		MontoCentavos *int64 `json:"monto_centavos"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	cuentaID, err := validarID(in.CuentaID, "cuenta_id")
	if err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	// Orden de locks: anticipo → cuenta (siempre el mismo, para no bloquearse mutuamente).
	if err := t.Repo.BloquearAnticipo(t.EmpresaID, anticipoID); err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAnticipo(t.EmpresaID, t.Alcance(), anticipoID)
	if err != nil {
		return 0, nil, err
	}
	EvaluarAnticipo(&a)
	c, _, hoy, err := bloquearYEvaluar(t, cuentaID)
	if err != nil {
		return 0, nil, err
	}
	monto, err := ValidarAplicacionAnticipo(a, c, in.MontoCentavos, cfg)
	if err != nil {
		return 0, nil, err
	}
	movID, err := t.Repo.CrearMovimiento(t.EmpresaID, NuevoMovimiento{
		CuentaID: cuentaID, Tipo: TipoPago, MontoCentavos: monto, Fecha: hoy,
		Metodo: MetodoAnticipo, Referencia: a.Numero,
		Nota:          fmt.Sprintf("Aplicación del anticipo %s", a.Numero),
		RegistradoPor: t.Usuario.ID, AnticipoID: &a.ID,
	})
	if err != nil {
		return 0, nil, err
	}
	return respuestaDeMovimiento(t, movID, cuentaID, http.StatusCreated)
}

func hAnularAnticipo(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errAnticipoNoEncontrado())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.BloquearAnticipo(t.EmpresaID, id); err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAnticipo(t.EmpresaID, t.Alcance(), id)
	if err != nil {
		return 0, nil, err
	}
	EvaluarAnticipo(&a)
	motivo, err := ValidarAnulacionAnticipo(a, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.AnularAnticipo(t.EmpresaID, id, t.Usuario.ID, motivo); err != nil {
		return 0, nil, err
	}
	a, err = t.Repo.ObtenerAnticipo(t.EmpresaID, AlcanceEmpresa(), id)
	if err != nil {
		return 0, nil, err
	}
	EvaluarAnticipo(&a)
	return http.StatusOK, a, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Liquidación del cobrador
// ─────────────────────────────────────────────────────────────────────────────

// cobradorDeLaPeticion: el vendedor siempre liquida lo suyo; el admin puede ver el corte de otro.
func cobradorDeLaPeticion(r *http.Request, t *Tenant) (Usuario, error) {
	id := strings.TrimSpace(r.URL.Query().Get("cobrador_id"))
	if id == "" || !t.EsAdmin() {
		return t.Usuario, nil
	}
	validado, err := validarID(id, "cobrador_id")
	if err != nil {
		return Usuario{}, err
	}
	return t.Repo.ObtenerUsuario(t.EmpresaID, validado)
}

// hCorteCobrador muestra lo que el cobrador tiene pendiente de entregar.
func hCorteCobrador(r *http.Request, t *Tenant) (int, any, error) {
	cobrador, err := cobradorDeLaPeticion(r, t)
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	movimientos, err := t.Repo.PagosSinLiquidar(t.EmpresaID, cobrador.ID, hoy)
	if err != nil {
		return 0, nil, err
	}
	anticipos, err := t.Repo.AnticiposSinLiquidar(t.EmpresaID, cobrador.ID, hoy)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, ArmarCorte(cobrador, hoy, movimientos, anticipos), nil
}

// hCerrarLiquidacion: el cobrador entrega el dinero y queda el corte registrado.
func hCerrarLiquidacion(r *http.Request, t *Tenant) (int, any, error) {
	var in struct {
		TotalEntregadoCentavos *int64 `json:"total_entregado_centavos"`
		Nota                   string `json:"nota"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	if in.TotalEntregadoCentavos == nil {
		return 0, nil, errEntrada("MONTO_REQUERIDO", "Indique cuánto dinero está entregando, en centavos.")
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return 0, nil, err
	}
	// PagosSinLiquidar bloquea las filas: dos cierres simultáneos no pueden incluir el mismo pago.
	movimientos, err := t.Repo.PagosSinLiquidar(t.EmpresaID, t.Usuario.ID, hoy)
	if err != nil {
		return 0, nil, err
	}
	anticipos, err := t.Repo.AnticiposSinLiquidar(t.EmpresaID, t.Usuario.ID, hoy)
	if err != nil {
		return 0, nil, err
	}
	corte := ArmarCorte(t.Usuario, hoy, movimientos, anticipos)
	nota, err := ValidarCierreLiquidacion(corte, *in.TotalEntregadoCentavos, in.Nota, cfg)
	if err != nil {
		return 0, nil, err
	}
	numero, err := t.Repo.ReservarNumero(t.EmpresaID, "liquidacion", cfg.PrefijoLiquidacion)
	if err != nil {
		return 0, nil, err
	}
	id, err := t.Repo.CrearLiquidacion(t.EmpresaID, Liquidacion{
		Numero: numero, CobradorID: t.Usuario.ID, Fecha: hoy,
		TotalEsperadoCentavos:  corte.TotalEsperadoCentavos,
		TotalEntregadoCentavos: *in.TotalEntregadoCentavos,
		CantidadMovimientos:    corte.CantidadMovimientos, Nota: nota,
	})
	if err != nil {
		return 0, nil, err
	}
	pagoIDs := make([]string, 0, len(movimientos))
	for _, m := range movimientos {
		pagoIDs = append(pagoIDs, m.ID)
	}
	anticipoIDs := make([]string, 0, len(anticipos))
	for _, a := range anticipos {
		anticipoIDs = append(anticipoIDs, a.ID)
	}
	if err := t.Repo.AsignarLiquidacion(t.EmpresaID, id, pagoIDs, anticipoIDs); err != nil {
		return 0, nil, err
	}
	l, err := t.Repo.ObtenerLiquidacion(t.EmpresaID, AlcanceEmpresa(), id, false)
	if err != nil {
		return 0, nil, err
	}
	EvaluarLiquidacion(&l)
	return http.StatusCreated, l, nil
}

func hListarLiquidaciones(r *http.Request, t *Tenant) (int, any, error) {
	estado := r.URL.Query().Get("estado")
	switch estado {
	case "", "todas":
		estado = ""
	case LiquidacionEntregada, LiquidacionAprobada, LiquidacionRechazada:
	default:
		return 0, nil, errEntrada("ESTADO_INVALIDO", "El estado debe ser entregada, aprobada, rechazada o todas.")
	}
	// El admin ve todas las de su empresa; el cobrador, solo las suyas.
	alc := AlcanceVendedor(t.Usuario.ID)
	if t.EsAdmin() {
		alc = AlcanceEmpresa()
	}
	lista, err := t.Repo.ListarLiquidaciones(t.EmpresaID, alc, estado)
	if err != nil {
		return 0, nil, err
	}
	for i := range lista {
		EvaluarLiquidacion(&lista[i])
	}
	return http.StatusOK, map[string]any{"estado": estado, "liquidaciones": lista}, nil
}

type respuestaLiquidacion struct {
	Liquidacion Liquidacion  `json:"liquidacion"`
	Movimientos []Movimiento `json:"movimientos"`
	Adjuntos    []Adjunto    `json:"adjuntos"`
}

func hObtenerLiquidacion(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errLiquidacionNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	alc := AlcanceVendedor(t.Usuario.ID)
	if t.EsAdmin() {
		alc = AlcanceEmpresa()
	}
	l, err := t.Repo.ObtenerLiquidacion(t.EmpresaID, alc, id, false)
	if err != nil {
		return 0, nil, err
	}
	EvaluarLiquidacion(&l)
	movs, err := t.Repo.MovimientosDeLiquidacion(t.EmpresaID, id)
	if err != nil {
		return 0, nil, err
	}
	adjuntos, err := t.Repo.ListarAdjuntos(t.EmpresaID, "liquidacion", id)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, respuestaLiquidacion{l, movs, adjuntos}, nil
}

func resolverLiquidacion(r *http.Request, t *Tenant, aprobar bool) (int, any, error) {
	id, err := idDeRuta(r, errLiquidacionNoEncontrada())
	if err != nil {
		return 0, nil, err
	}
	var in struct {
		Motivo string `json:"motivo"`
	}
	if err := leerJSON(r, &in); err != nil {
		return 0, nil, err
	}
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	l, err := t.Repo.ObtenerLiquidacion(t.EmpresaID, AlcanceEmpresa(), id, true)
	if err != nil {
		return 0, nil, err
	}
	motivo, err := ValidarResolucionLiquidacion(l, aprobar, in.Motivo, cfg)
	if err != nil {
		return 0, nil, err
	}
	estado := LiquidacionAprobada
	if !aprobar {
		estado = LiquidacionRechazada
	}
	if err := t.Repo.ResolverLiquidacion(t.EmpresaID, id, estado, t.Usuario.ID, motivo); err != nil {
		return 0, nil, err
	}
	if !aprobar {
		// Al rechazar, el dinero vuelve a quedar pendiente de entregar.
		if err := t.Repo.LiberarLiquidacion(t.EmpresaID, id); err != nil {
			return 0, nil, err
		}
	}
	l, err = t.Repo.ObtenerLiquidacion(t.EmpresaID, AlcanceEmpresa(), id, false)
	if err != nil {
		return 0, nil, err
	}
	EvaluarLiquidacion(&l)
	return http.StatusOK, l, nil
}

func hAprobarLiquidacion(r *http.Request, t *Tenant) (int, any, error) {
	return resolverLiquidacion(r, t, true)
}

func hRechazarLiquidacion(r *http.Request, t *Tenant) (int, any, error) {
	return resolverLiquidacion(r, t, false)
}

// ─────────────────────────────────────────────────────────────────────────────
// Adjuntos (fotos de recibos y facturas)
// ─────────────────────────────────────────────────────────────────────────────

// verificarAcceso comprueba que el registro al que se adjunta exista dentro del alcance del
// usuario: así un vendedor no puede pegar archivos a cuentas ajenas.
func verificarAcceso(t *Tenant, entidad, entidadID string) error {
	switch entidad {
	case "cuenta":
		_, err := t.Repo.ObtenerCuenta(t.EmpresaID, t.Alcance(), entidadID)
		return err
	case "cliente":
		_, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), entidadID)
		return err
	case "movimiento":
		_, err := t.Repo.ObtenerMovimiento(t.EmpresaID, t.Alcance(), entidadID)
		return err
	case "anticipo":
		_, err := t.Repo.ObtenerAnticipo(t.EmpresaID, t.Alcance(), entidadID)
		return err
	case "liquidacion":
		alc := AlcanceVendedor(t.Usuario.ID)
		if t.EsAdmin() {
			alc = AlcanceEmpresa()
		}
		_, err := t.Repo.ObtenerLiquidacion(t.EmpresaID, alc, entidadID, false)
		return err
	}
	return errEntrada("ENTIDAD_INVALIDA", "No se pueden adjuntar archivos a ese tipo de registro.")
}

func hListarAdjuntos(r *http.Request, t *Tenant) (int, any, error) {
	entidad := r.URL.Query().Get("entidad")
	entidadID, err := validarID(r.URL.Query().Get("entidad_id"), "entidad_id")
	if err != nil {
		return 0, nil, err
	}
	if !contiene(EntidadesConAdjunto, entidad) {
		return 0, nil, errEntrada("ENTIDAD_INVALIDA", "Tipo de registro no válido.").
			con("entidades_validas", EntidadesConAdjunto)
	}
	if err := verificarAcceso(t, entidad, entidadID); err != nil {
		return 0, nil, err
	}
	// La firma del cliente no es un comprobante más: se dibuja dentro del recibo y no se lista
	// aparte (R82). Una firma suelta, sin el recibo encima, no comprueba nada.
	lista, err := t.Repo.ListarComprobantes(t.EmpresaID, entidad, entidadID)
	return http.StatusOK, map[string]any{"adjuntos": lista}, err
}

func hEliminarAdjunto(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("ADJUNTO_NO_ENCONTRADO", "No encontramos el archivo indicado."))
	if err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAdjunto(t.EmpresaID, id)
	if err != nil {
		return 0, nil, err
	}
	if err := t.Repo.EliminarAdjunto(t.EmpresaID, id); err != nil {
		return 0, nil, err
	}
	borrarArchivo(a.archivo)
	return http.StatusOK, map[string]string{"mensaje": "Archivo eliminado."}, nil
}

var _ = errors.Is // usado por los helpers compartidos
