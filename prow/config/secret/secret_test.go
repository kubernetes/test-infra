/*
Copyright 2018 The Kubernetes Authors.

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

package secret

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadSingleSecret(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []byte
		wantErr string
	}{
		{"valid token", `121f3cb3e7f70feeb35f9204f5a988d7292c7ba1`, []byte("121f3cb3e7f70feeb35f9204f5a988d7292c7ba1"), ""},
		{"valid token with surrounding whitespace", ` 121f3cb3e7f70feeb35f9204f5a988d7292c7ba1
`, []byte("121f3cb3e7f70feeb35f9204f5a988d7292c7ba1"), ""},
		{"token containing linesbreak", `121f3cb3e7f70feeb35f
9204f5a988d7292c7ba1`, nil, "invalid token format"},
	}

	// Creating a temporary directory.
	secretDir, err := ioutil.TempDir("", "secretDir")
	if err != nil {
		t.Fatalf("fail to create a temporary directory: %v", err)
	}
	defer os.RemoveAll(secretDir)
	tempSecret := filepath.Join(secretDir, "tempSecret")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ioutil.WriteFile(tempSecret, []byte(tt.content), 0666); err != nil {
				t.Fatalf("fail to write secret: %v", err)
			}
			got, err := LoadSingleSecret(tempSecret)
			if (err != nil) != (tt.wantErr != "") || err != nil && !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("LoadSingleSecret() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("LoadSingleSecret() got = %v, want %v", got, tt.want)
			}
		})
	}
}
