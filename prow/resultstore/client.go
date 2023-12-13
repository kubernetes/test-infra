/*
Copyright 2023 The Kubernetes Authors.

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

	"google.golang.org/genproto/googleapis/devtools/resultstore/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

// TODO: have client connect itself? Move to flagutils?
const ResultStoreAddress = "resultstore.googleapis.com:443"

// Connect returns a ResultStore GRPC client connection.
func Connect(ctx context.Context) (*grpc.ClientConn, error) {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil, fmt.Errorf("system cert pool: %w", err)
	}
	creds := credentials.NewClientTLSFromCert(pool, "")
	const scope = "https://www.googleapis.com/auth/cloud-platform"
	perRPC, err := oauth.NewApplicationDefault(ctx, scope)
	if err != nil {
		return nil, fmt.Errorf("create oauth: %w", err)
	}
	conn, err := grpc.Dial(
		ResultStoreAddress,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(perRPC),
	)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

type Client struct {
	upload resultstore.ResultStoreUploadClient
}

// NewClient returns a new ResultStore client.
func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		upload: resultstore.NewResultStoreUploadClient(conn),
	}
}

func (c *Client) UploadClient() resultstore.ResultStoreUploadClient {
	return c.upload
}
