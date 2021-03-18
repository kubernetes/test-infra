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

package secretmanager

import (
	"context"
	"fmt"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/iterator"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type Client struct {
	// ProjectID is GCP project in which to store secrets in Secret Manager.
	ProjectID string
	client    *secretmanager.Client
}

type ClientInterface interface {
	CreateSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error)
	AddSecretVersion(ctx context.Context, secretName string, payload []byte) error
	ListSecrets(ctx context.Context) ([]*secretmanagerpb.Secret, error)
	GetSecret(ctx context.Context, secretName string) (*secretmanagerpb.Secret, error)
	GetSecretValue(ctx context.Context, secretName, versionName string) ([]byte, error)
}

func NewClient(projectID string) (*Client, error) {
	// Create the client.
	ctx := context.Background()
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to setup client: %v", err)
	}
	return &Client{ProjectID: projectID, client: client}, nil
}

func (c *Client) CreateSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error) {
	// Create the request to create the secret.
	createSecretReq := &secretmanagerpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", c.ProjectID),
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	}

	return c.client.CreateSecret(ctx, createSecretReq)
}

func (c *Client) AddSecretVersion(ctx context.Context, secretName string, payload []byte) error {
	// Build the request.
	addSecretVersionReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: fmt.Sprintf("projects/%s/secrets/%s", c.ProjectID, secretName),
		Payload: &secretmanagerpb.SecretPayload{
			Data: payload,
		},
	}

	// Call the API.
	_, err := c.client.AddSecretVersion(ctx, addSecretVersionReq)
	return err
}

func (c *Client) ListSecrets(ctx context.Context) ([]*secretmanagerpb.Secret, error) {
	var res []*secretmanagerpb.Secret
	// Build the request.
	listRequest := &secretmanagerpb.ListSecretsRequest{
		Parent: fmt.Sprintf("projects/%s", c.ProjectID),
	}

	// Call the API.
	it := c.client.ListSecrets(ctx, listRequest)
	for {
		s, err := it.Next()
		if err != nil {
			if err == iterator.Done {
				break
			}
			return nil, err
		}
		res = append(res, s)
	}
	return res, nil
}

func (c *Client) GetSecret(ctx context.Context, secretName string) (*secretmanagerpb.Secret, error) {
	// Build the request.
	accessRequest := &secretmanagerpb.GetSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s", c.ProjectID, secretName),
	}

	// Call the API.
	return c.client.GetSecret(ctx, accessRequest)
}

func (c *Client) GetSecretValue(ctx context.Context, secretName, versionName string) ([]byte, error) {
	// Build the request.
	accessRequest := &secretmanagerpb.AccessSecretVersionRequest{
		Name: fmt.Sprintf("projects/%s/secrets/%s/versions/%s", c.ProjectID, secretName, versionName),
	}

	// Call the API.
	result, err := c.client.AccessSecretVersion(ctx, accessRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to access secret version: %v", err)
	}
	return result.Payload.Data, nil
}
