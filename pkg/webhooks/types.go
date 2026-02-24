// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

import (
	"encoding/json"

	"github.com/ory/hydra/v2/oauth2"
)

type KratosIdentity struct {
	ID    string                 `json:"user_id"`
	Email string                 `json:"email"`
	Extra map[string]interface{} `json:"-"`
}

func (k *KratosIdentity) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &k.Extra); err != nil {
		return err
	}
	if v, ok := k.Extra["user_id"].(string); ok {
		k.ID = v
	}
	if v, ok := k.Extra["email"].(string); ok {
		k.Email = v
	}
	return nil
}

type TokenHookRequest = oauth2.TokenHookRequest

type TokenHookResponse struct {
	Session struct {
		IDToken     map[string]interface{} `json:"id_token,omitempty"`
		AccessToken map[string]interface{} `json:"access_token,omitempty"`
	} `json:"session"`
}
