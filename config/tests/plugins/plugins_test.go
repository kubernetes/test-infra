package plugins

import (
	"k8s.io/test-infra/prow/plugins"
	"os"
	"sigs.k8s.io/yaml"
	"strings"
	"testing"
)

// TestApprovePluginConfig validates that there are no duplicate repos in the approve plugin config.
func TestApprovePluginConfig(t *testing.T) {
	pa := &plugins.ConfigAgent{}

	b, err := os.ReadFile("../../prow/plugins.yaml")
	if err != nil {
		t.Fatalf("Failed to read plugin config: %v.", err)
	}
	np := &plugins.Configuration{}
	if err := yaml.Unmarshal(b, np); err != nil {
		t.Fatalf("Failed to unmarshal plugin config: %v.", err)
	}
	pa.Set(np)

	orgs := map[string]bool{}
	repos := map[string]bool{}
	for _, config := range pa.Config().Approve {
		for _, entry := range config.Repos {
			if strings.Contains(entry, "/") {
				if repos[entry] {
					t.Errorf("The repo %q is duplicated in the 'approve' plugin configuration.", entry)
				}
				repos[entry] = true
			} else {
				if orgs[entry] {
					t.Errorf("The org %q is duplicated in the 'approve' plugin configuration.", entry)
				}
				orgs[entry] = true
			}
		}
	}
}

// TestWelcomePluginConfig validates that there are no duplicate repos in the welcome plugin config.
func TestWelcomePluginConfig(t *testing.T) {
	pa := &plugins.ConfigAgent{}

	b, err := os.ReadFile("../../prow/plugins.yaml")
	if err != nil {
		t.Fatalf("Failed to read plugin config: %v.", err)
	}
	np := &plugins.Configuration{}
	if err := yaml.Unmarshal(b, np); err != nil {
		t.Fatalf("Failed to unmarshal plugin config: %v.", err)
	}
	pa.Set(np)

	orgs := map[string]bool{}
	repos := map[string]bool{}
	for _, config := range pa.Config().Welcome {
		for _, entry := range config.Repos {
			if strings.Contains(entry, "/") {
				if repos[entry] {
					t.Errorf("The repo %q is duplicated in the 'welcome' plugin configuration.", entry)
				}
				repos[entry] = true
			} else {
				if orgs[entry] {
					t.Errorf("The org %q is duplicated in the 'welcome' plugin configuration.", entry)
				}
				orgs[entry] = true
			}
		}
	}
	for repo := range repos {
		org := strings.Split(repo, "/")[0]
		if orgs[org] {
			t.Errorf("The repo %q is duplicated with %q in the 'welcome' plugin configuration.", repo, org)
		}
	}
}
