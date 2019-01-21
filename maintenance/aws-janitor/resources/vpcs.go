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

// VPCs: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeVpcs

type VPCs struct{}

func (VPCs) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("false")},
			},
		},
	})
	if err != nil {
		return err
	}

	for _, vp := range resp.Vpcs {
		v := &vpc{Account: acct, Region: region, ID: *vp.VpcId}
		if set.Mark(v) {
			glog.Warningf("%s: deleting %T: %v", v.ARN(), vp, vp)
			if vp.DhcpOptionsId != nil && *vp.DhcpOptionsId != "default" {
				_, err := svc.AssociateDhcpOptions(&ec2.AssociateDhcpOptionsInput{
					VpcId:         vp.VpcId,
					DhcpOptionsId: aws.String("default"),
				})
				if err != nil {
					glog.Warningf("%v: disassociating DHCP option set %v failed: %v", v.ARN(), vp.DhcpOptionsId, err)
				}
			}
			_, err := svc.DeleteVpc(&ec2.DeleteVpcInput{VpcId: vp.VpcId})
			if err != nil {
				glog.Warningf("%v: delete failed: %v", v.ARN(), err)
			}
		}
	}
	return nil
}

type vpc struct {
	Account string
	Region  string
	ID      string
}

func (vp vpc) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:vpc/%s", vp.Region, vp.Account, vp.ID)
}

func (vp vpc) ResourceKey() string {
	return vp.ARN()
}
