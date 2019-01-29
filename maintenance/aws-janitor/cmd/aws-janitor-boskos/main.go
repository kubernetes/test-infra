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
	"k8s.io/test-infra/maintenance/aws-janitor/account"
	"k8s.io/test-infra/maintenance/aws-janitor/regions"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
)

var (
	boskosURL = flag.String("boskos-url", "http://boskos", "Boskos URL")
)

const (
	sleepTime = time.Minute
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	boskos := client.NewClient("AWSJanitor", *boskosURL)
	err := run(boskos)
	logrus.WithError(err).Error("Janitor failure")
}

func run(boskos *client.Client) error {
	for {
		if res, err := boskos.Acquire(awsboskos.ResourceType, common.Dirty, common.Cleaning); errors.Cause(err) == client.NotFoundErr {
			time.Sleep(sleepTime)
			continue
		} else if err != nil {
			return errors.Wrap(err, "Couldn't retrieve resources from Boskos")
		} else {
			logrus.Infof("Acquired resource %q", res.Name)
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

	if err := cleanAll(s); err != nil {
		return errors.Wrapf(err, "Failed to clean resource %q", res.Name)
	}
	logrus.Infof("Finished cleaning %q", res.Name)
	return nil
}

func cleanAll(s *session.Session) error {
	regionList, err := regions.GetAll(s)
	if err != nil {
		return errors.Wrap(err, "Couldn't retrieve list of regions")
	}

	acct, err := account.GetAccount(s, regions.Default)
	if err != nil {
		return errors.Wrap(err, "failed to retrieve account")
	}

	for _, region := range regionList {
		for _, typ := range resources.RegionalTypeList {
			set, err := typ.ListAll(s, acct, region)
			if err != nil {
				return errors.Wrapf(err, "failed to list resources of type %T", typ)
			}
			if err := typ.MarkAndSweep(s, acct, region, set); err != nil {
				return errors.Wrapf(err, "couldn't sweep resources of type %T", typ)
			}
		}
	}

	for _, typ := range resources.GlobalTypeList {
		set, err := typ.ListAll(s, acct, regions.Default)
		if err != nil {
			return errors.Wrapf(err, "failed to list resources of type %T", typ)
		}
		if err := typ.MarkAndSweep(s, acct, regions.Default, set); err != nil {
			return errors.Wrapf(err, "couldn't sweep resources of type %T", typ)
		}
	}
	return nil
}
