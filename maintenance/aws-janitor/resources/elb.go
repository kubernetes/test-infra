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
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/elb"
	"github.com/golang/glog"
)

// Clean-up ELBs

type LoadBalancers struct{}

func (LoadBalancers) MarkAndSweep(sess *session.Session, account string, region string, set *Set) error {
	svc := elb.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*loadBalancer // Paged call, defer deletion until we have the whole list.
	if err := svc.DescribeLoadBalancersPages(&elb.DescribeLoadBalancersInput{}, func(page *elb.DescribeLoadBalancersOutput, _ bool) bool {
		for _, lb := range page.LoadBalancerDescriptions {
			a := &loadBalancer{region: region, account: account, name: *lb.LoadBalancerName}
			if set.Mark(a) {
				glog.Warningf("%s: deleting %T: %v", a.ARN(), lb, lb)
				toDelete = append(toDelete, a)
			}
		}
		return true
	}); err != nil {
		return err
	}

	for _, lb := range toDelete {
		_, err := svc.DeleteLoadBalancer(
			&elb.DeleteLoadBalancerInput{
				LoadBalancerName: aws.String(lb.name),
			})
		if err != nil {
			glog.Warningf("%v: delete failed: %v", lb.ARN(), err)
		}
	}
	return nil
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
