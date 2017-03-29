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

func findLatestIssueUpdate(db *gorm.DB, repository string) (time.Time, error) {
	var issue sql.Issue
	issue.IssueUpdatedAt = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

	query := db.
		Select("issue_updated_at").
		Where("repository = ?", repository).
		Order("issue_updated_at desc").
		First(&issue)
	if !query.RecordNotFound() && query.Error != nil {
		return time.Time{}, query.Error
	}

	return issue.IssueUpdatedAt, nil
}

// UpdateIssues downloads new issues and saves in database
func UpdateIssues(db *gorm.DB, client ClientInterface) {
	latest, err := findLatestIssueUpdate(db, client.RepositoryName())
	if err != nil {
		glog.Error("Failed to find last issue update: ", err)
		return
	}
	c := make(chan *github.Issue, 200)

	go client.FetchIssues(latest, c)
	for issue := range c {
		issueOrm, err := NewIssue(issue, client.RepositoryName())
		if err != nil {
			glog.Error("Can't create issue:", err)
			continue
		}
		if db.Create(issueOrm).Error != nil {
			// We assume record already exists. Let's
			// update. First we need to delete labels and
			// assignees, as they are just concatenated
			// otherwise.
			db.Delete(sql.Label{},
				"issue_id = ? AND repository = ?",
				issueOrm.ID, client.RepositoryName())
			db.Delete(sql.Assignee{},
				"issue_id = ? AND repository = ?",
				issueOrm.ID, client.RepositoryName())

			if err := db.Save(issueOrm).Error; err != nil {
				glog.Error("Failed to update database issue: ", err)
			}
		}

		// Issue is updated, find if we have new comments
		UpdateComments(*issue.Number, issueOrm.IsPR, db, client)
		// and find if we have new events
		UpdateIssueEvents(*issue.Number, db, client)
	}
}
