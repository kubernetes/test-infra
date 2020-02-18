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

package gencred

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	flag "github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/test-infra/gencred/pkg/certificate"
	"k8s.io/test-infra/gencred/pkg/serviceaccount"
	"k8s.io/test-infra/gencred/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	// defaultContextName is the default context name.
	defaultContextName = "build"
	// defaultConfigFileName is the default kubeconfig filename.
	defaultConfigFileName = "/dev/stdout"
)

// options are the available command-line flags.
type options struct {
	context        string
	name           string
	output         string
	certificate    bool
	serviceaccount bool
	overwrite      bool
}

// parseFlags parses the command-line flags.
func (o *options) parseFlags() {
	flag.StringVar(&o.context, "context", "", "The name of the kubeconfig context to use.")
	flag.StringVarP(&o.name, "name", "n", defaultContextName, "Context name for the kubeconfig entry.")
	flag.StringVarP(&o.output, "output", "o", defaultConfigFileName, "Output path for generated kubeconfig file.")
	flag.BoolVarP(&o.certificate, "certificate", "c", false, "Authorize with a client certificate and key.")
	flag.BoolVarP(&o.serviceaccount, "serviceaccount", "s", false, "Authorize with a service account.")
	flag.BoolVar(&o.overwrite, "overwrite", false, "Overwrite (rather than merge) output file if exists.")

	flag.Parse()
}

// validateFlags validates the command-line flags.
func (o *options) validateFlags() error {
	var err error

	if len(o.context) == 0 {
		return &util.ExitError{Message: "--context option is required.", Code: 1}
	}

	if len(o.name) == 0 {
		return &util.ExitError{Message: "-n, --name option is required.", Code: 1}
	}

	o.output, err = filepath.Abs(o.output)
	if err != nil {
		return &util.ExitError{Message: fmt.Sprintf("-o, --output option invalid: %v.", o.output), Code: 1}
	}

	if util.DirExists(o.output) {
		return &util.ExitError{Message: fmt.Sprintf("-o, --output already exists and is a directory: %v.", o.output), Code: 1}
	}

	if o.serviceaccount && o.certificate {
		return &util.ExitError{Message: "-c, --certificate and -s, --serviceaccount are mutually exclusive options.", Code: 1}
	}

	return nil
}

// mergeConfigs merges an existing kubeconfig file with a new entry with precedence given to the existing config.
func mergeConfigs(o options, kubeconfig []byte) ([]byte, error) {
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}
	defer os.Remove(tmpFile.Name())

	err = ioutil.WriteFile(tmpFile.Name(), kubeconfig, 0644)
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}

	loadingRules := clientcmd.ClientConfigLoadingRules{
		Precedence: []string{o.output, tmpFile.Name()},
	}

	mergedConfig, err := loadingRules.Load()
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}

	json, err := runtime.Encode(latest.Codec, mergedConfig)
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}

	kubeconfig, err = yaml.JSONToYAML(json)
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}

	return kubeconfig, nil
}

// writeConfig writes a kubeconfig file to an output file.
func writeConfig(o options, clientset kubernetes.Interface) error {
	// kubeconfig is a kubernetes config.
	var kubeconfig []byte

	dir, file := filepath.Split(o.output)

	err := os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return &util.ExitError{Message: fmt.Sprintf("unable to create output directory %v: %v.", dir, err), Code: 1}
	}

	if o.certificate {
		if kubeconfig, err = certificate.CreateKubeConfigWithCertificateCredentials(clientset, o.name); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to create kubeconfig file with cert and key for %v: %v.", o.name, err), Code: 1}
		}
	} else {
		// Service account credentials are the default if unspecified.
		if kubeconfig, err = serviceaccount.CreateKubeConfigWithServiceAccountCredentials(clientset, o.name); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to create kubeconfig file with service account for %v: %v.", o.name, err), Code: 1}
		}
	}

	if !o.overwrite && util.FileExists(o.output) {
		if kubeconfig, err = mergeConfigs(o, kubeconfig); err != nil {
			return err
		}
	}

	if err = ioutil.WriteFile(o.output, kubeconfig, 0644); err != nil {
		return &util.ExitError{Message: fmt.Sprintf("unable to write to file %v: %v.", file, err), Code: 1}
	}

	return nil
}

// main entry point.
func Main() {
	var o options

	o.parseFlags()
	if err := o.validateFlags(); err != nil {
		util.PrintErrAndExit(err)
	}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: o.context}
	kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	config, err := kubeconfig.ClientConfig()
	if err != nil {
		util.PrintErrAndExit(err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		util.PrintErrAndExit(err)
	}

	if err = writeConfig(o, clientset); err != nil {
		util.PrintErrAndExit(err)
	}
}
