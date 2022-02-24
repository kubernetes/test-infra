/*
Copyright 2022 The Kubernetes Authors.

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
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"context"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"
)

const (
	defaultArch = "linux/amd64"
	allArch     = "all"

	gatherStaicScriptName = "gather-static.sh"

	// Relative to root of the repo
	defaultProwImageListFile = "prow/.prow-images.yaml"

	defaultWorkersCount = 10
	defaultRetry        = 3

	// noOpKoDocerRepo is used when images are not pushed
	noOpKoDocerRepo = "ko.local"
)

var (
	rootDir     string
	otherArches = []string{
		"linux/arm64",
		"linux/s390x",
		"linux/ppc64le",
	}
	defaultTags = []string{
		"latest",
		"latest-root",
	}
)

func init() {
	out, err := runCmd("git", "rev-parse", "--show-toplevel")
	if err != nil {
		logrus.WithError(err).Error("Failed getting git root dir")
		os.Exit(1)
	}
	rootDir = out

	if _, err := runCmdInDirFunc(path.Join(rootDir, "hack/tools"), "go", "build", "-o", path.Join(rootDir, "_bin/ko"), "github.com/google/ko"); err != nil {
		logrus.WithError(err).Error("Failed ensure ko")
		os.Exit(1)
	}
}

type options struct {
	koDockerRepo      string
	prowImageListFile string
	workers           int
	push              bool
	maxRetry          int
}

// Mock for unit testing purpose
var runCmdInDirFunc = runCmdInDir

func runCmdInDir(dir, cmd string, args ...string) (string, error) {
	command := exec.Command(cmd, args...)
	if dir != "" {
		command.Dir = dir
	}
	stdOut, err := command.StdoutPipe()
	if err != nil {
		return "", err
	}
	stdErr, err := command.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := command.Start(); err != nil {
		return "", err
	}
	scanner := bufio.NewScanner(stdOut)
	var allOut string
	for scanner.Scan() {
		out := scanner.Text()
		allOut = allOut + out
		logrus.WithField("cmd", command.Args).Info(out)
	}
	allErr, _ := io.ReadAll(stdErr)
	err = command.Wait()
	// Print error only when command failed
	if err != nil && len(allErr) > 0 {
		logrus.WithField("cmd", command.Args).Error(string(allErr))
	}
	return strings.TrimSpace(allOut), err
}

func runCmd(cmd string, args ...string) (string, error) {
	return runCmdInDirFunc(rootDir, cmd, args...)
}

type imageDef struct {
	Dir            string `json:"dir"`
	Arch           string `json:"arch"`
	remainingRetry int
}

type imageDefs struct {
	Defs []imageDef `json:"images"`
}

func loadImageDefs(p string) ([]imageDef, error) {
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var res imageDefs
	if err := yaml.Unmarshal(b, &res); err != nil {
		return nil, err
	}
	return res.Defs, nil
}

func allBaseTags() ([]string, error) {
	gitTag, err := gitTag()
	if err != nil {
		return nil, err
	}
	// Add a `ko-<GIT_TAG>` tag so that it's easy to identify images built from
	// ko vs. images built from bazel, in case there is a revert needed.
	// TODO(chaodaiG): remove `ko-` tag once the images produced by ko proved to
	// be working
	return append(defaultTags, gitTag, "ko-"+gitTag), nil
}

func allTags(arch string) ([]string, error) {
	baseTags, err := allBaseTags()
	if err != nil {
		return nil, err
	}

	var allTags = baseTags
	for _, otherArch := range otherArches {
		if arch != allArch && arch != otherArch {
			continue
		}
		for _, base := range baseTags {
			// So far only platform supported is linux, trimming off the linux/
			// prefix so that there is no slash in tag. Also for consistency reasons.
			platform := strings.Replace(otherArch, "linux/", "", 1)
			allTags = append(allTags, fmt.Sprintf("%s-%s", base, platform))
		}
	}
	return allTags, nil
}

// gitTag returns YYYYMMDD-<GIT_TAG>
func gitTag() (string, error) {
	prefix, err := runCmd("date", "+v%Y%m%d")
	if err != nil {
		return "", err
	}
	postfix, err := runCmd("git", "describe", "--always", "--dirty")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", prefix, postfix), nil
}

func runGatherStaticScript(id *imageDef, args ...string) error {
	script := path.Join(rootDir, id.Dir, gatherStaicScriptName)
	if _, err := os.Lstat(script); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if _, err := runCmd(script, args...); err != nil {
		return err
	}
	return nil
}

func setup(id *imageDef) error {
	return runGatherStaticScript(id)
}

func teardown(id *imageDef) error {
	return runGatherStaticScript(id, "--cleanup")
}

func buildAndPush(id *imageDef, koDockerRepo string, push bool) error {
	logger := logrus.WithField("image", id.Dir)
	logger.Info("Build and push")
	// So far only supports certain arch
	isSupportedArch := (id.Arch == defaultArch || id.Arch == allArch)
	for _, otherArch := range otherArches {
		if id.Arch == otherArch {
			isSupportedArch = true
		}
	}
	if !isSupportedArch {
		return fmt.Errorf("Arch '%s' not supported, only support %v", id.Arch, append([]string{defaultArch, allArch}, otherArches...))
	}
	publishArgs := []string{"publish", fmt.Sprintf("--tarball=_bin/%s.tar", path.Base(id.Dir)), "--push=false"}
	if push {
		publishArgs = []string{"publish", "--push=true"}
	}
	tags, err := allTags(id.Arch)
	if err != nil {
		return fmt.Errorf("collecting tags: %w", err)
	}
	for _, tag := range tags {
		publishArgs = append(publishArgs, fmt.Sprintf("--tags=%s", tag))
	}
	publishArgs = append(publishArgs, "--base-import-paths", "--platform="+id.Arch, "./"+id.Dir)

	defer teardown(id)
	if err := setup(id); err != nil {
		return fmt.Errorf("setup: %w", err)
	}
	if _, err = runCmd("_bin/ko", publishArgs...); err != nil {
		return fmt.Errorf("running ko: %w", err)
	}
	return nil
}

func main() {
	var o options
	flag.StringVar(&o.prowImageListFile, "prow-images-file", path.Join(rootDir, defaultProwImageListFile), "Yaml file contains list of prow images")
	flag.StringVar(&o.koDockerRepo, "ko-docker-repo", os.Getenv("KO_DOCKER_REPO"), "KO_DOCKER_REPO override")
	flag.IntVar(&o.workers, "workers", defaultWorkersCount, "Number of workers in parallel")
	flag.BoolVar(&o.push, "push", false, "whether push or not")
	flag.IntVar(&o.maxRetry, "retry", defaultRetry, "Number of times retrying for each image")
	flag.Parse()

	if !o.push && o.koDockerRepo == "" {
		o.koDockerRepo = noOpKoDocerRepo
	}
	if err := os.Setenv("KO_DOCKER_REPO", o.koDockerRepo); err != nil {
		logrus.WithError(err).Error("Failed setting KO_DOCKER_REPO")
		os.Exit(1)
	}
	// By default ensures timestamp of images, ref:
	// https://github.com/google/ko#why-are-my-images-all-created-in-1970
	if err := os.Setenv("SOURCE_DATE_EPOCH", strconv.Itoa(int(time.Now().Unix()))); err != nil {
		logrus.WithError(err).Error("Failed setting SOURCE_DATE_EPOCH")
		os.Exit(1)
	}

	// Set VERSION for embedding versions with go build
	gitTag, err := gitTag()
	if err != nil {
		logrus.WithError(err).Error("Failed get git tag")
		os.Exit(1)
	}
	if err := os.Setenv("VERSION", gitTag); err != nil {
		logrus.WithError(err).Error("Failed setting VERSION")
		os.Exit(1)
	}

	ids, err := loadImageDefs(o.prowImageListFile)
	if err != nil {
		logrus.WithError(err).WithField("prow-image-file", o.prowImageListFile).Error("Failed loading")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	imageChan := make(chan imageDef, 10)
	errChan := make(chan error, len(ids))
	doneChan := make(chan imageDef, len(ids))
	// Start workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < o.workers; i++ {
		go func(ctx context.Context, imageChan chan imageDef, errChan chan error, doneChan chan imageDef) {
			for {
				select {
				case id := <-imageChan:
					err := buildAndPush(&id, o.koDockerRepo, o.push)
					if err != nil {
						if id.remainingRetry > 0 {
							// Let another routine handle this, better luck maybe?
							id.remainingRetry--
							imageChan <- id
							// Don't call wg.Done() as we are not done yet
							continue
						}
						errChan <- err
					}
					doneChan <- id
				case <-ctx.Done():
					return
				}
			}
		}(ctx, imageChan, errChan, doneChan)
	}

	for _, id := range ids {
		id := id
		id.remainingRetry = o.maxRetry
		if id.Arch == "" {
			id.Arch = defaultArch
		}
		// Feed into channel instead
		wg.Add(1)
		imageChan <- id
	}

	go func(ctx context.Context, wg *sync.WaitGroup, doneChan chan imageDef) {
		var done int
		for {
			select {
			case id := <-doneChan:
				done++
				logrus.WithFields(logrus.Fields{"image": id.Dir, "done": done, "total": len(ids)}).Info("Done with image.")
				wg.Done()
			case <-ctx.Done():
				return
			}
		}
	}(ctx, &wg, doneChan)

	wg.Wait()
	for {
		select {
		case err := <-errChan:
			logrus.WithError(err).Error("Failed.")
			os.Exit(1)
		default:
			return
		}
	}
}
