/*
Copyright 2016 The Kubernetes Authors.

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
	"github.com/aws/aws-sdk-go/aws/session"
)

type Type interface {
	// MarkAndSweep queries the resource in a specific region, using
	// the provided session (which has account-number acct), calling
	// res.Mark(<resource>) on each resource and deleting
	// appropriately.
	MarkAndSweep(sess *session.Session, acct string, region string, res *Set) error

	// ListAll queries all the resources this account has access to
	ListAll(sess *session.Session, acct string, region string) (*Set, error)
}

// AWS resource types known to this script, in dependency order.
var RegionalTypeList = []Type{
	CloudFormationStacks{},
	LoadBalancers{},
	AutoScalingGroups{},
	LaunchConfigurations{},
	LaunchTemplates{},
	Instances{},
	// Addresses
	NetworkInterfaces{},
	Subnets{},
	SecurityGroups{},
	// NetworkACLs
	// VPN Connections
	InternetGateways{},
	RouteTables{},
	NATGateway{},
	VPCs{},
	DHCPOptions{},
	Volumes{},
	Addresses{},
}

// Non-regional AWS resource types, in dependency order
var GlobalTypeList = []Type{
	IAMInstanceProfiles{},
	IAMRoles{},
	Route53ResourceRecordSets{},
}
