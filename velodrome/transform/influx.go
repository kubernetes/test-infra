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
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang/glog"
	influxdb "github.com/influxdata/influxdb/client/v2"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	"github.com/spf13/cobra"
)

// InfluxConfig creates an InfluxDB
type InfluxConfig struct {
	Host     string
	DB       string
	User     string
	Password string
}

// AddFlags parses options for database configuration
func (config *InfluxConfig) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&config.User, "influx-user", "root", "InfluxDB user")
	cmd.PersistentFlags().StringVar(&config.Password, "influx-password", "", "InfluxDB password")
	cmd.PersistentFlags().StringVar(&config.Host, "influx-host", "http://localhost:8086", "InfluxDB http server")
	cmd.PersistentFlags().StringVar(&config.DB, "influx-database", "github", "InfluxDB database name")
}

func dropSeries(client influxdb.Client, measurement, database string, tags map[string]string) error {
	query := influxdb.Query{
		Command:  fmt.Sprintf(`DROP SERIES FROM %s %s`, measurement, tagsToWhere(tags)),
		Database: database,
	}
	_, err := client.Query(query)
	return err
}

// CreateDatabase creates and connects a new instance of an InfluxDB
// It is created based on the fields set in the configuration.
func (config *InfluxConfig) CreateDatabase(tags map[string]string, measurement string) (*InfluxDB, error) {
	client, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     config.Host,
		Username: config.User,
		Password: config.Password,
	})
	if err != nil {
		return nil, err
	}

	err = dropSeries(client, measurement, config.DB, tags)
	if err != nil {
		return nil, err
	}

	bp, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  config.DB,
		Precision: "s",
	})
	if err != nil {
		return nil, err
	}

	return &InfluxDB{
		client:      client,
		database:    config.DB,
		batch:       bp,
		tags:        tags,
		measurement: measurement,
	}, err
}

// InfluxDB is a connection handler to a Influx database
type InfluxDB struct {
	client      influxdb.Client
	database    string
	measurement string
	batch       influxdb.BatchPoints
	batchSize   int
	tags        map[string]string
}

// mergeTags merges the default tags with the exta tags. Default will be overridden if it conflicts.
func mergeTags(defaultTags, extraTags map[string]string) map[string]string {
	newTags := map[string]string{}

	for k, v := range defaultTags {
		newTags[k] = v
	}
	for k, v := range extraTags {
		newTags[k] = v
	}

	return newTags
}

// tagsToWhere creates a where query to match tags element
func tagsToWhere(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}

	sortedKeys := []string{}
	for k := range tags {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	conditions := []string{}
	for _, key := range sortedKeys {
		conditions = append(conditions, fmt.Sprintf(`"%s" = '%v'`, key, tags[key]))
	}
	return "WHERE " + strings.Join(conditions, " AND ")
}

// Push a point to the database. This appends to current batchpoint
func (i *InfluxDB) Push(tags map[string]string, fields map[string]interface{}, date time.Time) error {
	pt, err := influxdb.NewPoint(i.measurement, mergeTags(i.tags, tags), fields, date)
	if err != nil {
		return err
	}

	i.batch.AddPoint(pt)
	i.batchSize++

	return nil
}

// PushBatchPoints pushes the batch points (for real)
func (i *InfluxDB) PushBatchPoints() error {
	// Push
	err := i.client.Write(i.batch)
	if err != nil {
		return err
	}
	glog.Infof("Sent to influx: %d points", i.batchSize)

	// Recreate new batch
	i.batch, err = influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  i.database,
		Precision: "s",
	})
	i.batchSize = 0

	if err != nil {
		return err
	}

	return nil
}
