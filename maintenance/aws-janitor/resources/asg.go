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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// AutoScalingGroups: https://docs.aws.amazon.com/sdk-for-go/api/service/autoscaling/#AutoScaling.DescribeAutoScalingGroups

type AutoScalingGroups struct{}

func (AutoScalingGroups) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := autoscaling.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*autoScalingGroup // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *autoscaling.DescribeAutoScalingGroupsOutput, _ bool) bool {
		for _, asg := range page.AutoScalingGroups {
			a := &autoScalingGroup{ID: *asg.AutoScalingGroupARN, Name: *asg.AutoScalingGroupName}
			if set.Mark(a) {
				klog.Warningf("%s: deleting %T: %v", a.ARN(), asg, asg)
				toDelete = append(toDelete, a)
			}
		}
		return true
	}

	if err := svc.DescribeAutoScalingGroupsPages(&autoscaling.DescribeAutoScalingGroupsInput{}, pageFunc); err != nil {
		return err
	}

	for _, asg := range toDelete {
		deleteInput := &autoscaling.DeleteAutoScalingGroupInput{
			AutoScalingGroupName: aws.String(asg.Name),
			ForceDelete:          aws.Bool(true),
		}

		if _, err := svc.DeleteAutoScalingGroup(deleteInput); err != nil {
			klog.Warningf("%v: delete failed: %v", asg.ARN(), err)
		}
	}

	// Block on ASGs finishing deletion. There are a lot of dependent
	// resources, so this just makes the rest go more smoothly (and
	// prevents a second pass).
	for _, asg := range toDelete {
		klog.Warningf("%v: waiting for delete", asg.ARN())

		describeInput := &autoscaling.DescribeAutoScalingGroupsInput{
			AutoScalingGroupNames: []*string{aws.String(asg.Name)},
		}

		if err := svc.WaitUntilGroupNotExists(describeInput); err != nil {
			klog.Warningf("%v: wait failed: %v", asg.ARN(), err)
		}
	}

	return nil
}

func (AutoScalingGroups) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := autoscaling.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &autoscaling.DescribeAutoScalingGroupsInput{}

	err := c.DescribeAutoScalingGroupsPages(input, func(asgs *autoscaling.DescribeAutoScalingGroupsOutput, isLast bool) bool {
		now := time.Now()
		for _, asg := range asgs.AutoScalingGroups {
			arn := autoScalingGroup{
				ID:   *asg.AutoScalingGroupARN,
				Name: *asg.AutoScalingGroupName,
			}.ARN()
			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe auto scaling groups for %q in %q", acct, region)
}

type autoScalingGroup struct {
	ID   string
	Name string
}

func (asg autoScalingGroup) ARN() string {
	return asg.ID
}

func (asg autoScalingGroup) ResourceKey() string {
	return asg.ARN()
}
