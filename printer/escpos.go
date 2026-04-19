package printer

import (
	"fmt"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ESC/POS command bytes
var (
	cmdInit           = []byte{0x1B, 0x40}
	cmdAlignLeft      = []byte{0x1B, 0x61, 0x00}
	cmdAlignCenter    = []byte{0x1B, 0x61, 0x01}
	cmdAlignRight     = []byte{0x1B, 0x61, 0x02}
	cmdBoldOn         = []byte{0x1B, 0x45, 0x01}
	cmdBoldOff        = []byte{0x1B, 0x45, 0x00}
	cmdDoubleHeight   = []byte{0x1B, 0x21, 0x10}
	cmdDoubleWidth    = []byte{0x1B, 0x21, 0x20}
	cmdDoubleSize     = []byte{0x1B, 0x21, 0x30} // width + height
	cmdNormalSize     = []byte{0x1B, 0x21, 0x00}
	cmdUnderlineOn    = []byte{0x1B, 0x2D, 0x01}
	cmdUnderlineOff   = []byte{0x1B, 0x2D, 0x00}
	cmdCodePageLatin  = []byte{0x1B, 0x74, 0x10}
	cmdCharsetPC858   = []byte{0x1B, 0x74, 0x13}
	cmdLineSpacingDef = []byte{0x1B, 0x32}
	cmdCutFull        = []byte{0x1D, 0x56, 0x00}
	cmdCutPartial     = []byte{0x1D, 0x56, 0x01}
	cmdLF             = []byte{0x0A}
)

func feedLines(n int) []byte {
	return []byte{0x1B, 0x64, byte(n)}
}

// sanitizeText converts accented/special chars to ASCII-safe equivalents.
func sanitizeText(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 {
			b.WriteRune(r)
			continue
		}
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã': b.WriteByte('a')
		case 'é', 'è', 'ë', 'ê': b.WriteByte('e')
		case 'í', 'ì', 'ï', 'î': b.WriteByte('i')
		case 'ó', 'ò', 'ö', 'ô', 'õ': b.WriteByte('o')
		case 'ú', 'ù', 'ü', 'û': b.WriteByte('u')
		case 'Á', 'À', 'Ä', 'Â', 'Ã': b.WriteByte('A')
		case 'É', 'È', 'Ë', 'Ê': b.WriteByte('E')
		case 'Í', 'Ì', 'Ï', 'Î': b.WriteByte('I')
		case 'Ó', 'Ò', 'Ö', 'Ô', 'Õ': b.WriteByte('O')
		case 'Ú', 'Ù', 'Ü', 'Û': b.WriteByte('U')
		case 'ñ': b.WriteByte('n')
		case 'Ñ': b.WriteByte('N')
		case '¿': b.WriteByte('?')
		case '¡': b.WriteByte('!')
		case '\u2026': b.WriteString("...")
		case '\u2013', '\u2014': b.WriteByte('-')
		case '\u2018', '\u2019': b.WriteByte('\'')
		case '\u201C', '\u201D': b.WriteByte('"')
		default:
			if unicode.IsPrint(r) && utf8.RuneLen(r) == 1 {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

func dashedLine(width int) string {
	return strings.Repeat("-", width)
}

func padLine(left, right string, width int) string {
	l := sanitizeText(left)
	r := sanitizeText(right)
	gap := width - len(l) - len(r)
	if gap <= 0 {
		maxL := width - len(r) - 1
		if maxL < 0 {
			maxL = 0
		}
		if len(l) > maxL {
			l = l[:maxL]
		}
		return l + " " + r
	}
	return l + strings.Repeat(" ", gap) + r
}

func centerText(s string, width int) string {
	s = sanitizeText(s)
	if len(s) >= width {
		return s[:width]
	}
	pad := (width - len(s)) / 2
	return strings.Repeat(" ", pad) + s
}

// ReceiptItem represents one line item in a receipt.
type ReceiptItem struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price"`
}

// ReceiptData is the full data for building a receipt.
type ReceiptData struct {
	BusinessName  string        `json:"businessName"`
	Date          string        `json:"date"`
	TxID          string        `json:"txId"`
	Items         []ReceiptItem `json:"items"`
	Subtotal      float64       `json:"subtotal"`
	DiscountTotal float64       `json:"discountTotal"`
	Total         float64       `json:"total"`
	PaymentMethod string        `json:"paymentMethod"`
	AmountTended  float64       `json:"amountTendered"`
	Change        float64       `json:"change"`
	PaperWidth    int           `json:"paperWidth"` // 58 or 80 (mm); defaults to 80
}

func lineSpacingN(n int) []byte {
	return []byte{0x1B, 0x33, byte(n)}
}

func charsPerLine(mmWidth int) int {
	switch {
	case mmWidth <= 58:
		return 30
	case mmWidth <= 80:
		return 46
	default:
		// "carta" or wider
		return 80
	}
}

func formatCLP(v float64) string {
	rounded := math.Round(v)
	abs := math.Abs(rounded)
	s := fmt.Sprintf("%.0f", abs)
	// thousands separator
	n := len(s)
	var result []byte
	for i, c := range s {
		if i > 0 && (n-i)%3 == 0 {
			result = append(result, '.')
		}
		result = append(result, byte(c))
	}
	if rounded < 0 {
		return "-$" + string(result)
	}
	return "$" + string(result)
}

// BuildReceipt constructs raw ESC/POS bytes for a receipt.
func BuildReceipt(r ReceiptData) []byte {
	W := charsPerLine(r.PaperWidth)
	var buf []byte
	add := func(b ...[]byte) {
		for _, bb := range b {
			buf = append(buf, bb...)
		}
	}
	text := func(s string) {
		add([]byte(sanitizeText(s)), cmdLF)
	}
	line := func() { text(dashedLine(W)) }
	blank := func() { add(cmdLF) }

	add(cmdInit, cmdCodePageLatin, cmdLineSpacingDef)

	// Header — double size like TS version
	add(cmdAlignCenter, cmdBoldOn, cmdDoubleSize)
	name := r.BusinessName
	if name == "" {
		name = "TICKET DE VENTA"
	}
	text(name)
	add(cmdNormalSize, cmdBoldOff)
	blank()
	text("Detalle de venta")
	add(cmdAlignLeft)
	line()

	// Date / Tx info
	if r.Date != "" {
		text("Fecha: " + r.Date)
	}
	if r.TxID != "" {
		text("Tx: #" + r.TxID)
	}
	line()

	// Items header
	add(cmdBoldOn)
	text(padLine("PRODUCTO", "TOTAL", W))
	add(cmdBoldOff)
	line()

	// Items
	for _, item := range r.Items {
		total := item.Price * item.Quantity
		totalStr := formatCLP(total)
		unitStr := formatCLP(item.Price) + " c/u"

		reserved := len(totalStr)
		if len(unitStr) > reserved {
			reserved = len(unitStr)
		}
		reserved += 2
		maxName := W - reserved
		qtyPfx := fmtQty(item.Quantity)
		avail := maxName - len(qtyPfx)
		name := item.Name
		if len(name) > avail {
			if avail > 2 {
				name = name[:avail-2] + ".."
			} else {
				name = name[:avail]
			}
		}
		productLine := qtyPfx + name
		text(padLine(productLine, totalStr, W))
		text(padLine("", unitStr, W))
	}
	line()

	// Subtotal
	text(padLine("Subtotal", formatCLP(r.Subtotal), W))
	if r.DiscountTotal > 0 {
		text(padLine("Descuento", formatCLP(-r.DiscountTotal), W))
	}

	// Total
	blank()
	add(cmdBoldOn, cmdDoubleHeight)
	text(padLine("TOTAL", formatCLP(r.Total), W))
	add(cmdNormalSize, cmdBoldOff)
	line()

	// Payment
	pm := r.PaymentMethod
	if pm == "" {
		pm = "efectivo"
	}
	text(padLine("Pago ("+pm+")", formatCLP(r.AmountTended), W))
	text(padLine("Cambio", formatCLP(r.Change), W))
	line()

	// Footer
	blank()
	add(cmdAlignCenter)
	text("Gracias por su compra")
	blank()
	text("Conserve este ticket")
	text("para cambios y devoluciones")
	blank()
	blank()

	add(feedLines(4), cmdCutPartial)
	return buf
}

// BuildTestPrint returns a simple test page.
func BuildTestPrint(mmWidth int) []byte {
	W := charsPerLine(mmWidth)
	var buf []byte
	add := func(b ...[]byte) {
		for _, bb := range b {
			buf = append(buf, bb...)
		}
	}
	text := func(s string) {
		add([]byte(sanitizeText(s)), cmdLF)
	}
	line := func() { text(dashedLine(W)) }
	blank := func() { add(cmdLF) }

	add(cmdInit, cmdCodePageLatin, cmdLineSpacingDef)
	add(cmdAlignCenter, cmdBoldOn)
	text("ZENPOS PRINT BRIDGE")
	add(cmdBoldOff)
	blank()
	text("Impresora conectada correctamente")
	blank()
	add(cmdAlignLeft)
	line()
	text("Ancho papel: " + fmt.Sprintf("%dmm (%d chars)", mmWidth, W))
	text("Transporte: OK")
	line()
	text("Caracteres: a e i o u n")
	text("Numeros: 0 1 2 3 4 5 6 7 8 9")
	text("Simbolos: $ % & * + - / =")
	line()
	add(cmdAlignCenter)
	text("zenpos.cl")
	blank()
	blank()
	add(feedLines(4), cmdCutPartial)
	return buf
}

// fmtQty formats quantity: "2x " for integers, "1.5x " for fractional.
func fmtQty(q float64) string {
	if q == math.Trunc(q) {
		return fmt.Sprintf("%dx ", int(q))
	}
	return fmt.Sprintf("%.1fx ", q)
}
