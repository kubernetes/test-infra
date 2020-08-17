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
	"strconv"
	"strings"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/resultstore"
	"github.com/GoogleCloudPlatform/testgrid/util/gcs"
)

func dur(seconds float64) time.Duration {
	return time.Duration(seconds * float64(time.Second))
}

// convertSuiteMeta converts a junit result in gcs to a ResultStore Suite.
func convertSuiteMeta(suiteMeta gcs.SuitesMeta) resultstore.Suite {
	out := resultstore.Suite{
		Name: path.Base(suiteMeta.Path),
		Files: []resultstore.File{
			{
				ContentType: "text/xml",
				ID:          path.Base(suiteMeta.Path),
				URL:         suiteMeta.Path, // ensure the junit.xml file appears in artifacts list
			},
		},
	}
	for _, suite := range suiteMeta.Suites.Suites {
		child := resultstore.Suite{
			Name:     suite.Name,
			Duration: dur(suite.Time),
		}

		for _, test := range suite.Results {
			if test.Properties != nil {
				for _, p := range test.Properties.PropertyList {
					resultProperty := resultstore.Property{
						Key:   fmt.Sprintf("%s:%s", test.Name, p.Name),
						Value: p.Value,
					}
					child.Properties = append(child.Properties, resultProperty)
				}
			}
		}
		switch {
		case suite.Failures > 0 && suite.Tests >= suite.Failures:
			child.Failures = append(child.Failures, resultstore.Failure{
				Message: fmt.Sprintf("%d out of %d tests failed (%.1f%% passing)", suite.Failures, suite.Tests, float64(suite.Tests-suite.Failures)*100.0/float64(suite.Tests)),
			})
		case suite.Failures > 0:
			child.Failures = append(child.Failures, resultstore.Failure{
				Message: fmt.Sprintf("%d tests failed", suite.Failures),
			})
		}
		for _, result := range suite.Results {
			name, tags := stripTags(result.Name)
			class := result.ClassName
			if class == "" {
				class = strings.Join(tags, " ")
			} else {
				class += " " + strings.Join(tags, " ")
			}
			c := resultstore.Case{
				Name:     strings.TrimSpace(name),
				Class:    strings.TrimSpace(class),
				Duration: dur(result.Time),
				Result:   resultstore.Completed,
			}
			const max = 5000 // truncate messages to this length
			msg := result.Message(max)
			switch {
			case result.Failure != nil:
				// failing tests have a completed result with an error
				if msg == "" {
					msg = "unknown failure"
				}
				c.Failures = append(c.Failures, resultstore.Failure{
					Message: msg,
				})
			case result.Skipped != nil:
				c.Result = resultstore.Skipped
				if msg != "" { // skipped results do not require an error, but may.
					c.Errors = append(c.Errors, resultstore.Error{
						Message: msg,
					})
				}
			}
			child.Cases = append(child.Cases, c)
			if c.Duration > child.Duration {
				child.Duration = c.Duration
			}
		}
		if child.Duration > out.Duration {
			// Assume suites run in parallel, so choose max
			out.Duration = child.Duration
		}
		out.Suites = append(out.Suites, child)
	}
	return out
}

