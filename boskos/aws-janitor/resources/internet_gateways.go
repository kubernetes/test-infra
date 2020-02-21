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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/pkg/errors"
	"k8s.io/klog"
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

	// Use a map to tolerate both more than one default vpc
	// (shouldn't happen) as well as no default VPC (not uncommon)
	defaultVPC := make(map[string]bool)
	for _, vpc := range vpcResp.Vpcs {
		defaultVPC[aws.StringValue(vpc.VpcId)] = true
	}

	for _, ig := range resp.InternetGateways {
		i := &internetGateway{Account: acct, Region: region, ID: *ig.InternetGatewayId}

		if set.Mark(i) {
			isDefault := false
			klog.Warningf("%s: deleting %T: %s", i.ARN(), ig, i.ID)

			for _, att := range ig.Attachments {
				if defaultVPC[aws.StringValue(att.VpcId)] {
					isDefault = true
					break
				}

				detachReq := &ec2.DetachInternetGatewayInput{
					InternetGatewayId: ig.InternetGatewayId,
					VpcId:             att.VpcId,
				}

				if _, err := svc.DetachInternetGateway(detachReq); err != nil {
					klog.Warningf("%s: detach from %s failed: %v", i.ARN(), *att.VpcId, err)
				}
			}

			if isDefault {
				klog.Infof("%s: skipping delete as IGW is the default for the VPC %T: %s", i.ARN(), ig, i.ID)
				continue
			}

			deleteReq := &ec2.DeleteInternetGatewayInput{
				InternetGatewayId: ig.InternetGatewayId,
			}

			if _, err := svc.DeleteInternetGateway(deleteReq); err != nil {
				klog.Warningf("%s: delete failed: %v", i.ARN(), err)
			}
		}
	}

	return nil
}

func (InternetGateways) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := ec2.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &ec2.DescribeInternetGatewaysInput{}

	gateways, err := svc.DescribeInternetGateways(input)
	if err != nil {
		return set, errors.Wrapf(err, "couldn't describe internet gateways for %q in %q", acct, region)
	}
	now := time.Now()
	for _, gateway := range gateways.InternetGateways {
		arn := internetGateway{
			Account: acct,
			Region:  region,
			ID:      *gateway.InternetGatewayId,
		}.ARN()
		set.firstSeen[arn] = now
	}

	return set, nil
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
