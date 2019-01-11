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

// InternetGateways: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeInternetGateways

type InternetGateways struct{}

func (InternetGateways) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeInternetGateways(nil)
	if err != nil {
		return err
	}

	vpcResp, err := svc.DescribeVpcs(&ec2.DescribeVpcsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("isDefault"),
				Values: []*string{aws.String("true")},
			},
		},
	})
	if err != nil {
		return err
	}

	defaultVpc := vpcResp.Vpcs[0]

	for _, ig := range resp.InternetGateways {
		i := &internetGateway{Account: acct, Region: region, ID: *ig.InternetGatewayId}
		if set.Mark(i) {
			isDefault := false
			glog.Warningf("%s: deleting %T: %v", i.ARN(), ig, ig)
			for _, att := range ig.Attachments {
				if att.VpcId == defaultVpc.VpcId {
					isDefault = true
					break
				}
				_, err := svc.DetachInternetGateway(&ec2.DetachInternetGatewayInput{
					InternetGatewayId: ig.InternetGatewayId,
					VpcId:             att.VpcId,
				})
				if err != nil {
					glog.Warningf("%v: detach from %v failed: %v", i.ARN(), *att.VpcId, err)
				}
			}
			if isDefault {
				glog.Infof("%s: skipping delete as IGW is the default for the VPC %T: %v", i.ARN(), ig, ig)
				continue
			}
			_, err := svc.DeleteInternetGateway(&ec2.DeleteInternetGatewayInput{InternetGatewayId: ig.InternetGatewayId})
			if err != nil {
				glog.Warningf("%v: delete failed: %v", i.ARN(), err)
			}
		}
	}
	return nil
}

type internetGateway struct {
	Account string
	Region  string
	ID      string
}

func (ig internetGateway) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:internet-gateway/%s", ig.Region, ig.Account, ig.ID)
}

func (ig internetGateway) ResourceKey() string {
	return ig.ARN()
}
