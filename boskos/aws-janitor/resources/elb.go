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

// Clean-up Classic ELBs

type ClassicLoadBalancers struct{}

func (ClassicLoadBalancers) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	svc := elb.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*classicLoadBalancer // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *elb.DescribeLoadBalancersOutput, _ bool) bool {
		for _, lb := range page.LoadBalancerDescriptions {
			a := &classicLoadBalancer{region: region, account: account, name: *lb.LoadBalancerName, dnsName: *lb.DNSName}
			if set.Mark(a) {
				klog.Warningf("%s: deleting %T: %s", a.ARN(), lb, a.name)
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
			klog.Warningf("%s: delete failed: %v", lb.ARN(), err)
		}
	}

	return nil
}

func (ClassicLoadBalancers) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	c := elb.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	input := &elb.DescribeLoadBalancersInput{}

	err := c.DescribeLoadBalancersPages(input, func(lbs *elb.DescribeLoadBalancersOutput, isLast bool) bool {
		now := time.Now()
		for _, lb := range lbs.LoadBalancerDescriptions {
			arn := classicLoadBalancer{
				region:  region,
				account: acct,
				name:    *lb.LoadBalancerName,
				dnsName: *lb.DNSName,
			}.ARN()
			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe classic load balancers for %q in %q", acct, region)
}

type classicLoadBalancer struct {
	region  string
	account string
	name    string
	dnsName string
}

func (lb classicLoadBalancer) ARN() string {
	return "fakearn:elb:" + lb.region + ":" + lb.account + ":" + lb.dnsName
}

func (lb classicLoadBalancer) ResourceKey() string {
	return lb.ARN()
}
