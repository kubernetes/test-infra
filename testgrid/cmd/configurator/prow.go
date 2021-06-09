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
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/testgrid/config"
	"github.com/GoogleCloudPlatform/testgrid/config/yamlcfg"
	configpb "github.com/GoogleCloudPlatform/testgrid/pb/config"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	prowConfig "k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/pjutil"
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
const testgridDaysOfResultsAnnotation = "testgrid-days-of-results"
const testgridInCellMetric = "testgrid-in-cell-metric"
const testGridDisableProwJobAnalysis = "testgrid-disable-prowjob-analysis"
const descriptionAnnotation = "description"
const minPresubmitNumColumnsRecent = 20

// Talk to @michelle192837 if you're thinking about adding more of these!

type prowAwareConfigurator struct {
	prowConfig            *prowConfig.Config
	defaultTestgridConfig *yamlcfg.DefaultConfiguration

	updateDescription bool
	prowJobConfigPath string
	prowJobURLPrefix  string
}

func (pac *prowAwareConfigurator) tabDescriptionForProwJob(j prowConfig.JobBase) string {
	fields := []string{}
	fields = append(fields, fmt.Sprintf("prowjob_name: %v", j.Name))
	if pac.prowJobURLPrefix != "" {
		url := pac.prowJobURLPrefix + strings.TrimPrefix(j.SourcePath, pac.prowJobConfigPath)
		fields = append(fields, fmt.Sprintf("prowjob_config_url: %v", url))
	}
	if d := j.Annotations[descriptionAnnotation]; d != "" {
		fields = append(fields, fmt.Sprintf("prowjob_description: %v", d))
		if !pac.updateDescription {
			return d
		}
	}
	return strings.Join(fields, "\n")
}

