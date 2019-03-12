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
	"github.com/aws/aws-sdk-go/service/ec2"
	"k8s.io/klog"
	"k8s.io/test-infra/maintenance/aws-janitor/account"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
	s3path "k8s.io/test-infra/maintenance/aws-janitor/s3"
)

const (
	defaultRegion = "us-east-1"
)

var (
	maxTTL = flag.Duration("ttl", 24*time.Hour, "Maximum time before we attempt deletion of a resource. Set to 0s to nuke all non-default resources.")
	path   = flag.String("path", "", "S3 path to store mark data in (required)")
)

func getRegions(sess *session.Session) ([]string, error) {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(defaultRegion)})

	resp, err := svc.DescribeRegions(nil)
	if err != nil {
		return nil, err
	}

	var regions []string
	for _, region := range resp.Regions {
		regions = append(regions, *region.RegionName)
	}

	return regions, nil
}

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

	s3p, err := s3path.GetPath(sess, *path)
	if err != nil {
		klog.Fatalf("--path %q isn't a valid S3 path: %v", *path, err)
	}
	acct, err := account.GetAccount(sess, defaultRegion)
	if err != nil {
		klog.Fatalf("Error getting current user: %v", err)
	}
	klog.Infof("Account: %s", acct)

	regions, err := getRegions(sess)
	if err != nil {
		klog.Fatalf("Error getting available regions: %v", err)
	}
	klog.Infof("Regions: %+v", regions)

	res, err := resources.LoadSet(sess, s3p, *maxTTL)
	if err != nil {
		klog.Fatalf("Error loading %q: %v", *path, err)
	}

	for _, region := range regions {
		for _, typ := range resources.RegionalTypeList {
			if err := typ.MarkAndSweep(sess, acct, region, res); err != nil {
				klog.Errorf("Error sweeping %T: %v", typ, err)
				return
			}
		}
	}

	for _, typ := range resources.GlobalTypeList {
		if err := typ.MarkAndSweep(sess, acct, "us-east-1", res); err != nil {
			klog.Errorf("Error sweeping %T: %v", typ, err)
			return
		}
	}

	swept := res.MarkComplete()
	if err := res.Save(sess, s3p); err != nil {
		klog.Fatalf("Error saving %q: %v", *path, err)
	}

	if swept > 0 {
		os.Exit(1)
	}
}