// Convert converts build metadata stored in gcp into the corresponding ResultStore Invocation, Target and Test.
func convert(project, details string, url gcs.Path, result downloadResult, maxFiles int) (resultstore.Invocation, resultstore.Target, resultstore.Test) {
	started := result.started
	finished := result.finished
	artifacts := result.artifactURLs

	basePath := trailingSlash(url.String())
	artifactsPath := basePath + "artifacts/"
	buildLog := basePath + "build-log.txt"
	bucket := url.Bucket()
	jobName := prowJobName(url)

	inv := resultstore.Invocation{
		Project: project,
		Details: details,
		Files: []resultstore.File{
			{
				ID:          resultstore.InvocationLog,
				ContentType: "text/plain",
				URL:         buildLog, // ensure build-log.txt appears as the invocation log
			},
		},
		Properties: []resultstore.Property{
			{
				Key:   "Job",
				Value: jobName,
			},
			{
				Key:   "Pull",
				Value: started.Pull, // may be empty if pull value is not specified
			},
		},
	}

	startedProperties := startedReposToProperties(started.Repos)
	inv.Properties = append(inv.Properties, startedProperties...)

	// Files need a unique identifier, trim the common prefix and provide this.
	seen := map[string]bool{}
	uniqPath := func(s string) string {

		want := strings.TrimPrefix(s, basePath)
		var idx int
		attempt := want
		for {
			if !seen[attempt] {
				seen[attempt] = true
				return attempt
			}
			idx++
			attempt = want + " - " + strconv.Itoa(idx)
		}
	}

	for i, a := range artifacts {
		artifacts[i] = "gs://" + bucket + "/" + a
	}

	var total int
	for _, a := range artifacts { // add started.json, etc to the invocation artifact list.
		if total >= maxFiles {
			continue
		}
		if strings.HasPrefix(a, artifactsPath) {
			continue // things under artifacts/ are owned by the test
		}
		if a == buildLog {
			continue // Handle this in InvocationLog
		}
		total++
		inv.Files = append(inv.Files, resultstore.File{
			ID:          uniqPath(a),
			ContentType: "text/plain",
			URL:         a,
		})
	}
	if started.Timestamp > 0 {
		inv.Start = time.Unix(started.Timestamp, 0)
		if finished.Timestamp != nil {
			inv.Duration = time.Duration(*finished.Timestamp-started.Timestamp) * time.Second
		}
	}

	const day = 24 * 60 * 60
	switch {
	case finished.Timestamp == nil && started.Timestamp < time.Now().Unix()+day:
		inv.Status = resultstore.Running
		inv.Description = "In progress..."
	case finished.Passed != nil && *finished.Passed:
		inv.Status = resultstore.Passed
		inv.Description = "Passed"
	case finished.Timestamp == nil:
		inv.Status = resultstore.Failed
		inv.Description = "Timed out"
	default:
		inv.Status = resultstore.Failed
		inv.Description = "Failed"
	}

	test := resultstore.Test{
		Action: resultstore.Action{
			Node: started.Node,
		},
		Suite: resultstore.Suite{
			Name: "test",
			Files: []resultstore.File{
				{
					ID:          resultstore.TargetLog,
					ContentType: "text/plain",
					URL:         buildLog, // ensure build-log.txt appears as the target log.
				},
			},
		},
	}

	for _, suiteMeta := range result.suiteMetas {
		child := convertSuiteMeta(suiteMeta)
		test.Suite.Suites = append(test.Suite.Suites, child)
		for _, f := range child.Files {
			f.ID = uniqPath(f.URL)
			test.Suite.Files = append(test.Suite.Files, f)
		}
	}

	for _, a := range artifacts {
		if total >= maxFiles {
			continue
		}
		if !strings.HasPrefix(a, artifactsPath) {
			continue // Non-artifacts (started.json, etc) are owned by the invocation
		}
		if a == buildLog {
			continue // Already in the list.
		}
		// TODO(fejta): use set.Strings instead
		var found bool
		for _, sm := range result.suiteMetas {
			if sm.Path == a {
				found = true
				break
			}
		}
		if found {
			continue
		}
		total++
		test.Suite.Files = append(test.Suite.Files, resultstore.File{
			ID:          uniqPath(a),
			ContentType: "text/plain",
			URL:         a,
		})
	}

	if total >= maxFiles {
		// TODO(fejta): expose this to edge case to user in a better way
		inv.Files = append(inv.Files, resultstore.File{
			ID:          fmt.Sprintf("exceeded %d files", maxFiles),
			ContentType: "text/plain",
			URL:         basePath,
		})
	}

	test.Suite.Start = inv.Start
	test.Action.Start = inv.Start
	test.Suite.Duration = inv.Duration
	test.Action.Duration = inv.Duration
	test.Status = inv.Status
	test.Description = inv.Description

	target := resultstore.Target{
		Start:       inv.Start,
		Duration:    inv.Duration,
		Status:      inv.Status,
		Description: inv.Description,
		Properties:  []resultstore.Property{},
	}

	for _, suites := range test.Suite.Suites {
		for _, s := range suites.Suites {
			target.Properties = append(target.Properties, s.Properties...)
		}
	}

	return inv, target, test
}

func startedReposToProperties(gitRepos map[string]string) []resultstore.Property {
	var properties []resultstore.Property

	knownOrg := make(map[string]bool)
	knownBranch := make(map[string]bool)
	for repo, branch := range gitRepos {
		orgRepo := strings.SplitN(repo, "/", 2)
		org := orgRepo[0]
		repoName := ""
		if len(orgRepo) == 2 {
			repoName = orgRepo[1]
		} else {
			repoName = org
		}

		if _, ok := knownOrg[org]; !ok {
			knownOrg[org] = true
			orgName := resultstore.Property{
				Key:   "Org",
				Value: org,
			}
			properties = append(properties, orgName)
		}

		if _, ok := knownBranch[branch]; !ok {
			knownBranch[branch] = true
			branchName := resultstore.Property{
				Key:   "Branch",
				Value: branch,
			}
			properties = append(properties, branchName)
		}

		repos := []resultstore.Property{
			{
				Key:   "Repo",
				Value: repoName,
			},
			{
				Key:   "Repo",
				Value: repo,
			},
			{
				Key:   "Repo",
				Value: fmt.Sprintf("%s:%s", repo, branch),
			},
		}
		properties = append(properties, repos...)
	}
	return properties
}

// prowJobName returns the prow job name parsed from the GCS bucket.
// If parsing fails, it returns an empty string.
// TODO: use prowjob.json when PR 15785 is done.
func prowJobName(url gcs.Path) string {
	paths := strings.Split(strings.TrimSuffix(url.Object(), "/"), "/")
	// Expect the returned split to contain ["logs", <job name>, <uid>]
	if len(paths) < 3 {
		return ""
	}
	return paths[len(paths)-2]
}
