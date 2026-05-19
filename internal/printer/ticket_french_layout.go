package printer

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"station-backend/internal/pricing"
)

// Compact French thermal layouts (Latin / code page 0). Markers: FR_SEP, FR_CENTER_TITLE, existing TALON_*, CENTER_SMALL, etc.

func frenchTwoCol(left, right string) string {
	return "{{TALON_BOTTOM_ROW:" + strings.TrimSpace(left) + "|" + strings.TrimSpace(right) + "}}\n"
}

func frenchOptionalAgentLine(data *TicketData) string {
	a := strings.TrimSpace(agentLineForTicket(data))
	if a == "" || strings.EqualFold(a, "Agent") {
		return ""
	}
	return "{{CENTER_SMALL:Agent: " + a + "}}\n"
}

func frenchAgentLabelForTalon(data *TicketData) string {
	a := strings.TrimSpace(agentLineForTicket(data))
	if a == "" || strings.EqualFold(a, "Agent") {
		return "{{CENTER_SMALL:Agent: -}}\n"
	}
	return "{{CENTER_SMALL:Agent: " + a + "}}\n"
}

func frenchDestLabelForTalon(data *TicketData) string {
	d := strings.TrimSpace(data.DestinationName)
	if d == "" {
		return "{{CENTER_SMALL:Dest: -}}\n"
	}
	return "{{CENTER_SMALL:Dest: " + d + "}}\n"
}

// Compact driver talon: tight vertical rhythm; first trip puts * on same row as HEURE (saves a band vs star-then-time).
func appendFrenchDriverTalonCompact(sb *strings.Builder, data *TicketData, when time.Time, firstTripOfDay bool) {
	plate := strings.ToUpper(strings.TrimSpace(data.LicensePlate))
	hm := tunisFmtHM(when)
	// Always use TALON_BOTTOM_ROW (same as compact Font B row) — avoids CENTER_SMALL path and extra vertical slack.
	if firstTripOfDay {
		sb.WriteString("{{TALON_BOTTOM_ROW:HEURE " + hm + "|*}}\n")
	} else {
		sb.WriteString("{{TALON_BOTTOM_ROW:HEURE " + hm + "|}}\n")
	}
	sb.WriteString(fmt.Sprintf("{{FR_SEAT_FOCUS:%d}}\n", data.SeatNumber))
	sb.WriteString("{{FR_VEH_MEDIUM:" + plate + "}}\n")
	sb.WriteString(frenchDestLabelForTalon(data))
	sb.WriteString(frenchAgentLabelForTalon(data))
}

// RenderFrenchBookingTicket: single thermal slip — no separate driver talon / second tear-off.
// Visibility order: destination (title), license plate, seat, price, agent, date & time.
func RenderFrenchBookingTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	fare := bookingDisplayFareTD(data)
	plate := strings.ToUpper(strings.TrimSpace(data.LicensePlate))
	dest := strings.TrimSpace(data.DestinationName)
	if dest == "" {
		dest = "-"
	}

	co := strings.TrimSpace(companyNameForTicket(data))
	st := strings.TrimSpace(data.StationName)
	if co != "" {
		sb.WriteString("{{CENTER_SMALL:" + co + "}}\n")
	}
	if st != "" && !strings.EqualFold(st, co) {
		sb.WriteString("{{CENTER_SMALL:" + st + "}}\n")
	}

	// 1) Destination — primary (double-width for emphasis)
	sb.WriteString("{{FR_TITLE_BIG:" + dest + "}}\n")
	sb.WriteString("{{FR_SEP}}\n")

	// 2) License plate — large
	sb.WriteString("{{TALON_LP_BIG:" + plate + "}}\n")

	// 3) Seat (per seat bookings)
	if data.SeatNumber > 0 {
		sb.WriteString(fmt.Sprintf("{{FR_BOLD_LINE:SIEGE %d}}\n", data.SeatNumber))
	}

	sb.WriteString("{{FR_SEP}}\n")

	// 4) Price
	sb.WriteString("{{FR_BOLD_LINE:Tarif: " + fare + " TND}}\n")

	// 5) Agent
	if a := strings.TrimSpace(agentLineForTicket(data)); a != "" && !strings.EqualFold(a, "Agent") {
		sb.WriteString("{{FR_BOLD_LINE:Agent: " + a + "}}\n")
	} else {
		sb.WriteString("{{CENTER_SMALL:Agent: -}}\n")
	}

	// 6) Date & time
	sb.WriteString("{{FR_BOLD_LINE:" + tunisFmtDateSlash(when) + "  " + tunisFmtHM(when) + "}}\n")

	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Non remboursable}}\n")
	sb.WriteString("{{FEED_BEFORE_CUT}}\n")
	return sb.String()
}

func RenderFrenchTalonOnly(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}

	sb.WriteString("{{TALON_COMPACT_ON}}\n")
	appendFrenchDriverTalonCompact(&sb, data, when, data.FirstTripOfDay)
	sb.WriteString("{{TALON_COMPACT_OFF}}\n")
	sb.WriteString("{{TALON_END_FEED}}\n")
	return sb.String()
}

func RenderFrenchEntryTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{FR_CENTER_TITLE:CARTE D'ENTREE}}\n")
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{TALON_LP_BIG:" + strings.TrimSpace(data.LicensePlate) + "}}\n")
	if r := strings.TrimSpace(data.RouteName); r != "" {
		sb.WriteString(frenchTwoCol("LIGNE", r))
	}
	sb.WriteString(frenchTwoCol("DATE "+tunisFmtDateSlash(when), "HEURE "+tunisFmtHM(when)))
	sb.WriteString(frenchOptionalAgentLine(data))
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Merci}}\n")
	return sb.String()
}

func RenderFrenchExitTripTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{FR_CENTER_TITLE:CARTE DE SORTIE}}\n")
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{TALON_LP_BIG:" + strings.TrimSpace(data.LicensePlate) + "}}\n")
	if r := strings.TrimSpace(data.RouteName); r != "" {
		sb.WriteString(frenchTwoCol("LIGNE", r))
	}
	if st := strings.TrimSpace(data.StationName); st != "" {
		sb.WriteString(frenchTwoCol("STATION", st))
	}
	sb.WriteString(frenchTwoCol("DATE "+tunisFmtDateSlash(when), "HEURE "+tunisFmtHM(when)))
	sb.WriteString(frenchOptionalAgentLine(data))
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Merci}}\n")
	return sb.String()
}

func RenderFrenchDayPassTicket(data *TicketData) string {
	var sb strings.Builder
	purchAt := data.PurchaseDate
	if purchAt.IsZero() {
		purchAt = data.CreatedAt
	}
	if purchAt.IsZero() {
		purchAt = time.Now()
	}
	validFrom, validUntil := data.ValidFrom, data.ValidUntil
	if validFrom.IsZero() || validUntil.IsZero() {
		vf, vt := tunisCalendarDayBounds(purchAt)
		if validFrom.IsZero() {
			validFrom = vf
		}
		if validUntil.IsZero() {
			validUntil = vt
		}
	}

	sb.WriteString("{{FR_CENTER_TITLE:PASS JOURNEE}}\n")
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{TALON_LP_BIG:" + strings.TrimSpace(data.LicensePlate) + "}}\n")
	if r := strings.TrimSpace(data.RouteName); r != "" {
		sb.WriteString(frenchTwoCol("LIGNE", r))
	}
	sb.WriteString(frenchTwoCol("MONTANT", fmt.Sprintf("%.3f", pricing.DayPassTotalPriceTND)))
	sb.WriteString("{{CENTER_SMALL:Sans frais de service}}\n")
	sb.WriteString(frenchTwoCol("ACHAT", formatTicketReadableDate(purchAt)))
	sb.WriteString("{{CENTER_SMALL:Du " + tunisFmtDateSlash(validFrom) + " au " + tunisFmtDateSlash(validUntil) + "}}\n")
	sb.WriteString(frenchOptionalAgentLine(data))
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Merci}}\n")
	return sb.String()
}

func frenchExitPassLines(data *TicketData) []string {
	var rows []string
	if data.BasePrice > 0 && data.SeatNumber > 0 {
		gross := data.BasePrice * float64(data.SeatNumber)
		if data.VehicleCapacity > 0 && data.SeatNumber == data.VehicleCapacity {
			rows = append(rows,
				frenchTwoCol("CAPACITE", strconv.Itoa(data.VehicleCapacity)),
			)
		} else {
			rows = append(rows,
				frenchTwoCol("PLACES RESERVEES", strconv.Itoa(data.SeatNumber)),
			)
		}
		if data.FirstTripOfDay && gross > 0 {
			ded := pricing.EntryDayPassFeeTND
			net := gross - ded
			if net < 0 {
				net = 0
			}
			rows = append(rows,
				frenchTwoCol("TOTAL TARIF", fmt.Sprintf("%.3f", gross)),
				frenchTwoCol("PASSE ENTREE JOURNALIER", "-"+fmt.Sprintf("%.3f", ded)),
				frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", net)),
			)
		} else {
			rows = append(rows, frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", gross)))
		}
	} else {
		rows = append(rows, frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", data.TotalAmount)))
	}
	return rows
}

func RenderFrenchExitPassTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{FR_CENTER_TITLE:AUTORISATION DE SORTIE}}\n")
	if data.ExitPassCount > 0 {
		sb.WriteString("{{CENTER_SMALL:N° autorisation du jour: " + strconv.Itoa(data.ExitPassCount) + "}}\n")
	}
	sb.WriteString("{{FR_SEP}}\n")
	if data.FirstTripOfDay {
		sb.WriteString("{{TALON_TOP_RIGHT_STAR}}\n")
	}
	sb.WriteString("{{TALON_LP_BIG:" + strings.TrimSpace(data.LicensePlate) + "}}\n")
	if d := strings.TrimSpace(data.DestinationName); d != "" {
		sb.WriteString("{{CENTER_SMALL:DESTINATION: " + d + "}}\n")
	}
	for _, ln := range frenchExitPassLines(data) {
		sb.WriteString(ln)
	}
	sb.WriteString(frenchTwoCol("DATE "+tunisFmtDateSlash(when), "HEURE "+tunisFmtHM(when)))
	sb.WriteString(frenchOptionalAgentLine(data))
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{FR_CENTER_TITLE:SORTIE AUTORISEE}}\n")
	return sb.String()
}

func RenderFrenchStandardTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{FR_CENTER_TITLE:BILLET}}\n")
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{TALON_LP_BIG:" + strings.TrimSpace(data.LicensePlate) + "}}\n")
	if d := strings.TrimSpace(data.DestinationName); d != "" {
		sb.WriteString("{{CENTER_SMALL:DESTINATION: " + d + "}}\n")
	}
	sb.WriteString(frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", data.TotalAmount)))
	sb.WriteString(frenchTwoCol("DATE "+tunisFmtDateSlash(when), "HEURE "+tunisFmtHM(when)))
	sb.WriteString(frenchOptionalAgentLine(data))
	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Merci}}\n")
	return sb.String()
}
