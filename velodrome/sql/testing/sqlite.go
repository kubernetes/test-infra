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
	"k8s.io/test-infra/velodrome/sql"

	"github.com/jinzhu/gorm"
	// SQLite needs to be initialized if you use this module
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

// SQLiteConfig helps you create a SQLite database
type SQLiteConfig struct {
	File string
}

// CreateDatabase the SQLite DB
func (config *SQLiteConfig) CreateDatabase() (*gorm.DB, error) {
	db, err := gorm.Open("sqlite3", config.File)
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(&sql.Assignee{}, &sql.Issue{}, &sql.IssueEvent{}, &sql.Label{}, &sql.Comment{}).Error
	if err != nil {
		return nil, err
	}

	return db, nil
}
