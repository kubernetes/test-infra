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
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func realPath(p string) (string, error) {
	return filepath.Abs(os.ExpandEnv(p))
}

func scanln(ctx context.Context) (string, error) {
	ch := make(chan string)
	go func() {
		defer close(ch)
		var out string
		fmt.Scanln(&out)
		ch <- out
	}()
	select {
	case s := <-ch:
		return s, nil
	case <-ctx.Done():
		os.Stdin.Close()
		return "", ctx.Err()
	}
}

func readMount(ctx context.Context, mount coreapi.VolumeMount) (string, error) {
	fmt.Fprintf(os.Stderr, "local %s path (%q mount): ", mount.MountPath, mount.Name)
	out, err := scanln(ctx)
	if err != nil {
		return "", fmt.Errorf("scan: %v", err)
	}
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

func readRepo(ctx context.Context, path string) (string, error) {
	wd, err := workingDir()
	if err != nil {
		return "", fmt.Errorf("workingDir: %v", err)
	}
	def, err := findRepo(wd, path)
	if err != nil { // If k8s/test-infra is not under GOPATH, find under GOPATH.
		def, err = findRepo(filepath.Join(os.Getenv("GOPATH"), "src"), path)
	}
	if err != nil {
		logrus.WithError(err).WithField("repo", path).Warn("could not find repo")
	}
	fmt.Fprintf(os.Stderr, "local /path/to/%s", path)
	if def != "" {
		fmt.Fprintf(os.Stderr, " [%s]", def)
	}
	fmt.Fprint(os.Stderr, ": ")
	out, err := scanln(ctx)
	if err != nil {
		return "", fmt.Errorf("scan: %v", err)
	}
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

func checkPrivilege(ctx context.Context, cont coreapi.Container, allow bool) (bool, error) {
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
	out, err := scanln(ctx)
	if err != nil {
		return false, fmt.Errorf("scan: %v", err)
	}
	if out == "no" || out == "n" {
		if !allow {
			return false, errors.New("privileged jobs are disallowed")
		}
		logrus.Warn("DANGER: privileged containers are unsafe security risks. Please refactor")
		return true, nil
	}
	return false, nil
}

func convertToLocal(ctx context.Context, log *logrus.Entry, pj prowapi.ProwJob, name string, allowPrivilege bool) ([]string, error) {
	log.Info("Converting job into docker run command...")
	var localArgs []string
	localArgs = append(localArgs, baseArgs...)
	localArgs = append(localArgs, "--name="+name)
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

	// TODO(fejta): capabilities
	priv, err := checkPrivilege(ctx, container, allowPrivilege)
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
			local, err := readMount(ctx, mount)
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
			repo, err := readRepo(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("bad repo(%s): %v", path, err)
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

	for k, v := range pj.Labels {
		localArgs = append(localArgs, "--label="+k+"="+v)
	}
	localArgs = append(localArgs, "--label=phaino=true")

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

func kill(cid, signal string) error {
	cmd := exec.Command("docker", "kill", "--signal="+signal, cid)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

var (
	nameLock sync.Mutex
	nameId   int
)

func containerID() string {
	nameLock.Lock()
	defer nameLock.Unlock()
	nameId++
	return fmt.Sprintf("phaino-%d-%d", os.Getpid(), nameId)
}

func convertJob(ctx context.Context, log *logrus.Entry, pj prowapi.ProwJob, priv, onlyPrint bool, timeout, grace time.Duration) error {
	cid := containerID()
	args, err := convertToLocal(ctx, log, pj, cid, priv)
	if err != nil {
		return fmt.Errorf("convert: %v", err)
	}
	printArgs(args)
	if onlyPrint {
		return nil
	}
	log.Info("Starting job...")
	// TODO(fejta): default grace and timeout to the job's decoration_config
	if timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd, err := start(args)
	if err != nil {
		return fmt.Errorf("start: %v", err)
	}
	log = log.WithField("container", cid)
	ch := make(chan error)
	go func() {
		log.Info("Waiting for job to finish...")
		ch <- cmd.Wait()
	}()

	select {
	case err := <-ch:
		return err
	case <-ctx.Done():
		// cancelled
	}

	if grace < time.Second {
		log.WithField("grace", grace).Info("Increasing grace period to the 1s minimum")
		grace = time.Second
	}
	log = log.WithFields(logrus.Fields{
		"grace":     grace,
		"interrupt": ctx.Err(),
	})
	abort, cancel := context.WithTimeout(context.Background(), grace)
	defer cancel()
	if err := kill(cid, "SIGINT"); err != nil {
		log.WithError(err).Error("Interrupt error")
	} else {
		log.Warn("Interrupted container...")
	}
	select {
	case err := <-ch:
		log.WithError(err).Info("Graceful exit after interrupt")
		return err
	case <-abort.Done():
	}
	if err := kill(cid, "SIGKILL"); err != nil {
		return fmt.Errorf("kill: %v", err)
	}
	return fmt.Errorf("grace period expired, aborted: %v", ctx.Err())
}
