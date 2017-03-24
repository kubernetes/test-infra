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

package testing

import (
	"reflect"
	"testing"
	"time"

	"k8s.io/test-infra/velodrome/sql"
)

func TestSQLiteCreateDatabase(t *testing.T) {
	config := SQLiteConfig{":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal(err)
	}

	issue := sql.Issue{
		ID:             "1",
		Repository:     "Repo",
		Labels:         nil,
		Title:          "Title",
		Body:           "Body",
		User:           "JohnDoe",
		State:          "State",
		Comments:       0,
		IsPR:           true,
		IssueUpdatedAt: time.Date(1900, time.January, 1, 12, 0, 0, 0, time.UTC),
		IssueCreatedAt: time.Date(1900, time.January, 1, 12, 0, 0, 0, time.UTC),
	}

	if err := db.Create(&issue).Error; err != nil {
		t.Fatal(err)
	}
	var foundIssue sql.Issue
	if err := db.First(&foundIssue).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(foundIssue, issue) {
		t.Fatal("FoundIssue:", foundIssue,
			"is different from inserted issue:", issue)
	}
	issue.Body = "Super Body"
	if err := db.Save(&issue).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&foundIssue).Error; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(foundIssue, issue) {
		t.Fatal("FoundIssue:", foundIssue,
			"is different from updated issue:", issue)
	}
}
