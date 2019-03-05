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

package regions

import (
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// Default is the region we use when no region is applicable
var Default string

func init() {
	if Default = os.Getenv("AWS_DEFAULT_REGION"); Default == "" {
		Default = "us-east-1"
	}
}

// GetAll retrieves all regions from the AWS API
func GetAll(sess *session.Session) ([]string, error) {
	var regions []string
	svc := ec2.New(sess, &aws.Config{Region: aws.String(Default)})
	resp, err := svc.DescribeRegions(nil)
	if err != nil {
		return nil, err
	}
	for _, region := range resp.Regions {
		regions = append(regions, *region.RegionName)
	}
	return regions, nil
}
