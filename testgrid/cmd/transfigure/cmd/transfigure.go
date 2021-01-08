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
		err := run()
		if err != nil {
			log.Fatal(err)
		}
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

func run() error {
	if !dryRun && githubToken == "" {
		return errors.New("Run was not a dry run, and flag 'github_token' was not set.")
	}

	err := createTempWorkingDir()
	if err != nil {
		return err
	}

	defer removeTempWorkingDir()

	err = cloneK8sTestInfraRepo()
	if err != nil {
		return err
	}

	if githubToken != "" {
		err = readGitHubToken()
		if err != nil {
			return err
		}
	}

	if gitUser == "" || gitEmail == "" {
		err = populateGitUserAndEmail()
		if err != nil {
			return err
		}
	} else {
		log.Print("Using user from Flag: " + gitUser)
		log.Print("Using email from Flag: " + gitEmail)
	}

	err = createAndCheckoutGitBranch()
	if err != nil {
		return err
	}

	createRepoSubdirForTestgridYAML()

	err = generateTestgridYAML()
	if err != nil {
		return err
	}

	err = gitAddAll()
	if err != nil {
		return err
	}

	if !gitDiffExists() {
		log.Print("Transfigure did not change anything. Aborting no-op bump.")
		return nil
	}

	err = runBazelTests()
	if err != nil {
		return err
	}

	if dryRun {
		log.Print("Dry-Run; skipping PR")
		return nil
	}

	err = gitCommitAndPush()
	if err != nil {
		return err
	}

	err = createPR()
	return err
}

