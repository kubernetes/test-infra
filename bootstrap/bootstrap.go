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

// bootstrap implements a drop-in-replacement for jenkins/bootstrap.py
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	flag "github.com/spf13/pflag"
)

// Constant Keys for known environment variables and URLs
const (
	BuildEnv          string = "BUILD_NUMBER"
	BootstrapEnv      string = "BOOTSTRAP_MIGRATION"
	CloudSDKEnv       string = "CLOUDSDK_CONFIG"
	GCEKeyEnv         string = "JENKINS_GCE_SSH_PRIVATE_KEY_FILE"
	Gubernator        string = "https://k8s-gubernator.appspot.com/build"
	HomeEnv           string = "HOME"
	JenkinsHomeEnv    string = "JENKINS_HOME"
	JobEnv            string = "JOB_NAME"
	NodeEnv           string = "NODE_NAME"
	ServiceAccountEnv string = "GOOGLE_APPLICATION_CREDENTIALS"
	WorkspaceEnv      string = "WORKSPACE"
	GCSArtifactsEnv   string = "GCS_ARTIFACTS_DIR"
)

// Args contains all of the (parsed) command line arguments for bootstrap
// NOTE: Repo should be further parsed by ParseRepos
type Args struct {
	Root           string
	Timeout        int32
	Repo           []string
	Bare           bool
	Job            string
	Upload         string
	ServiceAccount string
	SSH            string
	GitCache       string
	Clean          bool
}

// ParseArgs parses the command line to an Args instance
// arguments should be generally be os.Args[1:]
func ParseArgs(arguments []string) (*Args, error) {
	args := &Args{}
	flags := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	// used to mimic required=true
	requiredFlags := []string{"job"}

	// add all of the args from parse_args in jenkins/bootstrap.py
	flags.StringVar(&args.Root, "root", ".", "Root dir to work with")

	// NOTE: jenkins/bootstrap.py technically used a float for this arg
	// when parsing but only ever used the arg as an integer number
	// of timeout minutes.
	// int32 makes more sense and will let use time.Minute * timeout
	flags.Int32Var(&args.Timeout, "timeout", 0, "Timeout in minutes if set")

	flags.StringArrayVar(&args.Repo, "repo", []string{},
		"Fetch the specified repositories, with the first one considered primary")

	flags.BoolVar(&args.Bare, "bare", false, "Do not check out a repository")
	flags.Lookup("bare").NoOptDefVal = "true" // allows using --bare

	// NOTE: this arg is required (set above)
	flags.StringVar(&args.Job, "job", "", "Name of the job to run")

	flags.StringVar(&args.Upload, "upload", "",
		"Upload results here if set, requires --service-account")
	flags.StringVar(&args.ServiceAccount, "service-account", "",
		"Activate and use path/to/service-account.json if set.")
	flags.StringVar(&args.SSH, "ssh", "",
		"Use the ssh key to fetch the repository instead of https if set.")
	flags.StringVar(&args.GitCache, "git-cache", "", "Location of the git cache.")

	flags.BoolVar(&args.Clean, "clean", false, "Clean the git repo before running tests.")
	flags.Lookup("clean").NoOptDefVal = "true" // allows using --clean

	// parse then check required args
	err := flags.Parse(arguments)
	if err != nil {
		return nil, err
	}
	for _, arg := range requiredFlags {
		if flag := flags.Lookup(arg); !flag.Changed {
			err = fmt.Errorf("flag '--%s' is required but was not set", flag.Name)
			return nil, err
		}
	}

	// validate args
	if args.Bare == (len(args.Repo) != 0) {
		err = fmt.Errorf("expected --repo xor --bare, Got: --repo=%v, --bare=%v", args.Repo, args.Bare)
		return nil, err
	}
	if args.Job == "" {
		return nil, fmt.Errorf("--job=\"\" is not valid, please supply a job name")
	}

	return args, nil
}

// Repo contains the components of git repo refs used in bootstrap
type Repo struct {
	Name   string
	Branch string
	Pull   string
}

// Repos is a slice of Repo where Repos[0] is the main repo
type Repos []Repo

// Main returns the primary repo in a Repos produced by ParseRepos
func (r Repos) Main() *Repo {
	if len(r) == 0 {
		return nil
	}
	return &r[0]
}

// ParseRepos converts the refs related arguments to []Repo
// each repoArgs is expect to be "name=branch:commit,branch:commit"
// with one or more comma seperated "branch:commit".
// EG: "k8s.io/kubernetes=master:42e2ca8c18c93ba25eb0e5bd02ecba2eaa05e871,52057:b4f639f57ae0a89cdf1b43d1810b617c76f4b1b3"
func ParseRepos(repoArgs []string) (Repos, error) {
	repos := []Repo{}
	re := regexp.MustCompile(`([^=]+)(=([^:,~^\s]+(:[0-9a-fA-F]+)?(,|$))+)?$`)
	for _, repoArg := range repoArgs {
		match := re.FindStringSubmatch(repoArg)
		if len(match) == 0 {
			return nil, fmt.Errorf("could not parse repo: %s, %v", repoArg, repos)
		}
		thisRepo := match[1]
		// default to master
		if match[2] == "" {
			repos = append(repos, Repo{
				Name:   thisRepo,
				Branch: "master",
				Pull:   "",
			})
			continue
		}
		commitsString := match[2][1:]
		commits := strings.Split(commitsString, ",")
		// Checking out a branch, possibly at a specific commit
		if len(commits) == 1 {
			repos = append(repos, Repo{
				Name:   thisRepo,
				Branch: commits[0],
				Pull:   "",
			})
			continue
		}
		// Checking out one or more PRs
		repos = append(repos, Repo{
			Name:   thisRepo,
			Branch: "",
			Pull:   commitsString,
		})
	}
	return repos, nil
}

