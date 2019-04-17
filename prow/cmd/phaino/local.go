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
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func realPath(p string) (string, error) {
	return filepath.Abs(os.ExpandEnv(p))
}

func readMount(mount coreapi.VolumeMount) (string, error) {
	fmt.Fprintf(os.Stderr, "local %s path (%q mount): ", mount.MountPath, mount.Name)
	var out string
	fmt.Scanln(&out)
	return realPath(out)
}

func volume(pod coreapi.PodSpec, name string) *coreapi.Volume {
	for _, v := range pod.Volumes {
		if v.Name == name {
			return &v
		}
	}
	return nil
}

func pathAlias(r prowapi.Refs) string {
	if r.PathAlias == "" {
		return fmt.Sprintf("github.com/%s/%s", r.Org, r.Repo)
	}
	return r.PathAlias
}

func readRepo(path string) (string, error) {
	wd, err := workingDir()
	if err != nil {
		return "", fmt.Errorf("workingDir: %v", err)
	}
	def, err := findRepo(wd, path)
	if err != nil {
		logrus.WithError(err).WithField("repo", path).Warn("could not find repo")
	}
	fmt.Fprintf(os.Stderr, "local /path/to/%s", path)
	if def != "" {
		fmt.Fprintf(os.Stderr, " [%s]", def)
	}
	fmt.Fprint(os.Stderr, ": ")
	var out string
	fmt.Scanln(&out)
	if out == "" {
		out = def
	}
	return realPath(out)
}

func workingDir() (string, error) {
	if wd := os.Getenv("BUILD_WORKING_DIRECTORY"); wd != "" {
		return wd, nil // running via bazel run
	}
	return os.Getwd() // running outside bazel
}

// findRepo will attempt to find a repo in logical locations under path.
//
// It will first try to find foo/bar somewhere under $PWD or a $PWD dir.
// AKA if $PWD is /go/src it will match /go/src/foo/bar, /go/foo/bar or /foo/bar
// Next it will look for the basename somewhere under $PWD or a $PWD dir.
// AKA if $PWD is /go/src it will match /go/src/bar, /go/bar or /bar
// If both of these strategies fail it will return an error.
func findRepo(wd, path string) (string, error) {
	opwd, err := realPath(wd)
	if err != nil {
		return "", fmt.Errorf("wd not found: %v", err)
	}
	if strings.HasPrefix(path, "github.com/kubernetes/") {
		path = strings.Replace(path, "github.com/kubernetes/", "k8s.io/", 1)
	}

	var old string
	pwd := opwd
	for old != pwd {
		old = pwd
		if strings.HasSuffix(pwd, "/"+path) {
			return pwd, nil
		}
		pwd = filepath.Dir(pwd)
	}
	pwd = opwd
	for old != pwd {
		old = pwd
		check := filepath.Join(pwd, path)
		if info, err := os.Stat(check); err == nil && info.IsDir() {
			return check, nil
		}
		pwd = filepath.Dir(pwd)
	}

	base := filepath.Base(path)
	pwd = opwd
	for old != pwd {
		old = pwd
		check := filepath.Join(pwd, base)
		if info, err := os.Stat(check); err == nil && info.IsDir() {
			return check, nil
		}
		pwd = filepath.Dir(pwd)
	}
	return "", errors.New("cannot find repo")
}

var baseArgs = []string{"docker", "run", "--rm=true"}

func checkPrivilege(cont coreapi.Container, allow bool) (bool, error) {
	if cont.SecurityContext == nil {
		return false, nil
	}
	if cont.SecurityContext.Privileged == nil {
		return false, nil
	}
	if !*cont.SecurityContext.Privileged {
		return false, nil
	}
	fmt.Fprint(os.Stderr, "Privileged jobs are unsafe. Remove from local run? [yes]: ")
	var out string
	fmt.Scanln(&out)
	if out == "no" || out == "n" {
		if !allow {
			return false, errors.New("privileged jobs are disallowed")
		}
		logrus.Warn("DANGER: privileged containers are unsafe security risks. Please refactor")
		return true, nil
	}
	return false, nil
}

