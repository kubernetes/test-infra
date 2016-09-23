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

	"github.com/golang/glog"
	influxdb "github.com/influxdata/influxdb/client/v2"
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
	cmd.PersistentFlags().StringVar(&config.DB, "influx-database", "monitoring", "InfluxDB database name")
}

// CreateDatabaseClient creates and connects a new instance of an InfluxDB
// It is created based on the fields set in the configuration.
func (config *InfluxConfig) CreateDatabaseClient() (*InfluxDB, error) {
	client, err := influxdb.NewHTTPClient(influxdb.HTTPConfig{
		Addr:     config.Host,
		Username: config.User,
		Password: config.Password,
	})
	if err != nil {
		return nil, err
	}

	return &InfluxDB{
		client:   client,
		database: config.DB,
	}, nil
}

// InfluxDB is a connection handler to a Influx database
type InfluxDB struct {
	client   influxdb.Client
	database string
}

// Push a point to the database
func (i *InfluxDB) Push(measurement string, tags map[string]string, fields map[string]interface{}, date time.Time) error {
	batch, err := influxdb.NewBatchPoints(influxdb.BatchPointsConfig{
		Database:  i.database,
		Precision: "s",
	})
	if err != nil {
		return err
	}

	pt, err := influxdb.NewPoint(measurement, tags, fields, date)
	if err != nil {
		return err
	}

	batch.AddPoint(pt)

	err = i.client.Write(batch)
	if err != nil {
		return err
	}
	glog.Infof("Sent to influx: %s %+v %+v %s", measurement, tags, fields, date)

	return nil
}
