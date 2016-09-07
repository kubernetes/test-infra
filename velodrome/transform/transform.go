package main

import (
	"flag"
	"os"
	"path/filepath"
	"time"

	"k8s.io/test-infra/velodrome/sql"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

type transformConfig struct {
	InfluxConfig
	sql.MySQLConfig

	once      bool
	frequency int
}

func addRootFlags(cmd *cobra.Command, config *transformConfig) {
	cmd.PersistentFlags().IntVar(&config.frequency, "frequency", 2, "Number of iterations per hour")
	cmd.PersistentFlags().BoolVar(&config.once, "once", false, "Run once and then leave")
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
}

func runProgram(config *transformConfig) error {
	mysqldb, err := config.MySQLConfig.CreateDatabase()
	if err != nil {
		return err
	}
	influxdb, err := config.InfluxConfig.CreateDatabase()
	if err != nil {
		return err
	}

	plugins := NewPlugins(influxdb)
	fetcher := NewFetcher()

	// Plugins constantly wait for new issues/events/comments
	go plugins.Dispatch(fetcher.GetChannels())

	for {
		begin := time.Now()

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

		time.Sleep(time.Hour/time.Duration(config.frequency) - time.Since(begin))
	}

	return nil

}

func main() {
	config := &transformConfig{}
	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "Transform sql database info into influx stats",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runProgram(config)
		},
	}
	addRootFlags(root, config)
	config.MySQLConfig.AddFlags(root)
	config.InfluxConfig.AddFlags(root)

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
