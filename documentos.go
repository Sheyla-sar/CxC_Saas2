package main

// documentos.go — genera el recibo de pago y el estado de cuenta en PDF, con la marca de cada
// empresa (colores y nombre) y, si se piden, las fotos adjuntas de recibos y facturas.
// Los montos se formatean con formato de reglas.go; aquí no se calcula nada de negocio.

import (
	"bytes"
	"fmt"
	"log"
	"math"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-pdf/fpdf"
)

// Plantilla compartida de los PDF (docs/solvo-diseno/documentos/spec-plantilla.md).
// Carta, márgenes de 15 mm, tinta oscura sobre blanco. El único color de la empresa
// es el cuadrado de 2.4 mm junto a su nombre.
const (
	pdfL    = 15.0
	pdfR    = 200.9
	pdfCW   = 185.9
	pdfTope = 264.4
	// assets/pdf viaja con la aplicación. docs/ es material de diseño y no está en el repositorio.
	dirFuentesPDF = "assets/pdf"
	logoPDF       = "assets/pdf/solvo-horizontal-negro-1600.png"
)

type documento struct {
	pdf     *fpdf.Fpdf
	empresa Empresa
	marca   Marca
	cfg     ConfigEmpresa
	idDoc   string
	contTit string
	contSub string
	y       float64
}

func nuevoDocumento(e Empresa, m Marca, cfg ConfigEmpresa) (*documento, error) {
	pdf := fpdf.New("P", "mm", "Letter", "")
	pdf.SetMargins(pdfL, 38.5, pdfL)
	pdf.SetAutoPageBreak(true, 15)
	pdf.SetCellMargin(0)
	pdf.AliasNbPages("{nb}")
	d := &documento{pdf: pdf, empresa: e, marca: m, cfg: cfg}
	if err := d.cargarFuentes(); err != nil {
		return nil, err
	}
	pdf.SetHeaderFunc(func() {
		d.cabecera()
		if pdf.PageNo() > 1 && d.contTit != "" {
			d.txt(pdfL, 28.6, d.contTit, "MONOM", 7, 75, 60, 240)
			if d.contSub != "" {
				d.txt(42, 28.6, d.contSub, "MONO", 7, 90, 90, 112)
			}
		}
	})
	pdf.SetFooterFunc(func() {
		d.trazo(189, 189, 203, 0.2)
		pdf.Line(pdfL, 268, pdfR, 268)
		d.txt(pdfL, 272.3, fmt.Sprintf("Solvo · Página %d de {nb}", pdf.PageNo()), "MONO", 7, 90, 90, 112)
		id := d.cortar(d.idDoc, "MONO", 7, 80)
		d.txtDer(pdfR, 272.3, id, "MONO", 7, 90, 90, 112)
	})
	return d, nil
}

func (d *documento) cargarFuentes() error {
	pares := [][2]string{
		{"Syne", "Syne-Bold.ttf"},
		{"IS", "InstrumentSans-Regular.ttf"},
		{"ISB", "InstrumentSans-SemiBold.ttf"},
		{"TAB", "InstrumentSansTab-Regular.ttf"},
		{"TABB", "InstrumentSansTab-SemiBold.ttf"},
		{"MONO", "JetBrainsMono-Regular.ttf"},
		{"MONOM", "JetBrainsMono-Medium.ttf"},
	}
	for _, p := range pares {
		d.pdf.AddUTF8Font(p[0], "", filepath.Join(dirFuentesPDF, p[1]))
	}
	if d.pdf.Err() {
		return fmt.Errorf("fuentes del PDF: %w", d.pdf.Error())
	}
	return nil
}

// colorMarca convierte el color de la empresa (#RRGGBB) a RGB.
func (d *documento) colorMarca() (int, int, int) {
	hex := strings.TrimPrefix(d.marca.ColorPrimario, "#")
	if len(hex) != 6 {
		return 30, 58, 95
	}
	v, err := strconv.ParseInt(hex, 16, 64)
	if err != nil {
		return 30, 58, 95
	}
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF)
}

func (d *documento) fuente(fam string, pt float64) { d.pdf.SetFont(fam, "", pt) }

func (d *documento) txt(x, y float64, s, fam string, pt float64, r, g, b int) {
	if s == "" {
		return
	}
	d.fuente(fam, pt)
	d.pdf.SetTextColor(r, g, b)
	d.pdf.Text(x, y, s)
}

func (d *documento) txtDer(xDer, y float64, s, fam string, pt float64, r, g, b int) {
	if s == "" {
		return
	}
	d.fuente(fam, pt)
	d.pdf.SetTextColor(r, g, b)
	d.pdf.Text(xDer-d.pdf.GetStringWidth(s), y, s)
}

func (d *documento) trazo(r, g, b int, grueso float64) {
	d.pdf.SetDrawColor(r, g, b)
	d.pdf.SetLineWidth(grueso)
}

func (d *documento) ancho(s, fam string, pt float64) float64 {
	d.fuente(fam, pt)
	return d.pdf.GetStringWidth(s)
}

// cortar deja el texto dentro de la columna y marca con "…" lo que no cupo.
func (d *documento) cortar(s, fam string, pt, max float64) string {
	d.fuente(fam, pt)
	if s == "" || d.pdf.GetStringWidth(s) <= max {
		return s
	}
	runes := []rune(s)
	ell := "…"
	for len(runes) > 0 && d.pdf.GetStringWidth(string(runes)+ell) > max {
		runes = runes[:len(runes)-1]
	}
	if len(runes) == 0 {
		return ell
	}
	return string(runes) + ell
}

func (d *documento) lineas(s, fam string, pt, ancho float64) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []string
	var actual string
	for _, p := range strings.Fields(s) {
		cand := p
		if actual != "" {
			cand = actual + " " + p
		}
		if d.ancho(cand, fam, pt) <= ancho {
			actual = cand
			continue
		}
		if actual != "" {
			out = append(out, actual)
		}
		actual = d.cortar(p, fam, pt, ancho)
	}
	if actual != "" {
		out = append(out, actual)
	}
	return out
}

func (d *documento) cabecera() {
	alto := 27.0 * 261.0 / 1600.0
	d.pdf.ImageOptions(logoPDF, pdfL, 11.2, 27, alto, false, fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}, 0, "")
	nombre := d.empresa.NombreComercial
	if nombre == "" {
		nombre = d.empresa.NombreLegal
	}
	d.fuente("ISB", 9)
	ancho := d.pdf.GetStringWidth(nombre)
	br, bg, bb := d.colorMarca()
	d.pdf.SetFillColor(br, bg, bb)
	d.pdf.Rect(pdfR-ancho-2.4-2.2, 12.65, 2.4, 2.4, "F")
	d.txt(pdfR-ancho, 14.9, nombre, "ISB", 9, 11, 11, 20)
	d.trazo(75, 60, 240, 0.3)
	d.pdf.Line(pdfL, 21.6, pdfR, 21.6)
}

