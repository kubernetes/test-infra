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
	"bytes"
	"fmt"
	"io/ioutil"
)

// loadSingleSecret reads and returns the value of a single file.
func loadSingleSecret(path string) ([]byte, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %w", path, err)
	}
	return bytes.TrimSpace(b), nil
}

func loadSingleSecretWithParser[T any](path string, parsingFN func([]byte) (T, error)) ([]byte, T, error) {
	raw, err := loadSingleSecret(path)
	if err != nil {
		return nil, *(new(T)), err
	}
	parsed, err := parsingFN(raw)
	return raw, parsed, err
}
