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
	"errors"
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
const K8sTestInfraGitPath = "https://github.com/kubernetes/test-infra.git"
const TestInfraConfigTests = "test-infra/config/tests/..."
const TestInfraVerifySpelling = "test-infra/hack:verify-spelling"
const PrTitlePrefix = "Update TestGrid for "

type options struct {
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
}

var o = options{}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "transfigure",
	Short: "Generates a YAML Testgrid config from a Prow config and pushes it to testgrid.k8s.io.",
	Long: `Transfigure is an image that generates a YAML Testgrid configuration
		   from a Prow configuration and pushes it to be used on testgrid.k8s.io. It 
	       is used specifically for Prow instances other than the k8s instance of Prow.`,
	Run: func(cmd *cobra.Command, args []string) {
		err := run()
		if err != nil {
			log.Fatal(err)
		}
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func run() error {
	if !o.dryRun && o.githubToken == "" {
		return errors.New("Run was not a dry run, and flag 'github_token' was not set.")
	}

	if err := createTempWorkingDir(); err != nil {
		return err
	}

	defer removeTempWorkingDir()

	if err := cloneK8sTestInfraRepo(); err != nil {
		return err
	}

	if o.githubToken != "" {
		if err := readGitHubToken(); err != nil {
			return err
		}
	}

	if o.gitUser == "" || o.gitEmail == "" {
		if err := populateGitUserAndEmail(); err != nil {
			return err
		}
	} else {
		log.Print("Using user from Flag: " + o.gitUser)
		log.Print("Using email from Flag: " + o.gitEmail)
	}

	if err := createAndCheckoutGitBranch(); err != nil {
		return err
	}

	createRepoSubdirForTestgridYAML()

	if err := generateTestgridYAML(); err != nil {
		return err
	}

	if err := gitAddAll(); err != nil {
		return err
	}

	if !gitDiffExists() {
		log.Print("Transfigure did not change anything. Aborting no-op bump.")
		return nil
	}

	if err := runBazelTests(); err != nil {
		return err
	}

	if o.dryRun {
		log.Print("Dry-Run; skipping PR")
		return nil
	}

	if err := gitCommitAndPush(); err != nil {
		return err
	}

	if err := createPR(); err != nil {
		return err
	}
	return nil
}

func init() {
	// Required Flags
	rootCmd.PersistentFlags().StringVar(&o.prowConfig, "prow_config", "", "Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&o.prowJobConfig, "prow_job_config", "", "Job Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&o.testgridYaml, "testgrid_yaml", "", "TestGrid configuration directory or default file.")
	rootCmd.PersistentFlags().StringVar(&o.repoSubdir, "repo_subdir", "", "The subdirectory in /config/testgrids/... to push to. Usually <github_org> or <github_org>/<github_repository>.")
	for _, flag := range []string{"prow_config", "prow_job_config", "testgrid_yaml", "repo_subdir"} {
		rootCmd.MarkPersistentFlagRequired(flag)
	}

	// Optional Flags
	rootCmd.PersistentFlags().StringVar(&o.githubToken, "github_token", "", "A GitHub personal access token. A value of 'Test' will trigger a dry run.")
	rootCmd.PersistentFlags().StringVar(&o.remoteForkRepo, "remote_fork_repo", "", "The name of the user's fork of kubernetes/test-infra, if it is not named test-infra.")
	rootCmd.PersistentFlags().StringVar(&o.gitUser, "git_user", "", "The GitHub username to use. Pulled from github_token if not specified.")
	rootCmd.PersistentFlags().StringVar(&o.gitEmail, "git_email", "", "The GitHub email to use. Pulled from github_token if not specified.")
	rootCmd.PersistentFlags().BoolVar(&o.dryRun, "dry_run", false, "If true, no PR will be created.")
}

func createTempWorkingDir() error {
	tempDir, err := ioutil.TempDir(".", "transfigure_")
	if err != nil {
		return wrapErrorOrNil("Error creating temp dir", err)
	}
	o.workingDir, err = filepath.Abs(tempDir)
	if err != nil {
		return wrapErrorOrNil("Error getting abs path for working dir", err)
	}
	log.Printf("Created temp directory %v/", tempDir)
	return nil
}

func removeTempWorkingDir() {
	if err := os.RemoveAll(o.workingDir); err != nil {
		log.Printf("Error: Could not remove temp directory %v/", o.workingDir)
	}
	log.Printf("Removed temp directory %v/", o.workingDir)
}

func cloneK8sTestInfraRepo() error {
	log.Print("Cloning kubernetes/test-infra into temp directory")
	cmd := exec.Command("git", "clone", K8sTestInfraGitPath)
	_, err := runCmd(cmd)
	return wrapErrorOrNil("Error on git clone", err)
}

func createAndCheckoutGitBranch() error {
	log.Print("Checking out branch " + Branch)
	_, err := runCmd(exec.Command("git", "checkout", "-B", Branch))
	return wrapErrorOrNil("Error on git checkout -B "+Branch, err)
}

func createRepoSubdirForTestgridYAML() {
	o.testgridDir = filepath.Join(o.workingDir, "test-infra/config/testgrids", o.repoSubdir)
	if _, err := os.Stat(o.testgridDir); os.IsNotExist(err) {
		log.Printf("Directory %v does not exist; creating it.", o.testgridDir)
		os.MkdirAll(o.testgridDir, 0755)
	}
}

func generateTestgridYAML() error {
	yamlPath, err := filepath.Abs(o.testgridYaml)
	if err != nil {
		return wrapErrorOrNil("Invalid testgrid yaml path "+o.testgridYaml, err)
	}

	cmd := exec.Command(
		"/Users/joshuabone/go/src/k8s.io/test-infra/testgrid/cmd/transfigure/configurator",
		fmt.Sprintf("--prow-config=%s", o.prowConfig),
		fmt.Sprintf("--prow-job-config=%s", o.prowJobConfig),
		"--output-yaml",
		fmt.Sprintf("--yaml=%s", yamlPath),
		"--oneshot",
		fmt.Sprintf("--output=%s/gen-config.yaml", o.testgridDir),
	)
	output, err := runCmd(cmd)
	log.Print("Configurator output: " + output)
	return wrapErrorOrNil("Error running configurator", err)
}

func gitAddAll() error {
	_, err := runCmd(exec.Command("git", "add", "--all"))
	return wrapErrorOrNil("Error on git add", err)
}

func gitDiffExists() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--exit-code")
	cmd.Dir = o.workingDir
	err := cmd.Run()
	return err != nil
}

