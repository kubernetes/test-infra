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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/test-infra/prow/cmd/webhook-server/secretmanager"
)

type GCPClient struct {
	client   *secretmanager.Client
	secretID string
}

func newGCPClient(client *secretmanager.Client, secretID string) *GCPClient {
	return &GCPClient{
		client:   client,
		secretID: secretID,
	}
}

func (g *GCPClient) CreateSecret(ctx context.Context, secretID string) error {
	_, err := g.client.CreateSecret(ctx, secretID)
	if err != nil {
		return fmt.Errorf("could not create secret %v", err)
	}
	return nil
}

func (g *GCPClient) AddSecretVersion(ctx context.Context, secretName string, payload []byte) error {
	if err := g.client.AddSecretVersion(ctx, secretName, payload); err != nil {
		return fmt.Errorf("could not add secret data %v", err)
	}
	return nil
}

func (g *GCPClient) GetSecretValue(ctx context.Context, secretName string, versionName string) ([]byte, bool, error) {
	err := g.checkSecret(ctx, secretName)
	if err != nil && err == os.ErrNotExist {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	payload, err := g.client.GetSecretValue(ctx, secretName, versionName)
	if err != nil {
		return nil, false, fmt.Errorf("error getting secret value %v", err)
	}

	return payload, true, nil
}

func (g *GCPClient) checkSecret(ctx context.Context, secretName string) error {
	res, err := g.client.ListSecrets(ctx)
	if err != nil {
		return fmt.Errorf("could not make call to list secrets successfully %v", err)
	}
	for _, secret := range res {
		if strings.Contains(secret.Name, g.secretID) {
			return nil
		}
	}
	return os.ErrNotExist
}

// for integration testing purposes. Not to be used in prod
type localFSClient struct {
	path   string
	expiry int
	dns    []string
}

func NewLocalFSClient(path string, expiry int, dns []string) *localFSClient {
	return &localFSClient{
		path:   path,
		expiry: expiry,
		dns:    dns,
	}
}

func (l *localFSClient) CreateSecret(ctx context.Context, secretID string) error {
	if _, err := os.Stat(l.path); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(l.path, 0755)
		if err != nil {
			return fmt.Errorf("unable to create secret dir %v", err)
		}
	}
	return nil
}

func (l *localFSClient) AddSecretVersion(ctx context.Context, secretName string, payload []byte) error {
	certFile := filepath.Join(l.path, certFile)
	privKeyFile := filepath.Join(l.path, privKeyFile)
	caBundleFile := filepath.Join(l.path, caBundleFile)

	serverCertPerm, serverPrivKey, caPem, _, err := genSecretData(l.expiry, l.dns)
	if err != nil {
		return err
	}
	if err := os.WriteFile(certFile, []byte(serverCertPerm), 0666); err != nil {
		return fmt.Errorf("could not write contents of cert file")
	}
	if err := os.WriteFile(privKeyFile, []byte(serverPrivKey), 0666); err != nil {
		return fmt.Errorf("could not write contents of privkey file")
	}
	if err := os.WriteFile(caBundleFile, []byte(caPem), 0666); err != nil {
		return fmt.Errorf("could not write contents of caBundle file")
	}
	return nil
}

func (l *localFSClient) GetSecretValue(ctx context.Context, secretName string, versionName string) ([]byte, bool, error) {
	err := l.checkSecret(ctx, secretName)
	if err != nil && err == os.ErrNotExist {
		return nil, false, nil
	} else if err != nil {
		return nil, false, err
	}
	secretsMap := make(map[string]string)
	files, err := os.ReadDir(l.path)
	if err != nil {
		return nil, false, fmt.Errorf("could not read file path")
	}
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(l.path, f.Name()))
		if err != nil {
			return nil, false, fmt.Errorf("error reading file %v", err)
		}
		switch f.Name() {
		case certFile:
			secretsMap[certFile] = string(content)
		case privKeyFile:
			secretsMap[privKeyFile] = string(content)
		case caBundleFile:
			secretsMap[caBundleFile] = string(content)
		}
	}
	res, err := json.Marshal(secretsMap)
	if err != nil {
		return nil, false, fmt.Errorf("could not marshal secrets data %v", err)
	}
	return res, true, nil
}

func (l *localFSClient) checkSecret(ctx context.Context, secretName string) error {
	_, err := os.Stat(l.path)
	if err != nil && os.IsNotExist(err) {
		return os.ErrNotExist
	} else if err != nil {
		return err
	}

	files, err := os.ReadDir(l.path)
	if err != nil {
		return err
	}
	if len(files) < 2 {
		return nil
	}
	for _, f := range files {
		_, err := os.ReadFile(filepath.Join(l.path, f.Name()))
		if err != nil {
			return fmt.Errorf("error reading file %v", err)
		}
	}
	return nil
}
