// Copyright 2026 Canonical Ltd.
// SPDX-License-Identifier: AGPL-3.0

package types_test

import (
	"testing"

	"github.com/canonical/tenant-service/internal/types"
)

func TestResolvePageSize(t *testing.T) {
	tests := []struct {
		name     string
		opts     types.ListOptions
		wantSize uint64
	}{
		{
			name:     "zero returns default",
			opts:     types.ListOptions{PageSize: 0},
			wantSize: 100,
		},
		{
			name:     "negative returns default",
			opts:     types.ListOptions{PageSize: -1},
			wantSize: 100,
		},
		{
			name:     "one returns one",
			opts:     types.ListOptions{PageSize: 1},
			wantSize: 1,
		},
		{
			name:     "mid-range value passes through",
			opts:     types.ListOptions{PageSize: 50},
			wantSize: 50,
		},
		{
			name:     "exactly max passes through",
			opts:     types.ListOptions{PageSize: 100},
			wantSize: 100,
		},
		{
			name:     "above max clamped to max not default",
			opts:     types.ListOptions{PageSize: 101},
			wantSize: 100,
		},
		{
			name:     "large value clamped to max",
			opts:     types.ListOptions{PageSize: 10000},
			wantSize: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.opts.ResolvePageSize()
			if got != tt.wantSize {
				t.Errorf("ResolvePageSize() = %d, want %d", got, tt.wantSize)
			}
		})
	}
}