func (d *documento) salto() {
	d.pdf.AddPage()
	d.y = 38.5
}

func (d *documento) tituloBloque(titulo, sub, etiqueta, id string) {
	d.pdf.SetFillColor(75, 60, 240)
	d.pdf.Rect(pdfL, 28.5, 2.2, 12.2, "F")
	d.txt(20.2, 34.8, titulo, "Syne", 17, 11, 11, 20)
	d.txt(20.2, 39.9, sub, "IS", 8.5, 90, 90, 112)
	d.txtDer(pdfR, 31.5, etiqueta, "MONO", 6, 90, 90, 112)
	d.txtDer(pdfR, 37.1, id, "MONOM", 9.5, 11, 11, 20)
	if fiscal := d.lineaFiscal(); fiscal != "" {
		d.txt(20.2, 43.6, d.cortar(fiscal, "IS", 7.5, pdfCW-8), "IS", 7.5, 90, 90, 112)
	}
	d.y = 46.5
}

func (d *documento) lineaFiscal() string {
	var partes []string
	if d.empresa.NIT != "" {
		partes = append(partes, "NIT "+d.empresa.NIT)
	}
	if d.empresa.NRC != "" {
		partes = append(partes, "NRC "+d.empresa.NRC)
	}
	if d.empresa.Telefono != "" {
		partes = append(partes, "Tel. "+d.empresa.Telefono)
	}
	if d.empresa.Direccion != "" {
		partes = append(partes, d.empresa.Direccion)
	}
	return strings.Join(partes, "  ·  ")
}

func (d *documento) filaMeta(campos [][3]string) {
	y := d.y
	d.trazo(189, 189, 203, 0.18)
	d.pdf.Line(pdfL, y, pdfR, y)
	d.pdf.Line(pdfL, y+11, pdfR, y+11)
	for i, c := range campos {
		x := 0.0
		fmt.Sscan(c[2], &x)
		siguiente := pdfR
		if i+1 < len(campos) {
			fmt.Sscan(campos[i+1][2], &siguiente)
		}
		d.txt(x, y+4.1, c[0], "MONO", 6, 90, 90, 112)
		d.txt(x, y+8.6, d.cortar(c[1], "TAB", 9.5, siguiente-x-2), "TAB", 9.5, 11, 11, 20)
	}
	d.y = y + 11
}

func (d *documento) seccion(n int, titulo string, alto float64) {
	base := d.y + 13.6
	if base < 38.5 {
		base = 38.5
	}
	if base+alto > pdfTope {
		d.salto()
		base = 38.5
	}
	d.txt(pdfL, base, fmt.Sprintf("%02d /", n), "MONOM", 7, 75, 60, 240)
	d.txt(23.2, base, titulo, "Syne", 10, 11, 11, 20)
	d.y = base + 3.6
}

func partirMonto(s string) (string, string) {
	i := strings.LastIndex(s, ".")
	if i < 0 {
		return s, ""
	}
	return s[:i], s[i:]
}

func (d *documento) cifra(x, y float64, monto string, tamE, tamC float64) {
	ent, cen := partirMonto(monto)
	d.txt(x, y, ent, "TABB", tamE, 11, 11, 20)
	if cen != "" {
		d.txt(x+d.ancho(ent, "TABB", tamE), y, cen, "TAB", tamC, 90, 90, 112)
	}
}

func (d *documento) caja(x, y, w, h float64, etiqueta, monto string, tamE, tamC float64) {
	d.pdf.SetFillColor(246, 246, 251)
	d.trazo(189, 189, 203, 0.25)
	d.pdf.Rect(x, y, w, h, "FD")
	d.pdf.SetFillColor(75, 60, 240)
	d.pdf.Rect(x, y-0.05, 12, 0.9, "F")
	d.txt(x+4.5, y+6.6, etiqueta, "MONO", 6.5, 90, 90, 112)
	d.cifra(x+4.5, y+h*0.56, monto, tamE, tamC)
}

type colPDF struct {
	titulo string
	ancho  float64
	der    bool
	clase  string // texto, num, mono, nombre, estado, tramo
}

func (d *documento) encabezadoTabla(cols []colPDF) {
	if d.y+6 > pdfTope {
		d.salto()
	}
	y := d.y
	x := pdfL
	for _, c := range cols {
		eti := strings.ToUpper(c.titulo)
		if c.der {
			d.txtDer(x+c.ancho, y+3.81, eti, "MONO", 6.3, 90, 90, 112)
		} else {
			d.txt(x, y+3.81, d.cortar(eti, "MONO", 6.3, c.ancho), "MONO", 6.3, 90, 90, 112)
		}
		x += c.ancho
	}
	d.trazo(75, 60, 240, 0.4)
	d.pdf.Line(pdfL, y+6, pdfR, y+6)
	d.y = y + 6
}

func (d *documento) filaTabla(cols []colPDF, fila []string, total bool) {
	y := d.y
	x := pdfL
	for i, c := range cols {
		valor := ""
		if i < len(fila) {
			valor = fila[i]
		}
		fam, pt := "IS", 8.5
		cr, cg, cb := 11, 11, 20
		if total {
			fam = "ISB"
			if c.der || c.clase == "num" {
				fam = "TABB"
			}
		}
		switch c.clase {
		case "num":
			fam = "TAB"
			if total {
				fam = "TABB"
			}
		case "mono":
			fam, pt = "MONO", 7
		case "nombre", "texto":
			if !total {
				fam = "IS"
			}
		case "estado":
			fam, pt = "MONOM", 7
			cr, cg, cb = colorEstado(valor)
		case "indice":
			fam, pt = "MONOM", 8
			cr, cg, cb = 75, 60, 240
		case "tramo":
			cr, cg, cb = colorTramo(valor)
			d.txt(x, y+4.18, "/", "MONOM", 8, cr, cg, cb)
			d.txt(x+3.6, y+4.18, d.cortar(valor, "IS", 8.5, c.ancho-3.6), "IS", 8.5, 11, 11, 20)
			x += c.ancho
			continue
		}
		if c.clase == "nombre" || (!c.der && c.clase != "num") {
			valor = d.cortar(valor, fam, pt, c.ancho-0.4)
		}
		base := y + 4.18
		if fam == "MONO" || fam == "MONOM" {
			base = y + 4.05
		}
		if c.der {
			d.txtDer(x+c.ancho, base, valor, fam, pt, cr, cg, cb)
		} else {
			d.txt(x, base, valor, fam, pt, cr, cg, cb)
		}
		x += c.ancho
	}
	d.y = y + 6.2
}

