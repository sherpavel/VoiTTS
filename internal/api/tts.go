package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

const maxBodyBytes = 8192

type ttsRequest struct {
	Text string `json:"text"`
}

func (s *APIMux) handleTTS(w http.ResponseWriter, r *http.Request) {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()

	var req ttsRequest
	if err := dec.Decode(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "text is too long")
			return
		}
		writeError(w, http.StatusBadRequest, `body must be JSON of the form {"text": "..."}`)
		return
	}

	text := strings.TrimSpace(req.Text)
	if text == "" {
		writeError(w, http.StatusBadRequest, "text is empty")
		return
	}

	if err := s.TTSEngine.Say(text); err != nil {
		// The detail is usually Piper's stderr, which belongs in the operator's
		// log rather than in a response body.
		slog.Error("synthesize", "error", err)
		writeError(w, http.StatusInternalServerError, "synthesizer unavailable")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}
