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
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/gencred/pkg/certificate"
	"k8s.io/test-infra/gencred/pkg/serviceaccount"
	"k8s.io/test-infra/gencred/pkg/util"
	"sigs.k8s.io/prow/prow/interrupts"
	"sigs.k8s.io/yaml"

	"google.golang.org/api/container/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// defaultContextName is the default context name.
	defaultContextName = "build"
	// defaultConfigFileName is the default kubeconfig filename.
	defaultConfigFileName = "/dev/stdout"
	defaultDuration       = 2 * 24 * time.Hour
)

// options are the available command-line flags.
type options struct {
	context        string
	name           string
	output         string
	certificate    bool
	serviceaccount bool
	duration       time.Duration
	overwrite      bool

	config string
	filter filter
	// RefreshInterval defines how frequently the secret is refreshed.
	refreshInterval time.Duration
}

type config struct {
	Clusters []*clusterConfig `json:"clusters"`
}

type clusterConfig struct {
	// GKEConnection is the connection string for a GKE cluster, in the format of
	// `projects/%s/locations/%s/clusters/%s`
	GKEConnection *string `json:"gke,omitempty"`
	// Context is the name of the kubeconfig context to use from local kube env.
	Context *string `json:"context,omitempty"`
	// Name is the alias of generated kubeconfig.
	Name string `json:"name,omitempty"`
	// WithCertificate means authorize with a client certificate and key.
	WithCertificate bool `json:"with-certificate,omitempty"`
	// WithServiceAccount means authorize with a service account. This is the
	// default if with-certificate is false.
	WithServiceAccount bool `json:"with-serviceaccount,omitempty"`
	// Duration is the duration how long the cred is valid, default is 2 days.
	Duration *metav1.Duration `json:"duration,omitempty"`
	// Overwrite (rather than merge) output file if exists.
	Overwrite bool `json:"overwrite,omitempty"`
	// GSMSecretConfig is the config for where to store the kubeconfig in Google secret manager.
	GSMSecretConfig *GSMSecretConfig `json:"gsm,omitempty"`
	// GSMSecretConfig is the local path for generated kubeconfig.
	Output *string `json:"output,omitempty"`
}

type GSMSecretConfig struct {
	Project string `json:"project"`
	Name    string `json:"name"`
}

type filter struct {
	gkeConnection string
	context       string
}

// parseFlags parses the command-line flags.
func (o *options) parseFlags() {
	flag.StringVar(&o.context, "context", "", "The name of the kubeconfig context to use.")
	flag.StringVarP(&o.name, "name", "n", defaultContextName, "Context name for the kubeconfig entry.")
	flag.StringVarP(&o.output, "output", "o", defaultConfigFileName, "Output path for generated kubeconfig file.")
	flag.BoolVarP(&o.certificate, "certificate", "c", false, "Authorize with a client certificate and key.")
	flag.BoolVarP(&o.serviceaccount, "serviceaccount", "s", false, "Authorize with a service account.")
	flag.DurationVar(&o.duration, "duration", defaultDuration, "How long the cred is valid, default is 2 days.")
	flag.BoolVar(&o.overwrite, "overwrite", false, "Overwrite (rather than merge) output file if exists.")

	flag.StringVar(&o.config, "config", "", "Configurations for running gencred.")
	flag.StringVar(&o.filter.context, "context-filter", "", "Once specified, gencred only works on this context from the config file, must be supplied together with --config.")
	flag.StringVar(&o.filter.gkeConnection, "gke-filter", "", "Once specified, gencred only works on this gkeConn from the config file, must be supplied together with --config.")
	flag.DurationVar(&o.refreshInterval, "refresh-interval", 0, "RefreshInterval defines how frequently the secret is refreshed, unit is second.")
	flag.Parse()
}

