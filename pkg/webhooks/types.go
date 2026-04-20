// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"encoding/json"

	"github.com/ory/hydra/v2/oauth2"
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

type TokenHookRequest = oauth2.TokenHookRequest

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
