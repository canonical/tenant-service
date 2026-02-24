// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"encoding/json"
	"net/http"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/go-chi/chi/v5"
	"github.com/ory/hydra/v2/oauth2"
)

type API struct {
	service ServiceInterface
	logger  logging.LoggerInterface
}

func NewAPI(service ServiceInterface, logger logging.LoggerInterface) *API {
	return &API{
		service: service,
		logger:  logger,
	}
}

func (a *API) RegisterEndpoints(mux *chi.Mux) {
	mux.Post("/api/v0/webhooks/registration", a.registration)
	mux.Post("/api/v0/webhooks/token", a.tokenHook)
}

func (a *API) tokenHook(w http.ResponseWriter, r *http.Request) {
	req := new(oauth2.TokenHookRequest)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		a.logger.Errorf("tokenHook: invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := a.service.HandleTokenHook(r.Context(), req)
	if err != nil {
		a.logger.Errorf("tokenHook: service error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.logger.Errorf("tokenHook: response encoding error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *API) registration(w http.ResponseWriter, r *http.Request) {
	var identity KratosIdentity
	if err := json.NewDecoder(r.Body).Decode(&identity); err != nil {
		a.logger.Errorf("registration: invalid request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	a.logger.Debugf("Received registration webhook for identity: %v", identity)

	if err := a.service.HandleRegistration(r.Context(), identity.ID, identity.Email); err != nil {
		a.logger.Errorf("registration: service error: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
