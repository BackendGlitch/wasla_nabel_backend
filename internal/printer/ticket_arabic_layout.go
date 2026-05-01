package printer

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"station-backend/internal/pricing"
)

// Arabic-facing ticket layout (markers consumed by escpos_arabic_markers + convertToESCPOS).

const (
	arPlateLabel          = "رقم المركبة"
	arTimeLabelFmt        = "التوقيت : %s"
	arDateLabelFmt        = "التاريخ : %s"
	arTimeColLabel        = "التوقيت :"
	arDateColLabel        = "التاريخ :"
	arCarColLabel         = "السيارة :"
	arFareColLabel        = "التعريفة :"
	arDirColLabel         = "الاتجاه :"
	arBaseTariffFmt       = "التعريفة الأساسية : %.3f"
	arServiceFeeFmt       = "رسوم الخدمة : %.3f"
	arTotalFmt            = "المجموع : %.3f"
	arTotalSimpleFmt      = "المجموع : %.3f"
	arStaffCaption        = "المسؤول"
	arDestinationCaption  = "الاتجاه"
	arNonRefundableInner  = "التذكرة لا ترجع"

	arExitPermitTitle      = "إذن بالخروج"
	arEntryTicketTitle     = "بطاقة الدخول"
	arExitTripTitle        = "بطاقة الخروج"
	arDayPassTitle         = "تصريح يوم كامل"
	arRouteLabel           = "المسار"
	arStationLabel         = "المحطة"
	arBookedSeatsFmt       = "المقاعد المحجوزة : %d"
	arVehicleCapacityFmt   = "سعة المركبة : %d مقاعد"
	arExitPassTariffGrossFmt   = "مجموع التعريفة : %.3f"
	arExitPassEntryDayDeductionFmt = "دخول المركبة — تصريح اليوم : -%.3f"
	arFarewellMerci        = "شكرا لكم"
	arExitAuthorized       = "الخروج مصرح به"
	arDayPassNoService     = "بدون رسوم خدمة"
	arDayPassPurchaseFmt   = "تاريخ الشراء : %s"
	arDayPassValidFromFmt  = "صالحة من %s"
	arDayPassValidUntilFmt = "إلى غاية %s"
	arStandardTitle        = "تذكرة"
	arDayPassAmountFmt     = "مبلغ التصريح : %.3f"

	// Default Arabic booking-slip banner (Fatma Zahra / Maghreb Arabi station, Nabeul).
	defaultBookingCompanyArabicKV  = "إسم الشركة : الشركة الاهلية الجهوية فاطمة الزهراء لخدمات النقل بنابل"
	defaultBookingStationArabicKV  = "إسم المحطة : محطة المغرب العربي بنابل"
	defaultBookingRoutesHeaderArab = "الخطوط المستغلة في المحطة :"

	arBookingCoLabel   = "إسم الشركة"
	arBookingStaLabel  = "إسم المحطة"
)

func defaultStationRoutesArabicStatic() []string {
	return []string{
		"قرنبالية",
		"-بني خلاد",
		"منزل بوزلفة",
		"سليمان",
		"بلي _ المهاذبة",
	}
}

