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

package mungers

import (
	"fmt"
	"os"

	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungers/sync"
)

const Title = "Repo publisher failure"
const ID = "repo publisher failure id"

type publisherFailure struct {
	err error
	log string
}

// Title implements IssueSource
func (p *publisherFailure) Title() string {
	return Title
}

// ID implements IssueSource
func (p *publisherFailure) ID() string {
	return ID
}

// Body implements IssueSource
func (p *publisherFailure) Body(newIssue bool) string {
	body := p.err.Error()
	body += fmt.Sprintf("\n")
	body += p.log
	return body
}

func (p *publisherFailure) AddTo(previous string) string {
	return previous
}

// Labels implements IssueSource
func (p *publisherFailure) Labels() []string {
	return []string{sync.PriorityP2.String()}
}

// Priority implements IssueSource
func (p *publisherFailure) Priority(obj *github.MungeObject) (sync.Priority, bool) {
	comments, ok := obj.ListComments()
	if !ok {
		return sync.PriorityP2, false
	}
	// Different IssueSource's Priority calculation may differ
	return autoPrioritize(comments, obj.Issue.CreatedAt), true
}

func readLog(logFilePath string, maxLogLength int64) (string, error) {
	file, err := os.Open(logFilePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	stat, err := os.Stat(logFilePath)
	if err != nil {
		return "", err
	}
	var outputSize int64
	if stat.Size() > maxLogLength {
		outputSize = maxLogLength
	} else {
		outputSize = stat.Size()
	}
	buf := make([]byte, outputSize)

	start := stat.Size() - outputSize
	_, err = file.ReadAt(buf, start)
	return string(buf), err
}
