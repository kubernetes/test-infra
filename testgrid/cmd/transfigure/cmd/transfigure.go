/*
Copyright © 2020 The Kubernetes Authors.

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
package transfigure

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
)

const Branch = "transfigure-branch"

var (
	dryRun         bool
	gitEmail       string
	githubToken    string
	gitUser        string
	prowConfig     string
	prowJobConfig  string
	prTitle        string
	remoteForkRepo string
	repoSubdir     string
	tokenContents  string
	testgridDir    string
	testgridYaml   string
	workingDir     string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "transfigure",
	Short: "Generates a YAML Testgrid config from a Prow config and pushes it to testgrid.k8s.io.",
	Long: `Transfigure is an image that generates a YAML Testgrid configuration
		   from a Prow configuration and pushes it to be used on testgrid.k8s.io. It 
	       is used specifically for Prow instances other than the k8s instance of Prow.`,
	Run: func(cmd *cobra.Command, args []string) {
		os.Exit(run())
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() int {
	if !dryRun && githubToken == "" {
		log.Panic("Run was not a dry run, and flag 'github_token' was not set.")
	}

	createTempWorkingDir()
	defer removeTempWorkingDir()

	cloneK8sTestInfraRepo()

	if gitUser == "" || gitEmail == "" {
		populateGitUserAndEmail()
	} else {
		log.Print("Using user from Flag: " + gitUser)
		log.Print("Using email from Flag: " + gitEmail)
	}
	createAndCheckoutGitBranch()
	createRepoSubdirForTestgridYAML()
	generateTestgridYAML()
	gitAddAll()
	if !gitDiffExists() {
		log.Print("Transfigure did not change anything. Aborting no-op bump.")
		return 0
	}
	runBazelTests()

	if dryRun {
		log.Print("Dry-Run; skipping PR")
	} else {
		gitCommitAndPush()
		createPR()
	}
	return 0
}

func init() {
	// Required Flags
	rootCmd.PersistentFlags().StringVar(&prowConfig, "prow_config", "", "Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&prowJobConfig, "prow_job_config", "", "Job Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&testgridYaml, "testgrid_yaml", "", "TestGrid configuration directory or default file.")
	rootCmd.PersistentFlags().StringVar(&prowJobConfig, "repo_subdir", "", "The subdirectory in /config/testgrids/... to push to. Usually <github_org> or <github_org>/<github_repository>.")
	for _, flag := range []string{"prow_config", "prow_job_config", "testgrid_yaml", "repo_subdir"} {
		rootCmd.MarkPersistentFlagRequired(flag)
	}

	// Optional Flags
	rootCmd.PersistentFlags().StringVar(&githubToken, "github_token", "", "A GitHub personal access token. A value of 'Test' will trigger a dry run.")
	rootCmd.PersistentFlags().StringVar(&remoteForkRepo, "remote_fork_repo", "", "The name of the user's fork of kubernetes/test-infra, if it is not named test-infra.")
	rootCmd.PersistentFlags().StringVar(&gitUser, "git_user", "", "The GitHub username to use. Pulled from github_token if not specified.")
	rootCmd.PersistentFlags().StringVar(&gitEmail, "git_email", "", "The GitHub email to use. Pulled from github_token if not specified.")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry_run", false, "If true, no PR will be created.")
}

func createTempWorkingDir() {
	tempDir, err := ioutil.TempDir(".", "transfigure_")
	if err != nil {
		log.Panic(err)
	}
	workingDir, err = filepath.Abs(tempDir)
	if err != nil {
		log.Panic(err)
	}
	log.Printf("Created temp directory %v/", tempDir)
}

func removeTempWorkingDir() {
	os.RemoveAll(workingDir)
	log.Printf("Removed temp directory %v/", workingDir)
}

func cloneK8sTestInfraRepo() {
	log.Printf("Cloning kubernetes/test-infra into temp directory")
	cmd := exec.Command("git", "clone", "https://github.com/kubernetes/test-infra.git")
	runCmd(cmd)
}

func createAndCheckoutGitBranch() {
	log.Print("Checking out branch " + Branch)
	runCmd(exec.Command("git", "checkout", "-B", Branch))
}

func createRepoSubdirForTestgridYAML() {
	testgridDir = filepath.Join(workingDir, "test-infra/config/testgrids", repoSubdir)
	if _, err := os.Stat(testgridDir); os.IsNotExist(err) {
		log.Printf("Directory %v does not exist; creating it.", testgridDir)
		os.MkdirAll(testgridDir, 0755)
	}
}

func generateTestgridYAML() {
	yamlPath, err := filepath.Abs(testgridYaml)
	if err != nil {
		log.Panic("Invalid testgrid yaml path: " + testgridYaml)
	}

	runCmd(exec.Command(
		"/configurator",
		fmt.Sprintf("--prow-config \"%s\"", prowConfig),
		fmt.Sprintf("--prow-job-config \"%s\"", prowJobConfig),
		"--output-yaml",
		fmt.Sprintf("--yaml \"%s\"", yamlPath),
		"--oneshot",
		fmt.Sprintf("--output \"%s/gen-config.yaml\"", testgridDir),
	))
}

func gitAddAll() {
	runCmd(exec.Command("git", "add", "--all"))
}

func gitDiffExists() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--exit-code")
	cmd.Dir = workingDir
	err := cmd.Run()
	return err != nil
}

func runBazelTests() {
	log.Print("Running kubernetes/test-infra tests...")
	cmd := exec.Command("bazel", "test", "//config/tests...", "//hack:verify-spelling")
	runCmd(cmd)
	log.Print("Tests successful!")
}

func ensureGitUserAndEmail() {
	log.Print("Checking Git Config user and email")
	runCmd(exec.Command("git", "config", "user.name", gitUser))
	runCmd(exec.Command("git", "config", "user.email", gitEmail))
	output := runCmd(exec.Command("git", "config", "user.name"))
	if diff := cmp.Diff(output, gitUser); diff != "" {
		log.Panic("Unexpected Git User: (-got +want)\n" + diff)
	}
	output = runCmd(exec.Command("git", "config", "user.email"))
	if diff := cmp.Diff(output, gitEmail); diff != "" {
		log.Panic("Unexpected Git Email: (-got +want)\n" + diff)
	}
}

func gitCommitAndPush() {
	prTitle = "Update TestGrid for " + repoSubdir
	runCmd(exec.Command("git", "commit", "-m", prTitle))

	log.Print(fmt.Sprintf("Pushing commit to %s/%s:%s...", gitUser, remoteForkRepo, Branch))
	pushTarget := fmt.Sprintf("https://%s:%s@github.com/%s/%s", gitUser, githubTokenContents(), gitUser, remoteForkRepo)
	runCmd(exec.Command("git", "push", "-f", pushTarget, "HEAD:"+Branch))
}

func createPR() {
	log.Printf("Creating PR to merge %s:%s into k8s/test-infra:master...", gitUser, Branch)
	runCmd(exec.Command("/pr-creator",
		"--github-token-path="+githubToken,
		"--org=\"kubernetes\"",
		"--repo=\"test-infra\"",
		"--branch=master",
		"--title=\""+prTitle+"\"",
		"--match-title=\""+prTitle+"\"",
		"--body=\"Generated by transfigure cmd\"",
		"--source=\""+gitUser+":"+Branch+"\"",
		"--confirm"))
	log.Print("PR created successfully!")
}

func runCmd(cmd *exec.Cmd) string {
	cmd.Dir = workingDir
	out, err := cmd.Output()
	if err != nil {
		log.Panic(err)
	}
	return strings.TrimSpace(string(out)) // Remove trailing newline
}

func populateGitUserAndEmail() {
	// Create a new HTTP request.
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		log.Panic(err)
	}
	// Append the token we just read from file, and send the request.
	req.Header.Add("Authorization", "token "+githubTokenContents())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Panic(err)
	}
	// Read in the desired fields from the response body.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Panic(err)
	}
	githubData := struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}{}
	json.Unmarshal(body, &githubData)

	// Use the new data unless the old data was specified by flag.
	if gitUser != "" {
		log.Print("Using user from Flag: " + gitUser)
	} else {
		gitUser = githubData.Login
		log.Print("Using user from GitHub: " + gitUser)
	}

	if gitEmail != "" {
		log.Print("Using email from Flag: " + gitEmail)
	} else {
		gitEmail = githubData.Email
		log.Print("Using email from GitHub: " + gitEmail)
	}
}

func githubTokenContents() string {
	if tokenContents == "" {
		token, err := ioutil.ReadFile(githubToken)
		if err != nil {
			log.Panic(err)
		}
		tokenContents = strings.TrimSpace(string(token))
	}
	return tokenContents
}
