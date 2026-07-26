package main

import (
	"encoding/json"
	"net/http"
)

// tokenOK returns true when auth is disabled (want == "") or the request
// presents the token via ?token=<t> or "Authorization: Bearer <t>".
// WebSocket clients (browsers) can't set headers, so ?token= is the portable way.
func tokenOK(r *http.Request, want string) bool {
	if want == "" {
		return true
	}
	if r.URL.Query().Get("token") == want {
		return true
	}
	return r.Header.Get("Authorization") == "Bearer "+want
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*") // 別オリジンの viewer から取得可能に
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// GET /games — active games の一覧(スナップショット)。
func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	if !tokenOK(r, s.cfg.Auth.ReceiverToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	writeJSON(w, http.StatusOK, s.mgr.List())
}

// GET /games/{id} — 1試合のスナップショット。
func (s *Server) handleGameByID(w http.ResponseWriter, r *http.Request) {
	if !tokenOK(r, s.cfg.Auth.ReceiverToken) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}
	g := s.mgr.Get(r.PathValue("id"))
	if g == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "game not found"})
		return
	}
	writeJSON(w, http.StatusOK, g.Snapshot())
}
