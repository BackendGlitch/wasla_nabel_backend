package printer

import (
	"bytes"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ESC/POS marker lines for Arabic thermal output (parsed by convertToESCPOS).

func escAlignArab(buf *bytes.Buffer, mode byte) {
	buf.WriteByte(0x1B)
	buf.WriteByte(0x61)
	buf.WriteByte(mode)
}

func escArabicPage(buf *bytes.Buffer) {
	buf.WriteByte(0x1B)
	buf.WriteByte(0x74)
	buf.WriteByte(arabicThermalEscTable)
}

func escWesternPage(buf *bytes.Buffer) {
	buf.WriteByte(0x1B)
	buf.WriteByte(0x74)
	buf.WriteByte(westernThermalEscTable())
}

func escTextStyle(buf *bytes.Buffer, mode byte) {
	buf.WriteByte(0x1B)
	buf.WriteByte(0x21)
	buf.WriteByte(mode)
}

func escTextScale(buf *bytes.Buffer, mode byte) {
	buf.WriteByte(0x1D)
	buf.WriteByte(0x21)
	buf.WriteByte(mode)
}

func resetEscStyle(buf *bytes.Buffer) {
	escAlignArab(buf, 0x00)
	escTextStyle(buf, 0x00)
	escTextScale(buf, 0x00)
}

// tryConsumeArabEscPosDirective returns true if line was consumed (marker handled).
func tryConsumeArabEscPosDirective(buf *bytes.Buffer, line string, paperWidth int, isCompactTalon bool) bool {
	line = strings.TrimRight(line, "\r")
	recompact := func() {
		if !isCompactTalon {
			return
		}
		buf.Write([]byte{0x1B, 0x4D, 0x01}) // Font B
		buf.Write([]byte{0x1B, 0x33, compactTalonLineSpacingDots}) // match French compact talon density
	}

	switch {
	case line == "{{AR_INIT}}":
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case line == "{{AR_SEP}}":
		resetEscStyle(buf)
		escWesternPage(buf)
		w := clampSepWidth(paperWidth)
		buf.WriteString(strings.Repeat("-", w) + "\n")
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_TITLE:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_TITLE:"), "}}")
		resetEscStyle(buf)
		escArabicPage(buf)
		escAlignArab(buf, 0x01)
		escTextStyle(buf, 0x08)
		escTextScale(buf, 0x01)
		buf.Write(encodeArabicThermalUTF8(raw))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_SUBTITLE:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_SUBTITLE:"), "}}")
		resetEscStyle(buf)
		escArabicPage(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x01})
		escAlignArab(buf, 0x01)
		escTextStyle(buf, 0x08)
		escTextScale(buf, 0x00)
		buf.Write(encodeArabicThermalUTF8(raw))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x00})
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_LINE:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_LINE:"), "}}")
		resetEscStyle(buf)
		if arabLbl, latinVal, ok := splitArabLabelLatinValueTail(raw); ok && isLatinDominantField(latinVal) {
			// Value on physical left (emitted first), Arabic label (+ colon) toward the right edge.
			escArabicPage(buf)
			escAlignArab(buf, 0x00)
			escWesternPage(buf)
			buf.Write(encodeWesternASCII(latinVal))
			escArabicPage(buf)
			arStem := strings.TrimSpace(arabLbl)
			arStem = strings.TrimSuffix(arStem, ":")
			arStem = strings.TrimSpace(arStem)
			buf.Write(encodeArabicThermalUTF8(arStem + " :"))
			buf.WriteByte('\n')
		} else {
			escArabicPage(buf)
			escAlignArab(buf, 0x02)
			buf.Write(encodeArabicThermalUTF8(raw))
			buf.WriteByte('\n')
		}
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_CENTER_BODY:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_CENTER_BODY:"), "}}")
		resetEscStyle(buf)
		escArabicPage(buf)
		escAlignArab(buf, 0x01)
		buf.Write(encodeArabicThermalUTF8(raw))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{W_PLATE_LARGE:") && strings.HasSuffix(line, "}}"):
		plate := strings.TrimSuffix(strings.TrimPrefix(line, "{{W_PLATE_LARGE:"), "}}")
		resetEscStyle(buf)
		escWesternPage(buf)
		escAlignArab(buf, 0x01)
		escTextStyle(buf, 0x08)
		escTextScale(buf, 0x11)
		buf.Write(encodeWesternASCII(plate))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{W_BIG_CENTER:") && strings.HasSuffix(line, "}}"):
		txt := strings.TrimSuffix(strings.TrimPrefix(line, "{{W_BIG_CENTER:"), "}}")
		resetEscStyle(buf)
		escWesternPage(buf)
		escAlignArab(buf, 0x01)
		escTextStyle(buf, 0x08)
		escTextScale(buf, 0x22)
		buf.Write(encodeWesternASCII(txt))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{W_SMALL_CENTER:") && strings.HasSuffix(line, "}}"):
		txt := strings.TrimSuffix(strings.TrimPrefix(line, "{{W_SMALL_CENTER:"), "}}")
		resetEscStyle(buf)
		escWesternPage(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x01})
		escAlignArab(buf, 0x01)
		buf.Write(encodeWesternASCII(txt))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x00})
		escArabicPage(buf)
		recompact()
		return true

	case line == "{{STAR_TR}}":
		resetEscStyle(buf)
		escWesternPage(buf)
		escAlignArab(buf, 0x00)
		buf.WriteString(talonTopRightStar(paperWidth))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_TWOCOL:") && strings.HasSuffix(line, "}}"):
		inner := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_TWOCOL:"), "}}")
		parts := strings.Split(inner, "|")
		if len(parts) == 4 {
			resetEscStyle(buf)
			thermalArabLatinTwoColumns(buf, paperWidth,
				strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]),
				strings.TrimSpace(parts[2]), strings.TrimSpace(parts[3]),
			)
			escArabicPage(buf)
			recompact()
		}
		return true

	case strings.HasPrefix(line, "{{AR_SEAT_NBR:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_SEAT_NBR:"), "}}")
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n <= 0 {
			return true
		}
		resetEscStyle(buf)
		escArabicPage(buf)
		escAlignArab(buf, 0x00)
		escWesternPage(buf)
		buf.Write(encodeWesternASCII("(" + strconv.Itoa(n) + ") "))
		escArabicPage(buf)
		buf.Write(encodeArabicThermalUTF8("المقعد عدد"))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_DRIVER_HDR_NBR:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_DRIVER_HDR_NBR:"), "}}")
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n <= 0 {
			return true
		}
		resetEscStyle(buf)
		escArabicPage(buf)
		escAlignArab(buf, 0x00)
		escWesternPage(buf)
		buf.Write(encodeWesternASCII("(" + strconv.Itoa(n) + ") "))
		escArabicPage(buf)
		buf.Write(encodeArabicThermalUTF8("تذكرة السائق : المقعد عدد"))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_DIR_MIX_CENTER:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_DIR_MIX_CENTER:"), "}}")
		parts := strings.SplitN(raw, "|", 2)
		if len(parts) < 2 {
			return true
		}
		lbl := strings.TrimSpace(parts[0])
		west := strings.TrimSpace(parts[1])
		resetEscStyle(buf)
		escArabicPage(buf)
		escAlignArab(buf, 0x01)
		escWesternPage(buf)
		buf.Write(encodeWesternASCII(west))
		escArabicPage(buf)
		buf.Write(encodeArabicThermalUTF8(lbl))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{AR_NOTE_SMALL:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{AR_NOTE_SMALL:"), "}}")
		resetEscStyle(buf)
		escArabicPage(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x01})
		escAlignArab(buf, 0x01)
		buf.Write(encodeArabicThermalUTF8(raw))
		buf.WriteByte('\n')
		resetEscStyle(buf)
		buf.Write([]byte{0x1B, 0x4D, 0x00})
		escArabicPage(buf)
		recompact()
		return true

	case strings.HasPrefix(line, "{{W_EXIT_COUNT:") && strings.HasSuffix(line, "}}"):
		raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{W_EXIT_COUNT:"), "}}")
		resetEscStyle(buf)
		escWesternPage(buf)
		escAlignArab(buf, 0x02)
		buf.WriteByte('(')
		buf.WriteString(raw)
		buf.WriteString(")\n")
		resetEscStyle(buf)
		escArabicPage(buf)
		recompact()
		return true

	default:
		return false
	}
}

