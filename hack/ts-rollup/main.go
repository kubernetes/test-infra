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
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

const (
	defaultOutputDir = "_output/js"

	rimraf = "node_modules/rimraf/bin.js"
	tsc    = "node_modules/typescript/bin/tsc"
	rollup = "node_modules/rollup/dist/bin/rollup"
	terser = "node_modules/terser/bin/terser"

	defaultRollupConfig = "rollup.config.js"
	defaultTerserConfig = "hack/ts.rollup_bundle.min.minify_options.json"

	defaultWorkersCount = 5
)

var (
	rootDir string
)

func rootDirWithGit() {
	// Best effort
	out, err := runCmd(nil, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		logrus.WithError(err).Warn("Failed getting git root dir")
	}
	rootDir = out
}

type options struct {
	packages    string
	workers     int
	cleanupOnly bool
}

type packagesInfo struct {
	Packages []packageInfo `yaml:"packages"`
}

type packageInfo struct {
	Dir        string `yaml:"dir"`
	Entrypoint string `yaml:"entrypoint"`
	Dst        string `yaml:"dst"`
}

func loadPackagesInfo(f string) (*packagesInfo, error) {
	b, err := os.ReadFile(f)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", f, err)
	}
	var res packagesInfo
	return &res, yaml.Unmarshal(b, &res)
}

// Mock for unit testing purpose
var runCmdInDirFunc = runCmdInDir

func runCmdInDir(dir string, additionalEnv []string, cmd string, args ...string) (string, error) {
	log := logrus.WithFields(logrus.Fields{"cmd": cmd, "args": args})
	command := exec.Command(cmd, args...)
	if dir != "" {
		command.Dir = dir
	}
	command.Env = append(os.Environ(), additionalEnv...)
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
		log.Info(out)
	}
	allErr, _ := io.ReadAll(stdErr)
	err = command.Wait()
	if len(allErr) > 0 {
		if err != nil {
			log.Error(string(allErr))
		} else {
			log.Warn(string(allErr))
		}
	}
	return strings.TrimSpace(allOut), err
}

func runCmd(additionalEnv []string, cmd string, args ...string) (string, error) {
	return runCmdInDirFunc(rootDir, additionalEnv, cmd, args...)
}

func rollupOne(pi *packageInfo, cleanupOnly bool) error {
	entrypointFileBasename := strings.TrimSuffix(path.Base(pi.Entrypoint), ".ts")
	// Intermediate output files, stored under `_output` dir
	jsOutputFile := path.Join(defaultOutputDir, pi.Dir, entrypointFileBasename+".js")
	bundleOutputDir := path.Join(defaultOutputDir, pi.Dir)
	rollupOutputFile := path.Join(bundleOutputDir, fmt.Sprintf("%s_bundle.js", entrypointFileBasename))
	// terserOutputFile is the minified bundle, which is placed next to all
	// other static files in the source tree
	terserOutputFile := path.Join(pi.Dir, pi.Dst)
	if cleanupOnly {
		return os.Remove(terserOutputFile)
	}
	if _, err := runCmd(nil, rimraf, "dist"); err != nil {
		return fmt.Errorf("running rimraf: %w", err)
	}
	if _, err := runCmd(nil, tsc, "-p", path.Join(pi.Dir, "tsconfig.json"), "--outDir", defaultOutputDir); err != nil {
		return fmt.Errorf("running tsc: %w", err)
	}
	if _, err := runCmd(nil, rollup, "--environment", fmt.Sprintf("ROLLUP_OUT_FILE:%s,ROLLUP_ENTRYPOINT:%s", rollupOutputFile, jsOutputFile), "-c", defaultRollupConfig, "--preserveSymlinks"); err != nil {
		return fmt.Errorf("running rollup: %w", err)
	}
	if _, err := runCmd(nil, terser, rollupOutputFile, "--output", terserOutputFile, "--config-file", defaultTerserConfig); err != nil {
		return fmt.Errorf("running terser: %w", err)
	}
	return nil
}

func main() {
	var o options
	flag.StringVar(&o.packages, "packages", "", "Yaml file contains list of packages to be rolled up.")
	flag.IntVar(&o.workers, "workers", defaultWorkersCount, "Number of workers in parallel.")
	flag.BoolVar(&o.cleanupOnly, "cleanup-only", false, "Indicate cleanup only.")
	flag.StringVar(&rootDir, "root-dir", "", "Root dir of this repo, where everything happens.")
	flag.Parse()

	if rootDir == "" {
		rootDirWithGit()
	}
	if rootDir == "" {
		logrus.Error("Unable to determine root dir, please pass in --root-dir.")
		os.Exit(1)
	}

	pis, err := loadPackagesInfo(o.packages)
	if err != nil {
		logrus.WithError(err).WithField("packages", o.packages).Error("Failed loading")
		os.Exit(1)
	}

	var wg sync.WaitGroup
	packageChan := make(chan packageInfo, 10)
	errChan := make(chan error, len(pis.Packages))
	doneChan := make(chan packageInfo, len(pis.Packages))
	// Start workers
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < o.workers; i++ {
		go func(ctx context.Context, packageChan chan packageInfo, errChan chan error, doneChan chan packageInfo) {
			for {
				select {
				case pi := <-packageChan:
					err := rollupOne(&pi, o.cleanupOnly)
					if err != nil {
						errChan <- fmt.Errorf("rollup package %q failed: %v", pi.Entrypoint, err)
					}
					doneChan <- pi
				case <-ctx.Done():
					return
				}
			}
		}(ctx, packageChan, errChan, doneChan)
	}

	for _, pi := range pis.Packages {
		pi := pi
		wg.Add(1)
		packageChan <- pi
	}

	go func(ctx context.Context, wg *sync.WaitGroup, doneChan chan packageInfo) {
		var done int
		for {
			select {
			case pi := <-doneChan:
				done++
				logrus.WithFields(logrus.Fields{"entrypoint": pi.Entrypoint, "done": done, "total": len(pis.Packages)}).Info("Done with package.")
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
