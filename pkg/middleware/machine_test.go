package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMachineType_DefaultsToNormal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if got := MachineType(c); got != MachineTypeNormal {
		t.Fatalf("expected default %q, got %q", MachineTypeNormal, got)
	}
}

func TestMachineType_PosFromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []string{"pos", "POS", "  Pos  "}
	for _, v := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		c.Request.Header.Set(MachineTypeHeader, v)
		if got := MachineType(c); got != MachineTypePOS {
			t.Fatalf("MachineType(%q)=%q, want %q", v, got, MachineTypePOS)
		}
	}
}

func TestMachineType_UnknownFallsBackToNormal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set(MachineTypeHeader, "kiosk")
	if got := MachineType(c); got != MachineTypeNormal {
		t.Fatalf("unknown machine type should fall back to %q, got %q", MachineTypeNormal, got)
	}
}

func TestMachineID_TrimsWhitespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Request.Header.Set(MachineIDHeader, "  pos-001  ")
	if got := MachineID(c); got != "pos-001" {
		t.Fatalf("MachineID trimming failed: got %q", got)
	}
}
