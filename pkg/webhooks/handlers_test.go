// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ory/hydra/v2/oauth2"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_handlers.go -source=./interfaces.go ServiceInterface
//go:generate mockgen -build_flags=--mod=mod -package webhooks -destination ./mock_handlers_logger.go -source=../../internal/logging/interfaces.go

func TestAPI_TokenHook(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		setupMocks     func(*MockServiceInterface, *MockLoggerInterface)
		expectedStatus int
		validateResp   func(*testing.T, *http.Response)
	}{
		{
			name: "success",
			requestBody: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession("user-123"),
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				response := &TokenHookResponse{
					Session: struct {
						IDToken     map[string]interface{} `json:"id_token,omitempty"`
						AccessToken map[string]interface{} `json:"access_token,omitempty"`
					}{
						IDToken: map[string]interface{}{
							"tenants": []string{"tenant-1", "tenant-2"},
						},
						AccessToken: map[string]interface{}{
							"tenants": []string{"tenant-1", "tenant-2"},
						},
					},
				}
				mockSvc.EXPECT().HandleTokenHook(gomock.Any(), gomock.Any()).Return(response, nil)
			},
			expectedStatus: http.StatusOK,
			validateResp: func(t *testing.T, resp *http.Response) {
				var result TokenHookResponse
				if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
					t.Errorf("failed to decode response: %v", err)
				}
				if result.Session.IDToken["tenants"] == nil {
					t.Error("expected tenants in ID token")
				}
			},
		},
		{
			name:        "invalid request body",
			requestBody: "not-json",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "service error",
			requestBody: &oauth2.TokenHookRequest{
				Session: oauth2.NewSession("user-123"),
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockSvc.EXPECT().HandleTokenHook(gomock.Any(), gomock.Any()).Return(nil, errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := NewMockServiceInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			api := NewAPI(mockService, mockLogger)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatalf("failed to marshal request: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/webhooks/token", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			tt.setupMocks(mockService, mockLogger)

			mux := chi.NewMux()
			api.RegisterEndpoints(mux)
			mux.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				body, _ := io.ReadAll(res.Body)
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, res.StatusCode, string(body))
			}

			if tt.validateResp != nil {
				tt.validateResp(t, res)
			}
		})
	}
}

func TestAPI_Registration(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		setupMocks     func(*MockServiceInterface, *MockLoggerInterface)
		expectedStatus int
	}{
		{
			name: "success",
			requestBody: KratosIdentity{
				ID:    "identity-123",
				Email: "user@example.com",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any())
				mockSvc.EXPECT().HandleRegistration(gomock.Any(), "identity-123", "user@example.com").Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:        "invalid request body",
			requestBody: "not-json",
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "service error",
			requestBody: KratosIdentity{
				ID:    "identity-456",
				Email: "error@example.com",
			},
			setupMocks: func(mockSvc *MockServiceInterface, mockLogger *MockLoggerInterface) {
				mockLogger.EXPECT().Debugf(gomock.Any(), gomock.Any())
				mockSvc.EXPECT().HandleRegistration(gomock.Any(), "identity-456", "error@example.com").Return(errors.New("service error"))
				mockLogger.EXPECT().Errorf(gomock.Any(), gomock.Any())
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockService := NewMockServiceInterface(ctrl)
			mockLogger := NewMockLoggerInterface(ctrl)

			api := NewAPI(mockService, mockLogger)

			var body []byte
			var err error
			if str, ok := tt.requestBody.(string); ok {
				body = []byte(str)
			} else {
				body, err = json.Marshal(tt.requestBody)
				if err != nil {
					t.Fatalf("failed to marshal request: %v", err)
				}
			}

			req := httptest.NewRequest(http.MethodPost, "/webhooks/registration", bytes.NewBuffer(body))
			w := httptest.NewRecorder()

			tt.setupMocks(mockService, mockLogger)

			mux := chi.NewMux()
			api.RegisterEndpoints(mux)
			mux.ServeHTTP(w, req)

			res := w.Result()
			defer res.Body.Close()

			if res.StatusCode != tt.expectedStatus {
				body, _ := io.ReadAll(res.Body)
				t.Errorf("expected status %d, got %d. Body: %s", tt.expectedStatus, res.StatusCode, string(body))
			}
		})
	}
}
