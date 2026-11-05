package main

// archivos.go — guarda las fotos y PDF que suben los usuarios (recibos, facturas, comprobantes de
// depósito) y los devuelve solo a usuarios con sesión de la misma empresa.
//
// Hay dos almacenes, según ALMACEN_ARCHIVOS:
//   - "disco" (por defecto): el archivo va a la carpeta RUTA_ARCHIVOS y la tabla adjunto guarda
//     la ruta en la columna archivo. Para un servidor con volumen persistente.
//   - "db": el archivo va en adjunto.contenido y archivo queda vacío. Para plataformas sin disco
//     persistente (Vercel). Hereda el aislamiento por empresa de la tabla adjunto.

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // registra el decodificador: se comprueba que la foto subida se pueda leer
	_ "image/png"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// tamanoMaximoSubida es el techo absoluto del servidor; cada empresa define el suyo (menor) en
// empresa_config.tamano_maximo_adjunto_mb.
const tamanoMaximoSubida = 26 << 20 // 26 MB

// tamanoMaximoFirma es el tope de la firma del cliente, aparte del de los comprobantes: el
// trazo de un dedo sobre el canvas pesa decenas de kilobytes y nunca necesita más.
const tamanoMaximoFirma = 2 << 20 // 2 MB

// TituloFirma lo pone el sistema: el cliente que firma no va a escribir un título.
const TituloFirma = "Firma del cliente"

var (
	rutaArchivos = "archivos"
	// guardarEnBase: true = los adjuntos nuevos se guardan en la base (ALMACEN_ARCHIVOS=db).
	guardarEnBase = false
)

func configurarArchivos(ruta string, enBase bool) error {
	guardarEnBase = enBase
	if strings.TrimSpace(ruta) != "" {
		rutaArchivos = ruta
	}
	if enBase {
		return nil // no hay carpeta que preparar: el disco de la plataforma puede ser de solo lectura
	}
	return os.MkdirAll(rutaArchivos, 0o755)
}

