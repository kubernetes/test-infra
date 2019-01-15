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
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/golang/glog"
)

// IAM Instance Profiles
type IAMInstanceProfiles struct{}

func (IAMInstanceProfiles) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := iam.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*iamInstanceProfile // Paged call, defer deletion until we have the whole list.
	if err := svc.ListInstanceProfilesPages(&iam.ListInstanceProfilesInput{}, func(page *iam.ListInstanceProfilesOutput, _ bool) bool {
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
				glog.Infof("ignoring unmanaged profile %s", aws.StringValue(p.Arn))
				continue
			}

			o := &iamInstanceProfile{profile: p}
			if set.Mark(o) {
				glog.Warningf("%s: deleting %T: %v", o.ARN(), o, o)
				toDelete = append(toDelete, o)
			}
		}
		return true
	}); err != nil {
		return err
	}

	for _, o := range toDelete {
		if err := o.delete(svc); err != nil {
			glog.Warningf("%v: delete failed: %v", o.ARN(), err)
		}
	}
	return nil
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
	// We need to unlink the roles first, before we can delete the instance profile
	{
		for _, role := range p.profile.Roles {
			request := &iam.RemoveRoleFromInstanceProfileInput{
				InstanceProfileName: p.profile.InstanceProfileName,
				RoleName:            role.RoleName,
			}
			if _, err := svc.RemoveRoleFromInstanceProfile(request); err != nil {
				return fmt.Errorf("error removing role %q: %v", aws.StringValue(role.RoleName), err)
			}
		}
	}

	// Delete the instance profile
	{
		request := &iam.DeleteInstanceProfileInput{
			InstanceProfileName: p.profile.InstanceProfileName,
		}
		if _, err := svc.DeleteInstanceProfile(request); err != nil {
			return err
		}
	}

	return nil
}
