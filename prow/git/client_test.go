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

package git

import (
	"net/url"
	"reflect"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
)

type runWithRetries struct {
	retries int
	args    []string
}

type pluggableGitExecutor struct {
	recordedRuns [][]string
	runHandlers  []func(args ...string) (handled bool, output []byte, err error)

	recordedRetries        []runWithRetries
	runWithRetriesHandlers []func(retries int, args ...string) (handled bool, output []byte, err error)

	remoteURL string
}

func (e *pluggableGitExecutor) run(args ...string) ([]byte, error) {
	e.recordedRuns = append(e.recordedRuns, args)
	for _, handler := range e.runHandlers {
		if handled, output, err := handler(args...); handled {
			return output, err
		}
	}
	return nil, nil
}

func (e *pluggableGitExecutor) runWithRetries(retries int, args ...string) ([]byte, error) {
	e.recordedRetries = append(e.recordedRetries, runWithRetries{retries: retries, args: args})
	for _, handler := range e.runWithRetriesHandlers {
		if handled, output, err := handler(retries, args...); handled {
			return output, err
		}
	}
	return nil, nil
}

func (e *pluggableGitExecutor) remote() string {
	return e.remoteURL
}

func TestGitClient(t *testing.T) {
	var testCases = []struct {
		name string
		// returns the gitExecutor mock to use and a func to check output state
		generator func() (gitExecutor, func(t *testing.T))
		run       func(underTest Repo, t *testing.T)
	}{
		{
			name: "checkout calls git checkout successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"checkout", "commitlike"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Checkout("commitlike"); err != nil {
					t.Errorf("expected no error from checkout, got %v", err)
				}
			},
		},
		{
			name: "checkout calls git checkout unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"checkout", "commitlike"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Checkout("commitlike"); err == nil {
					t.Error("expected an error from checkout, got none")
				}
			},
		},
		{
			name: "rev-parse calls git rev-parse successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, []byte("revision"), nil
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"rev-parse", "commitlike"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				revision, err := underTest.RevParse("commitlike")
				if err != nil {
					t.Errorf("expected no error from rev-parse, got %v", err)
				}
				if revision != "revision" {
					t.Errorf("incorrect response from rev-parse, expected revision got %s", revision)
				}
			},
		},
		{
			name: "rev-parse calls git rev-parse unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"rev-parse", "commitlike"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if _, err := underTest.RevParse("commitlike"); err == nil {
					t.Error("expected an error from rev-parse, got none")
				}
			},
		},
		{
			name: "checking out new branch calls git branch successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"checkout", "-b", "branch"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.CheckoutNewBranch("branch"); err != nil {
					t.Errorf("expected no error from checking out new branch, got %v", err)
				}
			},
		},
		{
			name: "checking out new  branch calls git branch unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"checkout", "-b", "branch"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.CheckoutNewBranch("branch"); err == nil {
					t.Error("expected an error from checking out new branch, got none")
				}
			},
		},
		{
			name: "merge calls git merge successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, nil
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"merge", "--no-ff", "--no-stat", "-m merge", "commitlike"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				completed, err := underTest.Merge("commitlike")
				if err != nil {
					t.Errorf("expected no error from merge, got %v", err)
				}
				if !completed {
					t.Error("expected merge to complete correctly")
				}
			},
		},
		{
			name: "merge calls git merge unsuccessfully but aborts",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							for _, arg := range args {
								if arg == "--abort" {
									return true, nil, nil
								}
							}
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"merge", "--no-ff", "--no-stat", "-m merge", "commitlike"}, {"merge", "--abort"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				completed, err := underTest.Merge("commitlike")
				if err != nil {
					t.Errorf("expected no error from merge, got %v", err)
				}
				if completed {
					t.Error("expected merge to not complete correctly")
				}
			},
		},
		{
			name: "merge calls git merge unsuccessfully and cant abort",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"merge", "--no-ff", "--no-stat", "-m merge", "commitlike"}, {"merge", "--abort"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				completed, err := underTest.Merge("commitlike")
				if err == nil {
					t.Error("expected an error from merge, got none")
				}
				if completed {
					t.Error("expected merge to not complete correctly")
				}
			},
		},
		{
			name: "am calls git am successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, nil
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"am", "--3way", "path"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Am("path"); err != nil {
					t.Errorf("expected no error from am, got %v", err)
				}
			},
		},
		{
			name: "am calls git am unsuccessfully but aborts",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							for _, arg := range args {
								if arg == "--abort" {
									return true, nil, nil
								}
							}
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"am", "--3way", "path"}, {"am", "--abort"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Am("path"); err == nil {
					t.Error("expected an error from am, got none")
				}
			},
		},
		{
			name: "am calls git am unsuccessfully and cant abort",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"am", "--3way", "path"}, {"am", "--abort"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Am("path"); err == nil {
					t.Error("expected an error from am, got none")
				}
			},
		},
		{
			name: "push calls git push successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					remoteURL: "git.com",
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"push", "git.com", "branch"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Push("branch"); err != nil {
					t.Errorf("expected no error from push, got %v", err)
				}
			},
		},
		{
			name: "push calls git push unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
					remoteURL: "git.com",
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"push", "git.com", "branch"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Push("branch"); err == nil {
					t.Error("expected an error from push, got none")
				}
			},
		},
		{
			name: "checking out pull request calls git fetch and checkout successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					remoteURL: "git.com",
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRetries, []runWithRetries{{retries: 3, args: []string{"fetch", "git.com", "pull/123/head:pull123"}}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
					if actual, expected := e.recordedRuns, [][]string{{"checkout", "pull123"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.CheckoutPullRequest(123); err != nil {
					t.Errorf("expected no error from push, got %v", err)
				}
			},
		},
		{
			name: "checking out pull request calls git fetch unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runWithRetriesHandlers: []func(retries int, args ...string) (handled bool, output []byte, err error){
						func(retries int, args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
					remoteURL: "git.com",
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRetries, []runWithRetries{{retries: 3, args: []string{"fetch", "git.com", "pull/123/head:pull123"}}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
					var nothing [][]string
					if actual, expected := e.recordedRuns, nothing; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.CheckoutPullRequest(123); err == nil {
					t.Error("expected an error from push, got none")
				}
			},
		},
		{
			name: "config calls git config successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"config", "key", "value"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Config("key", "value"); err != nil {
					t.Errorf("expected no error from config, got %v", err)
				}
			},
		},
		{
			name: "config calls git config unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"config", "key", "value"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if err := underTest.Config("key", "value"); err == nil {
					t.Error("expected an error from config, got none")
				}
			},
		},
		{
			name: "log calls git log successfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, []byte("log"), nil
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"log", "--oneline"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				log, err := underTest.Log()
				if err != nil {
					t.Errorf("expected no error from log, got %v", err)
				}
				if log != "log" {
					t.Errorf("incorrect response from log, expected log got %s", log)
				}
			},
		},
		{
			name: "log calls git log unsuccessfully",
			generator: func() (executor gitExecutor, check func(t *testing.T)) {
				e := &pluggableGitExecutor{
					runHandlers: []func(args ...string) (handled bool, output []byte, err error){
						func(args ...string) (handled bool, output []byte, err error) {
							return true, nil, errors.New("fail!")
						},
					},
				}
				return e, func(t *testing.T) {
					if actual, expected := e.recordedRuns, [][]string{{"log", "--oneline"}}; !reflect.DeepEqual(actual, expected) {
						t.Errorf("incorrect set of git calls: %v", diff.ObjectReflectDiff(actual, expected))
					}
				}
			},
			run: func(underTest Repo, t *testing.T) {
				if _, err := underTest.Log(); err == nil {
					t.Error("expected an error from log, got none")
				}
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			executor, check := testCase.generator()
			testCase.run(Repo{logger: logrus.WithField("test-case", testCase.name), executor: executor}, t)
			check(t)
		})
	}
}

