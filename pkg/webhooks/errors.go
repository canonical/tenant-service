// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0-only

package webhooks

import "errors"

// ErrNotMember is returned when a user is not an active member of the requested tenant.
// The handler maps this to HTTP 403 Forbidden.
var ErrNotMember = errors.New("user is not an active member of the tenant")
