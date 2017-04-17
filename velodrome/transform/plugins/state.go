/*
Copyright 2017 The Kubernetes Authors.

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

package plugins

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/test-infra/velodrome/sql"
)

type StatePlugin struct {
	states      BundledStates
	desc        string
	percentiles []int
}

var _ Plugin = &StatePlugin{}

func (s *StatePlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&s.desc, "state", "", "Description of the state (eg: `opened,!merged,labeled:cool`)")
	cmd.Flags().IntSliceVar(&s.percentiles, "percentiles", []int{}, "Age percentiles for state")
}

func (s *StatePlugin) CheckFlags() error {
	s.states = NewBundledStates(s.desc)
	return nil
}

func (s *StatePlugin) ReceiveIssue(issue sql.Issue) []Point {
	return nil
}

func (s *StatePlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	label := ""
	if event.Label != nil {
		label = *event.Label
	}

	if !s.states.ReceiveEvent(event.IssueId, event.Event, label, event.EventCreatedAt) {
		return nil
	}

	total, sum := s.states.Total(event.EventCreatedAt)
	values := map[string]interface{}{
		"count": total,
		"sum":   int(sum),
	}
	for _, percentile := range s.percentiles {
		values[fmt.Sprintf("%d%%", percentile)] = int(s.states.Percentile(event.EventCreatedAt, percentile))
	}

	return []Point{
		Point{
			Values: values,
			Date:   event.EventCreatedAt,
		},
	}
}

func (s *StatePlugin) ReceiveComment(comment sql.Comment) []Point {
	return nil
}
