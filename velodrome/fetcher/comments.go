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

func findLatestCommentUpdate(db *gorm.DB, repository string) time.Time {
	var comment sql.Comment

	if err := db.Select("comment_updated_at").
		Where("repository = ?", repository).
		Order("comment_updated_at desc").
		First(&comment).Error; err != nil {
		return time.Time{}
	}

	return comment.CommentUpdatedAt
}

func updateIssueComments(latest time.Time, db *gorm.DB, client ClientInterface) {
	c := make(chan *github.IssueComment, 200)

	go client.FetchIssueComments(latest, c)

	for comment := range c {
		commentOrm, err := NewIssueComment(comment, client.RepositoryName())
		if err != nil {
			glog.Error("Failed to create IssueComment: ", err)
			continue
		}
		if db.Create(commentOrm).Error != nil {
			// If we can't create, let's try update
			db.Save(commentOrm)
		}
	}
}

func updatePullComments(latest time.Time, db *gorm.DB, client ClientInterface) {
	c := make(chan *github.PullRequestComment, 200)

	go client.FetchPullComments(latest, c)

	for comment := range c {
		commentOrm, err := NewPullComment(comment, client.RepositoryName())
		if err != nil {
			glog.Error("Failed to create PullComment: ", err)
			continue
		}
		if db.Create(commentOrm).Error != nil {
			// If we can't create, let's try update
			db.Save(commentOrm)
		}
	}
}

// UpdateComments downloads issue and pull-request comments and save in DB
func UpdateComments(db *gorm.DB, client ClientInterface) {
	latest := findLatestCommentUpdate(db, client.RepositoryName())

	updateIssueComments(latest, db, client)
	updatePullComments(latest, db, client)
}
