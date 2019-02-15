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

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudformation"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// Clean-up ELBs

// CloudFormationStacks are all created CloudFormation instances in a specific region
// https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/aws-properties-stack.html
type CloudFormationStacks struct{}

// MarkAndSweep looks at the provided set, and removes resources older than its TTL that have been previously tagged.
func (CloudFormationStacks) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	c := cloudformation.New(sess)
	input := &cloudformation.ListStacksInput{}

	err := c.ListStacksPages(input, func(cf *cloudformation.ListStacksOutput, isLast bool) bool {
		for _, stack := range cf.StackSummaries {
			cfs := cloudFormationStack{*stack.StackId}

			if set.Mark(cfs) {
				klog.Warningf("%s: deleting %T: %v", cfs.ARN(), stack, stack)
			}

			dsInput := &cloudformation.DeleteStackInput{StackName: stack.StackName}
			if _, err := c.DeleteStack(dsInput); err != nil {
				klog.Warningf("%v: delete failed: %v", cfs.ARN(), err)
			}
		}

		return true
	})

	return errors.Wrapf(err, "couldn't describe load balancers for %q in %q", account, region)
}

// ListAll populates a set will all available CloudFormation Stack resources.
func (CloudFormationStacks) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := cloudformation.New(sess)
	set := NewSet(0)
	input := &cloudformation.ListStacksInput{}

	err := c.ListStacksPages(input, func(cf *cloudformation.ListStacksOutput, isLast bool) bool {
		now := time.Now()
		for _, stack := range cf.StackSummaries {
			arn := cloudFormationStack{*stack.StackId}.ARN()
			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe load balancers for %q in %q", acct, region)
}

type cloudFormationStack struct {
	id string
}

func (cf cloudFormationStack) ARN() string {
	return cf.id

}

func (cf cloudFormationStack) ResourceKey() string {
	return cf.ARN()
}
