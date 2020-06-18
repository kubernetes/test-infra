package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/test-infra/experiment/secret-sync-controller/client"
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
	k8sClientset := client.NewK8sClientset()

	secretManagerCtx := context.Background()
	secretManagerClient := client.NewSecretManagerClient(secretManagerCtx)

	secretSyncConfig, err := LoadConfig(o.configPath)
	if err != nil {
		fmt.Println(err)
	}

	// TODO: modularize source & destination secrets

	for i, spec := range secretSyncConfig.Specs {

		// always store secret values as map[string][]byte (?)
		// k8s requires & yields secrets with type map[string][]byte,
		// while SecretManager uses []byte (e.g. account: "foo"\n secret: "bar")
		// source
		var sourceSecret map[string][]byte
		sourceVersion := -1

		// dest
		var destSecret map[string][]byte
		destVersion := -1

		sourceVersion, sourceSecret = spec.Source.GetLatestSecretVersion(k8sClientset, secretManagerCtx, secretManagerClient)
		destVersion, destSecret = spec.Destination.GetLatestSecretVersion(k8sClientset, secretManagerCtx, secretManagerClient)

		fmt.Printf("Secret pair [%d]:\n{\n", i)
		fmt.Printf("\tSource secret version %d : \n", sourceVersion)
		for key, val := range sourceSecret {
			fmt.Printf("\t\t%s: \"%s\"\n", key, val)
		}
		fmt.Printf("\tDestination secret version %d : \n", destVersion)
		for key, val := range destSecret {
			fmt.Printf("\t\t%s: \"%s\"\n", key, val)
		}
		fmt.Printf("}\n========================\n")

	}

}

// LoadConfig loads from podConfig yaml file and returns the structure
func LoadConfig(config string) (*SecretSyncConfig, error) {
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

	syncConfig := SecretSyncConfig{}
	err = yaml.Unmarshal(yamlFile, &syncConfig)
	if err != nil {
		return nil, fmt.Errorf("Error reading YAML file: %s\n", err)
	}

	return &syncConfig, nil
}
