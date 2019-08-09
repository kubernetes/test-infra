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
	"bytes"
	"encoding/json"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"k8s.io/klog"

	s3path "k8s.io/test-infra/maintenance/aws-janitor/s3"
)

// Set keeps track of the first time we saw a particular
// ARN, and the global TTL. See Mark() for more details.
type Set struct {
	firstSeen map[string]time.Time // ARN -> first time we saw
	marked    map[string]bool      // ARN -> seen this run
	swept     []string             // List of resources we attempted to sweep (to summarize)
	ttl       time.Duration
}

func NewSet(ttl time.Duration) *Set {
	return &Set{
		firstSeen: make(map[string]time.Time),
		marked:    make(map[string]bool),
		ttl:       ttl,
	}
}

func (s *Set) GetARNs() []string {
	slice := make([]string, len(s.firstSeen))
	i := 0
	for key := range s.firstSeen {
		slice[i] = key
		i++
	}

	return slice
}

func LoadSet(sess *session.Session, p *s3path.Path, ttl time.Duration) (*Set, error) {
	s := NewSet(ttl)
	svc := s3.New(sess, &aws.Config{Region: aws.String(p.Region)})

	resp, err := svc.GetObject(&s3.GetObjectInput{Bucket: aws.String(p.Bucket), Key: aws.String(p.Key)})
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchKey" {
			return s, nil
		}
		return nil, err
	}

	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&s.firstSeen); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *Set) Save(sess *session.Session, p *s3path.Path) error {
	b, err := json.MarshalIndent(s.firstSeen, "", "  ")
	if err != nil {
		return err
	}

	svc := s3.New(sess, &aws.Config{Region: aws.String(p.Region)})

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket:       aws.String(p.Bucket),
		Key:          aws.String(p.Key),
		Body:         bytes.NewReader(b),
		CacheControl: aws.String("max-age=0"),
	})

	return err
}

// Mark marks a particular resource as currently present, and advises
// on whether it should be deleted. If Mark(r) returns true, the TTL
// has expired for r and it should be deleted.
func (s *Set) Mark(r Interface) bool {
	key := r.ResourceKey()
	now := time.Now()

	s.marked[key] = true
	if t, ok := s.firstSeen[key]; ok {
		since := now.Sub(t)
		if since > s.ttl {
			s.swept = append(s.swept, key)
			return true
		}
		klog.V(1).Infof("%s: seen for %v", key, since)
		return false
	}

	s.firstSeen[key] = now
	klog.V(1).Infof("%s: first seen", key)
	if s.ttl == 0 {
		// If the TTL is 0, it should be deleted now.
		s.swept = append(s.swept, key)
		return true
	}

	return false
}

// MarkComplete figures out which ARNs were in previous passes but not
// this one, and eliminates them. It should only be run after all
// resources have been marked.
func (s *Set) MarkComplete() int {
	var gone []string
	for key := range s.firstSeen {
		if !s.marked[key] {
			gone = append(gone, key)
		}
	}

	for _, key := range gone {
		klog.V(1).Infof("%s: deleted since last run", key)
		delete(s.firstSeen, key)
	}

	if len(s.swept) > 0 {
		klog.Errorf("%d resources swept: %v", len(s.swept), s.swept)
	}

	return len(s.swept)
}
