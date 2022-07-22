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
	"context"
	"errors"
	"fmt"
	"k8s.io/test-infra/prow/flagutil"
	"testing"
)

type secretStore struct {
	store map[string]string
}

type fakeClient struct {
	project secretStore
}

func (f *fakeClient) CreateSecret(ctx context.Context, secretID string) error {
	f.project.store[secretID] = ""
	return nil
}

func (f *fakeClient) AddSecretVersion(ctx context.Context, secretName string, payload []byte) error {
	f.project.store[secretName] = string(payload)
	return nil
}

func (f *fakeClient) GetSecretValue(ctx context.Context, secretName string, versionName string) ([]byte, bool, error) {
	if len(f.project.store) == 0 {
		return nil, false, errors.New("Secret was not created!")
	} else {
		if val, ok := f.project.store[secretName]; ok {
			return []byte(val), true, nil
		} else {
			err := fmt.Sprintf("Secret with name %s was never added", secretName)
			return nil, false, errors.New(err)
		}
	}
}

func (f *fakeClient) CheckSecret(ctx context.Context, secretName string) (bool, error) {
	if len(f.project.store) == 0 {
		return false, errors.New("Secret was not created!")
	} else {
		if _, ok := f.project.store[secretName]; ok {
			return true, nil
		} else {
			err := fmt.Sprintf("Secret with name %s was never added", secretName)
			return false, errors.New(err)
		}
	}
}

func TestCreateSecrets(t *testing.T) {
	dns := flagutil.NewStrings("validation-webhook-service", "validation-webhook-service.default", "validation-webhook-service.default.svc")
	store := make(map[string]string)
	ctx := context.Background()
	f := &fakeClient{
		project: secretStore{
			store: store,
		},
	}
	cert, key, caPem, err := createSecret(f, ctx, 30, dns.Strings())
	if err != nil {
		t.Errorf("Unable to create GCP Secrets %v", err)
	}
	if len(cert) == 0 || len(key) == 0 || len(caPem) == 0 {
		t.Errorf("Issue generating ca certificate")
	}
	if len(store) == 0 {
		t.Errorf("Error populating store")
	}
}
