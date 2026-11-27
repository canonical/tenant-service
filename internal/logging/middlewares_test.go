// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestRedactedURI(t *testing.T) {
	tests := []struct {
		uri, want string
	}{
		{"/api/v0/tenants", "/api/v0/tenants"},
		{"/api/v0/tenants/lookup?email=someone%40example.com&page_size=10", "/api/v0/tenants/lookup?email=REDACTED&page_size=REDACTED"},
		{"/api/v0/tenants?debug&email=", "/api/v0/tenants?debug&email=REDACTED"},
	}

	for _, tt := range tests {
		u, err := url.ParseRequestURI(tt.uri)
		if err != nil {
			t.Fatal(err)
		}
		if got := redactedURI(u); got != tt.want {
			t.Errorf("redactedURI(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}

func TestLogEntry_LogsTheRedactedURI(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := NewMockLoggerInterface(ctrl)

	var line string
	logger.EXPECT().Debug(gomock.Any()).Do(func(args ...interface{}) { line = args[0].(string) })

	req := httptest.NewRequest(http.MethodGet, "/api/v0/tenants/lookup?email=someone@example.com", nil)
	NewLogFormatter(logger).NewLogEntry(req).Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)

	if !strings.Contains(line, "/api/v0/tenants/lookup?email=REDACTED ") {
		t.Errorf("request line = %q, want the URI with its query values redacted", line)
	}
}

func TestLogContextMiddleware_Host(t *testing.T) {
	testCases := []struct {
		name     string
		host     string
		hostname string
		port     any
	}{
		{name: "IPv6", host: "[::1]:8080", hostname: "::1", port: "8080"},
		{name: "no port", host: "tenants.example", hostname: "tenants.example"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var hostname, port any
			handler := LogContextMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hostname, port = r.Context().Value(HostnameKey), r.Context().Value(PortKey)
			}))

			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.Host = tc.host
			handler.ServeHTTP(httptest.NewRecorder(), request)

			if hostname != tc.hostname || port != tc.port {
				t.Errorf("expected hostname %q and port %v, got %v and %v", tc.hostname, tc.port, hostname, port)
			}
		})
	}
}
