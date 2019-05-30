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
	"regexp"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// Route53

type Route53ResourceRecordSets struct{}

// zoneIsManaged checks if the zone should be managed (and thus have records deleted) by us
func zoneIsManaged(z *route53.HostedZone) bool {
	// TODO: Move to a tag on the zone?
	name := aws.StringValue(z.Name)
	if "test-cncf-aws.k8s.io." == name {
		return true
	}

	klog.Infof("unknown zone %q; ignoring", name)
	return false
}

var managedNameRegexes = []*regexp.Regexp{
	// e.g. api.e2e-61246-dba53.test-cncf-aws.k8s.io.
	regexp.MustCompile(`^api\.e2e-[0-9]+-`),

	// e.g. api.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.
	regexp.MustCompile(`^api\.internal\.e2e-[0-9]+-`),

	// e.g. etcd-b.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.
	regexp.MustCompile(`^etcd-[a-z]\.internal\.e2e-[0-9]+-`),

	// e.g. etcd-events-b.internal.e2e-61246-dba53.test-cncf-aws.k8s.io.
	regexp.MustCompile(`^etcd-events-[a-z]\.internal\.e2e-[0-9]+-`),
}

// resourceRecordSetIsManaged checks if the resource record should be managed (and thus deleted) by us
func resourceRecordSetIsManaged(rrs *route53.ResourceRecordSet) bool {
	if "A" != aws.StringValue(rrs.Type) {
		return false
	}

	name := aws.StringValue(rrs.Name)

	for _, managedNameRegex := range managedNameRegexes {
		if managedNameRegex.MatchString(name) {
			return true
		}
	}

	klog.Infof("Ignoring unmanaged name %q", name)
	return false
}

func (Route53ResourceRecordSets) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := route53.New(sess, &aws.Config{Region: aws.String(region)})

	var listError error

	pageFunc := func(zones *route53.ListHostedZonesOutput, _ bool) bool {
		for _, z := range zones.HostedZones {
			if !zoneIsManaged(z) {
				continue
			}

			// Because route53 has such low rate limits, we collect the changes per-zone, to minimize API calls

			var toDelete []*route53ResourceRecordSet

			recordsPageFunc := func(records *route53.ListResourceRecordSetsOutput, _ bool) bool {
				for _, rrs := range records.ResourceRecordSets {
					if !resourceRecordSetIsManaged(rrs) {
						continue
					}

					o := &route53ResourceRecordSet{zone: z, obj: rrs}
					if set.Mark(o) {
						klog.Warningf("%s: deleting %T: %v", o.ARN(), rrs, rrs)
						toDelete = append(toDelete, o)
					}
				}
				return true
			}

			err := svc.ListResourceRecordSetsPages(&route53.ListResourceRecordSetsInput{HostedZoneId: z.Id}, recordsPageFunc)
			if err != nil {
				listError = err
				return false
			}

			var changes []*route53.Change
			for _, rrs := range toDelete {
				change := &route53.Change{
					Action:            aws.String(route53.ChangeActionDelete),
					ResourceRecordSet: rrs.obj,
				}

				changes = append(changes, change)
			}

			for len(changes) != 0 {
				// Limit of 1000 changes per request
				chunk := changes
				if len(chunk) > 1000 {
					chunk = chunk[:1000]
					changes = changes[1000:]
				} else {
					changes = nil
				}

				klog.Infof("Deleting %d route53 resource records", len(chunk))
				deleteReq := &route53.ChangeResourceRecordSetsInput{
					HostedZoneId: z.Id,
					ChangeBatch:  &route53.ChangeBatch{Changes: chunk},
				}

				if _, err := svc.ChangeResourceRecordSets(deleteReq); err != nil {
					klog.Warningf("unable to delete DNS records: %v", err)
				}
			}
		}

		return true
	}

	err := svc.ListHostedZonesPages(&route53.ListHostedZonesInput{}, pageFunc)

	if listError != nil {
		return listError
	}

	if err != nil {
		return err
	}

	return nil
}

func (Route53ResourceRecordSets) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := route53.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)

	err := svc.ListHostedZonesPages(&route53.ListHostedZonesInput{}, func(zones *route53.ListHostedZonesOutput, _ bool) bool {
		for _, z := range zones.HostedZones {
			if !zoneIsManaged(z) {
				continue
			}
			inp := &route53.ListResourceRecordSetsInput{HostedZoneId: z.Id}
			err := svc.ListResourceRecordSetsPages(inp, func(recordSets *route53.ListResourceRecordSetsOutput, _ bool) bool {
				now := time.Now()
				for _, recordSet := range recordSets.ResourceRecordSets {
					arn := route53ResourceRecordSet{
						zone: z,
						obj:  recordSet,
					}.ARN()
					set.firstSeen[arn] = now
				}
				return true
			})
			if err != nil {
				klog.Errorf("couldn't describe route53 resources for %q in %q zone %q: %v", acct, region, *z.Id, err)
			}

		}
		return true
	})

	return set, errors.Wrapf(err, "couldn't describe route53 instance profiles for %q in %q", acct, region)

}

type route53ResourceRecordSet struct {
	zone *route53.HostedZone
	obj  *route53.ResourceRecordSet
}

func (r route53ResourceRecordSet) ARN() string {
	return "route53::" + aws.StringValue(r.zone.Id) + "::" + aws.StringValue(r.obj.Type) + "::" + aws.StringValue(r.obj.Name)
}

func (r route53ResourceRecordSet) ResourceKey() string {
	return r.ARN()
}
