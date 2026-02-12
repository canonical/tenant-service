// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type API struct {
	service *Service
}

func NewAPI(service *Service) *API {
	return &API{
		service: service,
	}
}

func (a *API) RegisterEndpoints(mux *chi.Mux) {
	mux.Post("/webhooks/registration", a.registration)
}

func (a *API) registration(w http.ResponseWriter, r *http.Request) {
	var identity KratosIdentity
	if err := json.NewDecoder(r.Body).Decode(&identity); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := a.service.HandleRegistration(r.Context(), identity.ID, identity.Traits.Email); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
