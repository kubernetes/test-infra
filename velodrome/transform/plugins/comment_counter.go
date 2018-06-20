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

// CommentCounterPlugin counts comments
type CommentCounterPlugin struct {
	matcher []*regexp.Regexp
	pattern []string
}

var _ Plugin = &CommentCounterPlugin{}

// AddFlags adds "comments" <comments> to the command help
func (c *CommentCounterPlugin) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&c.pattern, "comments", []string{}, "Regexps to match comments")
}

// CheckFlags looks for comments matching regexes
func (c *CommentCounterPlugin) CheckFlags() error {
	for _, pattern := range c.pattern {
		matcher, err := regexp.Compile(pattern)
		if err != nil {
			return err
		}
		c.matcher = append(c.matcher, matcher)
	}
	return nil
}

// ReceiveIssue is needed to implement a Plugin
func (CommentCounterPlugin) ReceiveIssue(issue sql.Issue) []Point {
	return nil
}

// ReceiveIssueEvent is needed to implement a Plugin
func (CommentCounterPlugin) ReceiveIssueEvent(event sql.IssueEvent) []Point {
	return nil
}

// ReceiveComment adds matching comments to InfluxDB
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