// rutaDeAdjunto arma una ruta nueva e irrepetible dentro de la carpeta de la empresa.
func rutaDeAdjunto(empresaID, extension string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	carpeta := filepath.Join(rutaArchivos, empresaID)
	if err := os.MkdirAll(carpeta, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(carpeta, hex.EncodeToString(b)+extension), nil
}

func borrarArchivo(ruta string) {
	if ruta == "" {
		return
	}
	if err := os.Remove(ruta); err != nil && !os.IsNotExist(err) {
		log.Printf("no se pudo borrar el archivo %s: %v", ruta, err)
	}
}

// tipoMimeDeArchivo deduce el tipo real por los primeros bytes, sin confiar en lo que declara el
// navegador: así no se guarda un ejecutable renombrado como .jpg.
func tipoMimeDeArchivo(cabecera []byte) string {
	switch {
	case len(cabecera) > 3 && cabecera[0] == 0xFF && cabecera[1] == 0xD8 && cabecera[2] == 0xFF:
		return "image/jpeg"
	case len(cabecera) > 8 && string(cabecera[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(cabecera) > 4 && string(cabecera[:5]) == "%PDF-":
		return "application/pdf"
	}
	return ""
}

// hSubirAdjunto recibe el archivo (multipart) y lo guarda. El navegador manda la foto tomada con
// la cámara del teléfono en el mismo campo.
func hSubirAdjunto(r *http.Request, t *Tenant) (int, any, error) {
	cfg, err := t.Config()
	if err != nil {
		return 0, nil, err
	}
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		return 0, nil, errEntrada("ARCHIVO_INVALIDO",
			fmt.Sprintf("No se pudo leer el archivo. El tamaño máximo es %d MB.", cfg.TamanoMaximoAdjuntoMB))
	}
	defer r.MultipartForm.RemoveAll()

	entidad := strings.TrimSpace(r.FormValue("entidad"))
	entidadID, err := validarID(r.FormValue("entidad_id"), "entidad_id")
	if err != nil {
		return 0, nil, err
	}
	titulo, err := ValidarTexto(r.FormValue("titulo"), "título", LargoMaximoTitulo)
	if err != nil {
		return 0, nil, err
	}
	descripcion, err := ValidarTexto(r.FormValue("descripcion"), "descripción", LargoMaximoNota)
	if err != nil {
		return 0, nil, err
	}
	archivo, cabeceraArchivo, err := r.FormFile("archivo")
	if err != nil {
		return 0, nil, errEntrada("ARCHIVO_REQUERIDO", "Adjunte una foto o un archivo PDF.")
	}
	defer archivo.Close()

	a, err := guardarAdjunto(t, cfg, archivo, cabeceraArchivo, datosAdjunto{
		Entidad: entidad, EntidadID: entidadID, Clase: strings.TrimSpace(r.FormValue("clase")),
		Titulo: titulo, Descripcion: descripcion,
	})
	if err != nil {
		return 0, nil, err
	}
	return http.StatusCreated, a, nil
}

// datosAdjunto es lo que el usuario dice del archivo; el tipo, el tamaño y el nombre salen del
// archivo mismo. Clase vacía = comprobante, que es el caso de siempre.
type datosAdjunto struct {
	Entidad     string
	EntidadID   string
	Clase       string
	Titulo      string
	Descripcion string
}

// guardarAdjunto es el único camino por el que un archivo llega al disco: lo usan la subida de
// comprobantes y la firma del cliente. Valida el acceso al registro, el tipo REAL del archivo y
// que una imagen se pueda decodificar completa antes de dejar el metadato en la base.
func guardarAdjunto(t *Tenant, cfg ConfigEmpresa, archivo multipart.File, cabecera *multipart.FileHeader,
	d datosAdjunto) (Adjunto, error) {
	esFirma := d.Clase == ClaseFirma
	if esFirma {
		// El cliente que firma con el dedo no va a escribir un título: lo pone el sistema.
		d.Titulo = TituloFirma
	} else {
		d.Clase = ClaseComprobante
		// El título es obligatorio: un archivo llamado "IMG_20261017_142233.jpg" no le dice nada a
		// nadie dentro de seis meses, y es justo cuando hay que buscarlo para discutir un saldo.
		if d.Titulo == "" {
			return Adjunto{}, errEntrada("TITULO_REQUERIDO",
				"Escriba un título para el comprobante: es con lo que lo va a reconocer después.")
		}
	}
	if err := verificarAcceso(t, d.Entidad, d.EntidadID); err != nil {
		return Adjunto{}, err
	}

	inicio := make([]byte, 512)
	n, _ := io.ReadFull(archivo, inicio)
	tipoMime := tipoMimeDeArchivo(inicio[:n])
	if tipoMime == "" {
		return Adjunto{}, errEntrada("TIPO_ARCHIVO_INVALIDO", "Solo se aceptan fotos JPG o PNG y archivos PDF.")
	}
	extension, err := ValidarAdjunto(d.Entidad, tipoMime, cabecera.Size, cfg)
	if err != nil {
		return Adjunto{}, err
	}
	maximo := int64(cfg.TamanoMaximoAdjuntoMB) * 1024 * 1024
	if esFirma {
		if err := validarFirma(tipoMime, cabecera.Size); err != nil {
			return Adjunto{}, err
		}
		maximo = tamanoMaximoFirma
	}
	if _, err := archivo.Seek(0, io.SeekStart); err != nil {
		return Adjunto{}, err
	}

	// Almacén en base: el archivo se lee completo (con tope) y se valida en memoria. Almacén en
	// disco: se escribe en la carpeta de la empresa y se valida desde ahí.
	var (
		ruta      string
		contenido []byte
		escritos  int64
	)
	if guardarEnBase {
		contenido, err = leerConTope(archivo, maximo)
		if err != nil {
			return Adjunto{}, err
		}
		escritos = int64(len(contenido))
		if err := verificarImagenLegible(bytes.NewReader(contenido), tipoMime); err != nil {
			return Adjunto{}, err
		}
	} else {
		ruta, err = rutaDeAdjunto(t.EmpresaID, extension)
		if err != nil {
			return Adjunto{}, err
		}
		escritos, err = guardarEnDisco(ruta, archivo, maximo)
		if err != nil {
			borrarArchivo(ruta)
			return Adjunto{}, err
		}
		if err := verificarImagenEnDisco(ruta, tipoMime); err != nil {
			borrarArchivo(ruta)
			return Adjunto{}, err
		}
	}

	nombre, _ := ValidarTexto(filepath.Base(cabecera.Filename), "nombre", 200)
	if nombre == "" {
		nombre = "foto" + extension
	}
	if esFirma {
		nombre = "firma" + extension
	}
	id, err := t.Repo.CrearAdjunto(t.EmpresaID, Adjunto{
		Entidad: d.Entidad, EntidadID: d.EntidadID, Titulo: d.Titulo, Nombre: nombre, Clase: d.Clase,
		TipoMime: tipoMime, TamanoBytes: escritos, archivo: ruta, contenido: contenido,
		Descripcion: d.Descripcion, SubidoPor: t.Usuario.ID,
	})
	if err != nil {
		borrarArchivo(ruta)
		return Adjunto{}, err
	}
	return t.Repo.ObtenerAdjunto(t.EmpresaID, id)
}

// validarFirma: la firma la dibuja el propio sistema en un canvas, así que se sabe exactamente
// qué debe llegar. Solo PNG (el trazo sobre fondo transparente se imprime limpio sobre el
// recibo; un JPG traería un recuadro blanco encima de la línea) y un tope propio muy por debajo
// del de los comprobantes: una firma a dedo pesa decenas de kilobytes, no megabytes.
func validarFirma(tipoMime string, tamanoBytes int64) error {
	if tipoMime != "image/png" {
		return errEntrada("TIPO_ARCHIVO_INVALIDO", "La firma se guarda como imagen PNG.").
			con("tipo_recibido", tipoMime)
	}
	if tamanoBytes > tamanoMaximoFirma {
		return errRegla("ARCHIVO_DEMASIADO_GRANDE", "La firma pesa demasiado. Vuelva a firmar.").
			con("maximo_mb", 2)
	}
	return nil
}

func guardarEnDisco(ruta string, origen multipart.File, maximo int64) (int64, error) {
	destino, err := os.Create(ruta)
	if err != nil {
		return 0, err
	}
	defer destino.Close()
	escritos, err := io.Copy(destino, io.LimitReader(origen, maximo+1))
	if err != nil {
		return 0, err
	}
	if escritos > maximo {
		return 0, errRegla("ARCHIVO_DEMASIADO_GRANDE", "El archivo supera el tamaño permitido por su empresa.")
	}
	return escritos, nil
}

// leerConTope lee el archivo completo en memoria sin pasar de maximo bytes.
func leerConTope(origen io.Reader, maximo int64) ([]byte, error) {
	datos, err := io.ReadAll(io.LimitReader(origen, maximo+1))
	if err != nil {
		return nil, err
	}
	if int64(len(datos)) > maximo {
		return nil, errRegla("ARCHIVO_DEMASIADO_GRANDE", "El archivo supera el tamaño permitido por su empresa.")
	}
	return datos, nil
}

// hDescargarAdjunto entrega el archivo. Se escribe directo en la respuesta (no pasa por el
// envoltorio JSON) y solo después de confirmar que pertenece a la empresa de la sesión.
func (a *App) descargarAdjunto(w http.ResponseWriter, r *http.Request) {
	t := tenantDe(r.Context())
	if t == nil {
		escribirError(w, errNoAutenticado("NO_AUTENTICADO", "Su sesión no es válida o expiró. Inicie sesión nuevamente."))
		return
	}
	id, err := idDeRuta(r, errNoEncontrado("ADJUNTO_NO_ENCONTRADO", "No encontramos el archivo indicado."))
	if err != nil {
		escribirError(w, err)
		return
	}
	adj, err := t.Repo.ObtenerAdjunto(t.EmpresaID, id)
	if err != nil {
		escribirError(w, err)
		return
	}
	if err := verificarAcceso(t, adj.Entidad, adj.EntidadID); err != nil {
		escribirError(w, err)
		return
	}
	datos, err := leerAdjunto(adj)
	if err != nil {
		escribirError(w, errNoEncontrado("ARCHIVO_NO_DISPONIBLE", "El archivo ya no está disponible."))
		return
	}
	w.Header().Set("Content-Type", adj.TipoMime)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", adj.Nombre))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Length", strconv.Itoa(len(datos)))
	w.Write(datos)
}

// leerAdjunto devuelve el contenido de un archivo, venga del disco o de la base. Los adjuntos con
// ruta (almacén en disco, o subidos antes de pasar a la base) se leen del disco; los demás, de
// adjunto.contenido.
func leerAdjunto(a Adjunto) ([]byte, error) {
	if a.archivo != "" {
		return os.ReadFile(a.archivo)
	}
	if a.leerDeBase != nil {
		return a.leerDeBase()
	}
	return nil, errors.New("el adjunto no tiene contenido")
}

// verificarImagenLegible comprueba que la foto se pueda decodificar de verdad. Un archivo con la
// cabecera correcta pero el contenido dañado (una subida cortada, por ejemplo) pasa la detección
// de tipo y recién fallaría al imprimirlo; mejor rechazarlo cuando el usuario todavía está frente
// a la cámara y puede repetir la toma.
func verificarImagenEnDisco(ruta, tipoMime string) error {
	if tipoMime != "image/jpeg" && tipoMime != "image/png" {
		return nil
	}
	f, err := os.Open(ruta)
	if err != nil {
		return err
	}
	defer f.Close()
	return verificarImagenLegible(f, tipoMime)
}

// verificarImagenLegible hace la comprobación sobre cualquier lector: el archivo en disco o los
// bytes ya leídos en memoria.
func verificarImagenLegible(r io.Reader, tipoMime string) error {
	if tipoMime != "image/jpeg" && tipoMime != "image/png" {
		return nil
	}
	// Se decodifica COMPLETA, no solo la cabecera: una subida cortada conserva la cabecera
	// válida y solo se delata al leer los datos de la imagen.
	if _, _, err := image.Decode(r); err != nil {
		return errEntrada("IMAGEN_ILEGIBLE",
			"La foto está dañada o quedó incompleta. Vuelva a tomarla y súbala de nuevo.")
	}
	return nil
}