func clampSepWidth(w int) int {
	if w <= 0 {
		w = 32
	}
	if w > 48 {
		return 48
	}
	return w
}

func cellThermalWidthArabLatin(arabLbl, latin string) int {
	arabLbl = strings.TrimSpace(arabLbl)
	latin = strings.TrimSpace(latin)
	return len(encodeArabicThermalUTF8(arabLbl)) + utf8.RuneCountInString(latin)
}

// thermalArabLatinTwoColumns lays out two side-by-side fields on one line.
// For each field the Latin/value is emitted first (physical left), then the Arabic label
// (toward physical right). Two fields are concatenated left→col1→gap→col2→right edge.
func thermalArabLatinTwoColumns(buf *bytes.Buffer, paperWidth int, ll, lv, rl, rv string) {
	ll, lv = strings.TrimSpace(ll), strings.TrimSpace(lv)
	rl, rv = strings.TrimSpace(rl), strings.TrimSpace(rv)
	width := clampSepWidth(paperWidth)

	shrinkLatin := func(s string, limit int) string {
		s = strings.TrimSpace(s)
		if utf8.RuneCountInString(s) <= limit {
			return s
		}
		r := []rune(s)
		if limit <= 1 {
			return string(r[:1])
		}
		return string(r[:limit-1]) + "."
	}

	cellL := cellThermalWidthArabLatin(ll, lv)
	cellR := cellThermalWidthArabLatin(rl, rv)
	spaces := width - cellL - cellR
	iter := 0
	for spaces < 1 && iter < 64 {
		iter++
		prevL, prevR := lv, rv
		if cellR > cellL {
			rn := utf8.RuneCountInString(rv)
			if rn <= 4 {
				break
			}
			rv = shrinkLatin(rv, rn-1)
		} else {
			ln := utf8.RuneCountInString(lv)
			if ln <= 4 {
				break
			}
			lv = shrinkLatin(lv, ln-1)
		}
		if lv == prevL && rv == prevR {
			break
		}
		cellL = cellThermalWidthArabLatin(ll, lv)
		cellR = cellThermalWidthArabLatin(rl, rv)
		spaces = width - cellL - cellR
	}
	if spaces < 1 {
		spaces = 1
	}

	escAlignArab(buf, 0x00)
	escTextStyle(buf, 0x00)
	escTextScale(buf, 0x00)
	escWesternPage(buf)
	buf.Write(encodeWesternASCII(lv))
	escArabicPage(buf)
	buf.Write(encodeArabicThermalUTF8(ll))
	buf.Write(bytes.Repeat([]byte{' '}, spaces))
	escWesternPage(buf)
	buf.Write(encodeWesternASCII(rv))
	escArabicPage(buf)
	buf.Write(encodeArabicThermalUTF8(rl))
	buf.WriteByte('\n')
}

// splitArabLabelLatinValueTail splits "…ArabicLabel : latin tail" using the last ASCII ':'.
func splitArabLabelLatinValueTail(raw string) (arabLabel, latin string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	idx := strings.LastIndex(raw, ":")
	if idx < 1 || idx >= len(raw)-1 {
		return "", "", false
	}
	arabLabel = strings.TrimSpace(raw[:idx])
	latin = strings.TrimSpace(raw[idx+1:])
	if arabLabel == "" || latin == "" {
		return "", "", false
	}
	hasArabicLabel := false
	for _, r := range arabLabel {
		if unicode.Is(unicode.Arabic, r) {
			hasArabicLabel = true
			break
		}
	}
	if !hasArabicLabel {
		return "", "", false
	}
	return arabLabel, latin, true
}

// isLatinDominantField is true when s has no Arabic script characters (digits / Latin destinations / plates).
func isLatinDominantField(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if unicode.Is(unicode.Arabic, r) {
			return false
		}
	}
	return true
}

