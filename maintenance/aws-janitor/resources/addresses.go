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

// Elastic IPs: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeAddresses

type Addresses struct{}

func (Addresses) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeAddresses(nil)
	if err != nil {
		return err
	}

	for _, addr := range resp.Addresses {
		a := &address{Account: acct, Region: region, ID: *addr.AllocationId}
		if set.Mark(a) {
			glog.Warningf("%s: deleting %T: %v", a.ARN(), addr, addr)
			if addr.AssociationId != nil {
				glog.Warningf("%s: disassociating %T from active instance", a.ARN(), addr)
				_, err := svc.DisassociateAddress(&ec2.DisassociateAddressInput{AssociationId: addr.AssociationId})
				if err != nil {
					glog.Warningf("%s: disassociating %T failed: %v", a.ARN(), addr, err)
				}
			}
			_, err := svc.ReleaseAddress(&ec2.ReleaseAddressInput{AllocationId: addr.AllocationId})
			if err != nil {
				glog.Warningf("%v: delete failed: %v", a.ARN(), err)
			}
		}
	}
	return nil
}

type address struct {
	Account string
	Region  string
	ID      string
}

func (addr address) ARN() string {
	// This ARN is a complete hallucination - there doesn't seem to be
	// an ARN for elastic IPs.
	return fmt.Sprintf("arn:aws:ec2:%s:%s:address/%s", addr.Region, addr.Account, addr.ID)
}

func (addr address) ResourceKey() string {
	return addr.ARN()
}
