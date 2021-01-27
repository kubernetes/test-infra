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

package flagutil

import (
	"context"
	"flag"
	"fmt"

	"k8s.io/test-infra/prow/io"
)

type StorageClientOptions struct {
	// GCSCredentialsFile is used for reading/writing to GCS block storage.
	// It's optional, if you want to write to local paths or GCS credentials auto-discovery is used.
	// If set, this file is used to read/write to gs:// paths
	// If not, credential auto-discovery is used
	GCSCredentialsFile string `json:"gcs_credentials_file,omitempty"`
	// S3CredentialsFile is used for reading/writing to s3 block storage.
	// It's optional, if you want to write to local paths or S3 credentials auto-discovery is used.
	// If set, this file is used to read/write to s3:// paths
	// If not, go cloud credential auto-discovery is used
	// For more details see the prow/io/providers pkg.
	S3CredentialsFile string `json:"s3_credentials_file,omitempty"`
}

// AddFlags injects status client options into the given FlagSet.
func (o *StorageClientOptions) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.GCSCredentialsFile, "gcs-credentials-file", "", "File where GCS credentials are stored")
	fs.StringVar(&o.S3CredentialsFile, "s3-credentials-file", "", "File where s3 credentials are stored. For the exact format see https://github.com/kubernetes/test-infra/blob/master/prow/io/providers/providers.go")
}

func (o *StorageClientOptions) HasGCSCredentials() bool {
	return o.GCSCredentialsFile != ""
}

func (o *StorageClientOptions) HasS3Credentials() bool {
	return o.S3CredentialsFile != ""
}

// Validate validates options.
func (o *StorageClientOptions) Validate(dryRun bool) error {
	return nil
}

// StorageClient returns a Storage client.
func (o *StorageClientOptions) StorageClient(ctx context.Context) (io.Opener, error) {
	opener, err := io.NewOpener(ctx, o.GCSCredentialsFile, o.S3CredentialsFile)
	if err != nil {
		message := ""
		if o.GCSCredentialsFile != "" {
			message = fmt.Sprintf(" gcs-credentials-file: %s", o.GCSCredentialsFile)
		}
		if o.S3CredentialsFile != "" {
			message = fmt.Sprintf("%s s3-credentials-file: %s", message, o.S3CredentialsFile)
		}
		return opener, fmt.Errorf("error creating opener%s: %v", message, err)
	}
	return opener, nil
}