func init() {
	// Required Flags
	rootCmd.PersistentFlags().StringVar(&prowConfig, "prow_config", "", "Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&prowJobConfig, "prow_job_config", "", "Job Config path for your non-k8s Prow instance.")
	rootCmd.PersistentFlags().StringVar(&testgridYaml, "testgrid_yaml", "", "TestGrid configuration directory or default file.")
	rootCmd.PersistentFlags().StringVar(&repoSubdir, "repo_subdir", "", "The subdirectory in /config/testgrids/... to push to. Usually <github_org> or <github_org>/<github_repository>.")
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

func createTempWorkingDir() error {
	tempDir, err := ioutil.TempDir(".", "transfigure_")
	if err != nil {
		return wrapErrorOrNil("Error creating temp dir", err)
	}
	workingDir, err = filepath.Abs(tempDir)
	if err != nil {
		return wrapErrorOrNil("Error getting abs path for working dir", err)
	}
	log.Printf("Created temp directory %v/", tempDir)
	return nil
}

func removeTempWorkingDir() {
	os.RemoveAll(workingDir)
	log.Printf("Removed temp directory %v/", workingDir)
}

func cloneK8sTestInfraRepo() error {
	log.Printf("Cloning kubernetes/test-infra into temp directory")
	cmd := exec.Command("git", "clone", "https://github.com/kubernetes/test-infra.git")
	_, err := runCmd(cmd)
	return wrapErrorOrNil("Error on git clone", err)
}

func createAndCheckoutGitBranch() error {
	log.Print("Checking out branch " + Branch)
	_, err := runCmd(exec.Command("git", "checkout", "-B", Branch))
	return wrapErrorOrNil("Error on git checkout -B "+Branch, err)
}

func createRepoSubdirForTestgridYAML() {
	testgridDir = filepath.Join(workingDir, "test-infra/config/testgrids", repoSubdir)
	if _, err := os.Stat(testgridDir); os.IsNotExist(err) {
		log.Printf("Directory %v does not exist; creating it.", testgridDir)
		os.MkdirAll(testgridDir, 0755)
	}
}

func generateTestgridYAML() error {
	yamlPath, err := filepath.Abs(testgridYaml)
	if err != nil {
		return wrapErrorOrNil("Invalid testgrid yaml path "+testgridYaml, err)
	}

	cmd := exec.Command(
		"/Users/joshuabone/go/src/k8s.io/test-infra/testgrid/cmd/transfigure/configurator",
		fmt.Sprintf("--prow-config=%s", prowConfig),
		fmt.Sprintf("--prow-job-config=%s", prowJobConfig),
		"--output-yaml",
		fmt.Sprintf("--yaml=%s", yamlPath),
		"--oneshot",
		fmt.Sprintf("--output=%s/gen-config.yaml", testgridDir),
	)
	_, err = runCmd(cmd)
	return wrapErrorOrNil("Error running configurator", err)
}

func gitAddAll() error {
	_, err := runCmd(exec.Command("git", "add", "--all"))
	return wrapErrorOrNil("Error on git add", err)
}

func gitDiffExists() bool {
	cmd := exec.Command("git", "diff", "--cached", "--quiet", "--exit-code")
	cmd.Dir = workingDir
	err := cmd.Run()
	return err != nil
}

func runBazelTests() error {
	log.Print("Running kubernetes/test-infra tests...")
	cmd := exec.Command("bazel", "test", "test-infra/config/tests/...", "test-infra/hack:verify-spelling")
	out, err := runCmd(cmd)
	if err != nil {
		return wrapErrorOrNil("Error running bazel test: "+out, err)
	} else {
		log.Print("Tests successful!")
		return nil
	}
}

func ensureGitUserAndEmail() error {
	log.Print("Checking Git Config user and email")
	runCmd(exec.Command("git", "config", "user.name", gitUser))
	runCmd(exec.Command("git", "config", "user.email", gitEmail))

	output, err := runCmd(exec.Command("git", "config", "user.name"))
	if err != nil {
		return wrapErrorOrNil("Error on git config user.name", err)
	}
	if diff := cmp.Diff(output, gitUser); diff != "" {
		return errors.New("Unexpected Git User: (-got +want)\n" + diff)
	}

	output, err = runCmd(exec.Command("git", "config", "user.email"))
	if err != nil {
		return wrapErrorOrNil("Error on git config user.email", err)
	}
	if diff := cmp.Diff(output, gitEmail); diff != "" {
		return errors.New("Unexpected Git Email: (-got +want)\n" + diff)
	}
	return nil
}

func gitCommitAndPush() error {
	prTitle = "Update TestGrid for " + repoSubdir
	_, err := runCmd(exec.Command("git", "commit", "-m", prTitle))
	if err != nil {
		return wrapErrorOrNil("Error on git commit", err)
	}

	log.Print(fmt.Sprintf("Pushing commit to %s/%s:%s...", gitUser, remoteForkRepo, Branch))
	pushTarget := fmt.Sprintf("https://%s:%s@github.com/%s/%s", gitUser, tokenContents, gitUser, remoteForkRepo)
	_, err = runCmd(exec.Command("git", "push", "-f", pushTarget, "HEAD:"+Branch))
	return wrapErrorOrNil("Error on git push", err)
}

func createPR() error {
	log.Printf("Creating PR to merge %s:%s into k8s/test-infra:master...", gitUser, Branch)
	_, err := runCmd(exec.Command("/pr-creator",
		"--github-token-path="+githubToken,
		"--org=\"kubernetes\"",
		"--repo=\"test-infra\"",
		"--branch=master",
		"--title=\""+prTitle+"\"",
		"--match-title=\""+prTitle+"\"",
		"--body=\"Generated by transfigure cmd\"",
		"--source=\""+gitUser+":"+Branch+"\"",
		"--confirm"))
	if err == nil {
		log.Print("PR created successfully!")
	}
	return wrapErrorOrNil("Error creating PR", err)
}

func runCmd(cmd *exec.Cmd) (string, error) {
	cmd.Dir = workingDir
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
	req.Header.Add("Authorization", "token "+tokenContents)
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
	return nil
}

func readGitHubToken() error {
	token, err := ioutil.ReadFile(githubToken)
	tokenContents = strings.TrimSpace(string(token))
	return wrapErrorOrNil("Error reading github token "+githubToken, err)
}

func wrapErrorOrNil(msg string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%v (Caused by: %v)", msg, err)
}
