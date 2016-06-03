/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"reflect"
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type SQLiteConfig struct {
	file string
}

func (config *SQLiteConfig) CreateDatabase() (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", config.file)
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&Issue{}, &IssueEvent{}, &Label{}, &Comment{}).Error
	if err != nil {
		return nil, err
	}

	return db, nil
}

func TestSQLiteCreateDatabase(t *testing.T) {
	config := SQLiteConfig{":memory:"}
	db, err := config.CreateDatabase()
	if err != nil {
		t.Fatal(err)
	}

	issue := Issue{
		ID:             1,
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
	var foundIssue Issue
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
