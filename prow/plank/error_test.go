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
	"testing"
)

func TestTerminalError(t *testing.T) {
	tests := []struct {
		name       string
		gotErr     error
		isTerminal bool
	}{
		{
			name:       "nil",
			gotErr:     TerminalError(nil),
			isTerminal: true,
		},
		{
			name:       "wrap error",
			gotErr:     TerminalError(errors.New("ramdom error")),
			isTerminal: true,
		},
		{
			name:       "not nil",
			gotErr:     nil,
			isTerminal: false,
		},
		{
			name:       "not random error",
			gotErr:     errors.New("random error"),
			isTerminal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, want := IsTerminalError(tt.gotErr), tt.isTerminal
			if got != want {
				t.Fatalf("Error matching not expected. want match: %v. got match: %v", want, got)
			}
		})
	}
}