func (d *documento) tabla(cols []colPDF, filas [][]string, total []string) {
	d.encabezadoTabla(cols)
	for i, fila := range filas {
		if d.y+6.2 > pdfTope {
			d.salto()
			d.encabezadoTabla(cols)
		}
		d.filaTabla(cols, fila, false)
		if i < len(filas)-1 || total == nil {
			d.trazo(189, 189, 203, 0.15)
			d.pdf.Line(pdfL, d.y, pdfR, d.y)
		}
	}
	if total != nil {
		if d.y+6.2 > pdfTope {
			d.salto()
			d.encabezadoTabla(cols)
		}
		d.trazo(11, 11, 20, 0.3)
		d.pdf.Line(pdfL, d.y, pdfR, d.y)
		d.filaTabla(cols, total, true)
	}
}

func (d *documento) nota(s string) {
	for _, linea := range d.lineas(s, "IS", 7.5, pdfCW) {
		if d.y+5 > pdfTope {
			d.salto()
		}
		d.y += 4.6
		d.txt(pdfL, d.y, linea, "IS", 7.5, 90, 90, 112)
	}
}

func colorEstado(s string) (int, int, int) {
	u := strings.ToUpper(s)
	switch {
	case strings.Contains(u, "VENCID"):
		return 217, 69, 58
	case strings.Contains(u, "POR VENCER"):
		return 185, 122, 10
	case strings.Contains(u, "AL D"):
		return 14, 143, 122
	default:
		return 90, 90, 112
	}
}

func colorTramo(s string) (int, int, int) {
	switch {
	case strings.Contains(s, "90"), strings.Contains(s, "61"):
		return 217, 69, 58
	case strings.Contains(s, "30"), strings.Contains(s, "60"):
		return 185, 122, 10
	default:
		return 14, 143, 122
	}
}

func marcaEstado(estado string, dias int) string {
	et := strings.ToUpper(EtiquetaEstadoCuenta(estado))
	if estado == EstadoVencido && dias > 0 {
		return fmt.Sprintf("/ %s · %d D", et, dias)
	}
	return "/ " + et
}

func (d *documento) franja(celdas [][2]string, estado string) {
	if d.y+17 > pdfTope {
		d.salto()
	}
	y := d.y
	n := len(celdas)
	if n == 0 {
		return
	}
	ancho := pdfCW / float64(n)
	d.trazo(75, 60, 240, 0.4)
	d.pdf.Line(pdfL, y, pdfR, y)
	d.trazo(189, 189, 203, 0.18)
	d.pdf.Line(pdfL, y+17, pdfR, y+17)
	for i, c := range celdas {
		x := pdfL + float64(i)*ancho
		if i > 0 {
			d.trazo(189, 189, 203, 0.18)
			d.pdf.Line(x, y+2.2, x, y+14.8)
			x += 4
		}
		d.txt(x, y+5.6, c[0], "MONO", 6.2, 90, 90, 112)
		if strings.Contains(c[1], ".") {
			d.cifra(x, y+13.2, c[1], 14, 9)
		} else {
			d.txt(x, y+13.2, c[1], "TABB", 14, 11, 11, 20)
		}
	}
	if estado != "" {
		cr, cg, cb := colorEstado(estado)
		d.txtDer(pdfR, y+5.6, estado, "MONOM", 6.5, cr, cg, cb)
	}
	d.y = y + 17
}

func (d *documento) bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := d.pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type fotoLista struct {
	adjunto Adjunto
	nombre  string
	tipo    string
	pxAncho float64
	pxAlto  float64
}

// fotosRegistradas deja listas las imágenes que sí se pueden dibujar. La firma no entra:
// va dentro del recibo, sobre la línea del cliente.
func (d *documento) fotosRegistradas(adjuntos []Adjunto) []fotoLista {
	var out []fotoLista
	for _, a := range adjuntos {
		if !a.EsImagen() || a.EsFirma() {
			continue
		}
		datos, err := leerAdjunto(a)
		if err != nil {
			continue
		}
		tipo := "JPG"
		if a.TipoMime == "image/png" {
			tipo = "PNG"
		}
		nombre := "adj-" + a.ID
		if !d.registrarImagen(nombre, tipo, datos) {
			continue
		}
		info := d.pdf.GetImageInfo(nombre)
		if info == nil || info.Width() <= 0 || info.Height() <= 0 {
			continue
		}
		out = append(out, fotoLista{a, nombre, tipo, info.Width(), info.Height()})
	}
	return out
}

func (d *documento) anexarFotos(adjuntos []Adjunto, contexto string) {
	fotos := d.fotosRegistradas(adjuntos)
	for i, f := range fotos {
		d.contSub = fmt.Sprintf("Comprobante %d de %d", i+1, len(fotos))
		if contexto != "" {
			d.contSub += " · " + d.cortar(contexto, "MONO", 7, 120)
		}
		d.salto()
		d.paginaFoto(f, i+1, len(fotos))
	}
}

// paginaFoto dibuja una imagen subida, a escala y sin deformarla, con borde fino.
func (d *documento) paginaFoto(f fotoLista, n, total int) {
	d.pdf.SetFillColor(75, 60, 240)
	d.pdf.Rect(pdfL, 35.5, 2.2, 12.2, "F")
	d.txt(20.2, 41.8, d.cortar(f.adjunto.ComoSeLlama(), "Syne", 14, pdfCW-8), "Syne", 14, 11, 11, 20)
	if f.adjunto.Descripcion != "" {
		d.txt(20.2, 46.9, d.cortar(f.adjunto.Descripcion, "IS", 8.5, pdfCW-8), "IS", 8.5, 90, 90, 112)
	}
	y := 54.5
	d.trazo(189, 189, 203, 0.18)
	d.pdf.Line(pdfL, y, pdfR, y)
	d.pdf.Line(pdfL, y+11, pdfR, y+11)
	d.txt(pdfL, y+4.1, "TEMA", "MONO", 6, 90, 90, 112)
	d.txt(pdfL, y+8.6, d.cortar(f.adjunto.Descripcion, "TAB", 9.5, 80), "TAB", 9.5, 11, 11, 20)
	d.txt(99, y+4.1, "SUBIDO POR", "MONO", 6, 90, 90, 112)
	d.txt(99, y+8.6, d.cortar(f.adjunto.SubidoPorNombre, "TAB", 9.5, 50), "TAB", 9.5, 11, 11, 20)
	d.txt(153, y+4.1, "FECHA Y HORA", "MONO", 6, 90, 90, 112)
	d.txt(153, y+8.6, f.adjunto.CreadoEn.Format("2006-01-02, 15:04"), "TAB", 9.5, 11, 11, 20)

	topeImg := 139.4
	yImg := y + 11 + 7
	escala := math.Min(pdfCW/f.pxAncho, topeImg/f.pxAlto)
	w, h := f.pxAncho*escala, f.pxAlto*escala
	if yImg+h > pdfTope-8 {
		escala = math.Min(escala, (pdfTope-8-yImg)/f.pxAlto)
		w, h = f.pxAncho*escala, f.pxAlto*escala
	}
	xImg := pdfL + (pdfCW-w)/2
	d.imagenAislada(f.nombre, f.tipo, xImg, yImg, w, h)
	d.trazo(189, 189, 203, 0.25)
	d.pdf.Rect(xImg, yImg, w, h, "D")
	pieY := yImg + h + 4
	if pieY > pdfTope {
		pieY = pdfTope
	}
	d.txt(pdfL, pieY, d.cortar(fmt.Sprintf("%s · %.0f×%.0f", f.adjunto.Nombre, f.pxAncho, f.pxAlto), "MONO", 6.3, 150), "MONO", 6.3, 90, 90, 112)
	d.txtDer(pdfR, pieY, fmt.Sprintf("%02d /", n), "MONOM", 7, 75, 60, 240)
	_ = total
}

