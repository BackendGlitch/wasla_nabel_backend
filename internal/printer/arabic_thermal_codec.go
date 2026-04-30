package printer

import (
	"strings"
	"unicode"

	goarabic "github.com/01walid/goarabic"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/bidi"
	"golang.org/x/text/unicode/norm"
)

// Epson-style Arabic code page (table 46 on many clones maps to Cyrillic / "Russian").
const arabicThermalEscTable = byte(37)

func westernThermalEscTable() byte { return 0 }

func shapeArabic(s string) string {
	s = goarabic.RemoveTashkeel(goarabic.RemoveTatweel(s))
	s = goarabic.ToGlyph(s)
	return norm.NFKC.String(s)
}

func containsArabicRunes(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return true
		}
	}
	return false
}

// visualOrderLine maps logical UTF-8 to the glyph order ESC/POS needs.
//
// Thermal heads print sequential code points left→right across the paper. For Arabic,
// bidi Ordering runs are emitted in visual outer order, but RTL runs still expose
// substrings in *logical* order (Run.String) — reversing those RTL runs aligns the
// stream with RTL reading (start on the physical right side of the line).
//
// DefaultDirection(RightToLeft) biases the paragraph so weak numerics/punctuation stay
// attached to Arabic the way operators expect on louage slips.
func visualOrderLine(s string) string {
	shaped := shapeArabic(s)

	var p bidi.Paragraph
	var err error
	if containsArabicRunes(shaped) {
		_, err = p.SetString(shaped, bidi.DefaultDirection(bidi.RightToLeft))
	} else {
		_, err = p.SetString(shaped)
	}
	if err != nil {
		return shaped
	}
	order, err := p.Order()
	if err != nil {
		return shaped
	}
	var b strings.Builder
	for i := 0; i < order.NumRuns(); i++ {
		r := order.Run(i)
		txt := (&r).String()
		switch r.Direction() {
		case bidi.RightToLeft:
			txt = string(bidi.AppendReverse(nil, []byte(txt)))
		default:
			// LeftToRight runs (e.g. embedded Latin digits) stay in logical order for LTR ink.
		}
		b.WriteString(txt)
	}
	return b.String()
}

// encodeArabicThermalUTF8 reshapes RTL text and encodes to ISO 8859-6 for ESC/POS.
func encodeArabicThermalUTF8(utf8Line string) []byte {
	vis := visualOrderLine(strings.TrimRight(utf8Line, "\n\r"))
	out, _, err := transform.Bytes(charmap.ISO8859_6.NewEncoder(), []byte(vis))
	if err != nil || len(out) == 0 {
		out2, _, _ := transform.String(charmap.ISO8859_6.NewEncoder(), vis)
		if out2 != "" {
			return []byte(out2)
		}
		return []byte("?")
	}
	return out
}

// encodeWesternASCII emits printable ASCII after switching to western code page.
func encodeWesternASCII(s string) []byte {
	s = strings.TrimSpace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r <= 126 {
			b.WriteByte(byte(r))
		} else if r == '\n' || r == '\r' {
			continue
		} else {
			b.WriteByte('?')
		}
	}
	return []byte(b.String())
}
