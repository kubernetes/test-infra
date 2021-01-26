/*
Copyright 2020 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package plank

import (
	"errors"
	"fmt"
	"testing"
)

func TestCustomError(t *testing.T) {
	tests := []struct {
		name      string
		gotErr    error
		wantErr   error
		wantMatch bool
	}{
		{
			name:      "same error",
			gotErr:    ClusterClientNotExistError,
			wantErr:   ClusterClientNotExistError,
			wantMatch: true,
		},
		{
			name:      "wrap error",
			gotErr:    fmt.Errorf("wrapping %w", ClusterClientNotExistError),
			wantErr:   ClusterClientNotExistError,
			wantMatch: true,
		},
		{
			name:      "not random error",
			gotErr:    errors.New("random error"),
			wantErr:   ClusterClientNotExistError,
			wantMatch: false,
		},
		{
			name:      "not nil",
			gotErr:    nil,
			wantErr:   ClusterClientNotExistError,
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := errors.Is(tt.gotErr, tt.wantErr), tt.wantMatch
			if got != want {
				t.Fatalf("Error matching not expected. want match: %v. got match: %v", want, got)
			}
		})
	}
}