func runBazelTests() error {
	log.Print("Running kubernetes/test-infra tests...")
	cmd := exec.Command("bazel", "test", TestInfraConfigTests, TestInfraVerifySpelling)
	out, err := runCmd(cmd)
	log.Print("Bazel Test output: " + out)
	return err
}

func ensureGitUserAndEmail() error {
	log.Print("Checking Git Config user and email")
	if _, err := runCmd(exec.Command("git", "config", "user.name", o.gitUser)); err != nil {
		return wrapErrorOrNil("Error setting git user name", err)
	}
	if _, err := runCmd(exec.Command("git", "config", "user.email", o.gitEmail)); err != nil {
		return wrapErrorOrNil("Error setting git user email", err)
	}

	output, err := runCmd(exec.Command("git", "config", "user.name"))
	if err != nil {
		return wrapErrorOrNil("Error on git config user.name", err)
	}
	if diff := cmp.Diff(output, o.gitUser); diff != "" {
		return errors.New("Unexpected Git User: (-got +want)\n" + diff)
	}

	output, err = runCmd(exec.Command("git", "config", "user.email"))
	if err != nil {
		return wrapErrorOrNil("Error on git config user.email", err)
	}
	if diff := cmp.Diff(output, o.gitEmail); diff != "" {
		return errors.New("Unexpected Git Email: (-got +want)\n" + diff)
	}
	return nil
}

func gitCommitAndPush() error {
	o.prTitle = PrTitlePrefix + o.repoSubdir
	_, err := runCmd(exec.Command("git", "commit", "-m", o.prTitle))
	if err != nil {
		return wrapErrorOrNil("Error on git commit", err)
	}

	log.Print(fmt.Sprintf("Pushing commit to %s/%s:%s...", o.gitUser, o.remoteForkRepo, Branch))
	pushTarget := fmt.Sprintf("https://%s:%s@github.com/%s/%s", o.gitUser, o.tokenContents, o.gitUser, o.remoteForkRepo)
	_, err = runCmd(exec.Command("git", "push", "-f", pushTarget, "HEAD:"+Branch))
	return wrapErrorOrNil("Error on git push", err)
}

func createPR() error {
	log.Printf("Creating PR to merge %s:%s into k8s/test-infra:master...", o.gitUser, Branch)
	_, err := runCmd(exec.Command("/pr-creator",
		"--github-token-path="+o.githubToken,
		"--org=\"kubernetes\"",
		"--repo=\"test-infra\"",
		"--branch=master",
		"--title=\""+o.prTitle+"\"",
		"--match-title=\""+o.prTitle+"\"",
		"--body=\"Generated by transfigure cmd\"",
		"--source=\""+o.gitUser+":"+Branch+"\"",
		"--confirm"))
	if err == nil {
		log.Print("PR created successfully!")
	}
	return wrapErrorOrNil("Error creating PR", err)
}

func runCmd(cmd *exec.Cmd) (string, error) {
	cmd.Dir = o.workingDir
	log.Printf("Running command: \n%v", cmd)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err // Remove trailing newline
}

func populateGitUserAndEmail() error {
	// Create a new HTTP request.
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return err
	}
	// Append the token we just read from file, and send the request.
	req.Header.Add("Authorization", "token "+o.tokenContents)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return wrapErrorOrNil("Error sending request", err)
	}
	// Read in the desired fields from the response body.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return wrapErrorOrNil("Error reading from response", err)
	}
	githubData := struct {
		Login string `json:"login"`
		Email string `json:"email"`
	}{}
	json.Unmarshal(body, &githubData)

	// Use the new data unless the old data was specified by flag.
	if o.gitUser != "" {
		log.Print("Using user from Flag: " + o.gitUser)
	} else {
		o.gitUser = githubData.Login
		log.Print("Using user from GitHub: " + o.gitUser)
	}

	if o.gitEmail != "" {
		log.Print("Using email from Flag: " + o.gitEmail)
	} else {
		o.gitEmail = githubData.Email
		log.Print("Using email from GitHub: " + o.gitEmail)
	}
	return nil
}

func readGitHubToken() error {
	token, err := ioutil.ReadFile(o.githubToken)
	o.tokenContents = strings.TrimSpace(string(token))
	return wrapErrorOrNil("Error reading github token "+o.githubToken, err)
}

func wrapErrorOrNil(msg string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%v (Caused by: %v)", msg, err)
}
