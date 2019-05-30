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
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/pkg/errors"
	"k8s.io/klog"
)

// IAM Roles

type IAMRoles struct{}

// roleIsManaged checks if the role should be managed (and thus deleted) by us
// In particular, we want to avoid "system" AWS roles or roles that might support test-infra
func roleIsManaged(role *iam.Role) bool {
	name := aws.StringValue(role.RoleName)
	path := aws.StringValue(role.Path)

	// Most AWS system roles are in a directory called `aws-service-role`
	if strings.HasPrefix(path, "/aws-service-role/") {
		return false
	}

	// kops roles have names start with `masters.` or `nodes.`
	if strings.HasPrefix(name, "masters.") || strings.HasPrefix(name, "nodes.") {
		return true
	}

	klog.Infof("Unknown role name=%q, path=%q; assuming not managed", name, path)
	return false
}

func (IAMRoles) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := iam.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*iamRole // Paged call, defer deletion until we have the whole list.

	pageFunc := func(page *iam.ListRolesOutput, _ bool) bool {
		for _, r := range page.Roles {
			if !roleIsManaged(r) {
				continue
			}

			l := &iamRole{arn: aws.StringValue(r.Arn), roleID: aws.StringValue(r.RoleId), roleName: aws.StringValue(r.RoleName)}
			if set.Mark(l) {
				klog.Warningf("%s: deleting %T: %v", l.ARN(), r, r)
				toDelete = append(toDelete, l)
			}
		}
		return true
	}

	if err := svc.ListRolesPages(&iam.ListRolesInput{}, pageFunc); err != nil {
		return err
	}

	for _, r := range toDelete {
		if err := r.delete(svc); err != nil {
			klog.Warningf("%v: delete failed: %v", r.ARN(), err)
		}
	}

	return nil
}

func (IAMRoles) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := iam.New(sess, aws.NewConfig().WithRegion(region))
	set := NewSet(0)
	inp := &iam.ListRolesInput{}

	err := svc.ListRolesPages(inp, func(roles *iam.ListRolesOutput, _ bool) bool {
		now := time.Now()
		for _, role := range roles.Roles {
			arn := iamRole{
				arn:      aws.StringValue(role.Arn),
				roleID:   aws.StringValue(role.RoleId),
				roleName: aws.StringValue(role.RoleName),
			}.ARN()

			set.firstSeen[arn] = now
		}

		return true
	})

	return set, errors.Wrapf(err, "couldn't describe iam roles for %q in %q", acct, region)
}

type iamRole struct {
	arn      string
	roleID   string
	roleName string
}

func (r iamRole) ARN() string {
	return r.arn
}

func (r iamRole) ResourceKey() string {
	return r.roleID + "::" + r.ARN()
}

func (r iamRole) delete(svc *iam.IAM) error {
	roleName := r.roleName

	var policyNames []string

	rolePoliciesReq := &iam.ListRolePoliciesInput{RoleName: aws.String(roleName)}
	err := svc.ListRolePoliciesPages(rolePoliciesReq, func(page *iam.ListRolePoliciesOutput, lastPage bool) bool {
		for _, policyName := range page.PolicyNames {
			policyNames = append(policyNames, aws.StringValue(policyName))
		}
		return true
	})

	if err != nil {
		return fmt.Errorf("error listing IAM role policies for %q: %v", roleName, err)
	}

	for _, policyName := range policyNames {
		klog.V(2).Infof("Deleting IAM role policy %q %q", roleName, policyName)

		deletePolicyReq := &iam.DeleteRolePolicyInput{
			RoleName:   aws.String(roleName),
			PolicyName: aws.String(policyName),
		}

		if _, err := svc.DeleteRolePolicy(deletePolicyReq); err != nil {
			return fmt.Errorf("error deleting IAM role policy %q %q: %v", roleName, policyName, err)
		}
	}

	klog.V(2).Infof("Deleting IAM role %q", roleName)

	deleteReq := &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}

	if _, err := svc.DeleteRole(deleteReq); err != nil {
		return fmt.Errorf("error deleting IAM role %q: %v", roleName, err)
	}

	return nil
}
