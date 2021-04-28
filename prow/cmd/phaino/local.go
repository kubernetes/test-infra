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
	"go/build"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

const (
	// The well-known env name for gcloud credentials.
	gcloudCredEnvName = "GOOGLE_APPLICATION_CREDENTIALS"
	// The path to mount the gcloud default config files to the container.
	gcloudDefaultConfigMountPath = "/root/.config/gcloud"
	// The well-known env name for kubeconfig.
	kubeconfigEnvKey = "KUBECONFIG"
	// The path to mount the kubectl default config files to the container.
	kubectlDefaultConfigMountPath = "/root/.kube"
	// The default GOPATH in the container.
	// To be consistent with https://github.com/kubernetes/test-infra/blob/19829768bb8ff2a9bb8de76e4dbcc1e520aaeb18/prow/pod-utils/decorate/podspec.go#L52
	defaultGOPATH = "/home/prow/go"
)

var baseArgs = []string{"docker", "run", "--rm=true"}

func realPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("cannot find repo")
	}
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return "", fmt.Errorf("%q does not exist on local", p)
	}

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
	if r.PathAlias != "" {
		return r.PathAlias
	}
	repoPath := fmt.Sprintf("%s/%s", r.Org, r.Repo)
	if !strings.HasPrefix(r.Org, "http://") && !strings.HasPrefix(r.Org, "https://") {
		repoPath = fmt.Sprintf("github.com/%s", repoPath)
	}
	return repoPath
}

func readRepo(path string, readUserInput func(string, string) (string, error)) (string, error) {
	wd, err := workingDir()
	if err != nil {
		return "", fmt.Errorf("workingDir: %v", err)
	}
	// First finding repo from under GOPATH, then fall back from local path.
	// Prefers GOPATH as it's more accurate, as finding from local path performs
	// an aggressive searching, could return "${PWD}/src/test-infra" when search
	// for "someother-org/test-infra".
	def, err := findRepoUnderGopath(path)
	if err != nil { // Fall back to find repo from local
		def, err = findRepoFromLocal(wd, path)
	}
	if err == nil && def != "" {
		return realPath(def)
	}
	if err != nil {
		logrus.WithError(err).WithField("repo", path).Warn("could not find repo")
	}

	out, err := readUserInput(path, def)
	if err != nil {
		return "", fmt.Errorf("scan: %v", err)
	}
	return realPath(out)
}

func findRepoUnderGopath(path string) (string, error) {
	fmt.Fprintf(os.Stderr, "fallback to GOPATH: %s\n: ", build.Default.GOPATH)
	pkg, err := build.Default.Import(path, build.Default.GOPATH, build.FindOnly|build.IgnoreVendor)
	if err != nil {
		return "", err
	}
	return pkg.Dir, nil
}

func workingDir() (string, error) {
	if wd := os.Getenv("BUILD_WORKING_DIRECTORY"); wd != "" {
		return wd, nil // running via bazel run
	}
	return os.Getwd() // running outside bazel
}