func arabicKVLineBareOrFull(labelStem, fallbackFullLine, bareOrFullFromPayload string) string {
	s := strings.TrimSpace(bareOrFullFromPayload)
	if s == "" {
		return fallbackFullLine
	}
	idx := strings.Index(s, ":")
	if idx >= 0 {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(labelStem + " : " + s)
}

func bookingPrintedCompanyArabic(data *TicketData) string {
	return arabicKVLineBareOrFull(arBookingCoLabel, defaultBookingCompanyArabicKV, data.CompanyArabic)
}

func bookingPrintedStationArabic(data *TicketData) string {
	return arabicKVLineBareOrFull(arBookingStaLabel, defaultBookingStationArabicKV, data.StationArabic)
}

func bookingPrintedRoutesArabic(data *TicketData) []string {
	if len(data.StationRoutesArabic) > 0 {
		out := make([]string, 0, len(data.StationRoutesArabic))
		for _, r := range data.StationRoutesArabic {
			r = strings.TrimSpace(r)
			if r != "" {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	cp := append([]string(nil), defaultStationRoutesArabicStatic()...)
	return cp
}

func appendArabCenterBodyEscaped(sb *strings.Builder, arabicPlain string) {
	arabicPlain = strings.TrimSpace(arabicPlain)
	if arabicPlain == "" {
		return
	}
	sb.WriteString("{{AR_CENTER_BODY:" + arabicPlain + "}}\n")
}

func appendArabicBookingPassengerBanner(sb *strings.Builder, data *TicketData) {
	appendArabCenterBodyEscaped(sb, bookingPrintedCompanyArabic(data))
	appendArabCenterBodyEscaped(sb, bookingPrintedStationArabic(data))
	appendArabCenterBodyEscaped(sb, defaultBookingRoutesHeaderArab)
	for _, ln := range bookingPrintedRoutesArabic(data) {
		appendArabCenterBodyEscaped(sb, ln)
	}
}

func appendArabicBookingDriverBanner(sb *strings.Builder, data *TicketData) {
	appendArabCenterBodyEscaped(sb, bookingPrintedCompanyArabic(data))
	appendArabCenterBodyEscaped(sb, bookingPrintedStationArabic(data))
}

func arabicKVLineValueAfterFirstColon(full string) string {
	full = strings.TrimSpace(full)
	idx := strings.Index(full, ":")
	if idx < 0 || idx >= len(full)-1 {
		return full
	}
	return strings.TrimSpace(full[idx+1:])
}

func bookingThermalFooterArabic(data *TicketData) string {
	st := arabicKVLineValueAfterFirstColon(bookingPrintedStationArabic(data))
	if strings.TrimSpace(st) == "" {
		st = strings.TrimSpace(data.StationName)
	}
	if st == "" {
		st = strings.TrimSpace(companyNameForTicket(data))
	}
	st = truncateRunes(strings.TrimSpace(st), 26)
	if st == "" {
		return "( " + arNonRefundableInner + " )"
	}
	return fmt.Sprintf("( %s ترحب بضيوفها الكرام ( %s ) )", st, arNonRefundableInner)
}

func tunisFmtDateSlash(t time.Time) string {
	if t.IsZero() {
		t = nowTunis()
	}
	return t.In(tunisLocationForTickets()).Format("2006/01/02")
}

func tunisFmtHM(t time.Time) string {
	if t.IsZero() {
		t = nowTunis()
	}
	return t.In(tunisLocationForTickets()).Format("15:04")
}

func nowTunis() time.Time {
	return time.Now().In(tunisLocationForTickets())
}

func destinationArabicMarkers(dest string) []string {
	d := strings.TrimSpace(dest)
	if d == "" {
		return nil
	}
	if latinHeavy(d) {
		return []string{
			"{{AR_CENTER_BODY:" + arDestinationCaption + "}}\n",
			"{{W_SMALL_CENTER:" + d + "}}\n",
		}
	}
	return []string{"{{AR_CENTER_BODY:" + arDestinationCaption + " : " + d + "}}\n"}
}

// latinHeavy: values like license plates / Latin destination names split to western lines.
func latinHeavy(s string) bool {
	ar := 0
	la := 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Arabic, r):
			ar++
		case unicode.IsLetter(r) && r < 127:
			la++
		}
	}
	if la > 0 && ar == 0 {
		return true
	}
	if la == 0 {
		return false
	}
	return la >= ar
}

func staffArabicMarkers(data *TicketData) []string {
	a := strings.TrimSpace(agentLineForTicket(data))
	if a == "" || strings.EqualFold(a, "Agent") {
		return nil
	}
	if latinHeavy(a) {
		return []string{
			"{{AR_LINE:" + arStaffCaption + "}}\n",
			"{{W_SMALL_CENTER:" + a + "}}\n",
		}
	}
	return []string{"{{AR_LINE:" + arStaffCaption + " : " + a + "}}\n"}
}

func bookingDisplayFareTD(data *TicketData) string {
	v := data.TotalAmount
	if v <= 0 && data.BasePrice > 0 {
		v = data.BasePrice + resolveServiceFeePerSeat(data)
		if v < 0 {
			v = 0
		}
	}
	if v <= 0 {
		return "0.000"
	}
	return fmt.Sprintf("%.3f", v)
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func destinationBookingCompactLine(data *TicketData) string {
	d := strings.TrimSpace(data.DestinationName)
	if d == "" {
		return ""
	}
	if latinHeavy(d) {
		return "{{AR_DIR_MIX_CENTER:" + arDirColLabel + "|" + d + "}}\n"
	}
	return "{{AR_CENTER_BODY:" + arDestinationCaption + " : " + d + "}}\n"
}

func bookingTwoColTimeDate(when time.Time) string {
	return "{{AR_TWOCOL:" + arTimeColLabel + "|" + tunisFmtHM(when) + "|" + arDateColLabel + "|" + tunisFmtDateSlash(when) + "}}\n"
}

func bookingTwoColFarePlate(fare, plate string) string {
	fare = strings.TrimSpace(fare)
	plate = strings.TrimSpace(strings.ToUpper(plate))
	return "{{AR_TWOCOL:" + arFareColLabel + "|" + fare + "|" + arCarColLabel + "|" + plate + "}}\n"
}

func arabicPricingLinesBooking(data *TicketData) []string {
	var out []string
	if data.BasePrice > 0 {
		fee := resolveServiceFeePerSeat(data)
		baseLine := data.BasePrice
		if data.TotalAmount > 0 && data.BasePrice <= 0 {
			baseLine = data.TotalAmount - fee
			if baseLine < 0 {
				baseLine = 0
			}
		}
		sum := baseLine + fee
		out = append(out, fmt.Sprintf("{{AR_LINE:"+arBaseTariffFmt+"}}\n", baseLine))
		out = append(out, fmt.Sprintf("{{AR_LINE:"+arServiceFeeFmt+"}}\n", fee))
		out = append(out, fmt.Sprintf("{{AR_LINE:"+arTotalFmt+"}}\n", sum))
	} else {
		out = append(out, fmt.Sprintf("{{AR_LINE:"+arTotalSimpleFmt+"}}\n", data.TotalAmount))
	}
	return out
}

func writeLogoOptional(sb *strings.Builder, data *TicketData) {
	if lm := logoMarkerForTicket(data); lm != "" {
		sb.WriteString(lm)
		sb.WriteString("\n")
	}
}

// RenderArabicBookingTicket is a compact Arabic thermal layout (passenger + driver talon, one cut): no logo,
// tight rows, RTL-friendly two-column date/time and plate/fare matching common louage station slips.
func RenderArabicBookingTicket(data *TicketData) string {
	var sb strings.Builder

	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}

	fare := bookingDisplayFareTD(data)
	plate := strings.TrimSpace(data.LicensePlate)

	sb.WriteString("{{AR_INIT}}\n")
	appendArabicBookingPassengerBanner(&sb, data)

	sb.WriteString(fmt.Sprintf("{{AR_SEAT_NBR:%d}}\n", data.SeatNumber))
	sb.WriteString(bookingTwoColTimeDate(when))
	sb.WriteString(bookingTwoColFarePlate(fare, plate))

	if ln := destinationBookingCompactLine(data); ln != "" {
		sb.WriteString(ln)
	}

	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_NOTE_SMALL:" + bookingThermalFooterArabic(data) + "}}\n")

	sb.WriteString("{{SHORT_FEED_BEFORE_CUT}}\n")
	sb.WriteString("{{PARTIAL_CUT}}\n")

	if data.FirstTripOfDay {
		sb.WriteString("{{STAR_TR}}\n")
	}
	sb.WriteString("{{TALON_COMPACT_ON}}\n")
	sb.WriteString("{{AR_INIT}}\n")
	appendArabicBookingDriverBanner(&sb, data)
	sb.WriteString(fmt.Sprintf("{{AR_DRIVER_HDR_NBR:%d}}\n", data.SeatNumber))
	sb.WriteString(bookingTwoColTimeDate(when))
	sb.WriteString(bookingTwoColFarePlate(fare, plate))

	if ln := destinationBookingCompactLine(data); ln != "" {
		sb.WriteString(ln)
	}

	sb.WriteString("{{TALON_COMPACT_OFF}}\n")
	sb.WriteString("{{TALON_END_FEED}}\n")

	return sb.String()
}

