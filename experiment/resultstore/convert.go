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
	"strings"
	"time"

	"k8s.io/test-infra/testgrid/resultstore"
	"k8s.io/test-infra/testgrid/util/gcs"
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
				ID:          resultstore.UUID(),
				URL:         suiteMeta.Path, // ensure the junit.xml file appears in artifacts list
			},
		},
	}
	for _, suite := range suiteMeta.Suites.Suites {
		child := resultstore.Suite{
			Name:     suite.Name,
			Duration: dur(suite.Time),
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
				Name:     name,
				Class:    class,
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
func convert(project, details string, url gcs.Path, result downloadResult) (resultstore.Invocation, resultstore.Target, resultstore.Test) {
	started := result.started
	finished := result.finished
	artifacts := result.artifactURLs

	basePath := trailingSlash(url.String())
	artifactsPath := basePath + "artifacts/"
	buildLog := basePath + "build-log.txt"
	bucket := url.Bucket()
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
	}

	// Files need a unique identifier, trim the common prefix and provide this.
	uniqPath := func(s string) string { return strings.TrimPrefix(s, basePath) }

	for i, a := range artifacts {
		artifacts[i] = "gs://" + bucket + "/" + a
	}

	for _, a := range artifacts { // add started.json, etc to the invocation artifact list.
		if strings.HasPrefix(a, artifactsPath) {
			continue // things under artifacts/ are owned by the test
		}
		if a == buildLog {
			continue // Handle this in InvocationLog
		}
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
		test.Suite.Files = append(test.Suite.Files, child.Files...)
	}
	for _, a := range artifacts {
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
		test.Suite.Files = append(test.Suite.Files, resultstore.File{
			ID:          uniqPath(a),
			ContentType: "text/plain",
			URL:         a,
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
	}

	return inv, target, test
}
