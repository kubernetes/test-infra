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
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
)

var imageName = flag.String("image-name", "amzn-ami-hvm-2017.09.1.20180115-x86_64-gp2", "name of AMI to launch")
var imageOwner = flag.String("image-owner", "137112412989", "owner of AMI to launch")

var instanceType = flag.String("instance-type", "c4.large", "instance type to check")

var tagKey = "k8s.io/test-infra/check-stockout"

func main() {
	flag.Set("alsologtostderr", "true")

	flag.Parse()

	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	regions, err := findRegions()
	if err != nil {
		return err
	}

	// For faster testing:
	//regions = []string{"us-east-1"}

	defer func() {
		for _, r := range regions {
			if err := cleanupInstances(r); err != nil {
				glog.Warningf("failed to cleanup region %s: %v", r, err)
			}
		}
	}()

	avail, err := checkInstanceTypeAvailability(regions, *instanceType)
	if err != nil {
		return err
	}

	o, err := json.MarshalIndent(avail, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling json: %v", err)
	}

	fmt.Printf("\n%s\n", string(o))

	return nil
}

func findRegions() ([]string, error) {
	config := &aws.Config{}
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, fmt.Errorf("error creating aws session: %v", err)
	}

	svc := ec2.New(sess)

	var regions []string
	{
		response, err := svc.DescribeRegions(&ec2.DescribeRegionsInput{})
		if err != nil {
			return nil, fmt.Errorf("error from DescribeRegions: %v", err)
		}
		for _, r := range response.Regions {
			regions = append(regions, aws.StringValue(r.RegionName))
		}
	}
	glog.Infof("regions: %v", regions)
	return regions, nil
}

func checkInstanceTypeAvailability(regions []string, instanceType string) (map[string]bool, error) {
	avail := make(map[string]bool)

	var errorRegions []string
	for _, r := range regions {
		zone, err := launchInstance(r, instanceType)
		if err != nil {
			glog.Warningf("error from region %s: %v", r, err)
			errorRegions = append(errorRegions, r)
		} else {
			avail[r] = zone != ""
		}
	}

	if len(errorRegions) != 0 {
		return nil, fmt.Errorf("some regions had errors: %v", errorRegions)
	}

	return avail, nil
}

func awsErrorCode(err error) string {
	if awsErr, ok := err.(awserr.Error); ok {
		return awsErr.Code()
	}
	return ""
}

func buildFilter(name, value string) *ec2.Filter {
	return &ec2.Filter{Name: aws.String(name), Values: []*string{&value}}
}

func cleanupInstances(region string) error {
	config := &aws.Config{Region: aws.String(region)}
	sess, err := session.NewSession(config)
	if err != nil {
		return fmt.Errorf("error creating aws session: %v", err)
	}

	svc := ec2.New(sess)

	instancesResponse, err := svc.DescribeInstances(&ec2.DescribeInstancesInput{
		Filters: []*ec2.Filter{
			buildFilter("tag-key", tagKey),
		},
	})
	if err != nil {
		return fmt.Errorf("error from DescribeInstances: %v", err)
	}

	for _, r := range instancesResponse.Reservations {
		for _, i := range r.Instances {
			id := aws.StringValue(i.InstanceId)
			stateName := ""
			if i.State != nil {
				stateName = aws.StringValue(i.State.Name)
			}

			if stateName == "terminated" {
				continue
			}

			glog.Infof("terminating existing instance %s in state %s", id, stateName)

			_, err := svc.TerminateInstances(&ec2.TerminateInstancesInput{
				InstanceIds: []*string{i.InstanceId},
			})
			if err != nil {
				glog.Warningf("failed to terminate instance %s", aws.StringValue(i.InstanceId))
			}

		}
	}

	return nil
}

func launchInstance(region string, instanceType string) (string, error) {
	config := &aws.Config{Region: aws.String(region)}
	sess, err := session.NewSession(config)
	if err != nil {
		return "", fmt.Errorf("error creating aws session: %v", err)
	}

	svc := ec2.New(sess)

	images, err := svc.DescribeImages(&ec2.DescribeImagesInput{
		Filters: []*ec2.Filter{
			buildFilter("owner-id", *imageOwner),
			buildFilter("name", *imageName),
		},
	})
	if err != nil {
		return "", fmt.Errorf("error describing images: %v", err)
	}

	if len(images.Images) == 0 {
		return "", fmt.Errorf("did not find AMI with owner=%s, name=%s", *imageOwner, *imageName)
	}

	if len(images.Images) != 1 {
		return "", fmt.Errorf("found multiple AMIs with owner=%s, name=%s", *imageOwner, *imageName)
	}

	ami := aws.StringValue(images.Images[0].ImageId)
	glog.Infof("%s: found AMI %s", region, ami)

	createResponse, err := svc.RunInstances(&ec2.RunInstancesInput{
		MinCount:     aws.Int64(1),
		MaxCount:     aws.Int64(1),
		ImageId:      aws.String(ami),
		InstanceType: aws.String(instanceType),
		TagSpecifications: []*ec2.TagSpecification{
			{
				ResourceType: aws.String("instance"),
				Tags:         []*ec2.Tag{{Key: aws.String(tagKey), Value: aws.String("1")}},
			},
		},
	})
	if err != nil {
		if awsErrorCode(err) == "InsufficientInstanceCapacity" {
			glog.Infof("region %s has InsufficientInstanceCapacity for %s", region, instanceType)
			return "", nil
		}

		if awsErrorCode(err) == "Unsupported" {
			glog.Infof("region %s does not support instanceType %s", region, instanceType)
			return "", nil
		}

		return "", fmt.Errorf("error from RunInstances: %v", err)
	}

	zone := aws.StringValue(createResponse.Instances[0].Placement.AvailabilityZone)
	glog.Infof("created instance in region %s, zone %s: %s", region, zone, aws.StringValue(createResponse.Instances[0].InstanceId))

	return zone, nil
}