// ─────────────────────────────────────────────────────────────────────────────
// Recibo de pago
// ─────────────────────────────────────────────────────────────────────────────

type DatosRecibo struct {
	Empresa     Empresa
	Marca       Marca
	Config      ConfigEmpresa
	Cliente     Cliente
	Cuenta      Cuenta
	Movimiento  Movimiento
	SaldoAntes  int64
	SaldoActual int64
	Adjuntos    []Adjunto
}

// ReciboPDF arma el comprobante que el cobrador entrega al cliente.
func ReciboPDF(d DatosRecibo) ([]byte, error) {
	doc, err := nuevoDocumento(d.Empresa, d.Marca, d.Config)
	if err != nil {
		return nil, err
	}
	titulo := "RECIBO DE PAGO"
	sub := "Constancia del pago recibido"
	etiqueta := "N.º DE RECIBO"
	if d.Movimiento.Tipo == TipoNotaCredito {
		titulo = "NOTA DE CRÉDITO"
		sub = "Constancia de la nota de crédito"
		etiqueta = "N.º DE NOTA"
	}
	numero := ""
	if d.Movimiento.Numero != nil {
		numero = *d.Movimiento.Numero
	}
	if numero == "" {
		numero = "—"
	}
	doc.idDoc = numero
	doc.contTit = titulo
	doc.contSub = "continuación"
	doc.pdf.AddPage()
	doc.tituloBloque(titulo, sub, etiqueta, numero)

	metodo := EtiquetaMetodoPago(d.Movimiento.Metodo)
	if d.Movimiento.Referencia != "" {
		metodo += "  " + d.Movimiento.Referencia
	}
	doc.filaMeta([][3]string{
		{"FECHA", d.Movimiento.Fecha.String(), "15"},
		{"MÉTODO", metodo, "75"},
		{"COBRADOR", d.Movimiento.RegistradoPorNombre, "135"},
	})

	documento := d.Cuenta.NumeroDocumento
	if documento == "" {
		documento = "—"
	}
	filas := [][2]string{{"RECIBIMOS DE", d.Cliente.Nombre}}
	if d.Cliente.CodigoExterno != "" {
		filas = append(filas, [2]string{"CÓDIGO", d.Cliente.CodigoExterno})
	}
	filas = append(filas, [2]string{"CUENTA", documento})
	if d.Movimiento.Nota != "" {
		filas = append(filas, [2]string{"CONCEPTO", d.Movimiento.Nota})
	}
	filas = append(filas, [2]string{"VENCIMIENTO", d.Cuenta.FechaVencimiento.String()})

	doc.seccion(1, "Datos del pago", 40)
	yLista := doc.y
	altoLista := float64(len(filas)) * 7.4
	doc.trazo(75, 60, 240, 0.4)
	doc.pdf.Line(pdfL, yLista, pdfL+112, yLista)
	for i, f := range filas {
		base := yLista + float64(i)*7.4
		doc.txt(pdfL, base+4.7, f[0], "MONO", 6.3, 90, 90, 112)
		doc.txt(51, base+4.7, doc.cortar(f[1], "IS", 9, 74), "IS", 9, 11, 11, 20)
		doc.trazo(189, 189, 203, 0.15)
		doc.pdf.Line(pdfL, base+7.4, pdfL+112, base+7.4)
	}
	doc.caja(133, yLista, 67.9, altoLista, "MONTO PAGADO", FormatearMonto(d.Movimiento.MontoCentavos, d.Config), 26, 14)
	letras := doc.lineas(MontoEnLetras(d.Movimiento.MontoCentavos, d.Config), "IS", 7.5, 58)
	yLetra := yLista + altoLista*0.68
	usadas := 0
	for _, linea := range letras {
		if yLetra > yLista+altoLista-2.5 {
			break
		}
		doc.txt(137.5, yLetra, linea, "IS", 7.5, 90, 90, 112)
		yLetra += 3.2
		usadas++
	}
	doc.y = yLista + altoLista
	if usadas < len(letras) {
		for _, linea := range doc.lineas(MontoEnLetras(d.Movimiento.MontoCentavos, d.Config), "IS", 7.5, pdfCW) {
			doc.y += 4
			doc.txt(pdfL, doc.y, linea, "IS", 7.5, 90, 90, 112)
		}
	}

	doc.seccion(2, "Saldo de la cuenta", 20)
	doc.franja([][2]string{
		{"SALDO ANTERIOR", FormatearMonto(d.SaldoAntes, d.Config)},
		{"PAGO RECIBIDO", FormatearMonto(d.Movimiento.MontoCentavos, d.Config)},
		{"SALDO RESTANTE", FormatearMonto(d.SaldoActual, d.Config)},
	}, marcaEstado(d.Cuenta.Estado, d.Cuenta.DiasVencida))

	linea := doc.y + 22
	if linea+14 > pdfTope {
		doc.salto()
		linea = doc.y + 22
	}
	if firma, hay := firmaDelCliente(d.Adjuntos); hay {
		doc.dibujarFirma(firma, pdfR-84, linea, 84, 18)
	}
	doc.trazo(11, 11, 20, 0.3)
	doc.pdf.Line(pdfL, linea, pdfL+84, linea)
	doc.pdf.Line(pdfR-84, linea, pdfR, linea)
	doc.txt(pdfL, linea+4, "ENTREGADO POR", "MONO", 6.3, 90, 90, 112)
	doc.txt(pdfR-84, linea+4, "RECIBIDO POR EL CLIENTE", "MONO", 6.3, 90, 90, 112)
	doc.txt(pdfL, linea+8.2, doc.cortar(d.Movimiento.RegistradoPorNombre, "IS", 8.5, 84), "IS", 8.5, 11, 11, 20)
	doc.txt(pdfR-84, linea+8.2, doc.cortar(d.Cliente.Nombre, "IS", 8.5, 84), "IS", 8.5, 11, 11, 20)

	doc.anexarFotos(d.Adjuntos, numero)
	return doc.bytes()
}

// firmaDelCliente busca entre los adjuntos del movimiento la firma que el cliente trazó. Viene
// en la misma lista que los comprobantes porque es el mismo tipo de registro; lo que cambia es
// dónde se imprime.
func firmaDelCliente(adjuntos []Adjunto) (Adjunto, bool) {
	for i := len(adjuntos) - 1; i >= 0; i-- {
		if adjuntos[i].EsFirma() && adjuntos[i].TipoMime == "image/png" {
			return adjuntos[i], true
		}
	}
	return Adjunto{}, false
}

