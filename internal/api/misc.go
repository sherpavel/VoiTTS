package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write response", "error", err)
	}
}

func logAndRespond(msg string, err error, w http.ResponseWriter, status int) {
	slog.Error(msg, "error", err.Error())
	writeError(w, status, fmt.Sprintf("%s: %s", msg, err.Error()))
}
