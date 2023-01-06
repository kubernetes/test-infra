/*
Copyright 2018 The Kubernetes Authors.

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

package clonerefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

// Options configures the clonerefs tool
// completely and may be provided using JSON
// or user-specified flags, but not both.
type Options struct {
	// SrcRoot is the root directory under which
	// all source code is cloned
	SrcRoot string `json:"src_root"`
	// Log is the log file to which clone records are written
	Log string `json:"log"`

	// GitUserName is an optional field that is used with
	// `git config user.name`
	GitUserName string `json:"git_user_name,omitempty"`
	// GitUserEmail is an optional field that is used with
	// `git config user.email`
	GitUserEmail string `json:"git_user_email,omitempty"`

	// GitRefs are the refs to clone
	GitRefs []prowapi.Refs `json:"refs"`
	// KeyFiles are files containing SSH keys to be used
	// when cloning. Will be added to `ssh-agent`.
	KeyFiles []string `json:"key_files,omitempty"`

	// OauthTokenFile is the path of a file that contains an OAuth token.
	OauthTokenFile string `json:"oauth_token_file,omitempty"`

	// HostFingerPrints are ssh-keyscan host fingerprint lines to use
	// when cloning. Will be added to ~/.ssh/known_hosts
	HostFingerprints []string `json:"host_fingerprints,omitempty"`

	// MaxParallelWorkers determines how many repositories
	// can be cloned in parallel. If 0, interpreted as no
	// limit to parallelism
	MaxParallelWorkers int `json:"max_parallel_workers,omitempty"`

	Fail bool `json:"fail,omitempty"`

	CookiePath string `json:"cookie_path,omitempty"`

	GitHubAPIEndpoints      []string `json:"github_api_endpoints,omitempty"`
	GitHubAppID             string   `json:"github_app_id,omitempty"`
	GitHubAppPrivateKeyFile string   `json:"github_app_private_key_file,omitempty"`

	// used to hold flag values
	refs      gitRefs
	clonePath orgRepoFormat
	cloneURI  orgRepoFormat
	keys      stringSlice
}

// Validate ensures that the configuration options are valid
func (o *Options) Validate() error {
	if o.SrcRoot == "" {
		return errors.New("no source root specified")
	}

	if o.Log == "" {
		return errors.New("no log file specified")
	}

	if len(o.GitRefs) == 0 {
		return errors.New("no refs specified to clone")
	}

	seen := make(map[string]int)
	for i, ref := range o.GitRefs {
		path := clone.PathForRefs(o.SrcRoot, ref)
		if existing, ok := seen[path]; ok {
			existingRef := o.GitRefs[existing]
			err := fmt.Errorf("clone ref config %d (for %s/%s) will be extracted to %s, which clone ref %d (for %s/%s) is also using", i, ref.Org, ref.Repo, path, existing, existingRef.Org, existingRef.Repo)
			if existingRef.Org == ref.Org && existingRef.Repo == ref.Repo {
				return err
			}
			// preserving existing behavior where this is a warning, not an error
			logrus.WithError(err).WithField("path", path).Warning("multiple refs clone to the same location")
		}
		seen[path] = i
	}

	if o.GitHubAppID != "" || o.GitHubAppPrivateKeyFile != "" {
		if o.OauthTokenFile != "" {
			return errors.New("multiple authentication methods specified")
		}
		if len(o.GitHubAPIEndpoints) == 0 {
			return errors.New("no GitHub API endpoints for GitHub App authentication")
		}
	}
	if o.GitHubAppID != "" && o.GitHubAppPrivateKeyFile == "" {
		return errors.New("no GitHub App private key file specified")
	}
	if o.GitHubAppID == "" && o.GitHubAppPrivateKeyFile != "" {
		return errors.New("no GitHub App ID specified")
	}

	return nil
}

const (
	// JSONConfigEnvVar is the environment variable that
	// clonerefs expects to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "CLONEREFS_OPTIONS"
	// DefaultGitUserName is the default name used in git config
	DefaultGitUserName = "ci-robot"
	// DefaultGitUserEmail is the default email used in git config
	DefaultGitUserEmail = "ci-robot@k8s.io"
)

// ConfigVar exposes the environment variable used
// to store serialized configuration
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.GitRefs = o.refs.gitRefs
	o.KeyFiles = o.keys.data

	for _, ref := range o.GitRefs {
		alias, err := o.clonePath.Execute(OrgRepo{Org: ref.Org, Repo: ref.Repo})
		if err != nil {
			panic(err)
		}
		ref.PathAlias = alias

		alias, err = o.cloneURI.Execute(OrgRepo{Org: ref.Org, Repo: ref.Repo})
		if err != nil {
			panic(err)
		}
		ref.CloneURI = alias
	}
}

// AddFlags adds flags to the FlagSet that populate
// the GCS upload options struct given.
func (o *Options) AddFlags(fs *flag.FlagSet) {
	fs.StringVar(&o.SrcRoot, "src-root", "", "Where to root source checkouts")
	fs.StringVar(&o.Log, "log", "", "Where to write logs")
	fs.StringVar(&o.GitUserName, "git-user-name", DefaultGitUserName, "Username to set in git config")
	fs.StringVar(&o.GitUserEmail, "git-user-email", DefaultGitUserEmail, "Email to set in git config")
	fs.Var(&o.refs, "repo", "Mapping of Git URI to refs to check out, can be provided more than once")
	fs.Var(&o.keys, "ssh-key", "Path to SSH key to enable during cloning, can be provided more than once")
	fs.Var(&o.clonePath, "clone-alias", "Format string for the path to clone to")
	fs.Var(&o.cloneURI, "uri-prefix", "Format string for the URI prefix to clone from")
	fs.IntVar(&o.MaxParallelWorkers, "max-workers", 0, "Maximum number of parallel workers, unset for unlimited.")
	fs.StringVar(&o.CookiePath, "cookiefile", "", "Path to git http.cookiefile")
	fs.BoolVar(&o.Fail, "fail", false, "Exit with failure if any of the refs can't be fetched.")
}

type gitRefs struct {
	gitRefs []prowapi.Refs
}

func (r *gitRefs) String() string {
	representation := bytes.Buffer{}
	for _, ref := range r.gitRefs {
		fmt.Fprintf(&representation, "%s,%s=%s", ref.Org, ref.Repo, ref.String())
	}
	return representation.String()
}

// Set parses out a prowapi.Refs from the user string.
// The following example shows all possible fields:
//   org,repo=base-ref:base-sha[,pull-number:pull-sha]...
// For the base ref and every pull number, the SHAs
// are optional and any number of them may be set or
// unset.
func (r *gitRefs) Set(value string) error {
	gitRef, err := ParseRefs(value)
	if err != nil {
		return err
	}
	r.gitRefs = append(r.gitRefs, *gitRef)
	return nil
}

type stringSlice struct {
	data []string
}

func (r *stringSlice) String() string {
	return strings.Join(r.data, ",")
}

// Set records the value passed
func (r *stringSlice) Set(value string) error {
	r.data = append(r.data, value)
	return nil
}

type orgRepoFormat struct {
	raw    string
	format *template.Template
}

func (a *orgRepoFormat) String() string {
	return a.raw
}

// Set parses out overrides from user input
func (a *orgRepoFormat) Set(value string) error {
	templ, err := template.New("format").Parse(value)
	if err != nil {
		return err
	}
	a.raw = value
	a.format = templ
	return nil
}

// OrgRepo hold both an org and repo name.
type OrgRepo struct {
	Org, Repo string
}

func (a *orgRepoFormat) Execute(data OrgRepo) (string, error) {
	if a.format != nil {
		output := bytes.Buffer{}
		err := a.format.Execute(&output, data)
		return output.String(), err
	}
	return "", nil
}

// Encode will encode the set of options in the format that
// is expected for the configuration environment variable
func Encode(options Options) (string, error) {
	encoded, err := json.Marshal(options)
	return string(encoded), err
}
