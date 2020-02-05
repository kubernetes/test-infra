/*
Copyright 2017 The Kubernetes Authors.

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
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	flag "github.com/spf13/pflag"

	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
)

var (
	bufferSize      = 1 // Maximum holding resources
	rTypes          common.CommaSeparatedStrings
	poolSize        int
	updateFrequency time.Duration
	janitorPath     = flag.String("janitor-path", "/bin/gcp_janitor.py", "Path to janitor binary path")
	boskosURL       = flag.String("boskos-url", "http://boskos", "Boskos URL")
	username        = flag.String("username", "", "Username used to access the Boskos server")
	passwordFile    = flag.String("password-file", "", "The path to password file used to access the Boskos server")
	logLevel        = flag.String("log-level", "info", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
)

func init() {
	flag.Var(&rTypes, "resource-type", "comma-separated list of resources need to be cleaned up")
	flag.IntVar(&poolSize, "pool-size", 20, "number of concurrent janitor goroutine")
	flag.DurationVar(&updateFrequency, "update-frequency", 5*time.Minute, "How often to heartbeat owning resources.")
}

func main() {
	// Activate service account
	flag.Parse()
	extraJanitorFlags := flag.CommandLine.Args()

	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("invalid log level specified")
	}
	logrus.SetLevel(level)

	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos, err := client.NewClient("Janitor", *boskosURL, *username, *passwordFile)
	if err != nil {
		logrus.WithError(err).Fatal("unable to create a Boskos client")
	}
	logrus.Info("Initialized boskos client!")

	if len(rTypes) == 0 {
		logrus.Fatal("--resource-type must not be empty!")
	}

	go func(boskos boskosClient) {
		for range time.Tick(updateFrequency) {
			if err := boskos.SyncAll(); err != nil {
				logrus.WithError(err).Warn("SyncAll failed")
			}
		}
	}(boskos)

	buffer := setup(boskos, poolSize, bufferSize, janitorClean, extraJanitorFlags)

	for {
		run(boskos, buffer, rTypes)
		time.Sleep(time.Minute)
	}
}

type clean func(resource *common.Resource, extraFlags []string) error

// TODO(amwat): remove this logic when we get rid of --project.

func format(rtype string) string {
	splits := strings.Split(rtype, "-")
	return splits[len(splits)-1]
}

// Clean by janitor script
func janitorClean(resource *common.Resource, flags []string) error {
	args := append([]string{fmt.Sprintf("--%s=%s", format(resource.Type), resource.Name)}, flags...)
	logrus.Infof("executing janitor: %s %s", *janitorPath, strings.Join(args, " "))
	cmd := exec.Command(*janitorPath, args...)
	b, err := cmd.CombinedOutput()
	if err != nil {
		logrus.WithError(err).Infof("failed to clean up project %s, error info: %s", resource.Name, string(b))
	} else {
		logrus.Tracef("output from janitor: %s", string(b))
		logrus.Infof("successfully cleaned up resource %s", resource.Name)
	}
	return err
}

type boskosClient interface {
	Acquire(rtype string, state string, dest string) (*common.Resource, error)
	ReleaseOne(name string, dest string) error
	SyncAll() error
}

func setup(c boskosClient, janitorCount int, bufferSize int, cleanFunc clean, flags []string) chan *common.Resource {
	buffer := make(chan *common.Resource, bufferSize)
	for i := 0; i < janitorCount; i++ {
		go janitor(c, buffer, cleanFunc, flags)
	}
	return buffer
}

func run(c boskosClient, buffer chan<- *common.Resource, rtypes []string) int {
	totalAcquire := 0
	res := make(map[string]int)
	for _, s := range rtypes {
		res[s] = 0
	}

	for {
		for r := range res {
			if resource, err := c.Acquire(r, common.Dirty, common.Cleaning); err != nil {
				logrus.WithError(err).Infof("no available resource %s", r)
				totalAcquire += res[r]
				delete(res, r)
			} else if resource == nil {
				// To Sen: I don t understand why this would happen
				logrus.Warning("received nil resource")
				totalAcquire += res[r]
				delete(res, r)
			} else {
				logrus.Infof("Acquired resources %s of type %s", resource.Name, resource.Type)
				buffer <- resource // will block until buffer has a free slot
				res[r]++
			}
		}

		if len(res) == 0 {
			break
		}
	}

	return totalAcquire
}

// async janitor goroutine
func janitor(c boskosClient, buffer <-chan *common.Resource, fn clean, flags []string) {
	for {
		resource := <-buffer

		dest := common.Free
		if err := fn(resource, flags); err != nil {
			logrus.WithError(err).Debugf("%s failed!", *janitorPath)
			dest = common.Dirty
		}

		if err := c.ReleaseOne(resource.Name, dest); err != nil {
			logrus.WithError(err).Error("boskos release failed!")
		}
	}
}
