// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package webhooks

import (
	"crypto/subtle"
	"net/http"

	"github.com/canonical/tenant-service/internal/logging"
	"github.com/canonical/tenant-service/internal/tracing"
)

type APITokenAuthMiddleware struct {
	token string

	tracer tracing.TracingInterface
	logger logging.LoggerInterface
}

func (m *APITokenAuthMiddleware) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")

		if subtle.ConstantTimeCompare([]byte(m.token), []byte(token)) != 1 {
			m.logger.Error("Got invalid authorization header, rejecting request")
			http.Error(w, "Invalid authorization header", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func NewAuthMiddleware(token string, tracer tracing.TracingInterface, logger logging.LoggerInterface) *APITokenAuthMiddleware {
	m := new(APITokenAuthMiddleware)

	m.token = token

	m.tracer = tracer
	m.logger = logger
	return m
}
