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
	"k8s.io/test-infra/prow/gerrit/adapter"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
)

type projectsFlag []string

func (p *projectsFlag) String() string {
	return fmt.Sprintf("%v", *p)
}

func (p *projectsFlag) Set(value string) error {
	*p = append(*p, value)
	return nil
}

type options struct {
	configPath string
	// instance : projects map
	projectsVar      projectsFlag
	projects         map[string][]string
	lastSyncFallback string
}

func (o *options) Validate() error {
	if len(o.projectsVar) == 0 {
		return errors.New("--gerrit-projects must set")
	}

	o.projects = make(map[string][]string)

	for _, projects := range o.projectsVar {
		split := strings.Split(projects, "=")
		if len(split) != 2 {
			return errors.New("--gerrit-projects must be in a form of --gerrit-projects=instance-foo=proj1,proj2")
		}

		instance := split[0]
		projects := strings.Split(split[1], ",")

		logrus.Infof("Added projects %v from instance %s", projects, instance)
		o.projects[instance] = projects
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.Var(&o.projectsVar, "gerrit-projects", "repeatable gerrit instance/projects list, example: --gerrit-projects=instance-foo=proj1,proj2")
	flag.StringVar(&o.lastSyncFallback, "last-sync-fallback", "", "Path to persistent volume to load the last sync time")
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

	ca := &config.Agent{}
	if err := ca.Start(o.configPath, ""); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(ca.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	c, err := adapter.NewController(o.lastSyncFallback, o.projects, kc, ca)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating gerrit client.")
	}

	logrus.Infof("Starting gerrit fetcher")

	tick := time.Tick(ca.Config().Gerrit.TickInterval)
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
		case <-sig:
			logrus.Info("gerrit fetcher is shutting down...")
			return
		}
	}
}
