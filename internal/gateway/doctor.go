package gateway

import (
	"net/http"

	"github.com/amemiya02/deepseekcode/internal/doctor"
)

// doctorCheck is one row in the GET /v1/doctor response.
type doctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

type doctorReport struct {
	AllOK  bool          `json:"allOk"`
	Checks []doctorCheck `json:"checks"`
}

// handleDoctor implements GET /v1/doctor. It runs the same checker set as the
// `dsc doctor` CLI against the loaded config and returns the results as JSON.
func (h *Handler) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c, err := loadConfig()
	if err != nil {
		http.Error(w, "load config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	checkers := []doctor.Checker{
		doctor.CheckKeyPresent,
		doctor.CheckKeyValid,
		doctor.CheckBaseURLReachable,
		doctor.CheckProxyConfigured,
		doctor.CheckCacheFieldsInProbe,
		doctor.CheckSandboxAvailable,
	}
	results := doctor.RunChecks(r.Context(), c, http.DefaultClient, checkers)
	rep := doctorReport{AllOK: true}
	for _, res := range results {
		if !res.OK {
			rep.AllOK = false
		}
		rep.Checks = append(rep.Checks, doctorCheck{Name: res.Name, OK: res.OK, Detail: res.Detail})
	}
	writeJSON(w, rep)
}