// dibujarFirma coloca la firma dentro del espacio de firma, apoyada sobre la línea y centrada,
// a escala y SIN deformarla: una firma estirada no se parece a la del cliente. baseY es la
// línea; anchoMax y altoMax, el recuadro disponible encima de ella.
// Una firma que no se pueda leer nunca puede impedir que el recibo se imprima: se omite y el
// recibo sale con la línea en blanco, como salía antes de que existiera la firma.
func (d *documento) dibujarFirma(a Adjunto, x, baseY, anchoMax, altoMax float64) {
	datos, err := leerAdjunto(a)
	if err != nil {
		log.Printf("firma %s no se pudo leer del disco: %v", a.ID, err)
		return
	}
	nombre := "firma-" + a.ID
	if !d.registrarImagen(nombre, "PNG", datos) {
		return
	}
	info := d.pdf.GetImageInfo(nombre)
	if info == nil || info.Width() <= 0 || info.Height() <= 0 {
		return
	}
	ancho, alto := anchoMax, anchoMax*info.Height()/info.Width()
	if alto > altoMax {
		ancho, alto = altoMax*info.Width()/info.Height(), altoMax
	}
	d.imagenAislada(nombre, "PNG", x+(anchoMax-ancho)/2, baseY-alto-1, ancho, alto)
}

