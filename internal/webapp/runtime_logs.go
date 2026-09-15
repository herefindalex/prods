package webapp

import (
	"errors"
	"net/http"
	"os"
	"strconv"

	"prods/internal/identity"
	"prods/internal/platform"
)

type runtimeLogResponse struct {
	Available     bool     `json:"available"`
	Generation    int      `json:"generation"`
	MaxGeneration int      `json:"max_generation"`
	FileName      string   `json:"file_name,omitempty"`
	SizeBytes     int64    `json:"size_bytes,omitempty"`
	Truncated     bool     `json:"truncated"`
	Lines         []string `json:"lines"`
}

func (s *Server) adminRuntimeLog(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireCapability(w, r, identity.CapabilitySystemManage, false); !ok {
		return
	}
	generation := 0
	if raw := r.URL.Query().Get("generation"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
			return
		}
		generation = parsed
	}
	if generation < 0 || generation > s.config.RuntimeLogFiles {
		s.writeAPIError(w, r, http.StatusBadRequest, apiCodeValidationFailed)
		return
	}
	tail, err := platform.ReadRuntimeLogTail(
		s.config.RuntimeLogPath, generation, s.config.RuntimeLogFiles, platform.DefaultRuntimeLogTailLines,
	)
	if errors.Is(err, os.ErrNotExist) {
		writeJSON(w, http.StatusOK, runtimeLogResponse{
			Available: false, Generation: generation, MaxGeneration: s.config.RuntimeLogFiles, Lines: make([]string, 0),
		})
		return
	}
	if err != nil {
		s.internalAPIError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, runtimeLogResponse{
		Available: true, Generation: tail.Generation, MaxGeneration: s.config.RuntimeLogFiles, FileName: tail.FileName,
		SizeBytes: tail.SizeBytes, Truncated: tail.Truncated, Lines: tail.Lines,
	})
}
