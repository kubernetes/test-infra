/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"sigs.k8s.io/yaml"
)

const (
	gcsSourceDir = "/source"
	gcsLogsDir   = "/logs"
)

func runCmd(command string, args ...string) error {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	return cmd.Run()
}

func getVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	t := time.Now().Format("20060102")
	return fmt.Sprintf("v%s-%s", t, strings.TrimSpace(string(output))), nil
}

func (o *options) validateConfigDir() error {
	configDir := o.configDir
	dirInfo, err := os.Stat(o.configDir)
	if os.IsNotExist(err) {
		log.Fatalf("Config directory (%s) does not exist", configDir)
	}

	if !dirInfo.IsDir() {
		log.Fatalf("Config directory (%s) is not actually a directory", configDir)
	}

	_, err = os.Stat(o.cloudbuildFile)
	if os.IsNotExist(err) {
		log.Fatalf("%s does not exist", o.cloudbuildFile)
	}

	return nil
}

func (o *options) uploadBuildDir(targetBucket string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)

	log.Printf("Creating source tarball at %s...\n", name)
	if err := runCmd("tar", "--exclude", ".git", "-czf", name, "."); err != nil {
		return "", fmt.Errorf("failed to tar files: %s", err)
	}

	u := uuid.New()
	uploaded := fmt.Sprintf("%s/%s.tgz", targetBucket, u.String())
	log.Printf("Uploading %s to %s...\n", name, uploaded)
	if err := runCmd("gsutil", "cp", name, uploaded); err != nil {
		return "", fmt.Errorf("failed to upload files: %s", err)
	}

	return uploaded, nil
}

func getExtraSubs(o options) map[string]string {
	envs := strings.Split(o.envPassthrough, ",")
	subs := map[string]string{}
	for _, e := range envs {
		e = strings.TrimSpace(e)
		if e != "" {
			subs[e] = os.Getenv(e)
		}
	}
	return subs
}

func runSingleJob(o options, jobName, uploaded, version string, subs map[string]string) error {
	s := make([]string, 0, len(subs)+1)
	for k, v := range subs {
		s = append(s, fmt.Sprintf("_%s=%s", k, v))
	}

	s = append(s, "_GIT_TAG="+version)
	args := []string{
		"builds", "submit",
		"--verbosity", "info",
		"--config", o.cloudbuildFile,
		"--substitutions", strings.Join(s, ","),
	}

	if o.project != "" {
		args = append(args, "--project", o.project)
	}

	if o.scratchBucket != "" {
		args = append(args, "--gcs-log-dir", o.scratchBucket+gcsLogsDir)
		args = append(args, "--gcs-source-staging-dir", o.scratchBucket+gcsSourceDir)
	}

	if uploaded != "" {
		args = append(args, uploaded)
	} else {
		if o.noSource {
			args = append(args, "--no-source")
		} else {
			args = append(args, ".")
		}
	}

	cmd := exec.Command("gcloud", args...)

	if o.logDir != "" {
		p := path.Join(o.logDir, strings.Replace(jobName, "/", "-", -1)+".log")
		f, err := os.Create(p)

		if err != nil {
			return fmt.Errorf("couldn't create %s: %v", p, err)
		}

		defer f.Sync()
		defer f.Close()

		cmd.Stdout = f
		cmd.Stderr = f
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("error running %s: %v", cmd.Args, err)
	}

	return nil
}

type variants map[string]map[string]string

func getVariants(o options) (variants, error) {
	content, err := ioutil.ReadFile(path.Join(o.configDir, "variants.yaml"))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to load variants.yaml: %v", err)
		}
		if o.variant != "" {
			return nil, fmt.Errorf("no variants.yaml found, but a build variant (%q) was specified", o.variant)
		}
		return nil, nil
	}
	v := struct {
		Variants variants `json:"variants"`
	}{}
	if err := yaml.UnmarshalStrict(content, &v); err != nil {
		return nil, fmt.Errorf("failed to read variants.yaml: %v", err)
	}
	if o.variant != "" {
		va, ok := v.Variants[o.variant]
		if !ok {
			return nil, fmt.Errorf("requested variant %q, which is not present in variants.yaml", o.variant)
		}
		return variants{o.variant: va}, nil
	}
	return v.Variants, nil
}

