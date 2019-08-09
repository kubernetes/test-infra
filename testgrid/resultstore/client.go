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

package resultstore

import (
	"context"
	"crypto/x509"
	"fmt"
	"strings"

	"github.com/google/uuid"
	resultstore "google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/metadata"
)

// Connect returns a secure gRPC connection.
//
// Authenticates as the service account if specified otherwise the default user.
func Connect(ctx context.Context, serviceAccountPath string) (*grpc.ClientConn, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("system cert pool: %v", err)
	}
	creds := credentials.NewClientTLSFromCert(pool, "")
	const scope = "https://www.googleapis.com/auth/cloud-platform"
	var perRPC credentials.PerRPCCredentials
	if serviceAccountPath != "" {
		perRPC, err = oauth.NewServiceAccountFromFile(serviceAccountPath, scope)
	} else {
		perRPC, err = oauth.NewApplicationDefault(ctx, scope)
	}
	if err != nil {
		return nil, fmt.Errorf("create oauth: %v", err)
	}
	conn, err := grpc.Dial(
		"resultstore.googleapis.com:443",
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(perRPC),
	)
	if err != nil {
		return nil, fmt.Errorf("dial: %v", err)
	}

	return conn, nil
}

// Secret represents a secret authorization uuid to protect invocations.
type Secret string

// UUID represents a universally "unique" identifier.
func UUID() string {
	return uuid.New().String()
}

// NewSecret returns a new, unique identifier.
func NewSecret() Secret {
	return Secret(UUID())
}

// Client provides ResultStore CRUD methods.
type Client struct {
	up    resultstore.ResultStoreUploadClient
	down  resultstore.ResultStoreDownloadClient
	ctx   context.Context
	token string
}

// NewClient uses the specified gRPC connection to connect to ResultStore.
func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		up:   resultstore.NewResultStoreUploadClient(conn),
		down: resultstore.NewResultStoreDownloadClient(conn),
		ctx:  context.Background(),
	}
}

// WithContext uses the specified context for all RPCs.
func (c *Client) WithContext(ctx context.Context) *Client {
	c.ctx = ctx
	return c
}

// WithSecret applies the specified secret to all requests.
func (c *Client) WithSecret(authorizationToken Secret) *Client {
	c.token = string(authorizationToken)
	return c
}

// Access resources

// Invocations provides Invocation CRUD methods.
func (c Client) Invocations() Invocations {
	return Invocations{
		Client: c,
	}
}

// Configurations provides CRUD methods for an invocation's configurations.
func (c Client) Configurations(invocationName string) Configurations {
	return Configurations{
		Client: c,
		inv:    invocationName,
	}
}

// Targets provides CRUD methods for an invocations's targets.
func (c Client) Targets(invocationName string) Targets {
	return Targets{
		Client: c,
		inv:    invocationName,
	}
}

// ConfiguredTargets provides CRUD methods for a target's configured targets.
func (c Client) ConfiguredTargets(targetName, configID string) ConfiguredTargets {
	return ConfiguredTargets{
		Client: c,
		target: targetName,
		config: configID,
	}
}

// Actions provides CRUD methods for a configured target.
func (c Client) Actions(configuredTargetName string) Actions {
	return Actions{
		Client:           c,
		configuredTarget: configuredTargetName,
	}
}

// Resources

// Invocations client.
type Invocations struct {
	Client
}

// Targets client.
type Targets struct {
	Client
	inv string
}

// Configurations client.
type Configurations struct {
	Client
	inv string
}

// ConfiguredTargets client.
type ConfiguredTargets struct {
	Client
	target string
	config string
}

// Actions client.
type Actions struct {
	Client
	configuredTarget string
}

// Mask methods

// fieldMask is required by gRPC for GET methods.
func fieldMask(ctx context.Context, fields ...string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "X-Goog-FieldMask", strings.Join(fields, ","))
}

// listMask adds the required next_page_token for list requests, as well as any other methods.
func listMask(ctx context.Context, fields ...string) context.Context {
	return fieldMask(ctx, append(fields, "next_page_token")...)
}

// Target methods

