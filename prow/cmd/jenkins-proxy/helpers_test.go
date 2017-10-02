package main

import (
	"net/url"
	"testing"
)

func TestGetRequestedJob(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "root",
			path:     "/",
			expected: "",
		},
		{
			name:     "bad call-out",
			path:     "/job",
			expected: "",
		},
		{
			name:     "anther bad call-out",
			path:     "/jobs/broken",
			expected: "",
		},
		{
			name:     "unrelated",
			path:     "/queue/api/json",
			expected: "",
		},
		{
			name:     "there is a job",
			path:     "/job/test_pull_request_this/json",
			expected: "test_pull_request_this",
		},
		{
			name:     "another job call",
			path:     "/job/foo_job/api/json?tree=builds[number,result,actions[parameters[name,value]]]",
			expected: "foo_job",
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)
		got := getRequestedJob(test.path)
		if got != test.expected {
			t.Errorf("expected %q, got %q", test.expected, got)
		}
	}
}

func TestReplaceHostname(t *testing.T) {
	tests := []struct {
		name      string
		u         *url.URL
		masterURL string
		expected  string
	}{
		{
			name: "simple",
			u: &url.URL{
				Scheme: "http",
				Host:   "jenkins-proxy",
				Path:   "/api/json",
			},
			masterURL: "https://ci.openshift.redhat.com/jenkins",
			expected:  "https://ci.openshift.redhat.com/jenkins/api/json",
		},
		{
			name: "includes raw query",
			u: &url.URL{
				Scheme:   "http",
				Host:     "jenkins-proxy",
				Path:     "/api/json",
				RawQuery: "tree=jobs[name]",
			},
			masterURL: "https://ci.openshift.redhat.com/jenkins",
			expected:  "https://ci.openshift.redhat.com/jenkins/api/json?tree=jobs[name]",
		},
	}

	for _, test := range tests {
		t.Logf("scenario %q", test.name)
		got := replaceHostname(test.u, test.masterURL)
		if got != test.expected {
			t.Errorf("expected %q, got %q", test.expected, got)
		}
	}
}
