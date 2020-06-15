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

package main

import (
	"b01901143.git/secret-sync/client_util"
	"context"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"os"
)

type options struct {
	configPath string
}

func (o *options) Validate() error {
	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()

	// TODO: modularize clients
	k8s_clientset := client_util.NewK8sClientset()

	secretManager_ctx := context.Background()
	secretManager_client := client_util.NewSecretManagerClient(secretManager_ctx)

	secretSyncSpecs, err := LoadConfig(o.configPath)
	if err != nil {
		fmt.Println(err)
	}

	// TODO: modularize source & destination secrets

	for i, spec := range *secretSyncSpecs {

		// always store secret values as map[string][]byte (?)
		// k8s requires & yields secrets with type map[string][]byte,
		// while SecretManager uses []byte (e.g. account: "foo"\n secret: "bar")
		// source
		var source_secret map[string][]byte
		source_version := -1

		// dest
		var dest_secret map[string][]byte
		dest_version := -1

		source_version, source_secret = spec.Source.GetLatestSecretVersion(k8s_clientset, secretManager_ctx, secretManager_client)
		dest_version, dest_secret = spec.Destination.GetLatestSecretVersion(k8s_clientset, secretManager_ctx, secretManager_client)

		fmt.Printf("Secret pair [%d]:\n{\n", i)
		fmt.Printf("\tSource secret version %d : \n", source_version)
		for key, val := range source_secret {
			fmt.Printf("\t\t%s: \"%s\"\n", key, val)
		}
		fmt.Printf("\tDestination secret version %d : \n", dest_version)
		for key, val := range dest_secret {
			fmt.Printf("\t\t%s: \"%s\"\n", key, val)
		}
		fmt.Printf("}\n========================\n")

	}

}

// LoadConfig loads from podConfig yaml file and returns the structure
func LoadConfig(config string) (*[]SecretSyncSpec, error) {
	stat, err := os.Stat(config)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("config cannot be a dir - %s", config)
	}

	yamlFile, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, fmt.Errorf("Error reading YAML file: %s\n", err)
	}

	specs := []SecretSyncSpec{}
	err = yaml.Unmarshal(yamlFile, &specs)
	if err != nil {
		return nil, fmt.Errorf("Error reading YAML file: %s\n", err)
	}

	return &specs, nil
}
