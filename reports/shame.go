/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package reports

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	githubhelper "k8s.io/contrib/mungegithub/github"

	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

// ShameReport lists flaky tests and writes group+individual email to nag people to fix them.
type ShameReport struct {
	Command             string
	From                string
	Cc                  string
	ReplyTo             string
	AllowedShameDomains string
}

func init() {
	RegisterReportOrDie(&ShameReport{})
}

// Name is the name usable in --issue-reports
func (s *ShameReport) Name() string { return "shame" }

// AddFlags will add any request flags to the cobra `cmd`
func (s *ShameReport) AddFlags(cmd *cobra.Command, config *githubhelper.Config) {
	cmd.Flags().StringVar(&s.Command, "shame-report-cmd", "tee -a shame.txt", "command to execute, passing the report as stdin")
	cmd.Flags().StringVar(&s.From, "shame-from", "", "From: header for shame report")
	cmd.Flags().StringVar(&s.Cc, "shame-cc", "", "Cc: header for shame report")
	cmd.Flags().StringVar(&s.ReplyTo, "shame-reply-to", "", "Reply-To: header for shame report")
	cmd.Flags().StringVar(&s.AllowedShameDomains, "allowed-shame-domains", "", "comma-separated list of domains we can send shame emails to")
}

type reportData struct {
	loginToEmail     map[string]string
	loginToIssues    map[string][]issueReportData
	totalTests       int
	lowPriorityTests int
}

type issueReportData struct {
	number       int
	age          time.Duration
	lastActivity time.Duration
	priority     string
	title        string
}

func (data issueReportData) String() string {
	days := data.age.Hours() / 24
	active := "inactive"
	if data.lastActivity < time.Hour*24*7 {
		active = "active"
	}
	return fmt.Sprintf("  [%v - %v] %v: %q (%.5v days old) http://issues.k8s.io/%v",
		data.priority, active, data.number, data.title, days, data.number,
	)
}

func gatherData(cfg *githubhelper.Config, labels []string, excludeLowPriority bool) (*reportData, error) {
	issues, err := cfg.ListAllIssues(&github.IssueListByRepoOptions{
		State:  "open",
		Sort:   "created",
		Labels: labels,
	})
	if err != nil {
		return nil, err
	}

	r := reportData{
		loginToEmail:  map[string]string{},
		loginToIssues: map[string][]issueReportData{},
	}
	for _, issue := range issues {
		assignee := "UNASSIGNED"
		if issue.Assignee != nil && issue.Assignee.Login != nil {
			assignee = *issue.Assignee.Login
			if _, ok := r.loginToEmail[assignee]; !ok {
				if u, err := cfg.GetUser(assignee); err == nil {
					if u.Email != nil {
						r.loginToEmail[assignee] = *u.Email
					} else {
						// Don't keep looking this up
						r.loginToEmail[assignee] = ""
					}
				}
			}
		}
		age := time.Duration(0)
		if issue.CreatedAt != nil {
			age = time.Now().Sub(*issue.CreatedAt)
		}
		lastActivity := time.Duration(0)
		if issue.UpdatedAt != nil {
			lastActivity = time.Now().Sub(*issue.UpdatedAt)
		}
		priority := "??"
		priorityLabels := githubhelper.GetLabelsWithPrefix(issue.Labels, "priority/")
		if len(priorityLabels) == 1 {
			priority = strings.TrimPrefix(priorityLabels[0], "priority/")
		}
		if priority == "P2" || priority == "P3" {
			r.lowPriorityTests++
			if excludeLowPriority {
				continue
			}
		}
		reportData := issueReportData{
			priority:     priority,
			number:       *issue.Number,
			title:        *issue.Title,
			age:          age,
			lastActivity: lastActivity,
		}
		r.loginToIssues[assignee] = append(r.loginToIssues[assignee], reportData)
		if priority == "??" {
			const unprioritized = "UNPRIORITIZED"
			r.loginToIssues[unprioritized] = append(r.loginToIssues[unprioritized], reportData)
		}
		r.totalTests++
	}
	return &r, nil
}