// validateFlags validates the command-line flags.
func (o *options) defaultAndValidateFlags() (*config, error) {
	// config is mutually exclusive from local cluster.
	if len(o.config) > 0 && len(o.context) > 0 {
		return nil, &util.ExitError{Message: "--config option is mutually exclusive with other options.", Code: 1}
	}

	if (len(o.filter.context) > 0 || len(o.filter.gkeConnection) > 0) && len(o.config) == 0 {
		return nil, &util.ExitError{Message: "--context-filter and --gke-filter can only be used when --config option is supplied.", Code: 1}
	}

	// Read value from yaml files
	var c config
	if len(o.config) > 0 {
		// Load from config yaml file
		body, err := os.ReadFile(o.config)
		if err != nil {
			util.PrintErrAndExit(err)
		}
		if err := yaml.Unmarshal(body, &c); err != nil {
			util.PrintErrAndExit(err)
		}
	} else {
		c.Clusters = []*clusterConfig{
			{
				Context:            &o.context,
				Name:               o.name,
				WithCertificate:    o.certificate,
				WithServiceAccount: o.serviceaccount,
				Duration:           &metav1.Duration{Duration: o.duration},
				Overwrite:          o.overwrite,
				Output:             &o.output,
			},
		}
	}

	for _, cc := range c.Clusters {
		if (cc.Context == nil || len(*cc.Context) == 0) && (cc.GKEConnection == nil || len(*cc.GKEConnection) == 0) {
			return nil, &util.ExitError{Message: "one of context or gke connection string is required.", Code: 1}
		}

		if len(cc.Name) == 0 {
			return nil, &util.ExitError{Message: "-n, --name option is required.", Code: 1}
		}

		if cc.Output != nil && len(*cc.Output) > 0 {
			absPath, err := filepath.Abs(*cc.Output)
			if err != nil {
				return nil, &util.ExitError{Message: fmt.Sprintf("-o, --output option invalid: %v.", cc.Output), Code: 1}
			}
			cc.Output = &absPath
			if util.DirExists(*cc.Output) {
				return nil, &util.ExitError{Message: fmt.Sprintf("-o, --output already exists and is a directory: %v.", cc.Output), Code: 1}
			}
		}

		if cc.WithServiceAccount && cc.WithCertificate {
			return nil, &util.ExitError{Message: "-c, --certificate and -s, --serviceaccount are mutually exclusive options.", Code: 1}
		}
	}

	return &c, nil
}

// mergeConfigs merges an existing kubeconfig file with a new entry with precedence given to the existing config.
func mergeConfigs(c clusterConfig, kubeconfig []byte) ([]byte, error) {
	tmpFile, err := os.CreateTemp("", "")
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}
	defer os.Remove(tmpFile.Name())

	err = os.WriteFile(tmpFile.Name(), kubeconfig, 0644)
	if err != nil {
		return nil, &util.ExitError{Message: err.Error(), Code: 1}
	}

	loadingRules := clientcmd.ClientConfigLoadingRules{
		Precedence: []string{*c.Output, tmpFile.Name()},
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
func writeConfig(c clusterConfig, clientset kubernetes.Interface) error {
	var err error
	// kubeconfig is a kubernetes config.
	var kubeconfig []byte

	if c.WithCertificate {
		if kubeconfig, err = certificate.CreateKubeConfigWithCertificateCredentials(clientset, c.Name); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to create kubeconfig file with cert and key for %v: %v.", c.Name, err), Code: 1}
		}
	} else {
		// Service account credentials are the default if unspecified.
		if kubeconfig, err = serviceaccount.CreateKubeConfigWithServiceAccountCredentials(clientset, c.Name, *c.Duration); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to create kubeconfig file with service account for %v: %v.", c.Name, err), Code: 1}
		}
	}

	if c.Output != nil {
		dir, file := filepath.Split(*c.Output)

		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to create output directory %v: %v.", dir, err), Code: 1}
		}

		if !c.Overwrite && util.FileExists(*c.Output) {
			if kubeconfig, err = mergeConfigs(c, kubeconfig); err != nil {
				return err
			}
		}

		if err = os.WriteFile(*c.Output, kubeconfig, 0644); err != nil {
			return &util.ExitError{Message: fmt.Sprintf("unable to write to file %v: %v.", file, err), Code: 1}
		}
	}

	if c.GSMSecretConfig != nil {
		client, err := secretmanager.NewClient(c.GSMSecretConfig.Project, false)
		if err != nil {
			return err
		}
		ctx := context.Background()
		allSecrets, err := client.ListSecrets(ctx)
		if err != nil {
			return err
		}
		var existing bool
		for _, s := range allSecrets {
			if strings.HasSuffix(s.Name, "/"+c.GSMSecretConfig.Name) {
				existing = true
			}
		}
		if !existing {
			if _, err := client.CreateSecret(ctx, c.GSMSecretConfig.Name); err != nil {
				return err
			}
		}
		if err := client.AddSecretVersion(ctx, c.GSMSecretConfig.Name, kubeconfig); err != nil {
			return err
		}
	}
	return nil
}

