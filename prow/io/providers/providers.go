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

package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"gocloud.dev/blob"
	_ "gocloud.dev/blob/memblob"
	"gocloud.dev/blob/s3blob"
)

const (
	S3 = "s3"
	GS = "gs"
)

// GetBucket opens and returns a gocloud blob.Bucket based on credentials and a path.
// The path is used to discover which storageProvider should be used.
//
// If the storageProvider file is detected, we don't need any credentials and just open a file bucket
// If no credentials are given, we just fall back to blob.OpenBucket which tries to auto discover credentials
// e.g. via environment variables. For more details, see: https://gocloud.dev/howto/blob/
//
// If we specify credentials and an 3:// path is used, credentials must be given in one of the
// following formats:
// * AWS S3 (s3://):
//    {
//      "region": "us-east-1",
//      "s3_force_path_style": true,
//      "access_key": "access_key",
//      "secret_key": "secret_key"
//    }
// * S3-compatible service, e.g. self-hosted Minio (s3://):
//    {
//      "region": "minio",
//      "endpoint": "https://minio-hl-svc.minio-operator-ns:9000",
//      "s3_force_path_style": true,
//      "access_key": "access_key",
//      "secret_key": "secret_key"
//    }
func GetBucket(ctx context.Context, s3Credentials []byte, path string) (*blob.Bucket, error) {
	storageProvider, bucket, _, err := ParseStoragePath(path)
	if err != nil {
		return nil, err
	}
	if storageProvider == S3 && len(s3Credentials) > 0 {
		return getS3Bucket(ctx, s3Credentials, bucket)
	}

	bkt, err := blob.OpenBucket(ctx, fmt.Sprintf("%s://%s", storageProvider, bucket))
	if err != nil {
		return nil, fmt.Errorf("error opening file bucket: %v", err)
	}
	return bkt, nil
}

// s3Credentials are credentials used to access S3 or an S3-compatible storage service
// Endpoint is an optional property. Default is the AWS S3 endpoint. If set, the specified
// endpoint will be used instead.
type s3Credentials struct {
	Region           string `json:"region"`
	Endpoint         string `json:"endpoint"`
	Insecure         bool   `json:"insecure"`
	S3ForcePathStyle bool   `json:"s3_force_path_style"`
	AccessKey        string `json:"access_key"`
	SecretKey        string `json:"secret_key"`
}

// getS3Bucket opens a gocloud blob.Bucket based on given credentials in the format the
// struct s3Credentials defines (see documentation of GetBucket for an example)
func getS3Bucket(ctx context.Context, creds []byte, bucketName string) (*blob.Bucket, error) {
	s3Creds := &s3Credentials{}
	if err := json.Unmarshal(creds, s3Creds); err != nil {
		return nil, fmt.Errorf("error getting S3 credentials from JSON: %v", err)
	}

	cfg := &aws.Config{}

	//  Use the default credential chain if no credentials are specified
	if s3Creds.AccessKey != "" && s3Creds.SecretKey != "" {
		staticCredentials := credentials.StaticProvider{
			Value: credentials.Value{
				AccessKeyID:     s3Creds.AccessKey,
				SecretAccessKey: s3Creds.SecretKey,
			},
		}

		cfg.Credentials = credentials.NewChainCredentials([]credentials.Provider{&staticCredentials})
	}

	cfg.Endpoint = aws.String(s3Creds.Endpoint)
	cfg.DisableSSL = aws.Bool(s3Creds.Insecure)
	cfg.S3ForcePathStyle = aws.Bool(s3Creds.S3ForcePathStyle)
	cfg.Region = aws.String(s3Creds.Region)

	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("error creating S3 Session: %v", err)
	}

	bkt, err := s3blob.OpenBucket(ctx, sess, bucketName, nil)
	if err != nil {
		return nil, fmt.Errorf("error opening S3 bucket: %v", err)
	}
	return bkt, nil
}

// HasStorageProviderPrefix returns true if the given string starts with
// any of the known storageProviders and a slash, e.g.
// * gs/kubernetes-jenkins returns true
// * kubernetes-jenkins returns false
func HasStorageProviderPrefix(path string) bool {
	return strings.HasPrefix(path, GS+"/") || strings.HasPrefix(path, S3+"/")
}

// ParseStoragePath parses storagePath and returns the storageProvider, bucket and relativePath
// For example gs://prow-artifacts/test.log results in (gs, prow-artifacts, test.log)
// Currently detected storageProviders are GS, S3 and file.
// Paths with a leading / instead of a storageProvider prefix are treated as file paths for backwards
// compatibility reasons.
// File paths are split into a directory and a file. Directory is returned as bucket, file is returned.
// as relativePath.
// For all other paths the first part is treated as storageProvider prefix, the second segment as bucket
// and everything after the bucket as relativePath.
func ParseStoragePath(storagePath string) (storageProvider, bucket, relativePath string, err error) {
	parsedPath, err := url.Parse(storagePath)
	if err != nil {
		return "", "", "", fmt.Errorf("unable to parse path %q: %v", storagePath, err)
	}

	storageProvider = parsedPath.Scheme
	bucket, relativePath = parsedPath.Host, parsedPath.Path
	relativePath = strings.TrimPrefix(relativePath, "/")

	if bucket == "" {
		return "", "", "", fmt.Errorf("could not find bucket in storagePath %q", storagePath)
	}
	return storageProvider, bucket, relativePath, nil
}
