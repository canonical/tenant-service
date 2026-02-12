// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package webhooks

type KratosIdentity struct {
	ID     string       `json:"id"`
	Traits KratosTraits `json:"traits"`
}

type KratosTraits struct {
	Email string `json:"email"`
}