func routeArabicMarkers(route string) []string {
	r := strings.TrimSpace(route)
	if r == "" {
		return nil
	}
	if latinHeavy(r) {
		return []string{
			"{{AR_LINE:" + arRouteLabel + "}}\n",
			"{{W_SMALL_CENTER:" + r + "}}\n",
		}
	}
	return []string{"{{AR_CENTER_BODY:" + arRouteLabel + " : " + r + "}}\n"}
}

func stationArabicMarkers(name string) []string {
	st := strings.TrimSpace(name)
	if st == "" {
		return nil
	}
	if latinHeavy(st) {
		return []string{
			"{{AR_LINE:" + arStationLabel + "}}\n",
			"{{W_SMALL_CENTER:" + st + "}}\n",
		}
	}
	return []string{"{{AR_CENTER_BODY:" + arStationLabel + " : " + st + "}}\n"}
}

func RenderArabicEntryTicket(data *TicketData) string {
	var sb strings.Builder
	writeLogoOptional(&sb, data)
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{AR_INIT}}\n")
	sb.WriteString("{{AR_TITLE:" + arEntryTicketTitle + "}}\n")
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_LINE:" + arPlateLabel + "}}\n")
	sb.WriteString(fmt.Sprintf("{{W_PLATE_LARGE:%s}}\n", strings.TrimSpace(data.LicensePlate)))
	for _, r := range routeArabicMarkers(data.RouteName) {
		sb.WriteString(r)
	}
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDateLabelFmt+"}}\n", tunisFmtDateSlash(when)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arTimeLabelFmt+"}}\n", tunisFmtHM(when)))
	for _, s := range staffArabicMarkers(data) {
		sb.WriteString(s)
	}
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_CENTER_BODY:" + arFarewellMerci + "}}\n")
	return sb.String()
}

