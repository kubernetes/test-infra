/*
Copyright 2016 The Kubernetes Authors.

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
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/jinzhu/gorm"
)

func findLatestIssueUpdate(db *gorm.DB) (time.Time, error) {
	var issue sql.Issue
	issue.IssueUpdatedAt = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

	query := db.Select("issue_updated_at").Order("issue_updated_at desc").First(&issue)
	if !query.RecordNotFound() && query.Error != nil {
		return time.Time{}, query.Error
	}

	return issue.IssueUpdatedAt, nil
}

// UpdateIssues downloads new issues and saves in database
func UpdateIssues(db *gorm.DB, client ClientInterface) {
	latest, err := findLatestIssueUpdate(db)
	if err != nil {
		glog.Error("Failed to find last issue update: ", err)
		return
	}
	c := make(chan *github.Issue, 200)

	go client.FetchIssues(latest, c)
	for issue := range c {
		issueOrm, err := NewIssue(issue)
		if err != nil {
			glog.Error("Can't create issue:", err)
			continue
		}
		if db.Create(issueOrm).Error != nil {
			// If we can't create, let's try update
			// First we need to delete labels, as they are just concatenated
			db.Delete(sql.Label{}, "issue_id = ?", issueOrm.ID)
			db.Save(issueOrm)
		}
		// Issue is updated, find if we have new comments
		UpdateComments(issueOrm.ID, issueOrm.IsPR, db, client)
	}
}
