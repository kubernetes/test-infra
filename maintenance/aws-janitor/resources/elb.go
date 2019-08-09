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
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// Clean-up ELBs

type LoadBalancers struct{}

func (LoadBalancers) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	svc := elb.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*loadBalancer // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *elb.DescribeLoadBalancersOutput, _ bool) bool {
		for _, lb := range page.LoadBalancerDescriptions {
			a := &loadBalancer{region: region, account: account, name: *lb.LoadBalancerName}
			if set.Mark(a) {
				klog.Warningf("%s: deleting %T: %v", a.ARN(), lb, lb)
				toDelete = append(toDelete, a)
			}
		}
		return true
	}

	if err := svc.DescribeLoadBalancersPages(&elb.DescribeLoadBalancersInput{}, pageFunc); err != nil {
		return err
	}

	for _, lb := range toDelete {
		deleteInput := &elb.DeleteLoadBalancerInput{
			LoadBalancerName: aws.String(lb.name),
		}

		if _, err := svc.DeleteLoadBalancer(deleteInput); err != nil {
			klog.Warningf("%v: delete failed: %v", lb.ARN(), err)
		}
	}

	return nil
}

func (LoadBalancers) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := elb.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &elb.DescribeLoadBalancersInput{}

	err := c.DescribeLoadBalancersPages(input, func(lbs *elb.DescribeLoadBalancersOutput, isLast bool) bool {
		now := time.Now()
		for _, lb := range lbs.LoadBalancerDescriptions {
			arn := loadBalancer{
				region:  region,
				account: acct,
				name:    *lb.LoadBalancerName,
			}.ARN()
			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe load balancers for %q in %q", acct, region)
}

type loadBalancer struct {
	region  string
	account string
	name    string
}

func (lb loadBalancer) ARN() string {
	return "fakearn:elb:" + lb.region + ":" + lb.account + ":" + lb.name
}

func (lb loadBalancer) ResourceKey() string {
	return lb.ARN()
}
