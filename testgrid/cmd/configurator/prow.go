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

package main

import (
	"fmt"
	"k8s.io/test-infra/testgrid/config"
	"path"
	"strconv"
	"strings"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowConfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"
	prowGCS "k8s.io/test-infra/prow/pod-utils/gcs"
)

const testgridCreateTestGroupAnnotation = "testgrid-create-test-group"
const testgridDashboardsAnnotation = "testgrid-dashboards"
const testgridTabNameAnnotation = "testgrid-tab-name"
const testgridEmailAnnotation = "testgrid-alert-email"
const testgridNumColumnsRecentAnnotation = "testgrid-num-columns-recent"
const testgridAlertStaleResultsHoursAnnotation = "testgrid-alert-stale-results-hours"
const testgridNumFailuresToAlertAnnotation = "testgrid-num-failures-to-alert"
const descriptionAnnotation = "description"
const minPresubmitNumColumnsRecent = 20

// Talk to @michelle192837 if you're thinking about adding more of these!

func applySingleProwjobAnnotations(c *Config, pc *prowConfig.Config, j prowConfig.JobBase, jobType prowapi.ProwJobType, repo string) error {
	tabName := j.Name
	testGroupName := j.Name
	description := j.Name

	mustMakeGroup := j.Annotations[testgridCreateTestGroupAnnotation] == "true"
	mustNotMakeGroup := j.Annotations[testgridCreateTestGroupAnnotation] == "false"
	dashboards, addToDashboards := j.Annotations[testgridDashboardsAnnotation]
	mightMakeGroup := (mustMakeGroup || addToDashboards || jobType != prowapi.PresubmitJob) && !mustNotMakeGroup
	var testGroup *config.TestGroup

	if mightMakeGroup {
		if testGroup = c.config.FindTestGroup(testGroupName); testGroup != nil {
			if mustMakeGroup {
				return fmt.Errorf("test group %q already exists", testGroupName)
			}
		} else {
			var prefix string
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				prefix = path.Join(j.DecorationConfig.GCSConfiguration.Bucket, j.DecorationConfig.GCSConfiguration.PathPrefix)
			} else if pc.Plank.DefaultDecorationConfig != nil && pc.Plank.DefaultDecorationConfig.GCSConfiguration != nil {
				prefix = path.Join(pc.Plank.DefaultDecorationConfig.GCSConfiguration.Bucket, pc.Plank.DefaultDecorationConfig.GCSConfiguration.PathPrefix)
			} else {
				return fmt.Errorf("job %s: couldn't figure out a default decoration config", j.Name)
			}

			testGroup = &config.TestGroup{
				Name:      testGroupName,
				GcsPrefix: path.Join(prefix, prowGCS.RootForSpec(&downwardapi.JobSpec{Job: j.Name, Type: jobType})),
			}
			if c.defaultConfig != nil {
				ReconcileTestGroup(testGroup, c.defaultConfig.DefaultTestGroup)
			}
			c.config.TestGroups = append(c.config.TestGroups, testGroup)
		}
	} else {
		testGroup = c.config.FindTestGroup(testGroupName)
	}

	if testGroup == nil {
		for _, a := range []string{testgridNumColumnsRecentAnnotation, testgridAlertStaleResultsHoursAnnotation,
			testgridNumFailuresToAlertAnnotation, testgridTabNameAnnotation, testgridEmailAnnotation} {
			_, ok := j.Annotations[a]
			if ok {
				return fmt.Errorf("no testgroup exists for job %q, but annotation %q implies one should exist", j.Name, a)
			}
		}
		// exit early: with no test group, there's nothing else for us to usefully do with the job.
		return nil
	}

	if ncr, ok := j.Annotations[testgridNumColumnsRecentAnnotation]; ok {
		ncrInt, err := strconv.ParseInt(ncr, 10, 32)
		if err != nil {
			return fmt.Errorf("%s value %q is not a valid integer", testgridNumColumnsRecentAnnotation, ncr)
		}
		testGroup.NumColumnsRecent = int32(ncrInt)
	} else if jobType == prowapi.PresubmitJob && testGroup.NumColumnsRecent < minPresubmitNumColumnsRecent {
		testGroup.NumColumnsRecent = minPresubmitNumColumnsRecent
	}

	if srh, ok := j.Annotations[testgridAlertStaleResultsHoursAnnotation]; ok {
		srhInt, err := strconv.ParseInt(srh, 10, 32)
		if err != nil {
			return fmt.Errorf("%s value %q is not a valid integer", testgridAlertStaleResultsHoursAnnotation, srh)
		}
		testGroup.AlertStaleResultsHours = int32(srhInt)
	}

	if nfta, ok := j.Annotations[testgridNumFailuresToAlertAnnotation]; ok {
		nftaInt, err := strconv.ParseInt(nfta, 10, 32)
		if err != nil {
			return fmt.Errorf("%s value %q is not a valid integer", testgridNumFailuresToAlertAnnotation, nfta)
		}
		testGroup.NumFailuresToAlert = int32(nftaInt)
	}

	if tn, ok := j.Annotations[testgridTabNameAnnotation]; ok {
		tabName = tn
	}
	if d := j.Annotations[descriptionAnnotation]; d != "" {
		description = d
	}

	if addToDashboards {
		firstDashboard := true
		for _, dashboardName := range strings.Split(dashboards, ",") {
			dashboardName = strings.TrimSpace(dashboardName)
			d := c.config.FindDashboard(dashboardName)
			if d == nil {
				return fmt.Errorf("couldn't find dashboard %q for job %q", dashboardName, j.Name)
			}
			if repo == "" {
				if len(j.ExtraRefs) > 0 {
					repo = fmt.Sprintf("%s/%s", j.ExtraRefs[0].Org, j.ExtraRefs[0].Repo)
				}
			}
			var linkTemplate *config.LinkTemplate
			if repo != "" {
				linkTemplate = &config.LinkTemplate{
					Url: fmt.Sprintf("https://github.com/%s/compare/<start-custom-0>...<end-custom-0>", repo),
				}
			}
			dt := &config.DashboardTab{
				Name:                  tabName,
				TestGroupName:         testGroupName,
				Description:           description,
				CodeSearchUrlTemplate: linkTemplate,
			}
			if firstDashboard {
				firstDashboard = false
				if emails, ok := j.Annotations[testgridEmailAnnotation]; ok {
					dt.AlertOptions = &config.DashboardTabAlertOptions{AlertMailToAddresses: emails}
				}
			}
			if c.defaultConfig != nil {
				ReconcileDashboardTab(dt, c.defaultConfig.DefaultDashboardTab)
			}
			d.DashboardTab = append(d.DashboardTab, dt)
		}
	}

	return nil
}

func applyProwjobAnnotations(c *Config, prowConfigAgent *prowConfig.Agent) error {
	pc := prowConfigAgent.Config()
	if pc == nil {
		return nil
	}
	jobs := prowConfigAgent.Config().JobConfig
	for _, j := range jobs.AllPeriodics() {
		if err := applySingleProwjobAnnotations(c, pc, j.JobBase, prowapi.PeriodicJob, ""); err != nil {
			return err
		}
	}

	for repo, js := range jobs.Postsubmits {
		for _, j := range js {
			if err := applySingleProwjobAnnotations(c, pc, j.JobBase, prowapi.PostsubmitJob, repo); err != nil {
				return err
			}
		}
	}

	for repo, js := range jobs.Presubmits {
		for _, j := range js {
			if err := applySingleProwjobAnnotations(c, pc, j.JobBase, prowapi.PresubmitJob, repo); err != nil {
				return err
			}
		}
	}
	return nil
}
