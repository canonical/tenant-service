// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/go-chi/chi/v5"
	"github.com/ory/hydra/v2/oauth2"
)

// API implements HTTP handler endpoints for webhooks.
type API struct {
	service    ServiceInterface
	middleware *APITokenAuthMiddleware

	logger logging.LoggerInterface
}

// NewAPI creates a new webhook API handler.
func NewAPI(
	service ServiceInterface,
	middleware *APITokenAuthMiddleware,
	logger logging.LoggerInterface,
) *API {
	a := new(API)

	a.service = service
	if middleware != nil {
		a.middleware = middleware
	}

	a.logger = logger

	return a
}

func (a *API) RegisterEndpoints(mux *chi.Mux) {
	var r chi.Router = mux
	if a.middleware != nil {
		r = mux.With(a.middleware.AuthMiddleware)
	}
	r.Post("/api/v0/webhooks/registration", a.registration)
	r.Post("/api/v0/webhooks/token", a.tokenHook)
	r.Post("/api/v0/webhooks/login", a.loginHook)
}

func (a *API) tokenHook(w http.ResponseWriter, r *http.Request) {
	req := new(oauth2.TokenHookRequest)
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		a.logger.Errorw("token hook: invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := a.service.HandleTokenHook(r.Context(), req)
	if errors.Is(err, ErrNotMember) {
		a.logger.Infow("token hook: user is not an active member of tenant")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "user is not an active member of the tenant"})
		return
	}
	if err != nil {
		a.logger.Errorw("token hook: service error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.logger.Errorw("token hook: response encoding error", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func (a *API) registration(w http.ResponseWriter, r *http.Request) {
	var identity KratosIdentity
	if err := json.NewDecoder(r.Body).Decode(&identity); err != nil {
		a.logger.Errorw("registration: invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	a.logger.Debugw("received registration webhook", "identity_id", identity.ID, "email", identity.Email)

	if err := a.service.HandleRegistration(r.Context(), identity.ID, identity.Email); err != nil {
		a.logger.Errorw("registration: service error",
			"identity_id", identity.ID,
			"email", identity.Email,
			"error", err,
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (a *API) loginHook(w http.ResponseWriter, r *http.Request) {
	var payload KratosLoginPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.logger.Errorw("login hook: invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	a.logger.Debugw("received login webhook",
		"identity_id", payload.IdentityID,
		"tenant_id", payload.TenantID,
	)

	err := a.service.HandleLoginHook(r.Context(), payload.IdentityID, payload.Email, payload.TenantID)
	if errors.Is(err, ErrNotMember) {
		a.logger.Infow("login hook: access denied",
			"identity_id", payload.IdentityID,
			"tenant_id", payload.TenantID,
		)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "user is not an active member of the tenant"})
		return
	}
	if err != nil {
		a.logger.Errorw("login hook: service error",
			"identity_id", payload.IdentityID,
			"tenant_id", payload.TenantID,
			"error", err,
		)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{})
}
