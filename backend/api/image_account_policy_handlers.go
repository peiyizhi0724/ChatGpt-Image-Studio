package api

import (
	"encoding/json"
	"net/http"

	"chatgpt2api/internal/accounts"
)

type imageAccountPolicyPayload struct {
	Policy accounts.ImageAccountRoutingPolicy `json:"policy"`
}

func (s *Server) handleGetImageAccountPolicy(w http.ResponseWriter, r *http.Request) {
	policy, err := s.getStore().GetImageRoutingPolicy()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, imageAccountPolicyPayload{Policy: policy})
}

func (s *Server) handleUpdateImageAccountPolicy(w http.ResponseWriter, r *http.Request) {
	var payload imageAccountPolicyPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
		return
	}
	if err := s.getStore().SaveImageRoutingPolicy(payload.Policy); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	updated, err := s.getStore().GetImageRoutingPolicy()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, imageAccountPolicyPayload{Policy: updated})
}