func refHasSHAs(ref string) bool {
	return strings.Contains(ref, ":")
}

// PullNumbers converts a reference list string into a list of PR number strings
func PullNumbers(pull string) []string {
	if refHasSHAs(pull) {
		res := []string{}
		parts := strings.Split(pull, ",")
		for _, part := range parts {
			res = append(res, strings.Split(part, ":")[0])
		}
		return res[1:]
	}
	return []string{pull}
}

// Repository returns the url associated with the repo
func Repository(repo string, ssh bool) string {
	if strings.HasPrefix(repo, "k8s.io/") {
		repo = "github.com/kubernetes" + strings.TrimPrefix(repo, "k8s.io/")
	}
	if ssh {
		if !refHasSHAs(repo) {
			repo = strings.Replace(repo, "/", ":", 1)
		}
		return "git@" + repo
	}
	return "https://" + repo
}

// Paths contains all of the upload/file paths used in a run of bootstrap
type Paths struct {
	Artifacts     string
	BuildLog      string
	PRPath        string
	PRBuildLink   string
	PRLatest      string
	PRResultCache string
	ResultCache   string
	Started       string
	Finished      string
	Latest        string
}

// CIPaths returns a Paths for a CI Job
func CIPaths(base, job, build string) *Paths {
	return &Paths{
		Artifacts:   filepath.Join(base, job, build, "artifacts"),
		BuildLog:    filepath.Join(base, job, build, "build-log.txt"),
		Finished:    filepath.Join(base, job, build, "finished.json"),
		Latest:      filepath.Join(base, job, "latest-build.txt"),
		ResultCache: filepath.Join(base, job, "jobResultsCache.json"),
		Started:     filepath.Join(base, job, build, "started.json"),
	}
}

// PRPaths returns a Paths for a Pull Request
func PRPaths(base string, repos Repos, job, build string) (*Paths, error) {
	if len(repos) == 0 {
		return nil, fmt.Errorf("repos should not be empty")
	}
	repo := repos.Main()
	var prefix string
	if repo.Name == "k8s.io/kubernetes" || repo.Name == "kubernetes/kubernetes" {
		prefix = ""
	} else if strings.HasPrefix(repo.Name, "k8s.io/") {
		prefix = repo.Name[len("k8s.io/"):]
	} else if strings.HasPrefix(repo.Name, "kubernetes/") {
		prefix = repo.Name[len("kubernetes/"):]
	} else if strings.HasPrefix(repo.Name, "github.com/") {
		prefix = strings.Replace(repo.Name[len("github.com/"):], "/", "_", -1)
	}
	// Batch merges are those with more than one PR specified.
	prNums := PullNumbers(repo.Pull)
	var pull string
	if len(prNums) > 1 {
		pull = filepath.Join(prefix, "batch")
	} else {
		pull = filepath.Join(prefix, prNums[0])
	}
	prPath := filepath.Join(base, "pull", pull, job, build)
	return &Paths{
		Artifacts:     filepath.Join(prPath, "artifacts"),
		BuildLog:      filepath.Join(prPath, "build-log.txt"),
		PRPath:        prPath,
		Finished:      filepath.Join(prPath, "finished.json"),
		Latest:        filepath.Join(base, "directory", job, "latest-build.txt"),
		PRBuildLink:   filepath.Join(base, "directory", job, build+".txt"),
		PRLatest:      filepath.Join(base, "pull", pull, job, "latest-build.txt"),
		PRResultCache: filepath.Join(base, "pull", pull, job, "jobResultsCache.json"),
		ResultCache:   filepath.Join(base, "directory", job, "jobResultsCache.json"),
		Started:       filepath.Join(prPath, "started.json"),
	}, nil
}

// Bootstrap is the "real main" for bootstrap, after argument parsing
func Bootstrap(args *Args) error {
	repos, err := ParseRepos(args.Repo)
	if err != nil {
		return err
	}

	originalCWD, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get CWD! %v", err)
	}

	build := "fake" // TODO: port computing this value

	var paths *Paths
	if args.Upload != "" {
		if repos.Main().Pull != "" {
			paths, err = PRPaths(args.Upload, repos, args.Job, build)
			if err != nil {
				return err
			}
		} else {
			paths = CIPaths(args.Upload, args.Job, build)
		}
		// TODO: Replace env var below with a flag eventually.
		os.Setenv(GCSArtifactsEnv, paths.Artifacts)
	}

	// TODO(bentheelder): mimic the rest of bootstrap.py here ¯\_(ツ)_/¯
	// Printing these so that it compiles ¯\_(ツ)_/¯
	fmt.Println(originalCWD)
	fmt.Println(paths)
	return nil
}

func main() {
	args, err := ParseArgs(os.Args[1:])
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	err = Bootstrap(args)
	if err != nil {
		log.Fatalf("%v", err)
	}
}
