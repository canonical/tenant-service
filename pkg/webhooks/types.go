// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package webhooks

import (
	"encoding/json"
)

// KratosIdentity represents a user identity from Kratos.
type KratosIdentity struct {
	ID    string                 `json:"identity_id"`
	Email string                 `json:"email"`
	Extra map[string]interface{} `json:"-"`
}

func (k *KratosIdentity) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &k.Extra); err != nil {
		return err
	}
	if v, ok := k.Extra["identity_id"].(string); ok {
		k.ID = v
	}
	if v, ok := k.Extra["email"].(string); ok {
		k.Email = v
	}
	return nil
}

// Session represents the session data sent in the token hook request.
type Session struct {
	Subject  string                 `json:"subject,omitempty"`
	Extra    map[string]interface{} `json:"extra,omitempty"`
	ClientID string                 `json:"client_id,omitempty"`
}

// NewSession creates a Session with the given subject and an initialized Extra map.
func NewSession(subject string) *Session {
	return &Session{
		Subject: subject,
		Extra:   make(map[string]interface{}),
	}
}

// TokenHookRequest is the request body sent to the Ory Hydra token hook.
type TokenHookRequest struct {
	Session *Session `json:"session"`
}

// TokenHookResponse represents the response containing the tokens session.
type TokenHookResponse struct {
	Session struct {
		IDToken     map[string]interface{} `json:"id_token,omitempty"`
		AccessToken map[string]interface{} `json:"access_token,omitempty"`
	} `json:"session"`
}

// KratosLoginPayload is the JSON body sent by Kratos to the login webhook.
// The body template in kratos.yml extracts identity_id, email, and tenant_id
// from the Kratos flow context.
type KratosLoginPayload struct {
	IdentityID string `json:"identity_id"`
	Email      string `json:"email"`
	TenantID   string `json:"tenant_id"` // may be empty when no tenant was pre-selected
}
