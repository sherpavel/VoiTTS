package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"voitts/internal/profile"
)

func (s *APIMux) handleProfileGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.ProfileStore.Get())
}

func (s *APIMux) handleProfilePatch(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond("Failed to read request body", err, w, http.StatusBadRequest)
		return
	}

	var profile profile.Profile
	err = json.Unmarshal(bodyBytes, &profile)
	if err != nil {
		logAndRespond("Failed to parse request body", err, w, http.StatusBadRequest)
		return
	}

	err = s.ProfileStore.Upsert(profile)
	if err != nil {
		logAndRespond("Failed to upsert profile", err, w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type profileReorderRequest struct {
	Names []string `json:"names"`
}

// Takes the full list of names in a new order. Must match fully, partial not allowed
func (s *APIMux) handleProfileReorder(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond("Failed to read request body", err, w, http.StatusBadRequest)
		return
	}

	var req profileReorderRequest
	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		logAndRespond("Failed to parse request body", err, w, http.StatusBadRequest)
		return
	}

	err = s.ProfileStore.Reorder(req.Names)
	if errors.Is(err, profile.ErrOrderMismatch) {
		logAndRespond("Profiles out of date", err, w, http.StatusConflict)
		return
	}
	if err != nil {
		logAndRespond("Failed to reorder profiles", err, w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

type profileDeleteRequest struct {
	Name string `json:"name"`
}

func (s *APIMux) handleProfileDelete(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		logAndRespond("Failed to read request body", err, w, http.StatusBadRequest)
		return
	}

	var req profileDeleteRequest
	err = json.Unmarshal(bodyBytes, &req)
	if err != nil {
		logAndRespond("Failed to parse request body", err, w, http.StatusBadRequest)
		return
	}

	err = s.ProfileStore.Delete(req.Name)
	if err != nil {
		logAndRespond("Failed to delete profile", err, w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