func (pac *prowAwareConfigurator) applySingleProwjobAnnotations(c *configpb.Configuration, j prowConfig.JobBase, pj prowapi.ProwJob) error {
	tabName := j.Name
	testGroupName := j.Name
	var repo string
	if pj.Spec.Refs != nil {
		repo = fmt.Sprintf("%s/%s", pj.Spec.Refs.Org, pj.Spec.Refs.Repo)
	}

	pc := pac.prowConfig
	dc := pac.defaultTestgridConfig

	mustMakeGroup := j.Annotations[testgridCreateTestGroupAnnotation] == "true"
	mustNotMakeGroup := j.Annotations[testgridCreateTestGroupAnnotation] == "false"
	dashboards, addToDashboards := j.Annotations[testgridDashboardsAnnotation]
	mightMakeGroup := (mustMakeGroup || addToDashboards || pj.Spec.Type != prowapi.PresubmitJob) && !mustNotMakeGroup
	var testGroup *configpb.TestGroup

	if mightMakeGroup {
		if testGroup = config.FindTestGroup(testGroupName, c); testGroup != nil {
			if mustMakeGroup {
				return fmt.Errorf("test group %q already exists", testGroupName)
			}
		} else {
			var prefix string
			if j.DecorationConfig != nil && j.DecorationConfig.GCSConfiguration != nil {
				prefix = path.Join(j.DecorationConfig.GCSConfiguration.Bucket, j.DecorationConfig.GCSConfiguration.PathPrefix)
			} else if def := pc.Plank.GuessDefaultDecorationConfig(repo, ""); def != nil && def.GCSConfiguration != nil {
				prefix = path.Join(def.GCSConfiguration.Bucket, def.GCSConfiguration.PathPrefix)
			} else {
				return fmt.Errorf("job %s: couldn't figure out a default decoration config", j.Name)
			}

			testGroup = &configpb.TestGroup{
				Name:      testGroupName,
				GcsPrefix: path.Join(prefix, prowGCS.RootForSpec(&downwardapi.JobSpec{Job: j.Name, Type: pj.Spec.Type})),
			}
			if dc != nil {
				yamlcfg.ReconcileTestGroup(testGroup, dc.DefaultTestGroup)
			}
			c.TestGroups = append(c.TestGroups, testGroup)
		}
	} else {
		testGroup = config.FindTestGroup(testGroupName, c)
	}

	if testGroup == nil {
		for _, a := range []string{testgridNumColumnsRecentAnnotation, testgridAlertStaleResultsHoursAnnotation,
			testgridNumFailuresToAlertAnnotation, testgridDaysOfResultsAnnotation, testgridTabNameAnnotation, testgridEmailAnnotation} {
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
	} else if pj.Spec.Type == prowapi.PresubmitJob && testGroup.NumColumnsRecent < minPresubmitNumColumnsRecent {
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

	if dora, ok := j.Annotations[testgridDaysOfResultsAnnotation]; ok {
		doraInt, err := strconv.ParseInt(dora, 10, 32)
		if err != nil {
			return fmt.Errorf("%s value %q is not a valid integer", testgridDaysOfResultsAnnotation, dora)
		}
		testGroup.DaysOfResults = int32(doraInt)
	}

	if stm, ok := j.Annotations[testgridInCellMetric]; ok {
		testGroup.ShortTextMetric = stm
	}

	if dpa, ok := j.Annotations[testGridDisableProwJobAnalysis]; ok {
		dpaBool, err := strconv.ParseBool(dpa)
		if err != nil {
			return fmt.Errorf("%s value %q in not a valid boolean", testGridDisableProwJobAnalysis, dpa)
		}
		testGroup.DisableProwjobAnalysis = dpaBool
	}

	if tn, ok := j.Annotations[testgridTabNameAnnotation]; ok {
		tabName = tn
	}

	description := pac.tabDescriptionForProwJob(j)

	if addToDashboards {
		firstDashboard := true
		for _, dashboardName := range strings.Split(dashboards, ",") {
			dashboardName = strings.TrimSpace(dashboardName)
			d := config.FindDashboard(dashboardName, c)
			if d == nil {
				return fmt.Errorf("couldn't find dashboard %q for job %q", dashboardName, j.Name)
			}
			if repo == "" {
				if len(j.ExtraRefs) > 0 {
					repo = fmt.Sprintf("%s/%s", j.ExtraRefs[0].Org, j.ExtraRefs[0].Repo)
				}
			}
			var codeSearchLinkTemplate, openBugLinkTemplate *configpb.LinkTemplate
			if repo != "" {
				codeSearchLinkTemplate = &configpb.LinkTemplate{
					Url: fmt.Sprintf("https://github.com/%s/compare/<start-custom-0>...<end-custom-0>", repo),
				}
				openBugLinkTemplate = &configpb.LinkTemplate{
					Url: fmt.Sprintf("https://github.com/%s/issues/", repo),
				}
			}

			jobURLPrefix := pac.prowConfig.Plank.GetJobURLPrefix(&pj)
			var openTestLinkTemplate *configpb.LinkTemplate
			if jobURLPrefix != "" {
				openTestLinkTemplate = &configpb.LinkTemplate{
					Url: strings.TrimRight(jobURLPrefix, "/") + "/gs/<gcs_prefix>/<changelist>",
				}
			}

			dt := &configpb.DashboardTab{
				Name:                  tabName,
				TestGroupName:         testGroupName,
				Description:           description,
				CodeSearchUrlTemplate: codeSearchLinkTemplate,
				OpenBugTemplate:       openBugLinkTemplate,
				OpenTestTemplate:      openTestLinkTemplate,
			}
			if firstDashboard {
				firstDashboard = false
				if emails, ok := j.Annotations[testgridEmailAnnotation]; ok {
					dt.AlertOptions = &configpb.DashboardTabAlertOptions{AlertMailToAddresses: emails}
				}
			}
			if dc != nil {
				yamlcfg.ReconcileDashboardTab(dt, dc.DefaultDashboardTab)
			}
			d.DashboardTab = append(d.DashboardTab, dt)
		}
	}

	return nil
}

// sortPeriodics sorts all periodics by name (ascending).
func sortPeriodics(per []prowConfig.Periodic) {
	sort.Slice(per, func(a, b int) bool {
		return per[a].Name < per[b].Name
	})
}

// sortPostsubmits sorts all postsubmits by name and returns a sorted list of org/repos (ascending).
func sortPostsubmits(post map[string][]prowConfig.Postsubmit) []string {
	postRepos := make([]string, 0, len(post))

	for k := range post {
		postRepos = append(postRepos, k)
	}

	sort.Strings(postRepos)

	for _, orgrepo := range postRepos {
		sort.Slice(post[orgrepo], func(a, b int) bool {
			return post[orgrepo][a].Name < post[orgrepo][b].Name
		})
	}

	return postRepos
}

// sortPresubmits sorts all presubmits by name and returns a sorted list of org/repos (ascending).
func sortPresubmits(pre map[string][]prowConfig.Presubmit) []string {
	preRepos := make([]string, 0, len(pre))

	for k := range pre {
		preRepos = append(preRepos, k)
	}

	sort.Strings(preRepos)

	for _, orgrepo := range preRepos {
		sort.Slice(pre[orgrepo], func(a, b int) bool {
			return pre[orgrepo][a].Name < pre[orgrepo][b].Name
		})
	}

	return preRepos
}

func (pac *prowAwareConfigurator) applyProwjobAnnotations(testgridConfig *configpb.Configuration) error {
	if pac.prowConfig == nil {
		return nil
	}
	jobs := pac.prowConfig.JobConfig

	per := jobs.AllPeriodics()
	sortPeriodics(per)
	for _, j := range per {
		pjSpec := pjutil.PeriodicSpec(prowConfig.Periodic{JobBase: j.JobBase})
		pj := pjutil.NewProwJob(pjSpec, nil, nil)
		if err := pac.applySingleProwjobAnnotations(testgridConfig, j.JobBase, pj); err != nil {
			return err
		}
	}

	post := jobs.PostsubmitsStatic
	postReposSorted := sortPostsubmits(post)
	for _, orgrepo := range postReposSorted {
		items := strings.Split(orgrepo, "/")
		for _, j := range post[orgrepo] {
			pjSpec := pjutil.PostsubmitSpec(
				prowConfig.Postsubmit{JobBase: j.JobBase},
				prowapi.Refs{
					Org:  items[0],
					Repo: items[1],
				},
			)
			pj := pjutil.NewProwJob(pjSpec, nil, nil)

			if err := pac.applySingleProwjobAnnotations(testgridConfig, j.JobBase, pj); err != nil {
				return err
			}
		}
	}

	pre := jobs.PresubmitsStatic
	preReposSorted := sortPresubmits(pre)
	for _, orgrepo := range preReposSorted {
		items := strings.Split(orgrepo, "/")
		for _, j := range pre[orgrepo] {
			pjSpec := pjutil.PresubmitSpec(
				prowConfig.Presubmit{JobBase: j.JobBase},
				prowapi.Refs{
					Org:  items[0],
					Repo: items[1],
				},
			)
			pj := pjutil.NewProwJob(pjSpec, nil, nil)
			if err := pac.applySingleProwjobAnnotations(testgridConfig, j.JobBase, pj); err != nil {
				return err
			}
		}
	}

	return nil
}
