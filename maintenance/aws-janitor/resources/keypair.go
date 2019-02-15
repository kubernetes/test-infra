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

package resources

import (
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/golang/glog"
)

// KeyPairs is https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/ec2-key-pairs.html
type KeyPairs struct{}

// MarkAndSweep looks at the provided set, and removes resources older than its TTL that have been previously tagged.
func (KeyPairs) MarkAndSweep(sess *session.Session, acct string, region string, set *Set) error {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})

	inp := &ec2.DescribeKeyPairsInput{}
	pairs, err := svc.DescribeKeyPairs(inp)
	if err != nil {
		return err
	}
	for _, keypair := range pairs.KeyPairs {
		p := &keyPair{
			Account: acct,
			Region:  region,
			KeyName: *keypair.KeyName,
		}

		if set.Mark(p) {
			inp := &ec2.DeleteKeyPairInput{KeyName: &p.KeyName}
			if _, err := svc.DeleteKeyPair(inp); err != nil {
				glog.Warningf("%v: delete failed: %v", p.ARN(), err)
			}
		}
	}

	return nil
}

// ListAll populates a set will all available KeyPair resources.
func (KeyPairs) ListAll(sess *session.Session, acct, region string) (*Set, error) {
	svc := ec2.New(sess, &aws.Config{Region: aws.String(region)})
	set := NewSet(0)

	inp := &ec2.DescribeKeyPairsInput{}
	pairs, err := svc.DescribeKeyPairs(inp)
	if err != nil {
		return nil, err
	}
	for _, keypair := range pairs.KeyPairs {
		now := time.Now()
		arn := keyPair{
			Account: acct,
			Region:  region,
			KeyName: *keypair.KeyName,
		}.ARN()

		set.firstSeen[arn] = now
	}

	return set, nil
}

type keyPair struct {
	Account string
	Region  string
	KeyName string
}

func (kp keyPair) ARN() string {
	return fmt.Sprintf("arn:aws:ec2:%s:%s:key-pair/%s", kp.Region, kp.Account, kp.KeyName)
}

func (kp keyPair) ResourceKey() string {
	return kp.ARN()
}
