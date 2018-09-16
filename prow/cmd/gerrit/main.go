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

type projectsFlag map[string][]string

func (p projectsFlag) String() string {
	var hosts []string
	for host, repos := range p {
		hosts = append(hosts, host+"="+strings.Join(repos, ","))
	}
	return strings.Join(hosts, " ")
}

func (p projectsFlag) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%s not in the form of host=repo-a,repo-b,etc", value)
	}
	host := parts[0]
	if _, ok := p[host]; ok {
		return fmt.Errorf("duplicate host: %s", host)
	}
	repos := strings.Split(parts[1], ",")
	p[host] = repos
	return nil
}

type options struct {
	cookiefilePath   string
	configPath       string
	projects         projectsFlag
	lastSyncFallback string
}

func (o *options) Validate() error {
	if len(o.projects) == 0 {
		return errors.New("--gerrit-projects unset")
	}

	if o.cookiefilePath == "" {
		logrus.Info("--cookiefile unset, using anonymous authentication")
	}

	if o.configPath == "" {
		return errors.New("--config-path unset")
	}

	if o.lastSyncFallback == "" {
		return errors.New("--last-sync-fallback unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{
		projects: projectsFlag{},
	}
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for anonymous")
	flag.Var(&o.projects, "gerrit-projects", "Set of gerrit repos to monitor on a host example: --gerrit-host=https://android.googlesource.com=platform/build,toolchain/llvm, repeat flag for each host")
	flag.StringVar(&o.lastSyncFallback, "last-sync-fallback", "", "Path to persistent volume to load the last sync time")
	flag.Parse()
	return o
}

func main() {
	logrus.SetFormatter(logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "gerrit"}))
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	ca := &config.Agent{}
	if err := ca.Start(o.configPath, ""); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(ca.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	c, err := adapter.NewController(o.lastSyncFallback, o.cookiefilePath, o.projects, kc, ca)
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
