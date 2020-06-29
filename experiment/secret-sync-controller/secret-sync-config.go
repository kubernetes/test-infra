package main

// configuration and sync-pair defination

import (
	"gopkg.in/yaml.v2"
)

// Structs for configuration
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
