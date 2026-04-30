package printer

import "testing"

func TestEncodeArabicThermalUTF8_NonEmpty(t *testing.T) {
	b := encodeArabicThermalUTF8("تجربة")
	if len(b) == 0 {
		t.Fatal("expected non-empty ESC/POS payload bytes")
	}
}
