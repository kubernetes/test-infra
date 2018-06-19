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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/test-infra/velodrome/sql"
	"k8s.io/test-infra/velodrome/transform/plugins"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type transformConfig struct {
	InfluxConfig
	sql.MySQLConfig

	repository string
	once       bool
	frequency  int
	metricName string
}

func (config *transformConfig) CheckRootFlags() error {
	if config.repository == "" {
		return fmt.Errorf("repository must be set")
	}
	config.repository = strings.ToLower(config.repository)

	if config.metricName == "" {
		return fmt.Errorf("metric name must be set")
	}

	return nil
}

func (config *transformConfig) AddFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().IntVar(&config.frequency, "frequency", 2, "Number of iterations per hour")
	cmd.PersistentFlags().BoolVar(&config.once, "once", false, "Run once and then leave")
	cmd.PersistentFlags().StringVar(&config.repository, "repository", "", "Repository to use for metrics")
	cmd.PersistentFlags().StringVar(&config.metricName, "name", "", "Name of the metric")
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
}

// Dispatch receives channels to each type of events, and dispatch them to each plugins.
func Dispatch(plugin plugins.Plugin, DB *InfluxDB, issues chan sql.Issue, eventsCommentsChannel chan interface{}) {
	for {
		var points []plugins.Point
		select {
		case issue, ok := <-issues:
			if !ok {
				return
			}
			points = plugin.ReceiveIssue(issue)
		case event, ok := <-eventsCommentsChannel:
			if !ok {
				return
			}
			switch event := event.(type) {
			case sql.IssueEvent:
				points = plugin.ReceiveIssueEvent(event)
			case sql.Comment:
				points = plugin.ReceiveComment(event)
			default:
				glog.Fatal("Received invalid object: ", event)
			}
		}

		for _, point := range points {
			if err := DB.Push(point.Tags, point.Values, point.Date); err != nil {
				glog.Fatal("Failed to push point: ", err)
			}
		}
	}
}

// Plugins constantly wait for new issues/events/comments
func (config *transformConfig) run(plugin plugins.Plugin) error {
	if err := config.CheckRootFlags(); err != nil {
		return err
	}

	mysqldb, err := config.MySQLConfig.CreateDatabase()
	if err != nil {
		return err
	}

	influxdb, err := config.InfluxConfig.CreateDatabase(
		map[string]string{"repository": config.repository},
		config.metricName)
	if err != nil {
		return err
	}

	fetcher := NewFetcher(config.repository)

	// Plugins constantly wait for new issues/events/comments
	go Dispatch(plugin, influxdb, fetcher.IssuesChannel,
		fetcher.EventsCommentsChannel)

	ticker := time.Tick(time.Hour / time.Duration(config.frequency))
	for {
		// Fetch new events from MySQL, push it to plugins
		if err := fetcher.Fetch(mysqldb); err != nil {
			return err
		}
		if err := influxdb.PushBatchPoints(); err != nil {
			return err
		}

		if config.once {
			break
		}
		<-ticker
	}

	return nil
}

func main() {
	config := &transformConfig{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "Transform sql database info into influx stats",
	}
	config.AddFlags(root)
	config.MySQLConfig.AddFlags(root)
	config.InfluxConfig.AddFlags(root)

	root.AddCommand(plugins.NewCountPlugin(config.run))

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
