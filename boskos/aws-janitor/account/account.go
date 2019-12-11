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

package account

import (
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
)

func GetAccount(sess *session.Session, region string) (string, error) {
	svc := iam.New(sess, &aws.Config{Region: aws.String(region)})
	resp, err := svc.GetUser(nil)
	if err != nil {
		return "", err
	}
	arn, err := parseARN(*resp.User.Arn)
	if err != nil {
		return "", err
	}
	return arn.account, nil
}

func parseARN(s string) (*arn, error) {
	pieces := strings.Split(s, ":")
	if len(pieces) != 6 || pieces[0] != "arn" || pieces[1] != "aws" {
		return nil, fmt.Errorf("invalid AWS ARN %q", s)
	}
	var resourceType string
	var resource string
	res := strings.SplitN(pieces[5], "/", 2)
	if len(res) == 1 {
		resource = res[0]
	} else {
		resourceType = res[0]
		resource = res[1]
	}
	return &arn{
		partition:    pieces[1],
		service:      pieces[2],
		region:       pieces[3],
		account:      pieces[4],
		resourceType: resourceType,
		resource:     resource,
	}, nil
}

// ARNs (used for uniquifying within our previous mark file)

type arn struct {
	partition    string
	service      string
	region       string
	account      string
	resourceType string
	resource     string
}