// imagenAislada dibuja una imagen ya registrada aislando el pánico, igual que registrarImagen:
// fpdf no siempre devuelve error con una imagen rara, a veces entra en pánico, y en ese caso el
// documento tiene que salir de todos modos.
func (d *documento) imagenAislada(nombre, tipo string, x, y, ancho, alto float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("imagen %s no se pudo dibujar: %v", nombre, r)
			d.pdf.ClearError()
		}
	}()
	d.pdf.ImageOptions(nombre, x, y, ancho, alto, false, fpdf.ImageOptions{ImageType: tipo}, 0, "")
	if d.pdf.Err() {
		log.Printf("imagen %s no se pudo dibujar: %v", nombre, d.pdf.Error())
		d.pdf.ClearError()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Estado de cuenta
// ─────────────────────────────────────────────────────────────────────────────

type DatosEstadoCuenta struct {
	Empresa        Empresa
	Marca          Marca
	Config         ConfigEmpresa
	Cliente        Cliente
	Hoy            Fecha
	Cuentas        []Cuenta // ya evaluadas
	Saldo          SaldoCliente
	Credito        CreditoCliente
	Comportamiento Comportamiento
	Antiguedad     []TramoAntiguedad
	Anticipos      []Anticipo
	Movimientos    map[string][]Movimiento // por cuenta, para el detalle
	Adjuntos       []Adjunto
	ConDetalle     bool
}

// EstadoCuentaPDF arma el documento que se le manda al cliente con todo lo que debe.
func EstadoCuentaPDF(d DatosEstadoCuenta) ([]byte, error) {
	doc, err := nuevoDocumento(d.Empresa, d.Marca, d.Config)
	if err != nil {
		return nil, err
	}
	id := "EC-" + d.Hoy.String()
	if len(id) >= 10 {
		id = "EC-" + d.Hoy.String()[:7]
	}
	doc.idDoc = id + "-" + doc.cortar(d.Cliente.Nombre, "MONO", 7, 40)
	doc.contTit = "ESTADO DE CUENTA"
	doc.contSub = d.Cliente.Nombre + " · continuación"
	doc.pdf.AddPage()
	doc.tituloBloque("ESTADO DE CUENTA", "Saldo y movimientos de un cliente", "N.º DE ESTADO", doc.idDoc)

	limite := "Sin límite"
	if !d.Credito.SinLimite {
		limite = FormatearMonto(d.Credito.LimiteCreditoCentavos, d.Config)
	}
	doc.filaMeta([][3]string{
		{"CLIENTE", d.Cliente.Nombre, "15"},
		{"EMITIDO", d.Hoy.String(), "81"},
		{"COBRADOR", d.Cliente.VendedorNombre, "139"},
		{"LÍMITE", limite, "172"},
	})
	var extra []string
	if d.Cliente.Contacto != "" {
		extra = append(extra, "Contacto: "+d.Cliente.Contacto)
	}
	if d.Cliente.Telefono != "" {
		extra = append(extra, "Teléfono: "+d.Cliente.Telefono)
	}
	extra = append(extra, "Comportamiento: "+TextoComportamiento(d.Comportamiento))
	doc.nota(strings.Join(extra, "   ·   "))

	n := 1
	doc.seccion(n, "Resumen de saldo", 20)
	n++
	doc.franja([][2]string{
		{"SALDO PENDIENTE", FormatearMonto(d.Saldo.SaldoCentavos, d.Config)},
		{"CUENTAS ABIERTAS", strconv.Itoa(d.Saldo.CuentasAbiertas)},
		{"SALDO VENCIDO", FormatearMonto(d.Saldo.SaldoVencidoCentavos, d.Config)},
	}, "")

	if len(d.Antiguedad) > 0 {
		doc.seccion(n, "Antigüedad de saldos", 20)
		n++
		filas := [][]string{}
		for _, t := range d.Antiguedad {
			filas = append(filas, []string{
				t.Etiqueta, strconv.Itoa(t.Cantidad),
				FormatearMonto(t.SaldoCentavos, d.Config),
				formatearPorcentaje(PorcentajeDe(t.SaldoCentavos, d.Saldo.SaldoCentavos)),
			})
		}
		doc.tabla([]colPDF{
			{"Tramo", 78, false, "tramo"},
			{"Cuentas", 30, true, "num"},
			{"Saldo", 42, true, "num"},
			{"% cartera", 35.9, true, "num"},
		}, filas, nil)
	}

	doc.seccion(n, "Documentos pendientes", 20)
	n++
	filas := [][]string{}
	var sumaMonto, sumaAbonado, sumaSaldo int64
	for _, c := range d.Cuentas {
		if c.SaldoCentavos <= 0 {
			continue
		}
		documento := c.NumeroDocumento
		if documento == "" {
			documento = "—"
		}
		abonado := c.PagadoCentavos + c.CondonadoCentavos + c.NotaCreditoCentavos
		sumaMonto += c.MontoTotalCentavos
		sumaAbonado += abonado
		sumaSaldo += c.SaldoCentavos
		filas = append(filas, []string{
			documento, c.FechaEmision.String(), c.FechaVencimiento.String(),
			FormatearMonto(c.MontoTotalCentavos, d.Config),
			FormatearMonto(abonado, d.Config),
			FormatearMonto(c.SaldoCentavos, d.Config),
			marcaEstado(c.Estado, c.DiasVencida),
		})
	}
	colsCuenta := []colPDF{
		{"Cuenta", 23, false, "mono"},
		{"Emitida", 25, false, "num"},
		{"Vence", 25, false, "num"},
		{"Original", 24, true, "num"},
		{"Pagado", 22, true, "num"},
		{"Saldo", 24, true, "num"},
		{"Estado", 42.9, true, "estado"},
	}
	if len(filas) == 0 {
		doc.nota("El cliente no tiene documentos pendientes.")
	} else {
		doc.tabla(colsCuenta, filas, []string{
			"Total", "", "",
			FormatearMonto(sumaMonto, d.Config),
			FormatearMonto(sumaAbonado, d.Config),
			FormatearMonto(sumaSaldo, d.Config),
			"",
		})
	}

	disponibles := [][]string{}
	for _, a := range d.Anticipos {
		if a.DisponibleCentavos > 0 {
			disponibles = append(disponibles, []string{a.Numero, a.Fecha.String(), EtiquetaMetodoPago(a.Metodo),
				FormatearMonto(a.DisponibleCentavos, d.Config)})
		}
	}
	if len(disponibles) > 0 {
		doc.seccion(n, "Anticipos a favor del cliente", 20)
		n++
		doc.tabla([]colPDF{
			{"Anticipo", 46, false, "mono"},
			{"Fecha", 40, false, "num"},
			{"Forma de pago", 55.9, false, "texto"},
			{"Disponible", 44, true, "num"},
		}, disponibles, nil)
	}

	if d.ConDetalle {
		for _, c := range d.Cuentas {
			movs := d.Movimientos[c.ID]
			if len(movs) == 0 {
				continue
			}
			documento := c.NumeroDocumento
			if documento == "" {
				documento = c.FechaEmision.String()
			}
			doc.seccion(n, "Movimientos de "+documento, 20)
			n++
			detalle := [][]string{}
			for _, m := range movs {
				numero := ""
				if m.Numero != nil {
					numero = *m.Numero
				}
				estado := ""
				if m.Anulado {
					estado = "/ ANULADO"
				}
				detalle = append(detalle, []string{
					m.Fecha.String(), EtiquetaTipoMovimiento(m.Tipo), numero,
					EtiquetaMetodoPago(m.Metodo), FormatearMonto(m.MontoCentavos, d.Config), estado,
				})
			}
			doc.tabla([]colPDF{
				{"Fecha", 28, false, "num"},
				{"Tipo", 36, false, "texto"},
				{"Número", 36, false, "mono"},
				{"Forma", 32, false, "texto"},
				{"Monto", 28, true, "num"},
				{"", 25.9, true, "estado"},
			}, detalle, nil)
		}
	}

	doc.anexarFotos(d.Adjuntos, d.Cliente.Nombre)
	return doc.bytes()
}

// ─────────────────────────────────────────────────────────────────────────────
// Monto en letras (se usa en los recibos)
// ─────────────────────────────────────────────────────────────────────────────

var (
	unidades = []string{"", "UN", "DOS", "TRES", "CUATRO", "CINCO", "SEIS", "SIETE", "OCHO", "NUEVE",
		"DIEZ", "ONCE", "DOCE", "TRECE", "CATORCE", "QUINCE", "DIECISÉIS", "DIECISIETE", "DIECIOCHO",
		"DIECINUEVE", "VEINTE", "VEINTIUNO", "VEINTIDÓS", "VEINTITRÉS", "VEINTICUATRO", "VEINTICINCO",
		"VEINTISÉIS", "VEINTISIETE", "VEINTIOCHO", "VEINTINUEVE"}
	decenas  = []string{"", "", "VEINTE", "TREINTA", "CUARENTA", "CINCUENTA", "SESENTA", "SETENTA", "OCHENTA", "NOVENTA"}
	centenas = []string{"", "CIENTO", "DOSCIENTOS", "TRESCIENTOS", "CUATROCIENTOS", "QUINIENTOS",
		"SEISCIENTOS", "SETECIENTOS", "OCHOCIENTOS", "NOVECIENTOS"}
)

func enLetras(n int64) string {
	switch {
	case n == 0:
		return "CERO"
	case n < 30:
		return unidades[n]
	case n < 100:
		if n%10 == 0 {
			return decenas[n/10]
		}
		return decenas[n/10] + " Y " + unidades[n%10]
	case n == 100:
		return "CIEN"
	case n < 1000:
		if n%100 == 0 {
			return centenas[n/100]
		}
		return centenas[n/100] + " " + enLetras(n%100)
	case n < 2000:
		if n%1000 == 0 {
			return "MIL"
		}
		return "MIL " + enLetras(n%1000)
	case n < 1000000:
		miles := enLetras(n/1000) + " MIL"
		if n%1000 == 0 {
			return miles
		}
		return miles + " " + enLetras(n%1000)
	case n < 2000000:
		if n%1000000 == 0 {
			return "UN MILLÓN"
		}
		return "UN MILLÓN " + enLetras(n%1000000)
	default:
		millones := enLetras(n/1000000) + " MILLONES"
		if n%1000000 == 0 {
			return millones
		}
		return millones + " " + enLetras(n%1000000)
	}
}

// MontoEnLetras: 123456 → "MIL DOSCIENTOS TREINTA Y CUATRO 56/100 DÓLARES".
func MontoEnLetras(centavos int64, cfg ConfigEmpresa) string {
	signo := ""
	if centavos < 0 {
		signo = "MENOS "
		centavos = -centavos
	}
	moneda := map[string]string{"USD": "DÓLARES", "GTQ": "QUETZALES", "HNL": "LEMPIRAS", "CRC": "COLONES"}[cfg.Moneda]
	if moneda == "" {
		moneda = cfg.Moneda
	}
	return fmt.Sprintf("%s%s %02d/100 %s", signo, enLetras(centavos/100), centavos%100, moneda)
}

// registrarImagen incrusta una foto en el documento y devuelve false si no se pudo. Con algunas
// imágenes dañadas fpdf no devuelve error sino que entra en pánico, así que se aísla aquí: un
// adjunto ilegible no puede impedir que el recibo o el estado de cuenta se impriman.
func (d *documento) registrarImagen(nombre, tipo string, datos []byte) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("adjunto %s no se pudo anexar al PDF: %v", nombre, r)
			d.pdf.ClearError()
			ok = false
		}
	}()
	d.pdf.RegisterImageOptionsReader(nombre, fpdf.ImageOptions{ImageType: tipo, ReadDpi: false}, bytes.NewReader(datos))
	if d.pdf.Err() {
		log.Printf("adjunto %s no se pudo anexar al PDF: %v", nombre, d.pdf.Error())
		d.pdf.ClearError()
		return false
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
// Informe mensual del dueño
// ─────────────────────────────────────────────────────────────────────────────

// DatosInformeMensual reúne lo que se imprime. La actividad (cobranza, ventas, salidas) es del
// mes; la cartera es la foto de HOY, y así se rotula en el documento: reconstruir la cartera de
// un mes cerrado exigiría historial que no se guarda.
type DatosInformeMensual struct {
	Empresa       Empresa
	Marca         Marca
	Config        ConfigEmpresa
	Mes           string // "septiembre de 2026"
	Desde, Hasta  Fecha
	Hoy           Fecha
	Antiguedad    []TramoAntiguedad
	SaldoTotal    int64
	SaldoVencido  int64
	CobradoMes    int64
	CobradoAnte   int64
	PorMetodo     []TotalPorMetodo
	Cobradores    []CobradorInforme
	Venta         VentaDelPeriodo
	Salidas       []SalidaSinCobro
	TotalSalidas  int64
	Deudores      []Deudor
	Puntos        []string
	Liquidaciones int
}

// CobradorInforme es una fila del cuadro de cobradores del informe.
type CobradorInforme struct {
	Nombre          string
	CobradoCentavos int64
	CarteraCentavos int64
	VencidoCentavos int64
	Clientes        int
	Diferencia      int64
	TieneLiquidado  bool
}

func InformeMensualPDF(d DatosInformeMensual) ([]byte, error) {
	doc, err := nuevoDocumento(d.Empresa, d.Marca, d.Config)
	if err != nil {
		return nil, err
	}
	id := "INF-" + d.Desde.String()
	if len(d.Desde.String()) >= 7 {
		id = "INF-" + d.Desde.String()[:7]
	}
	doc.idDoc = id
	doc.contTit = "INFORME MENSUAL"
	doc.contSub = d.Mes + " · continuación"
	doc.pdf.AddPage()
	doc.tituloBloque("INFORME MENSUAL", "Resumen de cobranza y cartera", "N.º DE DOCUMENTO", id)
	doc.filaMeta([][3]string{
		{"MES", d.Mes, "15"},
		{"EMITIDO", d.Hoy.String(), "75"},
	})

	y := doc.y + 6
	ancho := (pdfCW - 5) / 2
	doc.caja(pdfL, y, ancho, 27.5, "COBRADO EN EL MES", FormatearMonto(d.CobradoMes, d.Config), 20, 12)
	doc.txt(pdfL+4.5, y+21.6, "MES ANTERIOR", "MONO", 6.3, 90, 90, 112)
	doc.txt(pdfL+4.5+doc.ancho("MES ANTERIOR ", "MONO", 6.3), y+21.6, FormatearMonto(d.CobradoAnte, d.Config), "TAB", 8.5, 11, 11, 20)

	x2 := pdfL + ancho + 5
	doc.caja(x2, y, ancho, 27.5, "CARTERA POR COBRAR HOY", FormatearMonto(d.SaldoTotal, d.Config), 20, 12)
	ven := "/ VENCIDA"
	cr, cg, cb := colorEstado(ven)
	doc.txt(x2+4.5, y+21.6, ven, "MONOM", 6.3, cr, cg, cb)
	venDetalle := FormatearMonto(d.SaldoVencido, d.Config) + "  " + formatearPorcentaje(PorcentajeDe(d.SaldoVencido, d.SaldoTotal))
	doc.txt(x2+4.5+doc.ancho(ven+" ", "MONOM", 6.3), y+21.6, doc.cortar(venDetalle, "TAB", 8.5, ancho-36), "TAB", 8.5, 11, 11, 20)
	doc.y = y + 27.5

	n := 1
	if len(d.Puntos) > 0 {
		doc.seccion(n, "Puntos de atención", 12)
		n++
		for _, p := range d.Puntos {
			lineas := doc.lineas(p, "IS", 8.5, pdfCW-4)
			for i, linea := range lineas {
				if doc.y+5 > pdfTope {
					doc.salto()
				}
				base := doc.y + 3.4
				if i == 0 {
					doc.pdf.SetFillColor(75, 60, 240)
					doc.pdf.Rect(pdfL, base-2.0, 1.1, 1.1, "F")
				}
				doc.txt(19, base, linea, "IS", 8.5, 11, 11, 20)
				doc.y += 5
			}
		}
	}

	if len(d.Antiguedad) > 0 {
		doc.seccion(n, "Antigüedad de saldos", 24)
		n++
		filas := [][]string{}
		for _, t := range d.Antiguedad {
			filas = append(filas, []string{
				t.Etiqueta, strconv.Itoa(t.Cantidad),
				FormatearMonto(t.SaldoCentavos, d.Config),
				formatearPorcentaje(PorcentajeDe(t.SaldoCentavos, d.SaldoTotal)),
			})
		}
		doc.tabla([]colPDF{
			{"Tramo", 78, false, "tramo"},
			{"Cuentas", 30, true, "num"},
			{"Saldo", 42, true, "num"},
			{"% cartera", 35.9, true, "num"},
		}, filas, nil)
	}

	if len(d.PorMetodo) > 0 {
		doc.seccion(n, "Cómo entró el dinero", 24)
		n++
		filas := [][]string{}
		for _, m := range d.PorMetodo {
			filas = append(filas, []string{
				EtiquetaMetodoPago(m.Metodo), strconv.Itoa(m.Cantidad),
				FormatearMonto(m.MontoCentavos, d.Config),
				formatearPorcentaje(PorcentajeDe(m.MontoCentavos, d.CobradoMes)),
			})
		}
		doc.tabla([]colPDF{
			{"Forma de pago", 78, false, "texto"},
			{"Pagos", 30, true, "num"},
			{"Monto", 42, true, "num"},
			{"% del mes", 35.9, true, "num"},
		}, filas, nil)
	}

	if doc.pdf.PageNo() == 1 {
		doc.salto()
	}

	if len(d.Cobradores) > 0 {
		doc.seccion(n, "Cobradores", 24)
		n++
		filas := [][]string{}
		for _, c := range d.Cobradores {
			diferencia := "Sin liquidar"
			if c.TieneLiquidado {
				diferencia = EtiquetaDiferencia(c.Diferencia, d.Config)
			}
			filas = append(filas, []string{
				c.Nombre,
				FormatearMonto(c.CobradoCentavos, d.Config),
				FormatearMonto(c.CarteraCentavos, d.Config),
				FormatearMonto(c.VencidoCentavos, d.Config),
				formatearPorcentaje(PorcentajeDe(c.VencidoCentavos, c.CarteraCentavos)),
				diferencia,
			})
		}
		doc.tabla([]colPDF{
			{"Cobrador", 50, false, "nombre"},
			{"Cobrado", 27, true, "num"},
			{"Cartera", 30, true, "num"},
			{"Vencido", 28, true, "num"},
			{"% mora", 20, true, "num"},
			{"Liquidación", 30.9, true, "texto"},
		}, filas, nil)
		doc.nota("Cobrado: dinero que recibió en el mes. Cartera: la que tiene asignada hoy.")
	}

	doc.seccion(n, "Movimiento del mes", 20)
	n++
	doc.franja([][2]string{
		{"CUENTAS EMITIDAS", strconv.Itoa(d.Venta.Cuentas)},
		{"MONTO EMITIDO", FormatearMonto(d.Venta.MontoCentavos, d.Config)},
		{"CLIENTES NUEVOS", strconv.Itoa(d.Venta.ClientesNuevos)},
		{"SALDO SIN COBRAR", FormatearMonto(d.TotalSalidas, d.Config)},
	}, "")
	doc.nota("Liquidaciones cerradas: " + strconv.Itoa(d.Liquidaciones) + ".")

	if len(d.Salidas) > 0 {
		doc.seccion(n, "Notas de crédito y condonaciones", 24)
		n++
		filas := [][]string{}
		for _, s := range d.Salidas {
			filas = append(filas, []string{
				s.Fecha.String(), EtiquetaTipoMovimiento(s.Tipo), s.ClienteNombre,
				FormatearMonto(s.MontoCentavos, d.Config), s.Autorizo,
			})
		}
		doc.tabla([]colPDF{
			{"Fecha", 28, false, "num"},
			{"Tipo", 42, false, "texto"},
			{"Cliente", 65, false, "nombre"},
			{"Monto", 24, true, "num"},
			{"Autorizó", 26.9, false, "nombre"},
		}, filas, nil)
	}

	if len(d.Deudores) > 0 {
		doc.seccion(n, "Clientes que más deben", 24)
		filas := [][]string{}
		for _, x := range d.Deudores {
			filas = append(filas, []string{
				x.ClienteNombre, strconv.Itoa(x.CuentasAbiertas),
				FormatearMonto(x.SaldoVencidoCentavos, d.Config),
				FormatearMonto(x.SaldoCentavos, d.Config),
				formatearPorcentaje(PorcentajeDe(x.SaldoCentavos, d.SaldoTotal)),
			})
		}
		doc.tabla([]colPDF{
			{"Cliente", 78, false, "nombre"},
			{"Cuentas", 26, true, "num"},
			{"Vencido", 28, true, "num"},
			{"Saldo", 30, true, "num"},
			{"% cartera", 23.9, true, "num"},
		}, filas, nil)
	}

	return doc.bytes()
}

// ─────────────────────────────────────────────────────────────────────────────

type DatosComprobantes struct {
	Empresa  Empresa
	Marca    Marca
	Config   ConfigEmpresa
	Hoy      Fecha
	Contexto string // "Cuenta FAC-000218 · Tienda La Bendición"
	Adjuntos []Adjunto
}

// ComprobantesPDF arma un PDF con las fotos adjuntas, una por página, cada una con su título,
// su descripción y quién la subió. Los adjuntos que YA son PDF no se pueden incrustar con esta
// librería: se listan al final para que quien reciba el documento sepa que existen y los pida.
func ComprobantesPDF(d DatosComprobantes) ([]byte, error) {
	doc, err := nuevoDocumento(d.Empresa, d.Marca, d.Config)
	if err != nil {
		return nil, err
	}
	id := "CMP-" + d.Hoy.String()
	if len(d.Hoy.String()) >= 7 {
		id = "CMP-" + d.Hoy.String()[:7]
	}
	doc.idDoc = id
	doc.contTit = "COMPROBANTES"
	doc.contSub = "continuación"
	doc.pdf.AddPage()
	doc.tituloBloque("COMPROBANTES", "Fotos de respaldo de un registro", "N.º DE EXPEDIENTE", id)

	cliente, cuenta := d.Contexto, ""
	if i := strings.Index(d.Contexto, " · "); i >= 0 {
		cuenta = strings.TrimSpace(d.Contexto[:i])
		cliente = strings.TrimSpace(d.Contexto[i+len(" · "):])
	}
	doc.filaMeta([][3]string{
		{"CLIENTE", cliente, "15"},
		{"CUENTA", cuenta, "81"},
		{"EMITIDO", d.Hoy.String(), "119"},
		{"COMPROBANTES", strconv.Itoa(len(d.Adjuntos)), "157"},
	})

	if len(d.Adjuntos) == 0 {
		doc.seccion(1, "Contenido", 10)
		doc.nota("Este documento todavía no tiene comprobantes adjuntos.")
		return doc.bytes()
	}

	doc.seccion(1, "Contenido", 24)
	var otros int
	filas := [][]string{}
	for i, a := range d.Adjuntos {
		if !a.EsImagen() {
			otros++
		}
		filas = append(filas, []string{
			fmt.Sprintf("%02d", i+1),
			a.ComoSeLlama(),
			a.Descripcion,
			a.SubidoPorNombre,
			a.CreadoEn.Format("2006-01-02, 15:04"),
		})
	}
	doc.tabla([]colPDF{
		{"", 12, false, "indice"},
		{"Comprobante", 58, false, "nombre"},
		{"Tema", 50, false, "nombre"},
		{"Subido por", 32, false, "nombre"},
		{"Fecha y hora", 33.9, true, "num"},
	}, filas, nil)
	if otros > 0 {
		doc.nota(fmt.Sprintf("%d comprobante(s) ya venían en PDF y se entregan como archivo aparte: esta librería no puede incrustarlos.", otros))
	}

	fotos := doc.fotosRegistradas(d.Adjuntos)
	if len(fotos) > 0 {
		doc.seccion(2, "Vista previa", 56)
		colW := (pdfCW - 12) / 3
		yFila := doc.y
		for i, f := range fotos {
			col := i % 3
			if col == 0 && i > 0 {
				yFila += 43.5 + 12
			}
			if col == 0 && yFila+52 > pdfTope {
				doc.salto()
				yFila = doc.y
			}
			x := pdfL + float64(col)*(colW+6)
			escala := math.Min(colW/f.pxAncho, 43.5/f.pxAlto)
			w, h := f.pxAncho*escala, f.pxAlto*escala
			doc.imagenAislada(f.nombre, f.tipo, x+(colW-w)/2, yFila+(43.5-h)/2, w, h)
			doc.trazo(189, 189, 203, 0.25)
			doc.pdf.Rect(x, yFila, colW, 43.5, "D")
			doc.txt(x, yFila+43.5+4.2, fmt.Sprintf("%02d /", i+1), "MONOM", 7, 75, 60, 240)
			doc.txt(x+12, yFila+43.5+4.2, doc.cortar(f.adjunto.ComoSeLlama(), "ISB", 8, colW-12), "ISB", 8, 11, 11, 20)
		}
		doc.y = yFila + 43.5 + 8
		for i, f := range fotos {
			doc.contSub = fmt.Sprintf("Comprobante %d de %d", i+1, len(fotos))
			if d.Contexto != "" {
				doc.contSub += " · " + doc.cortar(d.Contexto, "MONO", 7, 110)
			}
			doc.salto()
			doc.paginaFoto(f, i+1, len(fotos))
		}
	}
	return doc.bytes()
}
