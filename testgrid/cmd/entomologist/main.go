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
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/testgrid/issue_state"
)

type githubClient interface {
	ListOpenIssues(org, repo string) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
}

const defaultPollInterval = 1 * time.Hour

type options struct {
	github         flagutil.GitHubOptions
	organization   string
	repository     string
	output         string
	gcsCredentials string
	oneshot        bool
	pollInterval   string
}

func (o *options) parseArgs(fs *flag.FlagSet, args []string) error {

	fs.StringVar(&o.organization, "github-org", "", "GitHub organization")
	fs.StringVar(&o.repository, "github-repo", "", "GitHub repository")
	fs.StringVar(&o.output, "output", "", "write proto to gs://bucket or /local/path")
	fs.StringVar(&o.gcsCredentials, "gcs-credentials-file", "", "/path/to/service/account/credentials (as .json)")
	fs.BoolVar(&o.oneshot, "oneshot", false, "Write proto once and exit instead of monitoring GitHub for changes")
	fs.StringVar(&o.pollInterval, "poll-interval", "", "How often the program polls GitHub for changes (e.g. '1h10m42s', default '1h')")
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
	if o.output == "" {
		return errors.New("--output is required")
	}
	if strings.HasPrefix(o.output, "gs://") && o.gcsCredentials == "" {
		return errors.New("--gcs-credentials-file required for write operations")
	}
	if o.oneshot && o.pollInterval != "" {
		return errors.New("--oneshot and --poll-interval cannot be specified together")
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

	var outputFile string
	if strings.HasPrefix(opt.output, "gs://") {
		// testgrid expects issue_state.proto files for an org/repo to be named this way
		outputFile = fmt.Sprintf("%s/bugs-%s-%s", opt.output, opt.organization, opt.repository)
	} else {
		fi, err := os.Stat(opt.output)
		if err == nil && fi.Mode().IsRegular() {
			outputFile = opt.output
		} else if err == nil && fi.Mode().IsDir() {
			outputFile = fmt.Sprintf("%s/bugs-%s-%s", opt.output, opt.organization, opt.repository)
		} else {
			// Try to create the file
			fd, err := os.Create(opt.output)
			defer fd.Close()
			if err != nil {
				logrus.Fatalf("Could not create file: %e", err)
			}
			outputFile = fd.Name()
		}
	}

	// kick off goroutines
	poll := func() {
		writer, err := client.Writer(ctx, outputFile)
		if err != nil {
			logrus.Errorf("Could not open writer to %s: %e", outputFile, err)
		}
		defer writer.Close()

		issueState, err := pinIssues(ghct, opt)
		if err != nil {
			logrus.Errorf("Could not get issues: %e", err)
			return
		}

		out, err := proto.Marshal(issueState)
		if err != nil {
			logrus.Errorf("Could not marshal proto %v: %e", issueState, err)
			return
		}
		n, err := writer.Write(out)
		if err != nil {
			logrus.Errorf("Could not write file: %e", err)
			return
		}

		logrus.Infof("Sending %d characters to %s", n, outputFile)
	}

	if opt.oneshot {
		poll()
		return
	}

	var pollInterval time.Duration
	if opt.pollInterval == "" {
		pollInterval = defaultPollInterval
	} else {
		var err error
		if pollInterval, err = time.ParseDuration(opt.pollInterval); err != nil {
			logrus.Fatalf("Could not parse --poll-interval: %e", err)
		}
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	wait.Until(poll, pollInterval, stopCh)

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
