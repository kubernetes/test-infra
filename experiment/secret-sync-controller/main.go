package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"k8s.io/test-infra/experiment/secret-sync-controller/client"
	"log"
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
	k8sClientset, err := client.NewK8sClientset()
	if err != nil {
		log.Fatal(err)
	}
	secretManagerCtx := context.Background()
	secretManagerClient, err := client.NewSecretManagerClient(secretManagerCtx)
	if err != nil {
		log.Fatal(err)
	}

	var clientInterface ClientInterface

	client := Client{
		K8sClientset:        k8sClientset,
		SecretManagerClient: secretManagerClient,
		Ctx:                 secretManagerCtx,
	}

	clientInterface = client

	secretSyncConfig, err := LoadConfig(o.configPath)
	if err != nil {
		log.Fatal(err)
	}

	secretSyncCollection := secretSyncConfig.Parse()

	// TODO: modularize source & destination secrets

	for i, pair := range secretSyncCollection.Pairs {

		// k8s requires & yields secrets with type map[string][]byte,
		// while SecretManager uses []byte (e.g. account: "foo"\n secret: "bar")

		updated, err := clientInterface.UpdatedVersion(pair.Source)
		fmt.Println(updated) // should be true
		updated, err = clientInterface.UpdatedVersion(pair.Destination)
		fmt.Println(updated) // should be true

		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Secret pair [%d]:\n{\n", i)

		fmt.Printf("\tSource secret version ")
		pair.Source.PrintSecret()

		fmt.Printf("\tDestination secret version ")
		pair.Destination.PrintSecret()
		fmt.Printf("}\n========================\n")

		updated, err = clientInterface.UpdatedVersion(pair.Source)
		fmt.Println(updated) // should be false
		updated, err = clientInterface.UpdatedVersion(pair.Destination)
		fmt.Println(updated) // should be false

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
