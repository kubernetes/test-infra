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

// VPCs: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeVpcs

// NATGateway is a VPC component: https://docs.aws.amazon.com/vpc/latest/userguide/vpc-nat-gateway.html
type NATGateway struct{}

// MarkAndSweep looks at the provided set, and removes resources older than its TTL that have been previously tagged.
func (NATGateway) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	inp := &ec2.DescribeNatGatewaysInput{}
	if err := svc.DescribeNatGatewaysPages(inp, func(page *ec2.DescribeNatGatewaysOutput, _ bool) bool {
		for _, gw := range page.NatGateways {
			g := &natGateway{
				Account: acct,
				Region:  region,
				ID:      *gw.NatGatewayId,
			}

			if set.Mark(g) {
				inp := &ec2.DeleteNatGatewayInput{NatGatewayId: gw.NatGatewayId}
				if _, err := svc.DeleteNatGateway(inp); err != nil {
					klog.Warningf("%v: delete failed: %v", g.ARN(), err)
				}
			}
		}
		return true
	}); err != nil {
		return err
	}

	return nil
}

// ListAll populates a set will all available NATGateway resources.
func (NATGateway) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})
	set := NewSet(0)
	inp := &ec2.DescribeNatGatewaysInput{}

	err := svc.DescribeNatGatewaysPages(inp, func(page *ec2.DescribeNatGatewaysOutput, _ bool) bool {
		for _, gw := range page.NatGateways {
			now := time.Now()
			arn := natGateway{
				Account: acct,
				Region:  region,
				ID:      *gw.NatGatewayId,
			}.ARN()

			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe nat gateways for %q in %q", acct, region)
}

type natGateway struct {
	Account string
	Region  string
	ID      string
}

func (ng natGateway) ARN() string {
	return fmt.Sprintf("arn:aws-cn:ec2:%s:%s:natgateway/%s", ng.Region, ng.Account, ng.ID)
}

func (ng natGateway) ResourceKey() string {
	return ng.ARN()
}
