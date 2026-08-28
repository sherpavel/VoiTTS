package api

import (
	"net/http"
	"voitts/internal/profile"
	"voitts/internal/tts"
)

type APIMux struct {
	TTSEngine    *tts.Piper
	ProfileStore *profile.ProfileStore
}

func New(profileStore *profile.ProfileStore, ttsEngine *tts.Piper) *APIMux {
	return &APIMux{
		TTSEngine:    ttsEngine,
		ProfileStore: profileStore,
	}
}

func (s *APIMux) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/tts", s.handleTTS)
	mux.HandleFunc("GET /api/profiles", s.handleProfileGet)
	mux.HandleFunc("PUT /api/profiles/order", s.handleProfileReorder)
	mux.HandleFunc("PATCH /api/profile", s.handleProfilePatch)
	mux.HandleFunc("DELETE /api/profile", s.handleProfileDelete)
	return mux
}