// main entry point.
func Main() {
	var o options

	o.parseFlags()
	c, err := o.defaultAndValidateFlags()
	if err != nil {
		util.PrintErrAndExit(err)
	}

	if o.refreshInterval == 0 {
		if err := runOnce(*c, o.filter); err != nil {
			util.PrintErrAndExit(err)
		}
		return
	}

	defer interrupts.WaitForGracefulShutdown()
	interrupts.Tick(func() {
		runOnce(*c, o.filter)
	}, func() time.Duration { return o.refreshInterval })
}

func runOnce(c config, filter filter) error {
	// Make sure process everyone before crying.
	var errs []error
	var config *rest.Config
	for _, cc := range c.Clusters {
		if cc.GKEConnection != nil && cc.Context != nil {
			errs = append(errs, errors.New("gke and context are mutually exclusive"))
			continue
		}
		if (filter.context != "" && cc.Context != nil && filter.context != *cc.Context) ||
			(filter.gkeConnection != "" && cc.GKEConnection != nil && filter.gkeConnection != *cc.GKEConnection) {
			continue
		}
		if cc.Duration.Duration == 0 {
			cc.Duration = &metav1.Duration{Duration: defaultDuration}
		}
		var clientset *kubernetes.Clientset
		if cc.Context != nil {
			loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
			configOverrides := &clientcmd.ConfigOverrides{CurrentContext: *cc.Context}
			kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

			var err error
			config, err = kubeconfig.ClientConfig()
			if err != nil {
				errs = append(errs, err)
				continue
			}
		} else {
			gkeService, err := container.NewService(context.Background())
			if err != nil {
				errs = append(errs, err)
				continue
			}
			cluster, err := container.NewProjectsLocationsClustersService(gkeService).Get(*cc.GKEConnection).Do()
			if err != nil {
				errs = append(errs, err)
				continue
			}

			decodedClientCertificate, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClientCertificate)
			if err != nil {
				errs = append(errs, fmt.Errorf("decode client certificate error: %v", err))
				continue
			}
			decodedClientKey, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClientKey)
			if err != nil {
				errs = append(errs, fmt.Errorf("decode client key error: %v", err))
				continue
			}
			decodedClusterCaCertificate, err := base64.StdEncoding.DecodeString(cluster.MasterAuth.ClusterCaCertificate)
			if err != nil {
				errs = append(errs, fmt.Errorf("decode cluster CA certificate error: %v", err))
				continue
			}

			config = &rest.Config{
				Host: "https://" + cluster.Endpoint,
				TLSClientConfig: rest.TLSClientConfig{
					Insecure: false,
					CertData: decodedClientCertificate,
					KeyData:  decodedClientKey,
					CAData:   decodedClusterCaCertificate,
				},
			}

			cred, err := google.DefaultTokenSource(context.Background(), container.CloudPlatformScope)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			config.Wrap(func(rt http.RoundTripper) http.RoundTripper {
				return &oauth2.Transport{
					Source: cred,
					Base:   rt,
				}
			})
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to initialise clientset from config: %s", err))
			continue
		}

		if err := writeConfig(*cc, clientset); err != nil {
			errs = append(errs, err)
			continue
		}

		log.Printf("Succeeded processing %s", cc.Name)
	}
	return utilerrors.NewAggregate(errs)
}
