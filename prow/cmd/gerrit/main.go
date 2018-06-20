/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/gerrit"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	configPath string
	instance   string
	projects   string
	storage    string
}

func (o *options) Validate() error {
	if o.instance == "" {
		return errors.New("--gerrit-instance must set")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.instance, "gerrit-instance", "", "URL to gerrit instance")
	flag.StringVar(&o.projects, "gerrit-projects", "", "comma separated gerrit projects to fetch from the gerrit instance")
	flag.StringVar(&o.storage, "storage", "", "Path to persistent volume to load the last sync time")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "gerrit"}),
	)

	projs := strings.Split(o.projects, ",")
	if len(projs) == 0 {
		logrus.Fatal("must have one or more target gerrit project")
	}

	ca := &config.Agent{}
	if err := ca.Start(o.configPath, ""); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(ca.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	c, err := gerrit.NewController(o.instance, o.storage, projs, kc, ca)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}

	if err := c.Auth(); err != nil {
		logrus.WithError(err).Fatal("Error auth gerrit client.")
	}

	logrus.Infof("Starting gerrit fetcher")

	tick := time.Tick(ca.Config().Gerrit.TickInterval)
	auth := time.Tick(time.Minute * 10)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			if err := c.Sync(); err != nil {
				logrus.WithError(err).Error("Error syncing.")
			}
			logrus.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Synced")
		case <-auth:
			if err := c.Auth(); err != nil {
				logrus.WithError(err).Error("Error auth to gerrit... (continue)")
			}
		case <-sig:
			logrus.Info("gerrit fetcher is shutting down...")
			return
		}
	}
}
