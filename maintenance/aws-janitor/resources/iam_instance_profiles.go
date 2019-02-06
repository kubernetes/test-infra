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
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// IAM Instance Profiles
type IAMInstanceProfiles struct{}

func (IAMInstanceProfiles) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := iam.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*iamInstanceProfile // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *iam.ListInstanceProfilesOutput, _ bool) bool {
		for _, p := range page.InstanceProfiles {
			// We treat an instance profile as managed if all its roles are
			managed := true
			if len(p.Roles) == 0 {
				// Just in case...
				managed = false
			}

			for _, r := range p.Roles {
				if !roleIsManaged(r) {
					managed = false
				}
			}

			if !managed {
				klog.Infof("ignoring unmanaged profile %s", aws.StringValue(p.Arn))
				continue
			}

			o := &iamInstanceProfile{profile: p}
			if set.Mark(o) {
				klog.Warningf("%s: deleting %T: %v", o.ARN(), o, o)
				toDelete = append(toDelete, o)
			}
		}
		return true
	}

	if err := svc.ListInstanceProfilesPages(&iam.ListInstanceProfilesInput{}, pageFunc); err != nil {
		return err
	}

	for _, o := range toDelete {
		if err := o.delete(svc); err != nil {
			klog.Warningf("%v: delete failed: %v", o.ARN(), err)
		}
	}
	return nil
}

func (IAMInstanceProfiles) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := iam.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	inp := &iam.ListInstanceProfilesInput{}

	err := svc.ListInstanceProfilesPages(inp, func(profiles *iam.ListInstanceProfilesOutput, _ bool) bool {
		now := time.Now()
		for _, profile := range profiles.InstanceProfiles {
			arn := iamInstanceProfile{
				profile: profile,
			}.ARN()

			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe iam instance profiles for %q in %q", acct, region)
}

type iamInstanceProfile struct {
	profile *iam.InstanceProfile
}

func (p iamInstanceProfile) ARN() string {
	return aws.StringValue(p.profile.Arn)
}

func (p iamInstanceProfile) ResourceKey() string {
	return aws.StringValue(p.profile.InstanceProfileId) + "::" + p.ARN()
}

func (p iamInstanceProfile) delete(svc *iam.IAM) error {
	// Unlink the roles first, before we can delete the instance profile.
	for _, role := range p.profile.Roles {
		request := &iam.RemoveRoleFromInstanceProfileInput{
			InstanceProfileName: p.profile.InstanceProfileName,
			RoleName:            role.RoleName,
		}

		if _, err := svc.RemoveRoleFromInstanceProfile(request); err != nil {
			return fmt.Errorf("error removing role %q: %v", aws.StringValue(role.RoleName), err)
		}
	}

	// Delete the instance profile.
	request := &iam.DeleteInstanceProfileInput{
		InstanceProfileName: p.profile.InstanceProfileName,
	}

	if _, err := svc.DeleteInstanceProfile(request); err != nil {
		return err
	}

	return nil
}
