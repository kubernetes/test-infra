/*
Copyright 2020 The Kubernetes Authors.

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
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// Clean-up ELBs

type LoadBalancers struct{}

func (LoadBalancers) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	svc := elbv2.New(sess, aws.NewConfig().WithRegion(region))

	var toDelete []*loadBalancer // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *elbv2.DescribeLoadBalancersOutput, _ bool) bool {
		for _, lb := range page.LoadBalancers {
			a := &loadBalancer{arn: *lb.LoadBalancerArn}
			if set.Mark(a) {
				klog.Warningf("%s: deleting %T: %s", a.ARN(), lb, *lb.LoadBalancerName)
				toDelete = append(toDelete, a)
			}
		}
		return true
	}

	if err := svc.DescribeLoadBalancersPages(&elbv2.DescribeLoadBalancersInput{}, pageFunc); err != nil {
		return err
	}

	for _, lb := range toDelete {
		deleteInput := &elbv2.DeleteLoadBalancerInput{
			LoadBalancerArn: aws.String(lb.ARN()),
		}

		if _, err := svc.DeleteLoadBalancer(deleteInput); err != nil {
			klog.Warningf("%s: delete failed: %v", lb.ARN(), err)
		}
	}

	return nil
}

func (LoadBalancers) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := elbv2.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &elbv2.DescribeLoadBalancersInput{}

	err := c.DescribeLoadBalancersPages(input, func(lbs *elbv2.DescribeLoadBalancersOutput, isLast bool) bool {
		now := time.Now()
		for _, lb := range lbs.LoadBalancers {
			a := &loadBalancer{arn: *lb.LoadBalancerArn}
			set.firstSeen[a.ResourceKey()] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe load balancers for %q in %q", acct, region)
}

type loadBalancer struct {
	arn string
}

func (lb loadBalancer) ARN() string {
	return lb.arn
}

func (lb loadBalancer) ResourceKey() string {
	return lb.ARN()
}
