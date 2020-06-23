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
	"reflect"
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
	secretManagerClient, err := client.NewSecretManagerClient(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	var clientInterface ClientInterface

	client := Client{
		K8sClientset:        k8sClientset,
		SecretManagerClient: secretManagerClient,
	}

	clientInterface = client

	secretSyncConfig, err := LoadConfig(o.configPath)
	if err != nil {
		log.Fatal(err)
	}

	for i, spec := range secretSyncConfig.Specs {
		fmt.Printf("Secret pair [%d]:\n{\n", i)
		err := Sync(clientInterface, spec)
		if err != nil {
			log.Fatal(err)
		}
	}

}

func Sync(clientInterface ClientInterface, spec SecretSyncSpec) error {
	// k8s requires & yields secrets with type map[string][]byte,
	// while SecretManager uses []uint8 (e.g. account: "foo"\n secret: "bar")

	// get source secret
	srcData, srcErr := clientInterface.GetSecretManagerSecret(spec.Source)
	if srcErr != nil {
		return srcErr
	}

	// get destination secret
	destData, destErr := clientInterface.GetKubernetesSecret(spec.Destination)
	if destErr != nil {
		return destErr
	}

	// update destination secret
	if reflect.DeepEqual(*srcData, *destData) {
		return nil
	}
	// update destination secret value
	// inserts a key-value pair if spec.Destination does not exist yet
	patchedData, patchErr := clientInterface.UpsertKubernetesSecret(spec.Destination, srcData)
	if patchErr != nil {
		return patchErr
	}

	fmt.Printf("\tSource secret: \n\t%s\n", srcData.Data)
	fmt.Printf("\tDestination secret: \n\t%s\n", destData.Data)
	fmt.Printf("\tPatched secret: \n\t%s\n", patchedData.Data)
	fmt.Printf("}\n========================\n")

	return nil
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
