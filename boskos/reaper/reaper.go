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
	"flag"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
)

var (
	rTypes    common.CommaSeparatedStrings
	boskosURL = flag.String("boskos-url", "http://boskos", "Boskos URL")
)

func init() {
	flag.Var(&rTypes, "resource-type", "comma-separated list of resources need to be reset")
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos := client.NewClient("Reaper", *boskosURL)
	logrus.Infof("Initialzied boskos client!")
	flag.Parse()

	if len(rTypes) == 0 {
		logrus.Fatal("--resource-type must not be empty!")
	}

	for range time.Tick(time.Minute * 5) {
		for _, r := range rTypes {
			sync(boskos, r)
		}
	}
}

func sync(c *client.Client, res string) {
	// kubetest busted
	if owners, err := c.Reset(res, common.Busy, 30*time.Minute, common.Dirty); err != nil {
		logrus.WithError(err).Error("Reset busy failed!")
	} else {
		logrus.Infof("Reset busy to dirty! Proj-owner: %v", owners)
	}

	// janitor, mason busted
	if owners, err := c.Reset(res, common.Cleaning, 30*time.Minute, common.Dirty); err != nil {
		logrus.WithError(err).Error("Reset cleaning failed!")
	} else {
		logrus.Infof("Reset cleaning to dirty! Proj-owner: %v", owners)
	}

	// mason busted
	if owners, err := c.Reset(res, common.Leased, 30*time.Minute, common.Dirty); err != nil {
		logrus.WithError(err).Error("Reset busy failed!")
	} else {
		logrus.Infof("Reset leased to dirty! Proj-owner: %v", owners)
	}
}