type initializeOutput struct {
	initial bool
	cache   cacheInteractor
	err     error
}

type cloneOutput struct {
	cache cacheInteractor
	err   error
}

type fakeInitializer struct {
	initializeOutputs map[string]initializeOutput

	lastClone    int
	cloneOutputs []cloneOutput
}

func (i *fakeInitializer) initialize(orgRepo string) (bool, cacheInteractor, error) {
	if output, exists := i.initializeOutputs[orgRepo]; exists {
		return output.initial, output.cache, output.err
	}
	return false, nil, nil
}

func (i *fakeInitializer) initializeClone() (cacheInteractor, error) {
	output := i.cloneOutputs[i.lastClone]
	i.lastClone++
	return output.cache, output.err
}

func TestClient_Clone(t *testing.T) {
	initializer := &fakeInitializer{
		initializeOutputs: map[string]initializeOutput{
			"org/repo":  {initial: true, cache: &cacheDirInteractor{cacheDir: "global"}, err: nil},
			"org/other": {initial: false, cache: &cacheDirInteractor{cacheDir: "globalOther"}, err: nil},
		},
		cloneOutputs: []cloneOutput{
			{cache: &cacheDirInteractor{cacheDir: "local"}, err: nil},
			{cache: &cacheDirInteractor{cacheDir: "localOther"}, err: nil},
		},
	}

	executors := map[string]map[string]*pluggableGitExecutor{
		"org/repo": {
			"global": &pluggableGitExecutor{
				remoteURL: "server.com/repo",
			},
			"local": &pluggableGitExecutor{},
		},
		"org/other": {
			"globalOther": &pluggableGitExecutor{
				remoteURL: "server.com/other",
			},
			"localOther": &pluggableGitExecutor{},
		},
	}
	factory := func(repo string, interactor cacheInteractor) gitExecutor {
		if options, exist := executors[repo]; exist {
			if executor, exists := options[interactor.dir()]; exists {
				return executor
			}
		}
		t.Errorf("asked for a executor for repo %q on dir %q, none exists", repo, interactor.dir())
		return nil
	}

	client := Client{
		logger:          logrus.WithField("test-case", "clone"),
		executorFactory: factory,
		initializer:     initializer,
		repoLocks:       make(map[string]*sync.Mutex),
	}

	_, err := client.Clone("org/repo")
	if err != nil {
		t.Errorf("failed to clone first repo: %v", err)
	}
	if actual, expected := executors["org/repo"]["global"].recordedRetries, []runWithRetries{
		{retries: 3, args: []string{"clone", "--mirror", "server.com/repo", "global"}},
		{retries: 3, args: []string{"fetch"}},
	}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect set of global git calls: %v", diff.ObjectReflectDiff(actual, expected))
	}
	if actual, expected := executors["org/repo"]["local"].recordedRuns, [][]string{{"clone", "global", "local"}}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect set of local git calls: %v", diff.ObjectReflectDiff(actual, expected))
	}

	_, err = client.Clone("org/other")
	if err != nil {
		t.Errorf("failed to clone second repo: %v", err)
	}
	if actual, expected := executors["org/other"]["globalOther"].recordedRetries, []runWithRetries{
		{retries: 3, args: []string{"fetch"}},
	}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect set of global git calls: %v", diff.ObjectReflectDiff(actual, expected))
	}
	if actual, expected := executors["org/other"]["localOther"].recordedRuns, [][]string{{"clone", "globalOther", "localOther"}}; !reflect.DeepEqual(actual, expected) {
		t.Errorf("incorrect set of local git calls: %v", diff.ObjectReflectDiff(actual, expected))
	}

}

func TestExecutorFor(t *testing.T) {
	remote, err := url.Parse("https://server.somewhere.com/foo/bar")
	if err != nil {
		t.Fatalf("got unexpected error parsing URL: %v", err)
	}

	originalRemote := *remote

	git := "git"
	executor := executorFor(git, remote)("org/repo", &cacheDirInteractor{cacheDir: "somewhere"})

	newRemote := *remote
	if actual, expected := newRemote, originalRemote; !reflect.DeepEqual(actual, expected) {
		t.Errorf("creating executor mutated seed URL: %s", diff.ObjectReflectDiff(actual, expected))
	}

	if actual, expected := "https://server.somewhere.com/foo/bar/org/repo", executor.remote(); !reflect.DeepEqual(actual, expected) {
		t.Errorf("executor has incorrect remote: %s", diff.ObjectReflectDiff(actual, expected))
	}
}
