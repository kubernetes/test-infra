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
	"sync"
	"syscall"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/test-infra/pkg/io"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/testgrid/config"
	"k8s.io/test-infra/testgrid/issue_state"
)

type githubClient interface {
	ListOpenIssues(org, repo string) ([]github.Issue, error)
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
}

type multiString []string

func (m multiString) String() string {
	return strings.Join(m, ",")
}

func (m *multiString) Set(v string) error {
	*m = strings.Split(v, ",")
	return nil
}

const defaultPollInterval = 1 * time.Hour

type options struct {
	github         flagutil.GitHubOptions
	repositories   multiString
	organization   string
	repository     string
	configPath     string
	output         string
	gcsCredentials string
	oneshot        bool
	pollInterval   string
	rateLimit      int
}

func (o *options) parseArgs(fs *flag.FlagSet, args []string) error {

	fs.Var(&o.repositories, "repos", "Target GitHub org/repos (ex. kubernetes/test-infra)")
	fs.StringVar(&o.organization, "github-org", "", "GitHub organization") //Deprecated; remove Sep 1 2018
	fs.StringVar(&o.repository, "github-repo", "", "GitHub repository")    //Deprecated; remove Sep 1 2018
	fs.StringVar(&o.configPath, "config", "", "TestGrid Config proto: gs://bucket or /local/path")
	fs.StringVar(&o.output, "output", "", "write proto to gs://bucket or /local/path")
	fs.StringVar(&o.gcsCredentials, "gcs-credentials-file", "", "/path/to/service/account/credentials (as .json)")
	fs.BoolVar(&o.oneshot, "oneshot", false, "Write proto once and exit instead of monitoring GitHub for changes")
	fs.StringVar(&o.pollInterval, "poll-interval", "", "How often the program polls GitHub for changes (e.g. '1h10m42s', default '1h')")
	fs.IntVar(&o.rateLimit, "rate-limit", 0, "Max requests per hour against GitHub. Unlimited by default.")
	o.github.AddFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	return o.validate()
}

