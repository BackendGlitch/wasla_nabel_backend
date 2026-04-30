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

// Stub: labeled HEURE in the upper band (uses space under the * line), then SIEGE max, spaced vehicle, Dest, Agent.
func appendFrenchDriverTalonCompact(sb *strings.Builder, data *TicketData, when time.Time) {
	plate := strings.ToUpper(strings.TrimSpace(data.LicensePlate))
	// Tie the top strip to clarify time before the oversized seat row.
	sb.WriteString("{{CENTER_SMALL:HEURE " + tunisFmtHM(when) + "}}\n")
	sb.WriteString("{{FR_LF_ONLY:1}}\n")
	sb.WriteString(fmt.Sprintf("{{FR_SEAT_FOCUS:%d}}\n", data.SeatNumber))
	sb.WriteString("{{FR_LF_ONLY:2}}\n")
	sb.WriteString("{{FR_VEH_MEDIUM:" + plate + "}}\n")
	sb.WriteString(frenchDestLabelForTalon(data))
	sb.WriteString(frenchAgentLabelForTalon(data))
}

// RenderFrenchBookingTicket: dense passenger slip + shorter driver talon; French only, ESC/POS-friendly.
func RenderFrenchBookingTicket(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	fare := bookingDisplayFareTD(data)
	plate := strings.ToUpper(strings.TrimSpace(data.LicensePlate))

	co := strings.TrimSpace(companyNameForTicket(data))
	st := strings.TrimSpace(data.StationName)
	if co != "" {
		sb.WriteString("{{CENTER_SMALL:" + co + "}}\n")
	}
	if st != "" && !strings.EqualFold(st, co) {
		sb.WriteString("{{CENTER_SMALL:" + st + "}}\n")
	}
	sb.WriteString("{{CENTER_SMALL:BILLET PASSAGER}}\n")

	sb.WriteString(fmt.Sprintf("{{FR_SEAT_FOCUS:%d}}\n", data.SeatNumber))
	sb.WriteString("{{FR_VEH_MEDIUM:Veh: " + plate + "}}\n")

	sb.WriteString("{{CENTER_SMALL:Date: " + tunisFmtDateSlash(when) + "  " + tunisFmtHM(when) + "}}\n")

	if d := strings.TrimSpace(data.DestinationName); d == "" {
		sb.WriteString("{{CENTER_SMALL:Dest: -}}\n")
	} else {
		sb.WriteString("{{CENTER_SMALL:Dest: " + d + "}}\n")
	}
	sb.WriteString("{{FR_BOLD_LINE:Tarif: " + fare + "}}\n")

	sb.WriteString("{{FR_SEP}}\n")
	sb.WriteString("{{CENTER_SMALL:Non remboursable}}\n")
	sb.WriteString("{{PASSENGER_PRE_PARTIAL_FEED}}\n")
	sb.WriteString("{{PARTIAL_CUT}}\n")

	if data.FirstTripOfDay {
		sb.WriteString("{{TALON_TOP_RIGHT_STAR}}\n")
	}
	sb.WriteString("{{TALON_COMPACT_ON}}\n")
	appendFrenchDriverTalonCompact(&sb, data, when)
	sb.WriteString("{{TALON_COMPACT_OFF}}\n")
	return sb.String()
}

func RenderFrenchTalonOnly(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}

	if data.FirstTripOfDay {
		sb.WriteString("{{TALON_TOP_RIGHT_STAR}}\n")
	}
	sb.WriteString("{{TALON_COMPACT_ON}}\n")
	appendFrenchDriverTalonCompact(&sb, data, when)
	sb.WriteString("{{TALON_COMPACT_OFF}}\n")
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
		lineTotal := data.BasePrice * float64(data.SeatNumber)
		if data.VehicleCapacity > 0 && data.SeatNumber == data.VehicleCapacity {
			rows = append(rows,
				frenchTwoCol("CAPACITE", strconv.Itoa(data.VehicleCapacity)),
				frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", lineTotal)),
			)
		} else {
			rows = append(rows,
				frenchTwoCol("PLACES RESERVEES", strconv.Itoa(data.SeatNumber)),
				frenchTwoCol("TOTAL", fmt.Sprintf("%.3f", lineTotal)),
			)
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