func RenderArabicExitTripTicket(data *TicketData) string {
	var sb strings.Builder
	writeLogoOptional(&sb, data)
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	sb.WriteString("{{AR_INIT}}\n")
	sb.WriteString("{{AR_TITLE:" + arExitTripTitle + "}}\n")
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_LINE:" + arPlateLabel + "}}\n")
	sb.WriteString(fmt.Sprintf("{{W_PLATE_LARGE:%s}}\n", strings.TrimSpace(data.LicensePlate)))
	for _, r := range routeArabicMarkers(data.RouteName) {
		sb.WriteString(r)
	}
	for _, st := range stationArabicMarkers(data.StationName) {
		sb.WriteString(st)
	}
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDateLabelFmt+"}}\n", tunisFmtDateSlash(when)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arTimeLabelFmt+"}}\n", tunisFmtHM(when)))
	for _, s := range staffArabicMarkers(data) {
		sb.WriteString(s)
	}
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_CENTER_BODY:" + arFarewellMerci + "}}\n")
	return sb.String()
}

func RenderArabicDayPassTicket(data *TicketData) string {
	var sb strings.Builder
	writeLogoOptional(&sb, data)

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

	sb.WriteString("{{AR_INIT}}\n")
	sb.WriteString("{{AR_TITLE:" + arDayPassTitle + "}}\n")
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_LINE:" + arPlateLabel + "}}\n")
	sb.WriteString(fmt.Sprintf("{{W_PLATE_LARGE:%s}}\n", strings.TrimSpace(data.LicensePlate)))
	for _, r := range routeArabicMarkers(data.RouteName) {
		sb.WriteString(r)
	}
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDayPassAmountFmt+"}}\n", pricing.DayPassTotalPriceTND))
	sb.WriteString("{{AR_LINE:" + arDayPassNoService + "}}\n")
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDayPassPurchaseFmt+"}}\n", formatTicketReadableDate(purchAt)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDayPassValidFromFmt+"}}\n", formatTicketReadableDate(validFrom)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDayPassValidUntilFmt+"}}\n", formatTicketReadableDate(validUntil)))
	for _, s := range staffArabicMarkers(data) {
		sb.WriteString(s)
	}
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_CENTER_BODY:" + arFarewellMerci + "}}\n")
	return sb.String()
}

func formatTicketReadableDate(t time.Time) string {
	if t.IsZero() {
		t = nowTunis()
	}
	tt := t.In(tunisLocationForTickets())
	return tt.Format("2006/01/02") + " " + tt.Format("15:04")
}

func arabicExitPassPricingBlocks(data *TicketData) []string {
	var blocks []string
	if data.BasePrice > 0 && data.SeatNumber > 0 {
		gross := data.BasePrice * float64(data.SeatNumber)
		if data.VehicleCapacity > 0 && data.SeatNumber == data.VehicleCapacity {
			blocks = append(blocks,
				fmt.Sprintf("{{AR_LINE:"+arVehicleCapacityFmt+"}}\n", data.VehicleCapacity),
			)
		} else {
			blocks = append(blocks,
				fmt.Sprintf("{{AR_LINE:"+arBookedSeatsFmt+"}}\n", data.SeatNumber),
			)
		}
		if data.FirstTripOfDay && gross > 0 {
			ded := pricing.EntryDayPassFeeTND
			net := gross - ded
			if net < 0 {
				net = 0
			}
			blocks = append(blocks,
				fmt.Sprintf("{{AR_LINE:"+arExitPassTariffGrossFmt+"}}\n", gross),
				fmt.Sprintf("{{AR_LINE:"+arExitPassEntryDayDeductionFmt+"}}\n", ded),
				fmt.Sprintf("{{AR_LINE:"+arTotalFmt+"}}\n", net),
			)
		} else {
			blocks = append(blocks, fmt.Sprintf("{{AR_LINE:"+arTotalFmt+"}}\n", gross))
		}
	} else {
		blocks = append(blocks, fmt.Sprintf("{{AR_LINE:"+arTotalSimpleFmt+"}}\n", data.TotalAmount))
	}
	return blocks
}

