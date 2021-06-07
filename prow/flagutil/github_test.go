/*
Copyright 2020 The Kubernetes Authors.

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

package flagutil

import (
	"flag"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"

	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/github"
)

func TestGitHubOptions_Validate(t *testing.T) {
	t.Parallel()
	var testCases = []struct {
		name                    string
		in                      *GitHubOptions
		expectedGraphqlEndpoint string
		expectedErr             bool
	}{
		{
			name:                    "when no endpoints, sets graphql endpoint",
			in:                      &GitHubOptions{},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             false,
		},
		{
			name: "when empty endpoint, sets graphql endpoint",
			in: &GitHubOptions{
				endpoint: NewStrings(""),
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             false,
		},
		{
			name: "when invalid github endpoint, returns error",
			in: &GitHubOptions{
				endpoint: NewStrings("not a github url"),
			},
			expectedErr: true,
		},
		{
			name: "both --github-hourly-tokens and --github-allowed-burst are zero: no error",
			in: &GitHubOptions{
				ThrottleHourlyTokens: 0,
				ThrottleAllowBurst:   0,
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
		},
		{
			name: "both --github-hourly-tokens and --github-allowed-burst are nonzero and hourly is higher or equal: no error",
			in: &GitHubOptions{
				ThrottleHourlyTokens: 100,
				ThrottleAllowBurst:   100,
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
		},
		{
			name: "both --github-hourly-tokens and --github-allowed-burst are nonzero and hourly is lower: error",
			in: &GitHubOptions{
				ThrottleHourlyTokens: 10,
				ThrottleAllowBurst:   100,
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             true,
		},
		{
			name: "only --github-hourly-tokens is nonzero: error",
			in: &GitHubOptions{
				ThrottleHourlyTokens: 10,
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             true,
		},
		{
			name: "only --github-hourly-tokens is zero: no error, allows easier throttling disable",
			in: &GitHubOptions{
				ThrottleAllowBurst: 10,
			},
			expectedGraphqlEndpoint: github.DefaultGraphQLEndpoint,
			expectedErr:             false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(s *testing.T) {
			err := testCase.in.Validate(false)
			if testCase.expectedErr && err == nil {
				t.Errorf("%s: expected an error but got none", testCase.name)
			}
			if !testCase.expectedErr && err != nil {
				t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
			}
			if testCase.expectedGraphqlEndpoint != testCase.in.graphqlEndpoint {
				t.Errorf("%s: unexpected graphql endpoint", testCase.name)
			}
		})
	}
}

// TestGitHubOptionsConstructsANewClientOnEachInvocation verifies that multiple invocations do not
// return the same client. This is important for components that use multiple clients with different
// settings, like for example for the throttling.
func TestGitHubOptionsConstructsANewClientOnEachInvocation(t *testing.T) {
	o := &GitHubOptions{}
	secretAgent := &secret.Agent{}

	firstClient, err := o.githubClient(secretAgent, false)
	if err != nil {
		t.Fatalf("failed to construct first client: %v", err)
	}
	secondClient, err := o.githubClient(secretAgent, false)
	if err != nil {
		t.Fatalf("failed to construct second client: %v", err)
	}

	firstClientAddr, secondClientAddr := fmt.Sprintf("%p", firstClient), fmt.Sprintf("%p", secondClient)
	if firstClientAddr == secondClientAddr {
		t.Error("got the same client twice on subsequent invocation")
	}
}

func TestCustomThrottlerOptions(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name   string
		params []FlagParameter

		expectPresent map[string]bool
		expectDefault map[string]int
	}{
		{
			name:          "no customizations",
			expectPresent: map[string]bool{"github-hourly-tokens": true, "github-allowed-burst": true},
			expectDefault: map[string]int{"github-hourly-tokens": 0, "github-allowed-burst": 0},
		},
		{
			name:          "suppress presence",
			params:        []FlagParameter{DisableThrottlerOptions()},
			expectPresent: map[string]bool{"github-hourly-tokens": false, "github-allowed-burst": false},
		},
		{
			name:          "custom defaults",
			params:        []FlagParameter{ThrottlerDefaults(100, 20)},
			expectPresent: map[string]bool{"github-hourly-tokens": true, "github-allowed-burst": true},
			expectDefault: map[string]int{"github-hourly-tokens": 100, "github-allowed-burst": 20},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet(tc.name, flag.ExitOnError)
			opts := &GitHubOptions{}
			opts.AddCustomizedFlags(fs, tc.params...)
			for _, name := range []string{"github-hourly-tokens", "github-allowed-burst"} {
				flg := fs.Lookup(name)
				if (flg != nil) != (tc.expectPresent[name]) {
					t.Errorf("Flag --%s presence differs: expected %t got %t", name, tc.expectPresent[name], flg != nil)
					continue
				}
				expected := strconv.Itoa(tc.expectDefault[name])
				if flg != nil && flg.DefValue != expected {
					t.Errorf("Flag --%s default value differs: expected %#v got '%#v'", name, expected, flg.DefValue)
				}
			}
		})
	}
}

func TestOrgThottlerOptions(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		parameters []string

		expectedErrorMsg            string
		expectedParsedOrgThrottlers map[string]throttlerSettings
	}{
		{
			name: "No org throttler, success",
		},
		{
			name:             "Invalid format, a colon too much",
			parameters:       []string{"--github-throttle-org=kubernetes:10:10:10"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:10:10:10 is not in org:hourlyTokens:burst format",
		},
		{
			name:             "Invalid format, a colon too little",
			parameters:       []string{"--github-throttle-org=kubernetes:10"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:10 is not in org:hourlyTokens:burst format",
		},
		{
			name:             "Invalid format, hourly tokens not an int",
			parameters:       []string{"--github-throttle-org=kubernetes:a:10"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:a:10 is not in org:hourlyTokens:burst format: hourlyTokens is not an int",
		},
		{
			name:             "Invalid format, burst not an int",
			parameters:       []string{"--github-throttle-org=kubernetes:10:a"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:10:a is not in org:hourlyTokens:burst format: burst is not an int",
		},
		{
			name:             "Invalid, burst > hourly tokens",
			parameters:       []string{"--github-throttle-org=kubernetes:10:11"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:10:11: burst must not be greater than hourlyTokens",
		},
		{
			name:             "Invalid, burst < 1",
			parameters:       []string{"--github-throttle-org=kubernetes:10:0"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:10:0: burst must be > 0",
		},
		{
			name:             "Invalid, hourly tokens < 1",
			parameters:       []string{"--github-throttle-org=kubernetes:0:10"},
			expectedErrorMsg: "-github-throttle-org=kubernetes:0:10: hourlyTokens must be > 0",
		},
		{
			name: "Invalid, multiple settings for same org",
			parameters: []string{
				"--github-throttle-org=kubernetes:10:10",
				"--github-throttle-org=kubernetes:10:10",
			},
			expectedErrorMsg: "got multiple -github-throttle-org for the kubernetes org",
		},
		{
			name:                        "Valid single org setting, success",
			parameters:                  []string{"--github-throttle-org=kubernetes:10:10"},
			expectedParsedOrgThrottlers: map[string]throttlerSettings{"kubernetes": {hourlyTokens: 10, burst: 10}},
		},
		{
			name: "Valid settings for multiple orgs, success",
			parameters: []string{
				"--github-throttle-org=kubernetes:10:10",
				"--github-throttle-org=kubernetes-sigs:10:10",
			},
			expectedParsedOrgThrottlers: map[string]throttlerSettings{
				"kubernetes":      {hourlyTokens: 10, burst: 10},
				"kubernetes-sigs": {hourlyTokens: 10, burst: 10},
			},
		},
	}

	exportThrottlerSettings := cmp.Exporter(func(t reflect.Type) bool {
		return t == reflect.TypeOf(throttlerSettings{})
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet(tc.name, flag.ContinueOnError)
			opts := &GitHubOptions{}
			opts.AddFlags(fs)
			if err := fs.Parse(tc.parameters); err != nil {
				t.Fatalf("flag parsing failed: %v", err)
			}
			opts.AppID = "10"
			opts.AppPrivateKeyPath = "/test/path"

			var actualErrMsg string
			if actualErr := opts.Validate(false); actualErr != nil {
				actualErrMsg = actualErr.Error()
			}
			if actualErrMsg != tc.expectedErrorMsg {
				t.Fatalf("actual error %s does not match expected error %s", actualErrMsg, tc.expectedErrorMsg)
			}
			if actualErrMsg != "" {
				return
			}

			if diff := cmp.Diff(tc.expectedParsedOrgThrottlers, opts.parsedOrgThrottlers, exportThrottlerSettings); diff != "" {
				t.Errorf("expected org throttlers differ from actual: %s", diff)
			}
		})
	}
}
