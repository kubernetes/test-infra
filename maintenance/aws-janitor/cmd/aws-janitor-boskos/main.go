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

package main

import (
	"flag"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/boskos/client"
	"k8s.io/test-infra/boskos/common"
	awsboskos "k8s.io/test-infra/boskos/common/aws"
	"k8s.io/test-infra/maintenance/aws-janitor/regions"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
)

var (
	boskosURL          = flag.String("boskos-url", "http://boskos", "Boskos URL")
	sweepCount         = flag.Int("sweep-count", 3, "Number of times to sweep the resources")
	sweepSleep         = flag.String("sweep-sleep", "30s", "The duration to pause between sweeps")
	sweepSleepDuration time.Duration
)

const (
	sleepTime = time.Minute
)

func main() {
	flag.Parse()
	if d, err := time.ParseDuration(*sweepSleep); err != nil {
		sweepSleepDuration = time.Second * 30
	} else {
		sweepSleepDuration = d
	}

	logrus.SetFormatter(&logrus.JSONFormatter{})

	boskos := client.NewClient("AWSJanitor", *boskosURL)
	if err := run(boskos); err != nil {
		logrus.WithError(err).Error("Janitor failure")
	}
}

func run(boskos *client.Client) error {
	for {
		if res, err := boskos.Acquire(awsboskos.ResourceType, common.Dirty, common.Cleaning); errors.Cause(err) == client.ErrNotFound {
			logrus.Info("no resource acquired. Sleeping.")
			time.Sleep(sleepTime)
			continue
		} else if err != nil {
			return errors.Wrap(err, "Couldn't retrieve resources from Boskos")
		} else {
			logrus.WithField("name", res.Name).Info("Acquired resource")
			if err := cleanResource(res); err != nil {
				return errors.Wrapf(err, "Couldn't clean resource %q", res.Name)
			}
			if err := boskos.ReleaseOne(res.Name, common.Free); err != nil {
				return errors.Wrapf(err, "Failed to release resoures %q", res.Name)
			}
		}
	}
}

func cleanResource(res *common.Resource) error {
	val, err := awsboskos.GetAWSCreds(res)
	if err != nil {
		return errors.Wrapf(err, "Couldn't get AWS creds from %q", res.Name)
	}
	creds := credentials.NewStaticCredentialsFromCreds(val)
	s, err := session.NewSession(aws.NewConfig().WithCredentials(creds))
	if err != nil {
		return errors.Wrapf(err, "Failed to create AWS session")

	}
	logrus.WithField("name", res.Name).Info("beginning cleaning")
	start := time.Now()

	for i := 0; i < *sweepCount; i++ {
		if err := resources.CleanAll(s, regions.Default); err != nil {
			if i == *sweepCount-1 {
				logrus.WithError(err).Warningf("Failed to clean resource %q", res.Name)
			}
		}
		if i < *sweepCount-1 {
			time.Sleep(sweepSleepDuration)
		}
	}

	duration := time.Since(start)

	logrus.WithFields(logrus.Fields{"name": res.Name, "duration": duration.Seconds()}).Info("Finished cleaning")
	return nil
}
