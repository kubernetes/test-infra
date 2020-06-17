/*
Copyright 2020 The Kubernetes Authors.

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

package deployer

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/klog"
	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) DumpClusterLogs() error {
	klog.Info("GCE deployer starting DumpClusterLogs()")

	if err := d.verifyFlags(); err != nil {
		return fmt.Errorf("dump cluster logs could not verify flags: %s", err)
	}

	if err := d.makeLogsDir(); err != nil {
		return fmt.Errorf("couldn't make logs dir: %s", err)
	}

	env := d.buildEnv()
	outfile, err := os.Create(filepath.Join(d.logsDir, "cluster-info.log"))
	if err != nil {
		return fmt.Errorf("failed to create cluster-info log file: %s", err)
	}
	defer outfile.Close()

	command := []string{
		d.kubectl,
		"cluster-info",
		"dump",
	}
	klog.Infof("About to run: %s", command)

	cmd := exec.Command(command[0], command[1:]...)
	cmd.SetEnv(env...)
	exec.InheritOutput(cmd)
	cmd.SetStderr(os.Stderr)
	cmd.SetStdout(outfile)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("couldn't use kubectl to dump cluster info: %s", err)
	}

	env = append(env, "KUBE_GCE_INSTANCE_PREFIX=kubernetes")

	command = []string{
		filepath.Join(d.RepoRoot, "cluster", "log-dump", "log-dump.sh"),
		d.logsDir,
	}
	klog.Infof("About to run: %s", command)

	cmd = exec.Command(command[0], command[1:]...)
	cmd.SetEnv(env...)
	exec.InheritOutput(cmd)
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to use log-dump.sh for cluster logs: %s", err)
	}

	return nil
}

func (d *deployer) makeLogsDir() error {
	_, err := os.Stat(d.logsDir)

	if err == nil {
		// TODO: removeall instead?
		return fmt.Errorf("cluster logs directory %s already exists, please clean up manually before continuing", d.logsDir)
	} else if os.IsNotExist(err) {
		err := os.Mkdir(d.logsDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create %s: %s", d.logsDir, err)
		}
		return nil
	}

	return fmt.Errorf("unexpected exception when making cluster logs directory: %s", err)
}
