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
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"k8s.io/klog"
	"k8s.io/test-infra/maintenance/aws-janitor/account"
	"k8s.io/test-infra/maintenance/aws-janitor/regions"
)

// CleanAll cleans all of the resources for all of the regions visible to
// the provided AWS session.
func CleanAll(sess *session.Session, region string) error {
	acct, err := account.GetAccount(sess, regions.Default)
	if err != nil {
		return errors.Wrap(err, "Failed to retrieve account")
	}
	klog.V(1).Infof("Account: %s", acct)

	var regionList []string
	if region == "" {
		regionList, err = regions.GetAll(sess)
		if err != nil {
			return errors.Wrap(err, "Couldn't retrieve list of regions")
		}
	} else {
		regionList = []string{region}
	}
	klog.Infof("Regions: %+v", regionList)

	for _, r := range regionList {
		for _, typ := range RegionalTypeList {
			set, err := typ.ListAll(sess, acct, r)
			if err != nil {
				return errors.Wrapf(err, "Failed to list resources of type %T", typ)
			}
			if err := typ.MarkAndSweep(sess, acct, r, set); err != nil {
				return errors.Wrapf(err, "Couldn't sweep resources of type %T", typ)
			}
		}
	}

	for _, typ := range GlobalTypeList {
		set, err := typ.ListAll(sess, acct, regions.Default)
		if err != nil {
			return errors.Wrapf(err, "Failed to list resources of type %T", typ)
		}
		if err := typ.MarkAndSweep(sess, acct, regions.Default, set); err != nil {
			return errors.Wrapf(err, "Couldn't sweep resources of type %T", typ)
		}
	}

	return nil
}
