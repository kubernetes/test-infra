/*
Copyright 2021 The Kubernetes Authors.

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

package configurator

import (
	"context"
	"os"
	"testing"
	"time"
)

func Test_announceChanges(t *testing.T) {
	tests := []struct {
		name    string
		touch   bool
		delete  bool
		addFile bool
	}{
		{
			name:  "Announce on edit file",
			touch: true,
		},
		{
			name:   "Announce on delete file",
			delete: true,
		},
		{
			name:    "Announce on added file to subdirectory",
			addFile: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()

			file, err := os.CreateTemp(directory, "1*.yaml")
			if err != nil {
				t.Fatalf("Error in creating temporary file: %v", err)
			}

			ctx, cancelFunc := context.WithCancel(context.Background())
			resultChannel := make(chan []string)
			go announceChanges(ctx, []string{directory}, resultChannel)

			initResult := <-resultChannel
			if len(initResult) != 1 && initResult[0] != file.Name() {
				t.Errorf("Unexpected initialization announcement; got %s, expected %s", initResult, []string{file.Name()})
			}

			switch {
			case test.touch:
				if err := os.Chtimes(file.Name(), time.Now().Local(), time.Now().Local()); err != nil {
					t.Fatalf("OS error with touching file")
				}
			case test.delete:
				if err := os.Remove(file.Name()); err != nil {
					t.Fatalf("OS error with deleting file")
				}
			case test.addFile:
				if _, err := os.CreateTemp(directory, "2*.yaml"); err != nil {
					t.Fatalf("OS error with adding new file")
				}
			}

			result := <-resultChannel
			cancelFunc()

			if len(result) != 1 {
				t.Errorf("Unexpected result: got %v, but expected only one result", result)
			}
		})
	}
}
