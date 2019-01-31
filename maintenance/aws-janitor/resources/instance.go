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

// Instances: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeInstances

type Instances struct{}

func (Instances) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	inp := &ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("instance-state-name"),
				Values: []*string{aws.String("running"), aws.String("pending")},
			},
		},
	}

	var toDelete []*string // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *ec2.DescribeInstancesOutput, _ bool) bool {
		for _, res := range page.Reservations {
			for _, inst := range res.Instances {
				i := &instance{
					Account:    acct,
					Region:     region,
					InstanceID: *inst.InstanceId,
				}

				if set.Mark(i) {
					klog.Warningf("%s: deleting %T: %v", i.ARN(), inst, inst)
					toDelete = append(toDelete, inst.InstanceId)
				}
			}
		}
		return true
	}

	if err := svc.DescribeInstancesPages(inp, pageFunc); err != nil {
		return err
	}

	if len(toDelete) > 0 {
		// TODO(zmerlynn): In theory this should be split up into
		// blocks of 1000, but burn that bridge if it ever happens...
		if _, err := svc.TerminateInstances(&ec2.TerminateInstancesInput{InstanceIds: toDelete}); err != nil {
			klog.Warningf("Termination failed: %v (for %v)", err, toDelete)
		}
	}

	return nil
}

func (Instances) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := ec2.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	inp := &ec2.DescribeInstancesInput{}

	err := svc.DescribeInstancesPages(inp, func(instances *ec2.DescribeInstancesOutput, _ bool) bool {
		for _, res := range instances.Reservations {
			for _, inst := range res.Instances {
				now := time.Now()
				arn := instance{
					Account:    acct,
					Region:     region,
					InstanceID: *inst.InstanceId,
				}.ARN()

				set.firstSeen[arn] = now
			}
		}
		return true

	})

	return set, errors.Wrapf(err, "couldn't describe instances for %q in %q", acct, region)
}

type instance struct {
	Account    string
	Region     string
	InstanceID string
}

func (i instance) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:instance/%s", i.Region, i.Account, i.InstanceID)
}

func (i instance) ResourceKey() string {
	return i.ARN()
}