// Create a new target with the specified id (target basename), returing the fully qualified path.
func (t Targets) Create(id string, target Target) (string, error) {
	tgt, err := t.up.CreateTarget(t.ctx, &resultstore.CreateTargetRequest{
		Parent:             t.inv,
		TargetId:           id,
		Target:             target.To(),
		AuthorizationToken: t.token,
	})
	if err != nil {
		return "", err
	}
	return tgt.Name, nil
}

// List requested fields in targets, does not currently handle paging.
func (t Targets) List(fields ...string) ([]Target, error) {
	resp, err := t.down.ListTargets(listMask(t.ctx, fields...), &resultstore.ListTargetsRequest{
		Parent: t.inv,
	})
	if err != nil {
		return nil, err
	}
	var targets []Target
	for _, r := range resp.Targets {
		targets = append(targets, fromTarget(r))
	}
	return targets, nil
}

// Configuration methods

const (
	// Default is the expected single-configuration id.
	Default = "default"
)

// Create a new configuration using the specified basename, returning the fully qualified path.
func (c Configurations) Create(id string) (string, error) {
	config, err := c.up.CreateConfiguration(c.ctx, &resultstore.CreateConfigurationRequest{
		Parent:             c.inv,
		ConfigId:           id,
		AuthorizationToken: c.token,
		// Configuration is useless
	})
	if err != nil {
		return "", err
	}
	return config.Name, nil
}

// ConfiguredTarget methods

// Create a new configured target, returning the fully qualified path.
func (ct ConfiguredTargets) Create(act Action) (string, error) {
	resp, err := ct.up.CreateConfiguredTarget(ct.ctx, &resultstore.CreateConfiguredTargetRequest{
		Parent:             ct.target,
		ConfigId:           ct.config,
		AuthorizationToken: ct.token,
		ConfiguredTarget: &resultstore.ConfiguredTarget{
			StatusAttributes: status(act.Status, act.Description),
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

// Action methods

// Create a test action under the specified ID, returning the fully-qualified path.
//
// Technically there are also build actions, but these do not show up in the ResultStore UI.
func (a Actions) Create(id string, test Test) (string, error) {
	resp, err := a.up.CreateAction(a.ctx, &resultstore.CreateActionRequest{
		Parent:             a.configuredTarget,
		ActionId:           id,
		AuthorizationToken: a.token,
		Action:             test.To(),
	})
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

// TestFields represent all fields this client cares about.
var TestFields = [...]string{
	"actions.name",
	"actions.test_action",
	"actions.description",
	"actions.timing",
}

// List tests in this configured target.
func (a Actions) List(fields ...string) ([]Test, error) {
	if len(fields) == 0 {
		fields = TestFields[:]
	}
	resp, err := a.down.ListActions(listMask(a.ctx, fields...), &resultstore.ListActionsRequest{
		Parent: a.configuredTarget,
	})
	if err != nil {
		return nil, err
	}
	var ret []Test
	for _, r := range resp.Actions {
		ret = append(ret, fromTest(r))
	}
	return ret, nil
}

// Invocation methods

// Create a new invocation (project must be specified).
func (i Invocations) Create(inv Invocation) (string, error) {
	resp, err := i.up.CreateInvocation(i.ctx, &resultstore.CreateInvocationRequest{
		Invocation:         inv.To(),
		AuthorizationToken: i.token,
	})
	if err != nil {
		return "", err
	}
	return resp.Name, nil
}

// Update a pre-existing invocation at name.
func (i Invocations) Update(inv Invocation, fields ...string) error {
	_, err := i.up.UpdateInvocation(i.ctx, &resultstore.UpdateInvocationRequest{
		Invocation: inv.To(),
		UpdateMask: &field_mask.FieldMask{
			Paths: fields,
		},
		AuthorizationToken: i.token,
	})
	return err
}

// Finish an invocation, preventing further updates.
func (i Invocations) Finish(name string) error {
	_, err := i.up.FinishInvocation(i.ctx, &resultstore.FinishInvocationRequest{
		Name:               name,
		AuthorizationToken: i.token,
	})
	return err
}

// Get an existing invocation at name.
func (i Invocations) Get(name string, fields ...string) (*Invocation, error) {
	inv, err := i.down.GetInvocation(fieldMask(i.ctx, fields...), &resultstore.GetInvocationRequest{Name: name})
	if err != nil {
		return nil, err
	}
	resp := fromInvocation(inv)
	return &resp, nil
}
