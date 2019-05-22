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
	"context"
	"errors"
	"flag"
	"fmt"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/testgrid/issue_state"
)

type githubClient interface {
	ListOpenIssues(org, repo string) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
}

const defaultPollTime = 1 * time.Hour

type options struct {
	github         flagutil.GitHubOptions
	organization   string
	repository     string
	gcsPath        string
	gcsCredentials string
	oneshot        bool
}

func (o *options) parseArgs(fs *flag.FlagSet, args []string) error {

	fs.StringVar(&o.organization, "github-org", "", "GitHub organization")
	fs.StringVar(&o.repository, "github-repo", "", "GitHub repository")
	fs.StringVar(&o.gcsPath, "output", "", "GCS output (gs://bucket)")
	fs.StringVar(&o.gcsCredentials, "gcs-credentials-file", "", "/path/to/service/account/credentials (as .json)")
	fs.BoolVar(&o.oneshot, "oneshot", false, "Write proto once and exit instead of monitoring GitHub for changes")
	o.github.AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	return o.validate()
}

func (o *options) validate() error {
	if o.organization == "" {
		return errors.New("--github-org is required")
	}
	if o.repository == "" {
		return errors.New("--github-repo is required")
	}
	if o.gcsPath == "" || !strings.HasPrefix(o.gcsPath, "gs://") {
		return errors.New("invalid or missing --output")
	}
	if o.gcsCredentials == "" {
		return errors.New("--gcs-credentials-file required for write operations")
	}
	return o.github.Validate(false)
}

func parseOptions() options {
	var o options

	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.WithError(err).Fatal("Invalid Flags")
	}

	return o
}

func main() {
	opt := parseOptions()

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{opt.github.TokenPath}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	ghct, err := opt.github.GitHubClient(secretAgent, false)
	if err != nil {
		logrus.WithError(err).Fatal("Error starting GitHub client")
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	client, _ := io.NewOpener(ctx, opt.gcsCredentials)
	if client == nil {
		logrus.Fatalf("Empty credentials (at %s) not allowed for write operation", opt.gcsCredentials)
	}

	// testgrid expects issue_state.proto files for an org/repo to be named this way
	gcsFile := fmt.Sprintf("/bugs-%s-%s", opt.organization, opt.repository)
	writer, _ := client.Writer(ctx, opt.gcsPath+gcsFile)
	defer writer.Close()

	// kick off goroutines
	poll := func() {
		issueState, err := pinIssues(ghct, opt)
		if err != nil {
			logrus.Fatalf("Could not get issues: %e", err)
		}

		out, _ := proto.Marshal(issueState)
		n, _ := writer.Write(out)

		logrus.Infof("Sending %d characters to GCS", n)
	}

	if opt.oneshot {
		poll()
		return
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	wait.Until(poll, defaultPollTime, stopCh)

	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm

	logrus.Info("Entomologist is closing...")
}

var targetRegExp = regexp.MustCompile(`(?m)^target:(.+)$`)

func getIssues(client githubClient, o options) ([]github.Issue, error) {
	issuesAndPRs, err := client.ListOpenIssues(o.organization, o.repository)

	issues := issuesAndPRs[:0]
	for _, issue := range issuesAndPRs {
		if !issue.IsPullRequest() {
			issues = append(issues, issue)
		}
	}

	return issues, err
}

// Algorithm for detecting and formatting test targets in issues
func pinIssues(client githubClient, o options) (*issue_state.IssueState, error) {
	issues, err := getIssues(client, o)

	logrus.Infof("Found %d open issues in %s/%s", len(issues), o.organization, o.repository)

	if err != nil {
		return nil, err
	}

	var results []*issue_state.IssueInfo

	for _, issue := range issues {

		var targets []string

		matches := targetRegExp.FindAllStringSubmatch(issue.Body, -1)
		for _, match := range matches {
			targets = append(targets, strings.TrimSpace(match[1]))
		}

		// TODO(chases2): separate the comments API calls into their own goroutines
		comments, err := client.ListIssueComments(o.organization, o.repository, issue.Number)
		if err != nil {
			return nil, err
		}
		for _, comment := range comments {
			matches := targetRegExp.FindAllStringSubmatch(comment.Body, -1)
			for _, match := range matches {
				targets = append(targets, strings.TrimSpace(match[1]))
			}
		}

		if len(targets) != 0 {
			newResult := issue_state.IssueInfo{
				IssueId: strconv.Itoa(issue.Number),
				Title:   issue.Title,
				RowIds:  targets,
			}

			results = append(results, &newResult)
		}
	}

	logrus.Printf("Pinning %d issues", len(results))
	file := &issue_state.IssueState{
		IssueInfo: results,
	}

	return file, nil
}
