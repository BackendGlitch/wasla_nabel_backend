package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// Headers used to advertise per-staff-machine capabilities to the backend.
//
// MachineTypeHeader is set by the staff app (wasla) Electron main from the
// STAFF_MACHINE_TYPE environment variable. The management app sets it to
// "normal" too, for symmetry and audit. Defaults to "normal" when missing.
//
// MachineIDHeader is an optional stable identifier of the physical machine
// (e.g. POS device UUID). It is used as the suffix of the print_jobs.printer_id
// for client-local jobs ("client:<machine-id>").
const (
	MachineTypeHeader = "X-Wasla-Machine-Type"
	MachineIDHeader   = "X-Wasla-Machine-Id"

	MachineTypePOS    = "pos"
	MachineTypeNormal = "normal"
)

// MachineType returns the machine type advertised by the request, defaulting
// to MachineTypeNormal when the header is missing or unrecognised.
func MachineType(c *gin.Context) string {
	v := strings.ToLower(strings.TrimSpace(c.GetHeader(MachineTypeHeader)))
	if v == MachineTypePOS {
		return MachineTypePOS
	}
	return MachineTypeNormal
}

// MachineID returns the optional per-machine identifier, or "" if not provided.
func MachineID(c *gin.Context) string {
	return strings.TrimSpace(c.GetHeader(MachineIDHeader))
}
