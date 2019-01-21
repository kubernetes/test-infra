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
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/golang/glog"
)

// LaunchConfigurations: http://docs.aws.amazon.com/sdk-for-go/api/service/autoscaling/#AutoScaling.DescribeLaunchConfigurations
type LaunchConfigurations struct{}

func (LaunchConfigurations) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := autoscaling.New(sess, &aws.Config{Region: aws.String(region)})

	var toDelete []*launchConfiguration // Paged call, defer deletion until we have the whole list.
	if err := svc.DescribeLaunchConfigurationsPages(nil, func(page *autoscaling.DescribeLaunchConfigurationsOutput, _ bool) bool {
		for _, lc := range page.LaunchConfigurations {
			l := &launchConfiguration{ID: *lc.LaunchConfigurationARN, Name: *lc.LaunchConfigurationName}
			if set.Mark(l) {
				glog.Warningf("%s: deleting %T: %v", l.ARN(), lc, lc)
				toDelete = append(toDelete, l)
			}
		}
		return true
	}); err != nil {
		return err
	}
	for _, lc := range toDelete {
		_, err := svc.DeleteLaunchConfiguration(
			&autoscaling.DeleteLaunchConfigurationInput{
				LaunchConfigurationName: aws.String(lc.Name),
			})
		if err != nil {
			glog.Warningf("%v: delete failed: %v", lc.ARN(), err)
		}
	}
	return nil
}

type launchConfiguration struct {
	ID   string
	Name string
}

func (lc launchConfiguration) ARN() string {
	return lc.ID
}

func (lc launchConfiguration) ResourceKey() string {
	return lc.ARN()
}
