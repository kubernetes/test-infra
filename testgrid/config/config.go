/*
Copyright 2019 The Kubernetes Authors.

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

package config

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"

	"k8s.io/test-infra/testgrid/util/gcs"
)

func read(r io.Reader) (*Configuration, error) {
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %v", err)
	}
	var cfg Configuration
	if err = proto.Unmarshal(buf, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse: %v", err)
	}
	return &cfg, nil
}

// ReadGCS reads the config from gcs and unmarshals it into a Configuration struct.
//
// Configuration is defined in config.Proto
func ReadGCS(ctx context.Context, obj *storage.ObjectHandle) (*Configuration, error) {
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open config: %v", err)
	}
	return read(r)
}

// ReadPath reads the config from the specified local file path.
func ReadPath(path string) (*Configuration, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %v", err)
	}
	return read(f)
}

// Read will read the Configuration proto message from a local or gs:// path.
//
// The ctx and client are only relevant when path refers to GCS.
func Read(path string, ctx context.Context, client *storage.Client) (*Configuration, error) {
	if strings.HasPrefix(path, "gs://") {
		var gcsPath gcs.Path
		if err := gcsPath.Set(path); err != nil {
			return nil, fmt.Errorf("bad gcs path: %v", err)
		}
		return ReadGCS(ctx, client.Bucket(gcsPath.Bucket()).Object(gcsPath.Object()))
	}
	return ReadPath(path)
}

func (cfg Configuration) FindTestGroup(name string) *TestGroup {
	for _, tg := range cfg.TestGroups {
		if tg.Name == name {
			return tg
		}
	}
	return nil
}
