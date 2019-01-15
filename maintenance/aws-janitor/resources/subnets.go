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

package resources

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
)

// Subnets: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeSubnets
type Subnets struct{}

func (Subnets) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeSubnets(&ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("defaultForAz"),
				Values: []*string{aws.String("false")},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, sub := range resp.Subnets {
		s := &subnet{Account: acct, Region: region, ID: *sub.SubnetId}
		if set.Mark(s) {
			glog.Warningf("%s: deleting %T: %v", s.ARN(), sub, sub)
			_, err := svc.DeleteSubnet(&ec2.DeleteSubnetInput{SubnetId: sub.SubnetId})
			if err != nil {
				glog.Warningf("%v: delete failed: %v", s.ARN(), err)
			}
		}
	}
	return nil
}

type subnet struct {
	Account string
	Region  string
	ID      string
}

func (sub subnet) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:subnet/%s", sub.Region, sub.Account, sub.ID)
}

func (sub subnet) ResourceKey() string {
	return sub.ARN()
}