func runBuildJobs(o options) []error {
	var uploaded string
	if o.scratchBucket != "" {
		if !o.noSource {
			var err error
			uploaded, err = o.uploadBuildDir(o.scratchBucket + gcsSourceDir)
			if err != nil {
				return []error{fmt.Errorf("failed to upload source: %v", err)}
			}
		}
	} else {
		log.Println("Skipping advance upload and relying on gcloud...")
	}

	log.Println("Running build jobs...")
	tag, err := getVersion()
	if err != nil {
		return []error{fmt.Errorf("failed to get current tag: %v", err)}
	}

	if !o.allowDirty && strings.HasSuffix(tag, "-dirty") {
		return []error{fmt.Errorf("the working copy is dirty")}
	}

	vs, err := getVariants(o)
	if err != nil {
		return []error{err}
	}
	if len(vs) == 0 {
		log.Println("No variants.yaml, starting single build job...")
		if err := runSingleJob(o, "build", uploaded, tag, getExtraSubs(o)); err != nil {
			return []error{err}
		}
		return nil
	}

	log.Printf("Found variants.yaml, starting %d build jobs...\n", len(vs))

	w := sync.WaitGroup{}
	w.Add(len(vs))
	var errors []error
	extraSubs := getExtraSubs(o)
	for k, v := range vs {
		go func(job string, vc map[string]string) {
			defer w.Done()
			log.Printf("Starting job %q...\n", job)
			if err := runSingleJob(o, job, uploaded, tag, mergeMaps(extraSubs, vc)); err != nil {
				errors = append(errors, fmt.Errorf("job %q failed: %v", job, err))
				log.Printf("Job %q failed: %v\n", job, err)
			} else {
				log.Printf("Job %q completed.\n", job)
			}
		}(k, v)
	}
	w.Wait()
	return errors
}

type options struct {
	buildDir       string
	configDir      string
	cloudbuildFile string
	logDir         string
	scratchBucket  string
	project        string
	allowDirty     bool
	noSource       bool
	variant        string
	envPassthrough string
}

func mergeMaps(maps ...map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.buildDir, "build-dir", "", "If provided, this directory will be uploaded as the source for the Google Cloud Build run.")
	flag.StringVar(&o.cloudbuildFile, "gcb-config", "cloudbuild.yaml", "If provided, this will be used as the name of the Google Cloud Build config file.")
	flag.StringVar(&o.logDir, "log-dir", "", "If provided, build logs will be sent to files in this directory instead of to stdout/stderr.")
	flag.StringVar(&o.scratchBucket, "scratch-bucket", "", "The complete GCS path for Cloud Build to store scratch files (sources, logs).")
	flag.StringVar(&o.project, "project", "", "If specified, use a non-default GCP project.")
	flag.BoolVar(&o.allowDirty, "allow-dirty", false, "If true, allow pushing dirty builds.")
	flag.BoolVar(&o.noSource, "no-source", false, "If true, no source will be uploaded with this build.")
	flag.StringVar(&o.variant, "variant", "", "If specified, build only the given variant. An error if no variants are defined.")
	flag.StringVar(&o.envPassthrough, "env-passthrough", "", "Comma-separated list of specified environment variables to be passed to GCB as substitutions with an _ prefix. If the variable doesn't exist, the substitution will exist but be empty.")

	flag.Parse()

	if flag.NArg() < 1 {
		_, _ = fmt.Fprintln(os.Stderr, "expected a config directory to be provided")
		os.Exit(1)
	}

	o.configDir = strings.TrimSuffix(flag.Arg(0), "/")

	return o
}

func main() {
	o := parseFlags()

	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			log.Fatalf("Failed to chdir to bazel workspace (%s): %v", bazelWorkspace, err)
		}
	}

	if o.buildDir == "" {
		o.buildDir = o.configDir
	}

	log.Printf("Build directory: %s\n", o.buildDir)

	// Canonicalize the config directory to be an absolute path.
	// As we're about to cd into the build directory, we need a consistent way to reference the config files
	// when the config directory is not the same as the build directory.
	absConfigDir, absErr := filepath.Abs(o.configDir)
	if absErr != nil {
		log.Fatalf("Could not resolve absolute path for config directory: %v", absErr)
	}

	o.configDir = absConfigDir
	o.cloudbuildFile = path.Join(o.configDir, o.cloudbuildFile)

	configDirErr := o.validateConfigDir()
	if configDirErr != nil {
		log.Fatalf("Could not validate config directory: %v", configDirErr)
	}

	log.Printf("Config directory: %s\n", o.configDir)

	log.Printf("cd-ing to build directory: %s\n", o.buildDir)
	if err := os.Chdir(o.buildDir); err != nil {
		log.Fatalf("Failed to chdir to build directory (%s): %v", o.buildDir, err)
	}

	errors := runBuildJobs(o)
	if len(errors) != 0 {
		log.Fatalf("Failed to run some build jobs: %v", errors)
	}
	log.Println("Finished.")
}
