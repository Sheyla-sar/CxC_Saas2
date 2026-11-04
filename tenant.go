package main

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

// Tenant es el contexto de una petición autenticada: empresa, usuario, transacción (vía Repo)
// con app.empresa_id ya fijado, y la configuración de la empresa cargada bajo demanda.
type Tenant struct {
	ctx         context.Context
	Repo        *Repo
	EmpresaID   string
	EmpresaSlug string
	Usuario     Usuario
	Ahora       time.Time
	cfg         *ConfigEmpresa
}

type claveTenant struct{}

// tenantDe recupera el Tenant que conTenant dejó en el context de la petición.
func tenantDe(ctx context.Context) *Tenant {
	t, _ := ctx.Value(claveTenant{}).(*Tenant)
	return t
}

func (t *Tenant) EsAdmin() bool { return t.Usuario.Rol == RolAdmin }

// Alcance del usuario para las lecturas: admin ve toda la empresa, vendedor solo lo suyo.
func (t *Tenant) Alcance() Alcance {
	if t.EsAdmin() {
		return AlcanceEmpresa()
	}
	return AlcanceVendedor(t.Usuario.ID)
}

func (t *Tenant) ExigirAdmin() error {
	if !t.EsAdmin() {
		return errSoloAdmin()
	}
	return nil
}

// Config devuelve la configuración de la empresa (una sola lectura por petición).
func (t *Tenant) Config() (ConfigEmpresa, error) {
	if t.cfg == nil {
		c, err := t.Repo.ObtenerConfig(t.EmpresaID)
		if err != nil {
			return c, err
		}
		t.cfg = &c
	}
	return *t.cfg, nil
}

// Hoy es la fecha actual en la zona horaria de la empresa (R14).
func (t *Tenant) Hoy() (Fecha, error) {
	cfg, err := t.Config()
	if err != nil {
		return Fecha{}, err
	}
	return HoyEn(cfg.ZonaHoraria, t.Ahora)
}

// ─────────────────────────────────────────────────────────────────────────────
// Resolución de empresa
// ─────────────────────────────────────────────────────────────────────────────

// slugSolicitado devuelve la empresa que la petición dice querer, si la indica: primero el header
// X-Empresa-Slug y luego el subdominio (<slug>.<DOMINIO_BASE>). Vacío si no indica ninguna.
func slugSolicitado(r *http.Request, dominioBase string) string {
	if s := strings.ToLower(strings.TrimSpace(r.Header.Get("X-Empresa-Slug"))); s != "" {
		return s
	}
	return slugDeSubdominio(r.Host, dominioBase)
}

func slugDeSubdominio(host, dominioBase string) string {
	if dominioBase == "" {
		return ""
	}
	h := strings.ToLower(host)
	if sinPuerto, _, err := net.SplitHostPort(h); err == nil {
		h = sinPuerto
	}
	sub, ok := strings.CutSuffix(h, "."+dominioBase)
	if !ok || sub == "" || strings.Contains(sub, ".") || sub == "www" || sub == "api" {
		return ""
	}
	return sub
}

// resolverEmpresa decide qué empresa atiende una petición autenticada. ES EL ÚNICO LUGAR donde se
// toma esta decisión.
//
// Hoy la fuente principal es la sesión. El header o el subdominio solo pueden CONFIRMARLA: si
// indican otra empresa, se rechaza con 403. Para pasar a subdominio como fuente principal basta con
// cambiar esta función; los handlers solo ven el empresa_id resultante.
func resolverEmpresa(r *http.Request, s Sesion, dominioBase string) (string, error) {
	if solicitado := slugSolicitado(r, dominioBase); solicitado != "" && solicitado != s.EmpresaSlug {
		return "", errProhibido("EMPRESA_NO_COINCIDE",
			"Su sesión pertenece a otra empresa. Inicie sesión en la empresa a la que intenta ingresar.")
	}
	return s.EmpresaID, nil
}