func convertToLocal(log *logrus.Entry, pj prowapi.ProwJob, allowPrivilege bool) ([]string, error) {
	log.Info("Converting job into docker run command...")
	var localArgs []string
	localArgs = append(localArgs, baseArgs...)
	container := pj.Spec.PodSpec.Containers[0]
	decoration := pj.Spec.DecorationConfig
	var entrypoint string
	args := container.Command
	args = append(args, container.Args...)
	if len(args) > 0 && decoration != nil {
		entrypoint = args[0]
		args = args[1:]
	}
	if entrypoint == "" && decoration != nil {
		return nil, errors.New("decorated jobs must specify command and/or args")
	}
	if entrypoint != "" {
		localArgs = append(localArgs, "--entrypoint="+entrypoint)
	}

	for _, env := range container.Env {
		localArgs = append(localArgs, "-e", env.Name+"="+env.Value)
	}

	priv, err := checkPrivilege(container, allowPrivilege)
	if err != nil {
		return nil, err
	}
	if priv {
		localArgs = append(localArgs, "--privileged")
	}

	if container.Resources.Requests != nil {
		// TODO(fejta): https://docs.docker.com/engine/reference/run/#runtime-constraints-on-resources
		log.Warn("Ignoring resource requirements")
	}

	for _, mount := range container.VolumeMounts {
		vol := volume(*pj.Spec.PodSpec, mount.Name)
		if vol == nil {
			return nil, fmt.Errorf("mount %q missing associated volume", mount.Name)
		}
		if vol.EmptyDir != nil {
			localArgs = append(localArgs, "-v", mount.MountPath)
		} else {
			local, err := readMount(mount)
			if err != nil {
				return nil, fmt.Errorf("bad mount %q: %v", mount.Name, err)
			}
			arg := local + ":" + mount.MountPath
			if mount.ReadOnly {
				arg += ":ro"
			}
			localArgs = append(localArgs, "-v", arg)
		}
	}

	var workingDir string

	if decoration != nil {
		var refs []prowapi.Refs
		if pj.Spec.Refs != nil {
			refs = append(refs, *pj.Spec.Refs)
		}
		refs = append(refs, pj.Spec.ExtraRefs...)
		for _, ref := range refs {
			path := pathAlias(ref)
			repo, err := readRepo(path)
			if err != nil {
				return nil, fmt.Errorf("bad %q repo: %v", path, err)
			}
			dest := filepath.Join("/go/src", path)
			if workingDir == "" {
				workingDir = dest
			}
			localArgs = append(localArgs, "-v", repo+":"+dest)

		}
	}
	if workingDir == "" {
		workingDir = container.WorkingDir
	}
	if workingDir != "" {
		localArgs = append(localArgs, "-v", workingDir, "-w", workingDir)
	}

	image := pj.Spec.PodSpec.Containers[0].Image
	localArgs = append(localArgs, image)
	localArgs = append(localArgs, args...)
	return localArgs, nil
}

func printArgs(localArgs []string) {
	base := len(baseArgs)
	for i, a := range localArgs {
		if i < base {
			fmt.Printf("%q ", a)
		} else {
			fmt.Printf("\\\n %q ", a)
		}
	}
	fmt.Println()
}

func start(args []string) (*exec.Cmd, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd, cmd.Start()
}

func abort(ctx context.Context, log *logrus.Entry, cmd *exec.Cmd) error {
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		log.WithError(err).Warn("Interrupt error")
	} else {
		log.Warn("Interrupted")
	}
	ch := make(chan error)
	go func() {
		defer close(ch)
		ch <- cmd.Wait()
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		if err := cmd.Process.Kill(); err != nil { // TODO(fejta): docker rm
			log.WithError(err).Warn("Kill error")
			return err
		}
		log.Warn("Killed")
	}
	return nil
}

func convertJob(ctx context.Context, log *logrus.Entry, pj prowapi.ProwJob, priv, onlyPrint bool, timeout, grace time.Duration) error {
	// TODO(fejta): default grace and timeout to the job's decoration_config
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	args, err := convertToLocal(log, pj, priv)
	if err != nil {
		return fmt.Errorf("convert: %v", err)
	}
	printArgs(args)
	if onlyPrint {
		return nil
	}
	log.Info("Starting job...")
	cmd, err := start(args)
	if err != nil {
		return fmt.Errorf("start: %v", err)
	}
	log = log.WithField("pid", cmd.Process.Pid)
	ch := make(chan error)
	go func() {
		log.Info("Waiting for job to finish...")
		ch <- cmd.Wait()
	}()
	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		if grace < time.Second {
			grace = time.Second
		}
		ctx2, c2 := context.WithTimeout(context.Background(), grace)
		defer c2()
		if err := abort(ctx2, log, cmd); err != nil {
			log.WithError(err).Error("Abort error")
		}
		return ctx.Err()
	}
}
