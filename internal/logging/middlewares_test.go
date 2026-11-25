// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package logging

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestLogEntry_LogsThePathWithoutTheQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	logger := NewMockLoggerInterface(ctrl)

	var line string
	logger.EXPECT().Debug(gomock.Any()).Do(func(args ...interface{}) { line = args[0].(string) })

	req := httptest.NewRequest(http.MethodGet, "/api/v0/tenants/lookup?email=someone@example.com", nil)
	NewLogFormatter(logger).NewLogEntry(req).Write(http.StatusOK, 0, http.Header{}, time.Millisecond, nil)

	if !strings.Contains(line, "/api/v0/tenants/lookup ") || strings.Contains(line, "someone@example.com") {
		t.Errorf("request line = %q, want the path without the query", line)
	}
}
