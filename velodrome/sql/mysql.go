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

package sql

import (
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/spf13/cobra"
)

// MySQLConfig is specific to this database
type MySQLConfig struct {
	Host     string
	Port     int
	Db       string
	User     string
	Password string
}

func (config *MySQLConfig) getDSN(db string) string {
	var password string
	if config.Password != "" {
		password = ":" + config.Password
	}

	return fmt.Sprintf("%v%v@tcp(%v:%d)/%s?parseTime=True",
		config.User,
		password,
		config.Host,
		config.Port,
		db)
}

// CreateDatabase for the MySQLConfig
func (config *MySQLConfig) CreateDatabase() (*gorm.DB, error) {
	db, err := gorm.Open("mysql", config.getDSN(""))
	if err != nil {
		return nil, err
	}

	db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %v;", config.Db))
	db.Close()

	db, err = gorm.Open("mysql", config.getDSN(config.Db))
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&Assignee{}, &Issue{}, &IssueEvent{}, &Label{}, &Comment{}).Error
	if err != nil {
		return nil, err
	}

	return db, nil
}

// AddFlags parses options for database configuration
func (config *MySQLConfig) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&config.User, "user", "root", "MySql user")
	cmd.PersistentFlags().StringVar(&config.Password, "password", "", "MySql password")
	cmd.PersistentFlags().StringVar(&config.Host, "host", "localhost", "MySql server IP")
	cmd.PersistentFlags().IntVar(&config.Port, "port", 3306, "MySql server port")
	cmd.PersistentFlags().StringVar(&config.Db, "database", "github", "MySql server database name")
}
