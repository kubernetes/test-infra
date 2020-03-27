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

package deployer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"k8s.io/test-infra/kubetest2/pkg/exec"
)

func (d *deployer) prepareGcpIfNeeded() error {
	// TODO(RonWeber): This is an almost direct copy/paste from kubetest's prepareGcp()
	// It badly needs refactored.

	var endpoint string
	switch env := d.environment; {
	case env == "test":
		endpoint = "https://test-container.sandbox.googleapis.com/"
	case env == "staging":
		endpoint = "https://staging-container.sandbox.googleapis.com/"
	case env == "staging2":
		endpoint = "https://staging2-container.sandbox.googleapis.com/"
	case env == "prod":
		endpoint = "https://container.googleapis.com/"
	case urlRe.MatchString(env):
		endpoint = env
	default:
		return fmt.Errorf("--environment must be one of {test,staging,staging2,prod} or match %v, found %q", urlRe, env)
	}

	//TODO(RonWeber): boskos
	if err := os.Setenv("CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS", "1"); err != nil {
		return fmt.Errorf("could not set CLOUDSDK_CORE_PRINT_UNHANDLED_TRACEBACKS=1: %v", err)
	}
	if err := os.Setenv("CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER", endpoint); err != nil {
		return err
	}

	if err := runWithOutput(exec.Command("gcloud", "config", "set", "project", d.project)); err != nil {
		return fmt.Errorf("Failed to set project %s : err %v", d.project, err)
	}

	// gcloud creds may have changed
	if err := activateServiceAccount(d.gcpServiceAccount); err != nil {
		return err
	}

	// Ensure ssh keys exist
	log.Print("Checking existing of GCP ssh keys...")
	k := filepath.Join(home(".ssh"), "google_compute_engine")
	if _, err := os.Stat(k); err != nil {
		return err
	}
	pk := k + ".pub"
	if _, err := os.Stat(pk); err != nil {
		return err
	}

	//TODO(RonWeber): kubemark
	return nil
}

// Activate service account if set or do nothing.
func activateServiceAccount(path string) error {
	if path == "" {
		return nil
	}
	return runWithOutput(exec.Command("gcloud", "auth", "activate-service-account", "--key-file="+path))
}

// home returns $HOME/part/part/part
func home(parts ...string) string {
	p := append([]string{os.Getenv("HOME")}, parts...)
	return filepath.Join(p...)
}

// insertPath does export PATH=path:$PATH
func insertPath(path string) error {
	return os.Setenv("PATH", fmt.Sprintf("%v:%v", path, os.Getenv("PATH")))
}
