package main

// configuration and sync-pair defination

import (
	"fmt"
	"gopkg.in/yaml.v2"
)

// Structs for secret sync configuration
type SecretSyncConfig struct {
	Specs []SecretSyncSpec `yaml:"specs"`
}

type SecretSyncSpec struct {
	Source      SecretManagerSpec `yaml:"source"`
	Destination KubernetesSpec    `yaml:"destination"`
}

type KubernetesSpec struct {
	Namespace string `yaml:"namespace"`
	Secret    string `yaml:"secret"`
	Key       string `yaml:"key"`
}

type SecretManagerSpec struct {
	Project string `yaml:"project"`
	Secret  string `yaml:"secret"`
}

func (config SecretSyncConfig) String() string {
	d, _ := yaml.Marshal(config)
	return string(d)
}
func (spec SecretSyncSpec) String() string {
	return fmt.Sprintf("{%s -> %s}", spec.Source, spec.Destination)
}
func (gsm SecretManagerSpec) String() string {
	return fmt.Sprintf("SecretManager:/projects/%s/secrets/%s", gsm.Project, gsm.Secret)
}
func (k8s KubernetesSpec) String() string {
	return fmt.Sprintf("Kubernetes:/namespaces/%s/secrets/%s[%s]", k8s.Namespace, k8s.Secret, k8s.Key)
}

func (config SecretSyncConfig) Validate() error {
	if len(config.Specs) == 0 {
		return fmt.Errorf("Empty secret sync configuration.")
	}
	syncFrom := make(map[KubernetesSpec]SecretManagerSpec)
	for _, spec := range config.Specs {
		switch {
		case spec.Source.Project == "":
			return fmt.Errorf("Missing <project> field for <source> in spec %s.", spec)
		case spec.Source.Secret == "":
			return fmt.Errorf("Missing <secret> field for <source> in spec %s.", spec)
		case spec.Destination.Namespace == "":
			return fmt.Errorf("Missing <namespace> field for <destination> in spec %s.", spec)
		case spec.Destination.Secret == "":
			return fmt.Errorf("Missing <secret> field for <destination> in spec %s.", spec)
		case spec.Destination.Key == "":
			return fmt.Errorf("Missing <key> field for <destination> in spec %s.", spec)
		}

		// check if spec.Destination already has a source
		src, ok := syncFrom[spec.Destination]
		if ok {
			return fmt.Errorf("Fail to generate sync pair %s: Secret %s already has a source (%s).", spec, spec.Destination, src)
		}
		syncFrom[spec.Destination] = spec.Source
	}
	return nil
}
