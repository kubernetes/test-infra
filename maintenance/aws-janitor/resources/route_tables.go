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

// RouteTables: https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#EC2.DescribeRouteTables

type RouteTables struct{}

func (RouteTables) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	resp, err := svc.DescribeRouteTables(nil)
	if err != nil {
		return err
	}

	for _, rt := range resp.RouteTables {
		// Filter out the RouteTables that have a main
		// association. Given the documentation for the main.association
		// filter, you'd think we could filter on the Describe, but it
		// doesn't actually work, see e.g.
		// https://github.com/aws/aws-cli/issues/1810
		main := false
		for _, assoc := range rt.Associations {
			main = main || *assoc.Main
		}
		if main {
			continue
		}
		r := &routeTable{Account: acct, Region: region, ID: *rt.RouteTableId}
		if set.Mark(r) {
			for _, assoc := range rt.Associations {
				glog.Infof("%v: disassociating from %v", r.ARN(), *assoc.SubnetId)
				_, err := svc.DisassociateRouteTable(&ec2.DisassociateRouteTableInput{
					AssociationId: assoc.RouteTableAssociationId})
				if err != nil {
					glog.Warningf("%v: disassociation from subnet %v failed: %v", r.ARN(), *assoc.SubnetId, err)
				}
			}
			glog.Warningf("%s: deleting %T: %v", r.ARN(), rt, rt)
			_, err := svc.DeleteRouteTable(&ec2.DeleteRouteTableInput{RouteTableId: rt.RouteTableId})
			if err != nil {
				glog.Warningf("%v: delete failed: %v", r.ARN(), err)
			}
		}
	}
	return nil
}

type routeTable struct {
	Account string
	Region  string
	ID      string
}

func (rt routeTable) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:route-table/%s", rt.Region, rt.Account, rt.ID)
}

func (rt routeTable) ResourceKey() string {
	return rt.ARN()
}
