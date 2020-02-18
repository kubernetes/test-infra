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

// Clean-up ENIs

type NetworkInterfaces struct{}

func (NetworkInterfaces) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*networkInterface // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *ec2.DescribeNetworkInterfacesOutput, _ bool) bool {
		for _, eni := range page.NetworkInterfaces {
			a := &networkInterface{Region: region, Account: account, ID: *eni.NetworkInterfaceId}
			if eni.Attachment != nil {
				a.AttachmentID = *eni.Attachment.AttachmentId
			}
			if set.Mark(a) {
				klog.Warningf("%s: deleting %T", a.ARN(), a)
				toDelete = append(toDelete, a)
			}
		}
		return true
	}

	if err := svc.DescribeNetworkInterfacesPages(&ec2.DescribeNetworkInterfacesInput{}, pageFunc); err != nil {
		return err
	}

	for _, eni := range toDelete {
		if eni.AttachmentID != "" {
			detachInput := &ec2.DetachNetworkInterfaceInput{
				AttachmentId: aws.String(eni.AttachmentID),
			}
			if _, err := svc.DetachNetworkInterface(detachInput); err != nil {
				klog.Warningf("%s: detach failed: %v", eni.ARN(), err)
			}
		}

		deleteInput := &ec2.DeleteNetworkInterfaceInput{
			NetworkInterfaceId: aws.String(eni.ID),
		}

		if _, err := svc.DeleteNetworkInterface(deleteInput); err != nil {
			klog.Warningf("%s: delete failed: %v", eni.ARN(), err)
		}
	}

	return nil
}

func (NetworkInterfaces) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := ec2.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &ec2.DescribeNetworkInterfacesInput{}

	err := c.DescribeNetworkInterfacesPages(input, func(enis *ec2.DescribeNetworkInterfacesOutput, isLast bool) bool {
		now := time.Now()
		for _, eni := range enis.NetworkInterfaces {
			arn := networkInterface{
				Region:  region,
				Account: acct,
				ID:      aws.StringValue(eni.NetworkInterfaceId),
			}.ARN()
			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe network interfaces for %q in %q", acct, region)
}

type networkInterface struct {
	Region       string
	Account      string
	AttachmentID string
	ID           string
}

func (eni networkInterface) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:network-interface/%s", eni.Region, eni.Account, eni.ID)
}

func (eni networkInterface) ResourceKey() string {
	return eni.ARN()
}
