/*
Copyright 2022 The Kubernetes Authors.

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

package main

import (
	"os"
	"path"
	"testing"
)

func TestCopy(t *testing.T) {
	tests := []struct {
		name     string
		fileMode os.FileMode
	}{
		{
			name:     "base",
			fileMode: 0644,
		},
		{
			name:     "another-mode",
			fileMode: 0755,
		},
	}

	srcDir := t.TempDir()
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			src := path.Join(srcDir, tc.name)
			os.WriteFile(src, []byte(tc.name+"\nsome\nother\ncontent"), tc.fileMode)

			// One level down, for exercising dir creation logic
			dst := path.Join(srcDir, "dst", tc.name)
			if err := copy(src, dst); err != nil {
				t.Fatalf("Failed copying: %v", err)
			}

			info, err := os.Stat(dst)
			if err != nil {
				t.Fatalf("Failed stat %q: %v", dst, err)
			}
			if want, got := tc.fileMode, info.Mode(); want != got {
				t.Errorf("File mode mismatch. Want: %s, got: %s", want, got)
			}

			gotContent, err := os.ReadFile(dst)
			if err != nil {
				t.Fatalf("Failed read %q: %v", dst, err)
			}

			if want, got := tc.name+"\nsome\nother\ncontent", string(gotContent); want != got {
				t.Errorf("File content mismatch. Want: %s, got: %s", want, got)
			}
		})
	}
}
