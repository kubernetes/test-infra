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
	"time"

	"github.com/Sirupsen/logrus"
	"k8s.io/test-infra/boskos/client"
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos := client.NewClient("Reaper", "http://boskos")
	logrus.Infof("Initialzied boskos client!")

	for range time.Tick(time.Minute * 10) {
		sync(boskos)
	}
}

func sync(c *client.Client) {
	if owners, err := c.Reset("project", "busy", time.Hour*2, "dirty"); err != nil {
		logrus.WithError(err).Error("Reset Failed!")
	} else {
		logrus.Infof("Reset Succeeded! Proj-owner: %v", owners)
	}
}
