package main

// handlers_comprobantes.go — convertir los comprobantes adjuntos en un PDF que se pueda archivar,
// imprimir o mandar por WhatsApp. La foto que el cobrador toma en la calle sirve de poco si queda
// encerrada en el sistema; acá sale como documento, con su título, su descripción y quién la subió.

import (
	"fmt"
	"net/http"
)

// etiquetaDeEntidad arma el rótulo humano que encabeza el PDF ("Cuenta FAC-000218 · Tienda La
// Bendición"). Además comprueba el acceso: si el usuario no puede ver el registro, no hay rótulo
// ni documento.
func etiquetaDeEntidad(t *Tenant, entidad, entidadID string) (string, error) {
	switch entidad {
	case "cuenta":
		c, err := t.Repo.ObtenerCuenta(t.EmpresaID, t.Alcance(), entidadID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Cuenta %s · %s", oGuion(c.NumeroDocumento), c.ClienteNombre), nil
	case "cliente":
		c, err := t.Repo.ObtenerCliente(t.EmpresaID, t.Alcance(), entidadID)
		if err != nil {
			return "", err
		}
		return "Cliente " + c.Nombre, nil
	case "movimiento":
		m, err := t.Repo.ObtenerMovimiento(t.EmpresaID, t.Alcance(), entidadID)
		if err != nil {
			return "", err
		}
		numero := "—"
		if m.Numero != nil {
			numero = *m.Numero
		}
		return fmt.Sprintf("%s %s", EtiquetaTipoMovimiento(m.Tipo), numero), nil
	case "anticipo":
		a, err := t.Repo.ObtenerAnticipo(t.EmpresaID, t.Alcance(), entidadID)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Anticipo %s · %s", a.Numero, a.ClienteNombre), nil
	case "liquidacion":
		alc := AlcanceVendedor(t.Usuario.ID)
		if t.EsAdmin() {
			alc = AlcanceEmpresa()
		}
		l, err := t.Repo.ObtenerLiquidacion(t.EmpresaID, alc, entidadID, false)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Liquidación %s · %s", l.Numero, l.CobradorNombre), nil
	}
	return "", errEntrada("ENTIDAD_INVALIDA", "No se pueden adjuntar archivos a ese tipo de registro.")
}

func oGuion(texto string) string {
	if texto == "" {
		return "sin documento"
	}
	return texto
}

// hComprobantePDF entrega UN comprobante como PDF. Si la foto es una imagen, se arma el documento
// con su ficha; si el adjunto ya venía en PDF, se entrega tal cual: volver a envolverlo solo
// degradaría el original.
func hComprobantePDF(r *http.Request, t *Tenant) (int, any, error) {
	id, err := idDeRuta(r, errNoEncontrado("ADJUNTO_NO_ENCONTRADO", "No encontramos el archivo indicado."))
	if err != nil {
		return 0, nil, err
	}
	a, err := t.Repo.ObtenerAdjunto(t.EmpresaID, id)
	if err != nil {
		return 0, nil, err
	}
	if err := verificarAcceso(t, a.Entidad, a.EntidadID); err != nil {
		return 0, nil, err
	}
	// La firma del cliente no se entrega como documento suelto: una firma sin el recibo encima
	// no comprueba nada. Sale impresa dentro del recibo del movimiento.
	if a.EsFirma() {
		return 0, nil, errNoEncontrado("ADJUNTO_NO_ENCONTRADO",
			"La firma del cliente no se entrega por separado: sale impresa en el recibo del pago.")
	}
	nombre := nombreDeArchivoPDF(a.ComoSeLlama())
	if !a.EsImagen() {
		datos, err := leerAdjunto(a)
		if err != nil {
			return 0, nil, err
		}
		return http.StatusOK, archivoPDF{nombre, datos}, nil
	}
	contexto, err := etiquetaDeEntidad(t, a.Entidad, a.EntidadID)
	if err != nil {
		return 0, nil, err
	}
	pdf, err := armarComprobantes(t, contexto, []Adjunto{a})
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, archivoPDF{nombre, pdf}, nil
}

// hComprobantesPDF entrega TODOS los comprobantes de un registro en un solo PDF.
func hComprobantesPDF(r *http.Request, t *Tenant) (int, any, error) {
	entidad := r.URL.Query().Get("entidad")
	entidadID, err := validarID(r.URL.Query().Get("entidad_id"), "entidad_id")
	if err != nil {
		return 0, nil, err
	}
	contexto, err := etiquetaDeEntidad(t, entidad, entidadID)
	if err != nil {
		return 0, nil, err
	}
	// Sin firmas: el paquete de comprobantes es lo que se archiva y se manda, y la firma del
	// cliente pertenece al recibo donde la estampó.
	adjuntos, err := t.Repo.ListarComprobantes(t.EmpresaID, entidad, entidadID)
	if err != nil {
		return 0, nil, err
	}
	pdf, err := armarComprobantes(t, contexto, adjuntos)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, archivoPDF{nombreDeArchivoPDF("comprobantes " + contexto), pdf}, nil
}

func armarComprobantes(t *Tenant, contexto string, adjuntos []Adjunto) ([]byte, error) {
	empresa, marca, cfg, err := datosDeMarca(t)
	if err != nil {
		return nil, err
	}
	hoy, err := t.Hoy()
	if err != nil {
		return nil, err
	}
	return ComprobantesPDF(DatosComprobantes{
		Empresa: empresa, Marca: marca, Config: cfg, Hoy: hoy,
		Contexto: contexto, Adjuntos: adjuntos,
	})
}

// nombreDeArchivoPDF convierte un rótulo humano en un nombre de archivo que cualquier sistema
// acepte: sin acentos raros, sin barras ni dos puntos, y con una longitud razonable.
func nombreDeArchivoPDF(rotulo string) string {
	limpio := make([]rune, 0, len(rotulo))
	for _, c := range rotulo {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			limpio = append(limpio, c)
		case c == ' ' || c == '-' || c == '_' || c == '.':
			limpio = append(limpio, '-')
		}
		if len(limpio) >= 60 {
			break
		}
	}
	if len(limpio) == 0 {
		return "comprobante.pdf"
	}
	return string(limpio) + ".pdf"
}
