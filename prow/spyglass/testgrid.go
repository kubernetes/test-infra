/*
Copyright 2019 The Kubernetes Authors.

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

package spyglass

import (
	"context"
	"fmt"
	"sync"
	"time"

	tgconf "github.com/GoogleCloudPlatform/testgrid/config"
	tgconfpb "github.com/GoogleCloudPlatform/testgrid/pb/config"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
)

// TestGrid manages a TestGrid configuration, and handles lookups of TestGrid configuration.
type TestGrid struct {
	mut    sync.RWMutex
	c      *tgconfpb.Configuration
	conf   config.Getter
	ctx    context.Context
	opener io.Opener
}

// Start synchronously requests the testgrid config, then continues to update it periodically.
func (tg *TestGrid) Start() {
	if err := tg.updateConfig(); err != nil {
		logrus.WithError(err).Error("Couldn't fetch TestGrid config.")
	}
	go func() {
		for range time.Tick(10 * time.Minute) {
			if err := tg.updateConfig(); err != nil {
				logrus.WithError(err).WithField("path", tg.conf().Deck.Spyglass.TestGridConfig).Error("Couldn't update TestGrid config.")
			}
		}
	}()
}

// FindPath returns a 'query' for a job name, e.g. 'sig-testing-misc#bazel' for ci-test-infra-bazel.
// This is based on the same logic Gubernator uses: find the dashboard with the fewest tabs, where
// our tab doesn't have any BaseOptions.
func (tg *TestGrid) FindQuery(jobName string) (string, error) {
	if tg.config() == nil {
		return "", fmt.Errorf("no testgrid config loaded")
	}
	bestOption := ""
	bestScore := 0
	for _, dashboard := range tg.config().Dashboards {
		for _, tab := range dashboard.DashboardTab {
			penalty := 0
			if tab.BaseOptions != "" {
				penalty = 1000
			}
			if tab.TestGroupName == jobName {
				score := -len(dashboard.DashboardTab) + penalty
				if bestOption == "" || score < bestScore {
					bestScore = score
					bestOption = dashboard.Name + "#" + tab.Name
				}
			}
		}
	}
	if bestOption == "" {
		return "", fmt.Errorf("couldn't find a testgrid tab for %q", jobName)
	}
	return bestOption, nil
}

// Ready returns true if a usable TestGrid config is loaded, otherwise false.
func (tg *TestGrid) Ready() bool {
	return tg.c != nil
}

func (tg *TestGrid) updateConfig() error {
	if tg.conf().Deck.Spyglass.TestGridConfig == "" {
		tg.setConfig(nil)
		return nil
	}
	r, err := tg.opener.Reader(tg.ctx, tg.conf().Deck.Spyglass.TestGridConfig)
	if err != nil {
		return err
	}
	c, err := tgconf.Unmarshal(r)
	if err != nil {
		return err
	}
	tg.setConfig(c)
	return nil
}

func (tg *TestGrid) setConfig(c *tgconfpb.Configuration) {
	tg.mut.Lock()
	defer tg.mut.Unlock()
	tg.c = c
}

func (tg *TestGrid) config() *tgconfpb.Configuration {
	tg.mut.RLock()
	defer tg.mut.RUnlock()
	return tg.c
}
