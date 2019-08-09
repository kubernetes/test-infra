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
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

func TestManagedNames(t *testing.T) {
	grid := []struct {
		rrs      *route53.ResourceRecordSet
		expected bool
	}{
		{
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("api.e2e-61246-dba53.test-cncf-aws.k8s.io.")},
			expected: true,
		},
		{
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("api.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.")},
			expected: true,
		},
		{
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("etcd-b.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.")},
			expected: true,
		},
		{
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("etcd-events-b.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.")},
			expected: true,
		},
		{
			// Ignores non-A records
			rrs:      &route53.ResourceRecordSet{Type: aws.String("CNAME"), Name: aws.String("api.e2e-61246-dba53.test-cncf-aws.k8s.io.")},
			expected: false,
		},
		{
			// Must ignore the hosted zone system records
			rrs:      &route53.ResourceRecordSet{Type: aws.String("NS"), Name: aws.String("test-cncf-aws.k8s.io.")},
			expected: false,
		},
		{
			// Must ignore the hosted zone system records
			rrs:      &route53.ResourceRecordSet{Type: aws.String("SOA"), Name: aws.String("test-cncf-aws.k8s.io.")},
			expected: false,
		},
		{
			// Ignore names that are from tests that reuse cluster names
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("api.e2e-e2e-kops-aws.test-cncf-aws.k8s.io.")},
			expected: false,
		},
		{
			// Ignore arbitrary name
			rrs:      &route53.ResourceRecordSet{Type: aws.String("A"), Name: aws.String("website.test-cncf-aws.k8s.io.")},
			expected: false,
		},
	}
	for _, g := range grid {
		actual := resourceRecordSetIsManaged(g.rrs)
		if actual != g.expected {
			t.Errorf("resource record %+v expected=%t actual=%t", g.rrs, g.expected, actual)
		}
	}
}