func RenderArabicExitPassTicket(data *TicketData) string {
	var sb strings.Builder
	writeLogoOptional(&sb, data)
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}

	sb.WriteString("{{AR_INIT}}\n")
	sb.WriteString("{{AR_TITLE:" + arExitPermitTitle + "}}\n")
	if data.ExitPassCount > 0 {
		sb.WriteString("{{W_EXIT_COUNT:" + strconv.Itoa(data.ExitPassCount) + "}}\n")
	}
	sb.WriteString("{{AR_SEP}}\n")
	if data.FirstTripOfDay {
		sb.WriteString("{{STAR_TR}}\n")
	}
	sb.WriteString("{{AR_LINE:" + arPlateLabel + "}}\n")
	sb.WriteString(fmt.Sprintf("{{W_PLATE_LARGE:%s}}\n", strings.TrimSpace(data.LicensePlate)))
	for _, d := range destinationArabicMarkers(data.DestinationName) {
		sb.WriteString(d)
	}

	for _, b := range arabicExitPassPricingBlocks(data) {
		sb.WriteString(b)
	}

	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDateLabelFmt+"}}\n", tunisFmtDateSlash(when)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arTimeLabelFmt+"}}\n", tunisFmtHM(when)))
	for _, s := range staffArabicMarkers(data) {
		sb.WriteString(s)
	}
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_CENTER_BODY:" + arExitAuthorized + "}}\n")
	return sb.String()
}

func RenderArabicStandardTicket(data *TicketData) string {
	var sb strings.Builder
	writeLogoOptional(&sb, data)
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}

	sb.WriteString("{{AR_INIT}}\n")
	sb.WriteString("{{AR_TITLE:" + arStandardTitle + "}}\n")
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_LINE:" + arPlateLabel + "}}\n")
	sb.WriteString(fmt.Sprintf("{{W_PLATE_LARGE:%s}}\n", strings.TrimSpace(data.LicensePlate)))
	for _, d := range destinationArabicMarkers(data.DestinationName) {
		sb.WriteString(d)
	}
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arTotalSimpleFmt+"}}\n", data.TotalAmount))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arDateLabelFmt+"}}\n", tunisFmtDateSlash(when)))
	sb.WriteString(fmt.Sprintf("{{AR_LINE:"+arTimeLabelFmt+"}}\n", tunisFmtHM(when)))
	for _, s := range staffArabicMarkers(data) {
		sb.WriteString(s)
	}
	sb.WriteString("{{AR_SEP}}\n")
	sb.WriteString("{{AR_CENTER_BODY:" + arFarewellMerci + "}}\n")
	return sb.String()
}

func RenderArabicTalonOnly(data *TicketData) string {
	var sb strings.Builder
	when := data.CreatedAt
	if when.IsZero() {
		when = time.Now()
	}
	fare := bookingDisplayFareTD(data)
	plate := strings.TrimSpace(data.LicensePlate)

	if data.FirstTripOfDay {
		sb.WriteString("{{STAR_TR}}\n")
	}
	sb.WriteString("{{TALON_COMPACT_ON}}\n")
	sb.WriteString("{{AR_INIT}}\n")
	appendArabicBookingDriverBanner(&sb, data)
	sb.WriteString(fmt.Sprintf("{{AR_DRIVER_HDR_NBR:%d}}\n", data.SeatNumber))
	sb.WriteString(bookingTwoColTimeDate(when))
	sb.WriteString(bookingTwoColFarePlate(fare, plate))
	if ln := destinationBookingCompactLine(data); ln != "" {
		sb.WriteString(ln)
	}
	sb.WriteString("{{TALON_COMPACT_OFF}}\n")
	sb.WriteString("{{TALON_END_FEED}}\n")
	return sb.String()
}