func (s *ShameReport) runCmd(r io.Reader) error {
	args := strings.Split(s.Command, " ")
	bin := args[0]
	args = args[1:]
	cmd := exec.Command(bin, args...)
	cmd.Stdin = r
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Report is the workhorse that actually makes the report.
func (s *ShameReport) Report(cfg *githubhelper.Config) error {
	r, err := gatherData(cfg, []string{"kind/flake"}, true)
	if err != nil {
		return err
	}

	individuals, err := s.groupReport(r)
	if err != nil {
		return err
	}
	for user := range individuals {
		if err := s.individualReport(user, r); err != nil {
			return err
		}
	}
	return nil
}

func (s *ShameReport) groupReport(r *reportData) (map[string]bool, error) {
	needsIndividualEmail := map[string]bool{}
	// Gather report body
	chunks := []string{}
	for assignee, issues := range r.loginToIssues {
		strs := []string{}
		// Exclude issues less than three days old if we can
		// individually email the owner.
		for _, data := range issues {
			if data.age < 3*24*time.Hour && s.mayEmail(r.loginToEmail[assignee]) {
				continue
			}
			strs = append(strs, data.String())
		}
		if len(strs) == 0 {
			needsIndividualEmail[assignee] = true
			continue
		}
		sort.Strings(strs)
		chunks = append(chunks, fmt.Sprintf("%v:\n%v", assignee, strings.Join(strs, "\n")))
	}
	sort.Strings(chunks)

	// Gather addresses
	to := []string{}
	missingAddresses := []string{}
	for u, e := range r.loginToEmail {
		if s.mayEmail(e) {
			to = append(to, e)
		} else {
			missingAddresses = append(missingAddresses, u)
		}
	}
	sort.Strings(to)
	sort.Strings(missingAddresses)

	dest := &bytes.Buffer{}

	// Write the report
	if s.From != "" {
		fmt.Fprintf(dest, "From: %v\n", s.From)
	}
	if s.ReplyTo != "" {
		fmt.Fprintf(dest, "Reply-To: %v\n", s.ReplyTo)
	}
	if s.Cc != "" {
		fmt.Fprintf(dest, "Cc: %v\n", s.Cc)
	}
	fmt.Fprintf(dest, "To: %v\n", strings.Join(to, ","))
	fmt.Fprintf(dest, "Subject: Kubernetes flaky Test Report: %v flaky tests\n", r.totalTests)
	fmt.Fprintf(dest, `
If you are in the To: line of this email, you have flaky tests to fix! Flaky
tests, even if they flake only a small percentage of the time, cause the merge
queue to become very long, which causes everyone on the team pain and
suffering. Therefore, if you have flaky tests assigned to you, you should fix
them before doing anything else.  Please either fix the tests assigned to you
or find them an owner who will fix them.

There were %v P2/P3 issues which are not reported here.

Full report:
`, r.lowPriorityTests)
	fmt.Fprintf(dest, "%s\n", strings.Join(chunks, "\n\n"))
	if len(missingAddresses) > 0 {
		fmt.Fprintf(dest, `
These users couldn't be added to the To: line, as we have no address for them:

%v

Individuals with an accessible email and no assignments older than 3 days will
be left off the group email, so please make your email address public in github!

Note: only users with public email addresses ending in %v
are emailed by this system.

`, strings.Join(missingAddresses, ", "), s.AllowedShameDomains)
	}

	return needsIndividualEmail, s.runCmd(dest)
}

func (s *ShameReport) individualReport(user string, r *reportData) error {
	// Gather report body
	issues := r.loginToIssues[user]
	strs := []string{}
	for _, data := range issues {
		strs = append(strs, data.String())
	}
	sort.Strings(strs)
	chunk := fmt.Sprintf("%v:\n%v\n", user, strings.Join(strs, "\n"))

	to := []string{}
	email := r.loginToEmail[user]
	if s.mayEmail(email) {
		to = append(to, email)
	}
	sort.Strings(to)

	dest := &bytes.Buffer{}

	// Write the report
	if s.From != "" {
		fmt.Fprintf(dest, "From: %v\n", s.From)
	}
	if s.ReplyTo != "" {
		fmt.Fprintf(dest, "Reply-To: %v\n", s.ReplyTo)
	}
	// No Cc on individual emails!
	fmt.Fprintf(dest, "To: %v\n", strings.Join(to, ","))
	fmt.Fprintf(dest, "Subject: Kubernetes flaky Test Report: %v flaky tests\n", r.totalTests)
	fmt.Fprintf(dest, `
Hi %v,

This is a note to let you know that you have flaky tests assigned to you.
Owners of tests broken for less than 3 days are left off the group email!

Full report:
`, user)
	fmt.Fprint(dest, chunk)

	return s.runCmd(dest)
}

func (s *ShameReport) mayEmail(email string) bool {
	for _, domain := range strings.Split(s.AllowedShameDomains, ",") {
		if strings.HasSuffix(email, "@"+domain) {
			return true
		}
	}
	return false
}
