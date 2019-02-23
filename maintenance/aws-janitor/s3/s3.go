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

package s3

import (
	"fmt"
	"net/url"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"

	"k8s.io/test-infra/maintenance/aws-janitor/regions"
)

type Path struct {
	Region string
	Bucket string
	Key    string
}

func GetPath(sess *session.Session, s string) (*Path, error) {
	url, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	if url.Scheme != "s3" {
		return nil, fmt.Errorf("Scheme %q != 's3'", url.Scheme)
	}

	svc := s3.New(sess, &aws.Config{Region: aws.String(regions.Default)})

	resp, err := svc.GetBucketLocation(&s3.GetBucketLocationInput{Bucket: aws.String(url.Host)})
	if err != nil {
		return nil, err
	}

	region := regions.Default
	if resp.LocationConstraint != nil {
		region = *resp.LocationConstraint
	}

	return &Path{Region: region, Bucket: url.Host, Key: url.Path}, nil
}
