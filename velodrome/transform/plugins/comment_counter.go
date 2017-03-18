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
	"regexp"

	"github.com/spf13/cobra"
	"k8s.io/test-infra/velodrome/sql"
)

type CommentCounterPlugin struct {
	matcher []*regexp.Regexp
	pattern []string
}

var _ Plugin = &CommentCounterPlugin{}

func (c *CommentCounterPlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&c.pattern, "comments", []string{}, "Regexps to match comments")
}

func (c *CommentCounterPlugin) CheckFlags() error {
	for _, pattern := range c.pattern {
		c.matcher = append(c.matcher, regexp.MustCompile(pattern))
	}
	return nil
}

func (CommentCounterPlugin) ReceiveIssue(issue sql.Issue) []Point {
	return nil
}

func (CommentCounterPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	return nil
}

func (c *CommentCounterPlugin) ReceiveComment(comment sql.Comment) []Point {
	points := []Point{}
	for _, matcher := range c.matcher {
		if matcher.MatchString(comment.Body) {
			points = append(points, Point{
				Values: map[string]interface{}{
					"comment": 1,
				},
				Date: comment.CommentCreatedAt,
			})
		}
	}
	return points
}