func (o *options) validate() error {
	if o.organization != "" || o.repository != "" {
		logrus.Warn("--github-org and --github-repo are deprecated and may disappear after August 2018; use --repos=org/repo instead")
		o.repositories = append(o.repositories, o.organization+"/"+o.repository)
	}
	if len(o.repositories) == 0 {
		return errors.New("--repos is required")
	}
	if o.output == "" {
		return errors.New("--output is required")
	}
	if o.configPath == "" {
		return errors.New("--config is required")
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
	ghct.Throttle(opt.rateLimit, 1)

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	client, err := io.NewOpener(ctx, opt.gcsCredentials)
	if err != nil {
		logrus.WithError(err).Fatal("Could not create client")
	}

	// Check or create output dir
	if !strings.HasPrefix(opt.output, "gs://") {
		fi, err := os.Stat(opt.output)
		if err == nil && fi.Mode().IsRegular() {
			logrus.Fatalf("Output is a file, not a directory or cloud bucket")
		} else if err != nil { // Target may not exist
			err := os.Mkdir(opt.output, 664)
			if err != nil {
				logrus.Fatalf("Could not create directory at %s: %e", opt.output, err)
			}
		}
	}

	doOneshot := func(ctx context.Context) {
		//Read test groups
		reader, err := client.Reader(ctx, opt.configPath)
		if err != nil {
			logrus.Errorf("Could not open reader from %s: %e", opt.configPath, err)
			return
		}
		defer reader.Close()
		tgConfig, err := config.Unmarshal(reader)
		if err != nil {
			logrus.Errorf("Could not unmarshal proto at %s: %e", opt.configPath, err)
			return
		}

		issueStates := getTestGroups(tgConfig)

		if err := pinIssues(ghct, opt.repositories, issueStates, ctx); err != nil {
			logrus.Errorf("Could not get issues: %e", err)
			return
		}

		if err := writeIssueStates(issueStates, opt.output, client, ctx); err != nil {
			logrus.Errorf("Could not write issue states: %e", err)
		}
	}

	// kick off goroutines
	if opt.oneshot {
		doOneshot(ctx)
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

	wait.UntilWithContext(ctx, doOneshot, pollInterval)

	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)
	<-sigTerm

	logrus.Info("Entomologist is closing...")
}

func writeIssueStates(issueStates map[string]*issue_state.IssueState, basePath string, client io.Opener, ctx context.Context) error {
	for testGroup, issueState := range issueStates {
		if issueState != nil {
			if err := writeProto(basePath, testGroup, issueState, client, ctx); err != nil {
				return err
			}
		} else {
			// The bug state needs to be deleted, if it exists
			if _, err := client.Reader(ctx, fullIssuePath(basePath, testGroup)); err == nil {
				if err := writeProto(basePath, testGroup, nil, client, ctx); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeProto(path, testGroupName string, file *issue_state.IssueState, client io.Opener, ctx context.Context) error {
	fullPath := fullIssuePath(path, testGroupName)
	writer, err := client.Writer(ctx, fullPath)
	if err != nil {
		return fmt.Errorf("open writer to %s: %e", fullPath, err)
	}
	defer writer.Close()

	var out []byte
	if file != nil {
		out, err = proto.Marshal(file)
		if err != nil {
			return fmt.Errorf("marshal proto: %e", err)
		}
	}
	n, err := writer.Write(out)
	if err != nil {
		return fmt.Errorf("write file: %e", err)
	}

	logrus.Infof("Sending %d characters to %s", n, fullPath)
	return nil
}

// Returns the path where the issue_state proto should be read/written
func fullIssuePath(path, testGroupName string) string {
	return fmt.Sprintf("%s/bugs-%s", path, testGroupName)
}

// Returns a map containing every test group as a key, with nil as a value
func getTestGroups(tgConfig *config.Configuration) map[string]*issue_state.IssueState {
	if tgConfig == nil {
		return nil
	}

	result := make(map[string]*issue_state.IssueState)
	for _, testGroup := range tgConfig.TestGroups {
		if testGroup != nil {
			result[testGroup.Name] = nil
		}
	}

	return result
}

func getIssues(client githubClient, org, repo string) ([]github.Issue, error) {
	issuesAndPRs, err := client.ListOpenIssues(org, repo)

	issues := issuesAndPRs[:0]
	for _, issue := range issuesAndPRs {
		if !issue.IsPullRequest() {
			issues = append(issues, issue)
		}
	}

	return issues, err
}

// matchesPattern determines what Entomologist sees as a potential association to a test group
var targetRegExp = regexp.MustCompile(`(?mi)^pin:(.+)$`)

func matchesPattern(body string) []string {
	allMatches := targetRegExp.FindAllStringSubmatch(body, -1)
	results := make([]string, 0)

	for _, match := range allMatches {
		results = append(results, strings.TrimSpace(match[1]))
	}
	return results
}

// Gets information from the repositories on gitHub, and populates testGroups with IssueStates
func pinIssues(client githubClient, repositories []string, testGroups map[string]*issue_state.IssueState, ctx context.Context) error {

	for _, repository := range repositories {
		orgAndRepo := strings.Split(repository, "/")
		if len(orgAndRepo) != 2 {
			logrus.Errorf("Can't process %s: not in 'org/repo' format", repository)
			continue
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		issues, err := getIssues(client, orgAndRepo[0], orgAndRepo[1])
		logrus.Infof("Found %d open issues in %s/%s", len(issues), orgAndRepo[0], orgAndRepo[1])
		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		var testGroupAccess sync.Mutex

		pinIssue := func(issue github.Issue) {
			defer wg.Done()
			matchingTestGroups := matchesPattern(issue.Body)
			comments, err := client.ListIssueComments(orgAndRepo[0], orgAndRepo[1], issue.Number)
			if err != nil {
				logrus.Warnf("Could not reach comments at %s/%s#%d", orgAndRepo[0], orgAndRepo[1], issue.Number)
			}
			for _, comment := range comments {
				matchingTestGroups = append(matchingTestGroups, matchesPattern(comment.Body)...)
			}

			if ctx.Err() != nil {
				logrus.WithError(ctx.Err()).Warnf("Thread terminated due to expiration at %s/%s#%d", orgAndRepo[0], orgAndRepo[1], issue.Number)
				return
			}

			testGroupAccess.Lock()
			defer testGroupAccess.Unlock()
			for _, matchingGroup := range matchingTestGroups {
				issueStates, matches := testGroups[matchingGroup]
				if matches {
					logrus.Infof("Pinning issue %s to %s", issue.Title, matchingGroup)
					newResult := issue_state.IssueInfo{
						IssueId: strconv.Itoa(issue.Number),
						Title:   issue.Title,
					}

					if issueStates == nil {
						testGroups[matchingGroup] = &issue_state.IssueState{
							IssueInfo: []*issue_state.IssueInfo{&newResult},
						}
					} else {
						testGroups[matchingGroup].IssueInfo = append(testGroups[matchingGroup].IssueInfo, &newResult)
					}
				}
			}
		}

		if ctx.Err() != nil {
			return ctx.Err()
		}

		for _, issue := range issues {
			wg.Add(1)
			pinIssue(issue)
		}
		wg.Wait()
	}

	return nil
}
