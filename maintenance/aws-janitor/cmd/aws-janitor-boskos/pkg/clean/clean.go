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

package clean

import (
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
)

func RunOnce(boskos *client.Client) error {
	// Acquire all dirty accounts
	resource, err := boskos.Acquire("aws-account", "dirty", "cleaning")
	if err != nil {
		return errors.Wrap(err, "failed to acquire resource")
	}

	acct, err := accountFromMap(resource.UserData)
	if err != nil {
		return errors.Wrap(err, "couldn't retrieve account")
	}

	// cleatAccount
	cleanAccount(boskos, acct)

	return nil

}

type awsAccount struct{}

func accountFromMap(m *common.UserData) (*awsAccount, error) {
	return nil, errors.New("not implemented")
}

func cleanAccount(boskos *client.Client, account *awsAccount) error {
	// Get account resources

	ac := &session.Session{}
	set := &resources.Set{}

	for _, resource := range resources.RegionalTypeList {
		// retrieve all
		_ = ac
		_ = set
		_ = resource
	}

	for _, resource := range resources.GlobalTypeList {
		// retrieve all
		_ = resource
	}

	// Delete all account resources

	// TODO(regions)
	for _, region := range []string{"us-east-1"} {
		for _, resource := range resources.RegionalTypeList {
			// TODO set
			if err := resource.MarkAndSweep(ac, "", region, set); err != nil {
				// handle error

			}
		}
	}

	for _, resource := range resources.GlobalTypeList {
		// TODO set
		if err := resource.MarkAndSweep(ac, "", "us-east-1", set); err != nil {
			// handle error

		}
	}

	return nil
}
