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

package fakesecretmanager

import (
	"context"
	"fmt"
	"strconv"

	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type FakeClient struct {
	ProjectID string

	SecretMap     map[string]*secretmanagerpb.Secret
	SecretVersion map[string]map[string][]byte

	PreloadedErrors []error
	NextNumber      int
}

func NewFakeClient() *FakeClient {
	return &FakeClient{
		SecretMap:     make(map[string]*secretmanagerpb.Secret),
		SecretVersion: make(map[string]map[string][]byte),
	}
}

func (c *FakeClient) GetProject() string {
	return c.ProjectID
}

func (c *FakeClient) getNextError() error {
	var err error
	if len(c.PreloadedErrors) > 0 {
		err = c.PreloadedErrors[0]
		c.PreloadedErrors = c.PreloadedErrors[1:]
	}
	return err
}

func (c *FakeClient) getNextVersion() string {
	curBestVer := c.NextNumber - 1
	// Override with existing number set by user
	for _, versions := range c.SecretVersion {
		for v := range versions {
			vn, err := strconv.Atoi(v)
			if err == nil && vn > curBestVer {
				curBestVer = vn
			}
		}
	}
	curVer := curBestVer + 1
	c.NextNumber = curVer + 1
	return strconv.Itoa(curVer)
}

func (c *FakeClient) CreateSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error) {
	err := c.getNextError()
	if err != nil {
		return nil, err
	}
	if _, ok := c.SecretMap[secretID]; ok {
		return nil, fmt.Errorf("secret %s already exist", secretID)
	}
	c.SecretMap[secretID] = &secretmanagerpb.Secret{Name: fmt.Sprintf("projects/%s/secrets/%s", c.ProjectID, secretID)}
	return c.SecretMap[secretID], nil
}

func (c *FakeClient) AddSecretVersion(ctx context.Context, secretID string, payload []byte) error {
	err := c.getNextError()
	if err != nil {
		return err
	}
	if _, ok := c.SecretMap[secretID]; !ok {
		return fmt.Errorf("secret %s doesn't exist", secretID)
	}
	if _, ok := c.SecretVersion[secretID]; !ok {
		c.SecretVersion[secretID] = make(map[string][]byte)
	}
	c.SecretVersion[secretID][c.getNextVersion()] = payload
	c.SecretVersion[secretID]["latest"] = payload
	return nil
}

func (c *FakeClient) ListSecrets(ctx context.Context) ([]*secretmanagerpb.Secret, error) {
	err := c.getNextError()
	if err != nil {
		return nil, err
	}
	var res []*secretmanagerpb.Secret
	for _, s := range c.SecretMap {
		res = append(res, s)
	}
	return res, nil
}

func (c *FakeClient) GetSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error) {
	err := c.getNextError()
	if err != nil {
		return nil, err
	}
	if _, ok := c.SecretMap[secretID]; !ok {
		return nil, fmt.Errorf("secret %s doesn't exist", secretID)
	}
	return c.SecretMap[secretID], nil
}

func (c *FakeClient) GetSecretValue(ctx context.Context, secretID, versionName string) ([]byte, error) {
	err := c.getNextError()
	if err != nil {
		return nil, err
	}
	if _, ok := c.SecretMap[secretID]; !ok {
		return nil, fmt.Errorf("secret %s doesn't exist", secretID)
	}
	if _, ok := c.SecretVersion[secretID]; ok {
		return nil, fmt.Errorf("secret version %s doesn't have any version", secretID)
	}
	if _, ok := c.SecretVersion[secretID][versionName]; !ok {
		return nil, fmt.Errorf("secret version %s/%s doesn't exist", secretID, versionName)
	}
	return c.SecretVersion[secretID][versionName], nil
}
