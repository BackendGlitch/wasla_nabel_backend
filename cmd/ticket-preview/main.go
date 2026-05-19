// Command ticket-preview prints the booking ticket layout (markers + ~thermal ASCII) without a printer.
//
// Usage:
//
//	cd wasla_backend && go run ./cmd/ticket-preview
//	go run ./cmd/ticket-preview -out booking-ticket-preview.txt
//	go run ./cmd/ticket-preview -html preview.html
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"station-backend/internal/printer"
)

const paperCols = 32

func main() {
	textOut := flag.String("out", "", "write a UTF-8 .txt file you can open in any editor (thermal-style + raw markers)")
	quiet := flag.Bool("quiet", false, "with -out or -html: do not print the preview to stdout")
	htmlOut := flag.String("html", "", "optional path to write a simple HTML receipt preview")
	flag.Parse()

	when, _ := time.Parse(time.RFC3339, "2026-05-08T14:30:00+01:00")
	data := &printer.TicketData{
		LicensePlate:    "87 TUN 4521",
		DestinationName: "Sousse — Bab Bhar",
		SeatNumber:      3,
		TotalAmount:     8.5,
		BasePrice:       7.0,
		CreatedBy:       "Samia Trabelsi",
		CreatedByName:   "Samia Trabelsi",
		CreatedAt:       when,
		StationName:     "Station Centre",
		CompanyName:     "Wasla Demo Co",
		FirstTripOfDay:  true,
	}

	raw := printer.RenderFrenchBookingTicket(data)
	ascii := humanThermalPreview(raw, paperCols)

	if !*quiet {
		fmt.Println("=== Raw layout (same markers the printer service expands to ESC/POS) ===")
		fmt.Println(strings.TrimRight(raw, "\n"))
		fmt.Println()
		fmt.Println("=== Approx. 32-column thermal preview (ASCII; not pixel-perfect) ===")
		fmt.Println(ascii)
	}

	if *textOut != "" {
		text := buildTextReport(data, ascii, raw)
		if err := os.WriteFile(*textOut, []byte(text), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write -out: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Wrote text preview: %s (open in any text editor)\n", *textOut)
	}

	if *htmlOut != "" {
		if err := writeHTMLPreview(*htmlOut, data, ascii); err != nil {
			fmt.Fprintf(os.Stderr, "html: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Wrote HTML preview: %s (open in a browser)\n", *htmlOut)
	}
}

func buildTextReport(data *printer.TicketData, ascii, raw string) string {
	var b strings.Builder
	b.WriteString("Wasla — booking ticket preview (sample data)\n")
	b.WriteString("Open this file in a text editor with a monospace font for best results.\n")
	b.WriteString(strings.Repeat("=", 48))
	b.WriteString("\n\n")
	b.WriteString("Sample fields: destination, plate, seat, amount, agent, createdAt.\n")
	b.WriteString(fmt.Sprintf("  Destination: %s\n", data.DestinationName))
	b.WriteString(fmt.Sprintf("  Plate:       %s\n", data.LicensePlate))
	b.WriteString(fmt.Sprintf("  Seat:        %d\n", data.SeatNumber))
	b.WriteString(fmt.Sprintf("  Amount:      %.3f TND\n", data.TotalAmount))
	b.WriteString(fmt.Sprintf("  Agent:       %s\n", strings.TrimSpace(data.CreatedBy)))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", 48))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("~%d-column thermal-style preview\n", paperCols))
	b.WriteString(strings.Repeat("-", 48))
	b.WriteString("\n")
	b.WriteString(ascii)
	b.WriteString("\n\n")
	b.WriteString(strings.Repeat("=", 48))
	b.WriteString("\n")
	b.WriteString("Raw template (markers — what printer-service turns into ESC/POS)\n")
	b.WriteString(strings.Repeat("=", 48))
	b.WriteString("\n")
	b.WriteString(strings.TrimRight(raw, "\n"))
	b.WriteString("\n")
	return b.String()
}

func centerRunes(s string, width int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) >= width {
		if len(r) > width {
			return string(r[:width])
		}
		return s
	}
	pad := width - len(r)
	left := pad / 2
	right := pad - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func dashLine(width int) string {
	if width < 4 {
		width = 32
	}
	return strings.Repeat("-", width)
}

// humanThermalPreview maps FR/* markers to plain lines for a quick terminal glance.
func humanThermalPreview(content string, w int) string {
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		switch {
		case line == "{{FR_SEP}}":
			b.WriteString(dashLine(w))
			b.WriteByte('\n')
		case strings.HasPrefix(line, "{{FR_CENTER_TITLE:") && strings.HasSuffix(line, "}}"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_CENTER_TITLE:"), "}}")
			b.WriteString(centerRunes(raw, w))
			b.WriteByte('\n')
		case strings.HasPrefix(line, "{{FR_TITLE_BIG:") && strings.HasSuffix(line, "}}"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_TITLE_BIG:"), "}}")
			b.WriteString(centerRunes(raw, w))
			b.WriteByte('\n')
		case strings.HasPrefix(line, "{{TALON_LP_BIG:") && strings.HasSuffix(line, "}}"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{TALON_LP_BIG:"), "}}")
			b.WriteString(centerRunes(strings.TrimSpace(raw), w))
			b.WriteByte('\n')
		case strings.HasPrefix(line, "{{FR_BOLD_LINE:") && strings.HasSuffix(line, "}}"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{FR_BOLD_LINE:"), "}}")
			rs := []rune(strings.TrimSpace(raw))
			if len(rs) > w {
				b.WriteString(string(rs[:w]))
			} else {
				b.WriteString(string(rs))
			}
			b.WriteByte('\n')
		case strings.HasPrefix(line, "{{CENTER_SMALL:") && strings.HasSuffix(line, "}}"):
			raw := strings.TrimSuffix(strings.TrimPrefix(line, "{{CENTER_SMALL:"), "}}")
			// Indent to suggest "small" secondary line
			small := "  " + strings.TrimSpace(raw)
			rs := []rune(small)
			if len(rs) > w {
				b.WriteString(string(rs[:w]))
			} else {
				b.WriteString(string(rs))
			}
			b.WriteByte('\n')
		case line == "{{FEED_BEFORE_CUT}}":
			b.WriteString(centerRunes("(paper feed)", w))
			b.WriteByte('\n')
		default:
			if strings.HasPrefix(line, "{{") {
				b.WriteString(centerRunes(strings.Trim(line, "{}"), w))
				b.WriteByte('\n')
				continue
			}
			rs := []rune(line)
			if len(rs) > w {
				b.WriteString(string(rs[:w]))
			} else {
				b.WriteString(line)
			}
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeHTMLPreview(path string, data *printer.TicketData, ascii string) error {
	var body strings.Builder
	body.WriteString(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>Wasla booking ticket preview</title>`)
	body.WriteString(`<style>
body { font-family: system-ui, sans-serif; background:#e8e8e8; padding:24px; }
.receipt { font-family: ui-monospace, "Cascadia Code", Consolas, monospace; white-space: pre; width: 32ch; margin: 0 auto; background: #fffef8; border: 1px solid #333; padding: 12px 10px 20px; box-shadow: 4px 4px 0 #bbb; font-size: 13px; line-height: 1.35; }
.meta { max-width: 42rem; margin: 16px auto; font-size: 14px; color: #333; }
</style></head><body>`)
	body.WriteString(`<div class="meta"><strong>Booking ticket preview</strong> — sample data; deploy <code>printer-service</code> for real ESC/POS output.</div>`)
	body.WriteString(`<div class="receipt">`)
	for _, line := range strings.Split(ascii, "\n") {
		body.WriteString(htmlEscape(line))
		body.WriteString("\n")
	}
	body.WriteString(`</div><div class="meta">`)
	body.WriteString(fmt.Sprintf("Plate: %s · Dest: %s · Seat: %d · Amount: %.3f TND<br>",
		htmlEscape(data.LicensePlate), htmlEscape(data.DestinationName), data.SeatNumber, data.TotalAmount))
	body.WriteString(`</div></body></html>`)
	return os.WriteFile(path, []byte(body.String()), 0o644)
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
