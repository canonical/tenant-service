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