// findRepoFromLocal will attempt to find a repo in logical locations under path.
//
// It will first try to find foo/bar somewhere under $PWD or a $PWD dir.
// AKA if $PWD is /go/src it will match /go/src/foo/bar, /go/foo/bar or /foo/bar
// Next it will look for the basename somewhere under $PWD or a $PWD dir.
// AKA if $PWD is /go/src it will match /go/src/bar, /go/bar or /bar
// If both of these strategies fail it will return an error.
func findRepoFromLocal(wd, path string) (string, error) {
	opwd, err := realPath(wd)
	if err != nil {
		return "", fmt.Errorf("wd not found: %v", err)
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

func checkPrivilege(cont coreapi.Container, pjName string, allow bool) (bool, error) {
	if cont.SecurityContext == nil {
		return false, nil
	}
	if cont.SecurityContext.Privileged == nil {
		return false, nil
	}
	if !*cont.SecurityContext.Privileged {
		return false, nil
	}
	if !allow {
		return false, errors.New("privileged jobs are disallowed")
	}

	logrus.Warningf("WARNING: running privileged job %q can allow nearly all access to the host, please be careful with it", pjName)
	return true, nil
}

func (opts *options) convertToLocal(ctx context.Context, log *logrus.Entry, pj prowapi.ProwJob, name string) ([]string, error) {
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

	// TODO(fejta): capabilities
	priv, err := checkPrivilege(container, pj.Spec.Job, opts.priv)
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

	volumeMounts, err := opts.resolveVolumeMounts(ctx, pj, container, readMount)
	if err != nil {
		return nil, errors.New("error resolving the volume mounts")
	}

	envs := opts.resolveEnvVars(container)

	// Setup gcloud credentials
	if opts.useLocalGcloudCredentials {
		setupGcloudCredentials(volumeMounts, envs)
	}

	// Setup kubeconfig
	if opts.useLocalKubeconfig {
		setupKubeconfig(volumeMounts, envs)
	}

	workingDir, err := opts.resolveRefs(ctx, volumeMounts, pj, container)
	if err != nil {
		return nil, errors.New("error resolving the refs")
	}

	if workingDir != "" {
		localArgs = append(localArgs, "-w", workingDir)
	}
	// Add args for volume mounts.
	for target, src := range volumeMounts {
		localArgs = append(localArgs, "-v", src+":"+target)
	}
	// Add args for env vars.
	for envKey, envVal := range envs {
		localArgs = append(localArgs, "-e", envKey+"="+envVal)
	}
	// Add args for labels.
	for k, v := range pj.Labels {
		localArgs = append(localArgs, "--label="+k+"="+v)
	}
	localArgs = append(localArgs, "--label=phaino=true")

	image := pj.Spec.PodSpec.Containers[0].Image
	localArgs = append(localArgs, image)
	localArgs = append(localArgs, args...)
	return localArgs, nil
}

func (opts *options) resolveVolumeMounts(ctx context.Context, pj prowapi.ProwJob, container coreapi.Container,
	getMount func(ctx context.Context, mount coreapi.VolumeMount) (string, error)) (map[string]string, error) {
	skippedVolumesMounts := sets.NewString(opts.skippedVolumesMounts...)
	// A map of volume mounts for the run.
	// Key is the mount path and value is the local path.
	volumeMounts := make(map[string]string)
	for _, mount := range container.VolumeMounts {
		if skippedVolumesMounts.Has(mount.Name) {
			logrus.Infof("Volume mount %q skipped", mount.Name)
			continue
		}
		vol := volume(*pj.Spec.PodSpec, mount.Name)
		if vol == nil {
			return nil, fmt.Errorf("mount %q missing associated volume", mount.Name)
		}
		if vol.EmptyDir != nil {
			volumeMounts[mount.MountPath] = ""
		} else {
			local, err := getMount(ctx, mount)
			if err != nil {
				return nil, fmt.Errorf("bad mount %q: %v", mount.Name, err)
			}
			mountPath := mount.MountPath
			if mount.ReadOnly {
				mountPath += ":ro"
			}
			volumeMounts[mountPath] = local
		}
	}
	for pathInContainer, localPath := range opts.extraVolumesMounts {
		volumeMounts[pathInContainer] = localPath
	}
	return volumeMounts, nil
}

func (opts *options) resolveEnvVars(container coreapi.Container) map[string]string {
	skippedEnvVars := sets.NewString(opts.skippedEnvVars...)
	// A map of env vars for the run.
	// Key is the env name and value is the env value.
	envs := make(map[string]string)
	for _, env := range container.Env {
		if skippedEnvVars.Has(env.Name) {
			continue
		}
		envs[env.Name] = env.Value
	}
	for name, value := range opts.extraEnvVars {
		envs[name] = value
	}
	return envs
}

func setupGcloudCredentials(volumeMounts, envs map[string]string) {
	gcloudKey := os.Getenv(gcloudCredEnvName)
	// If GOOGLE_APPLICATION_CREDENTIALS is not empty, also mount the key file
	// to the container
	if gcloudKey != "" {
		if _, err := os.Stat(gcloudKey); !os.IsNotExist(err) {
			volumeMounts[gcloudKey+":ro"] = gcloudKey
			envs[gcloudCredEnvName] = gcloudKey
		} else {
			logrus.Warningf("The GOOGLE_APPLICATION_CREDENTIALS file does not exist on your local machine, thus gcloud authentication won't work in the container")
		}
	} else {
		// We only want to use the default gcloud credentials if GOOGLE_APPLICATION_CREDENTIALS env var is not explicitly set.
		// Its default location is `~/.config/gcloud` on MacOS and Linux, see https://cloud.google.com/sdk/docs/configurations#what_is_a_configuration
		defaultGcloudConfigPath := path.Join(os.Getenv("HOME"), ".config/gcloud")
		if _, err := os.Stat(defaultGcloudConfigPath); !os.IsNotExist(err) {
			volumeMounts[gcloudDefaultConfigMountPath] = defaultGcloudConfigPath
			// Overwrite the gcloud config path, as per https://stackoverflow.com/a/48343135
			envs["CLOUDSDK_CONFIG"] = gcloudDefaultConfigMountPath
		} else {
			logrus.Warningf("The default gcloud credentials does not exist on your local machine, thus gcloud authentication won't work in the container")
		}
	}
}

func setupKubeconfig(volumeMounts, envs map[string]string) {
	kubeconfigEnvVarVal := os.Getenv(kubeconfigEnvKey)
	// If KUBECONFIG is not empty, also mount the kubeconfig files to the
	// container
	if kubeconfigEnvVarVal != "" {
		envs[kubeconfigEnvKey] = kubeconfigEnvVarVal
		var inexistentKubeconfigFiles []string
		for _, f := range strings.Split(kubeconfigEnvVarVal, string(os.PathListSeparator)) {
			if _, err := os.Stat(f); !os.IsNotExist(err) {
				inexistentKubeconfigFiles = append(inexistentKubeconfigFiles, f)
			}
		}
		if len(inexistentKubeconfigFiles) == 0 {
			for _, f := range strings.Split(kubeconfigEnvVarVal, string(os.PathListSeparator)) {
				volumeMounts[f+":ro"] = f
			}
			envs[kubeconfigEnvKey] = kubeconfigEnvVarVal
		} else {
			logrus.Warningf("kubeconfig files %v do not exist on your local machine, thus kubectl authentication won't work in the container", inexistentKubeconfigFiles)
		}
	} else {
		// We only want to use the default kube context if KUBECONFIG env var is not explicitly set.
		// Its default location is `~/.kube`, see https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/
		defaultKubeconfigPath := path.Join(os.Getenv("HOME"), ".kube")
		if _, err := os.Stat(defaultKubeconfigPath); !os.IsNotExist(err) {
			volumeMounts[kubectlDefaultConfigMountPath] = defaultKubeconfigPath
			envs[kubeconfigEnvKey] = path.Join(kubectlDefaultConfigMountPath, "config")
		} else {
			logrus.Warning("The default kube context does not exist on your local machine, thus kubectl authentication won't work in the container")
		}
	}
}

func (opts *options) resolveRefs(ctx context.Context, volumeMounts map[string]string,
	pj prowapi.ProwJob, container coreapi.Container) (string, error) {
	var workingDir string

	readUserInput := func(path, def string) (string, error) {
		fmt.Fprintf(os.Stderr, "local /path/to/%s", path)
		if def != "" {
			fmt.Fprintf(os.Stderr, " [%s]", def)
		}
		fmt.Fprint(os.Stderr, ": ")
		return scanln(ctx)
	}

	goSrcPath := filepath.Join(opts.gopath, "src")
	var refs []prowapi.Refs
	if pj.Spec.Refs != nil {
		refs = append(refs, *pj.Spec.Refs)
	}
	refs = append(refs, pj.Spec.ExtraRefs...)
	for _, ref := range refs {
		repoPath := pathAlias(ref)
		dest := filepath.Join(goSrcPath, repoPath)
		// The repo hasn't been mounted.
		if _, ok := opts.extraVolumesMounts[dest]; !ok {
			repo, err := readRepo(repoPath, readUserInput)
			if err != nil {
				return "", fmt.Errorf("bad repo(%s) when resolving the refs: %v", repoPath, err)
			}
			volumeMounts[dest] = repo
		}
		if workingDir == "" {
			workingDir = dest
		}
	}
	if workingDir == "" {
		workingDir = container.WorkingDir
	}
	return workingDir, nil
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

func (opts *options) convertJob(ctx context.Context, log *logrus.Entry, pj prowapi.ProwJob) error {
	cid := containerID()
	args, err := opts.convertToLocal(ctx, log, pj, cid)
	if err != nil {
		return fmt.Errorf("convert: %v", err)
	}
	printArgs(args)
	if opts.printCmd {
		return nil
	}
	log.Info("Starting job...")
	// TODO(fejta): default grace and timeout to the job's decoration_config
	if opts.timeout > 0 {
		var cancel func()
		ctx, cancel = context.WithTimeout(ctx, opts.timeout)
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

	grace := opts.grace
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
