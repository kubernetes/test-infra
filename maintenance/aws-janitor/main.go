/*
Copyright 2016 The Kubernetes Authors.

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
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/pkg/errors"
	"k8s.io/klog"
	"k8s.io/test-infra/maintenance/aws-janitor/account"
	"k8s.io/test-infra/maintenance/aws-janitor/regions"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
	s3path "k8s.io/test-infra/maintenance/aws-janitor/s3"
)

var (
	maxTTL   = flag.Duration("ttl", 24*time.Hour, "Maximum time before attempting to delete a resource. Set to 0s to nuke all non-default resources.")
	region   = flag.String("region", regions.Default, "The default AWS region")
	path     = flag.String("path", "", "S3 path for mark data (required when -all=false)")
	cleanAll = flag.Bool("all", false, "Clean all resources (ignores -path)")
)

func main() {
	klog.InitFlags(nil)
	flag.Lookup("logtostderr").Value.Set("true")
	flag.Parse()
	defer klog.Flush()

	// Retry aggressively (with default back-off). If the account is
	// in a really bad state, we may be contending with API rate
	// limiting and fighting against the very resources we're trying
	// to delete.
	sess := session.Must(session.NewSessionWithOptions(session.Options{Config: aws.Config{MaxRetries: aws.Int(100)}}))

	if *cleanAll {
		if err := resources.CleanAll(sess, *region); err != nil {
			klog.Fatalf("Error cleaning all resources: %v", err)
		}
	} else if ok, err := markAndSweep(sess); err != nil {
		klog.Fatalf("Error marking and sweeping resources: %v", err)
	} else if !ok {
		os.Exit(1)
	}
}

func markAndSweep(sess *session.Session) (bool, error) {
	s3p, err := s3path.GetPath(sess, *path)
	if err != nil {
		return false, errors.Wrapf(err, "-path %q isn't a valid S3 path", *path)
	}

	acct, err := account.GetAccount(sess, regions.Default)
	if err != nil {
		return false, errors.Wrap(err, "Error getting current user")
	}
	klog.V(1).Infof("account: %s", acct)

	regionList, err := regions.GetAll(sess)
	if err != nil {
		return false, errors.Wrap(err, "Error getting available regions")
	}
	klog.Infof("Regions: %+v", regionList)

	res, err := resources.LoadSet(sess, s3p, *maxTTL)
	if err != nil {
		return false, errors.Wrapf(err, "Error loading %q", *path)
	}

	for _, region := range regionList {
		for _, typ := range resources.RegionalTypeList {
			if err := typ.MarkAndSweep(sess, acct, region, res); err != nil {
				return false, errors.Wrapf(err, "Error sweeping %T", typ)
			}
		}
	}

	for _, typ := range resources.GlobalTypeList {
		if err := typ.MarkAndSweep(sess, acct, *region, res); err != nil {
			return false, errors.Wrapf(err, "Error sweeping %T", typ)
		}
	}

	swept := res.MarkComplete()
	if err := res.Save(sess, s3p); err != nil {
		return false, errors.Wrapf(err, "Error saving %q", *path)
	}

	return swept == 0, nil
}
