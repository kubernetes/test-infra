package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"sigs.k8s.io/yaml"
)

func getVersion() (string, error) {
	cmd := exec.Command("git", "describe", "--tags", "--always", "--dirty")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func cdToRootDir() error {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := cmd.Output()
	if err != nil {
		return err
	}
	return os.Chdir(strings.TrimSpace(string(output)))
}

func uploadWorkingDir(targetBucket string) (string, error) {
	f, err := ioutil.TempFile("", "")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %v", err)
	}
	name := f.Name()
	_ = f.Close()
	defer os.Remove(name)
	cmd := exec.Command("tar", "--exclude", ".git", "-czf", name, ".")
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to tar files: %v", err)
	}
	uploaded := targetBucket + path.Base(name) + ".tgz"
	cmd = exec.Command("gsutil", "cp", name, uploaded)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to upload files: %v", err)
	}
	return uploaded, nil
}

type variants struct {
	Variants []map[string]string `json:"variants"`
}

func runSingleJob(image, uploaded, version string, subs map[string]string) error {
	s := make([]string, 0, len(subs)+1)
	for k, v := range subs {
		s = append(s, fmt.Sprintf("_%s=%s", k, v))
	}
	s = append(s, "_GIT_TAG="+version)
	cmd := exec.Command("gcloud", "builds", "submit", "--config", image+"/cloudbuild.yaml", "--substitutions", strings.Join(s, ","), uploaded)
	return cmd.Run()
}

func runBuildJobs(image, uploaded string) []error {
	tag, err := getVersion()
	if err != nil {
		log.Fatalf("Failed to get current tag: %v\n", err)
	}

	if strings.HasSuffix(tag, "-dirty") {
		log.Fatalf("The working copy is dirty!")
	}

	content, err := ioutil.ReadFile(image + "/variants.yaml")
	if err != nil {
		if !os.IsNotExist(err) {
			return []error{err}
		}
		return []error{runSingleJob(image, uploaded, tag, nil)}
	}
	var v variants
	if err := yaml.UnmarshalStrict(content, &v); err != nil {
		return []error{fmt.Errorf("failed to read variants.yaml: %v", err)}
	}

	w := sync.WaitGroup{}
	w.Add(len(v.Variants))
	var errors []error
	for _, variant := range v.Variants {
		go func(va map[string]string) {
			defer w.Done()
			if err := runSingleJob(image, uploaded, tag, va); err != nil {
				errors = append(errors, err)
			}
		}(variant)
	}
	w.Wait()
	return errors
}

func main() {
	if err := cdToRootDir(); err != nil {
		log.Fatalf("Failed to cd to root: %v\n", err)
	}

	dir := os.Args[1]
	uploadedFile, err := uploadWorkingDir("gs://ktbry-prow-dev_cloudbuild/source/")
	if err != nil {
		log.Fatalf("Failed to upload source: %v", err)
	}

	errors := runBuildJobs(dir, uploadedFile)
	if len(errors) != 0 {
		log.Fatalf("Failed to push some images: %v", errors)
	}
}
