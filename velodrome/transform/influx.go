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

// CreateDatabase creates and connects a new instance of an InfluxDB
// It is created based on the fields set in the configuration.
func (config *InfluxConfig) CreateDatabase() (*InfluxDB, error) {
	client, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     config.Host,
		Username: config.User,
		Password: config.Password,
	})
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
		client:   client,
		database: config.DB,
		batch:    bp,
	}, err
}

// InfluxDB is a connection handler to a Influx database
type InfluxDB struct {
	client    influxdb.Client
	database  string
	batch     influxdb.BatchPoints
	batchSize int
}

// GetLastMeasurement returns the time of the last measurement pushed to the database
func (i *InfluxDB) GetLastMeasurement(measurement string) (time.Time, error) {
	query := influxdb.Query{
		Command:  fmt.Sprintf("SELECT * FROM %s GROUP BY * ORDER BY time DESC LIMIT 1", measurement),
		Database: i.database,
	}
	response, err := i.client.Query(query)
	if err != nil {
		return time.Time{}, err
	}
	if response.Error() != nil {
		return time.Time{}, response.Error()
	}

	if len(response.Results) == 0 ||
		len(response.Results[0].Series) == 0 ||
		len(response.Results[0].Series[0].Values) == 0 ||
		len(response.Results[0].Series[0].Values[0]) == 0 {
		return time.Time{}, nil
	}

	return time.Parse(time.RFC3339, response.Results[0].Series[0].Values[0][0].(string))
}

// Push a point to the database. This appends to current batchpoint
func (i *InfluxDB) Push(measurement string, tags map[string]string, fields map[string]interface{}, date time.Time) error {
	pt, err := influxdb.NewPoint(measurement, tags, fields, date)
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
