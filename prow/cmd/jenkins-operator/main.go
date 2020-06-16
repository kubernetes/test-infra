/*
Copyright 2017 The Kubernetes Authors.

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
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/logrusutil"
	m "k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	configPath    string
	jobConfigPath string
	selector      string
	totURL        string

	jenkinsURL             string
	jenkinsUserName        string
	jenkinsTokenFile       string
	jenkinsBearerTokenFile string
	certFile               string
	keyFile                string
	caCertFile             string
	csrfProtect            bool
	skipReport             bool

	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
	github                 prowflagutil.GitHubOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	if _, err := url.ParseRequestURI(o.jenkinsURL); err != nil {
		return fmt.Errorf("invalid -jenkins-url URI: %q", o.jenkinsURL)
	}

	if o.jenkinsTokenFile == "" && o.jenkinsBearerTokenFile == "" {
		return errors.New("either --jenkins-token-file or --jenkins-bearer-token-file must be set")
	} else if o.jenkinsTokenFile != "" && o.jenkinsBearerTokenFile != "" {
		return errors.New("only one of --jenkins-token-file or --jenkins-bearer-token-file can be set")
	}

	var transportSecretsProvided int
	if o.certFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if o.keyFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if o.caCertFile == "" {
		transportSecretsProvided = transportSecretsProvided + 1
	}
	if transportSecretsProvided != 0 && transportSecretsProvided != 3 {
		return errors.New("either --cert-file, --key-file, and --ca-cert-file must all be provided or none of them must be provided")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.jenkinsURL, "jenkins-url", "http://jenkins-proxy", "Jenkins URL")
	fs.StringVar(&o.jenkinsUserName, "jenkins-user", "jenkins-trigger", "Jenkins username")
	fs.StringVar(&o.jenkinsTokenFile, "jenkins-token-file", "", "Path to the file containing the Jenkins API token.")
	fs.StringVar(&o.jenkinsBearerTokenFile, "jenkins-bearer-token-file", "", "Path to the file containing the Jenkins API bearer token.")
	fs.StringVar(&o.certFile, "cert-file", "", "Path to a PEM-encoded certificate file.")
	fs.StringVar(&o.keyFile, "key-file", "", "Path to a PEM-encoded key file.")
	fs.StringVar(&o.caCertFile, "ca-cert-file", "", "Path to a PEM-encoded CA certificate file.")
	fs.BoolVar(&o.csrfProtect, "csrf-protect", false, "Request a CSRF protection token from Jenkins that will be used in all subsequent requests to Jenkins.")

	fs.BoolVar(&o.skipReport, "skip-report", false, "Whether or not to ignore report with githubClient")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub/Kubernetes/Jenkins.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

	if _, err := labels.Parse(o.selector); err != nil {
		logrus.WithError(err).Fatal("Error parsing label selector.")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	prowJobClient, err := o.kubernetes.ProwJobClient(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting ProwJob client for the infrastructure cluster.")
	}

	ac := &jenkins.AuthConfig{
		CSRFProtect: o.csrfProtect,
	}

	var tokens []string
	tokens = append(tokens, o.github.TokenPath)

	if o.jenkinsTokenFile != "" {
		tokens = append(tokens, o.jenkinsTokenFile)
	}

	if o.jenkinsBearerTokenFile != "" {
		tokens = append(tokens, o.jenkinsBearerTokenFile)
	}

	// Start the secret agent.
	secretAgent := &secret.Agent{}
	if err := secretAgent.Start(tokens); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	if o.jenkinsTokenFile != "" {
		ac.Basic = &jenkins.BasicAuthConfig{
			User:     o.jenkinsUserName,
			GetToken: secretAgent.GetTokenGenerator(o.jenkinsTokenFile),
		}
	} else if o.jenkinsBearerTokenFile != "" {
		ac.BearerToken = &jenkins.BearerTokenAuthConfig{
			GetToken: secretAgent.GetTokenGenerator(o.jenkinsBearerTokenFile),
		}
	}
	var tlsConfig *tls.Config
	if o.certFile != "" && o.keyFile != "" {
		config, err := loadCerts(o.certFile, o.keyFile, o.caCertFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read certificate files.")
		}
		tlsConfig = config
	}
	metrics := jenkins.NewMetrics()
	jc, err := jenkins.NewClient(o.jenkinsURL, o.dryRun, tlsConfig, ac, nil, metrics.ClientMetrics)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not setup Jenkins client.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	c, err := jenkins.NewController(prowJobClient, jc, githubClient, nil, cfg, o.totURL, o.selector, o.skipReport)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to instantiate Jenkins controller.")
	}

	// Expose prometheus metrics
	m.ExposeMetrics("jenkins-operator", cfg().PushGateway, o.instrumentationOptions.MetricsPort)

	// Serve Jenkins logs here and proxy deck to use this endpoint
	// instead of baking agent-specific logic in deck
	logMux := http.NewServeMux()
	logMux.Handle("/", gziphandler.GzipHandler(handleLog(jc)))
	server := &http.Server{Addr: ":8080", Handler: logMux}
	interrupts.ListenAndServe(server, 5*time.Second)

	// gather metrics for the jobs handled by the jenkins controller.
	interrupts.TickLiteral(func() {
		start := time.Now()
		c.SyncMetrics()
		logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
	}, 30*time.Second)

	// run the controller
	interrupts.TickLiteral(func() {
		start := time.Now()
		if err := c.Sync(); err != nil {
			logrus.WithError(err).Error("Error syncing.")
		}
		duration := time.Since(start)
		logrus.WithField("duration", fmt.Sprintf("%v", duration)).Info("Synced")
		metrics.ResyncPeriod.Observe(duration.Seconds())
	}, 30*time.Second)
}

func loadCerts(certFile, keyFile, caCertFile string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if caCertFile != "" {
		caCert, err := ioutil.ReadFile(caCertFile)
		if err != nil {
			return nil, err
		}
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)
		tlsConfig.RootCAs = caCertPool
	}

	tlsConfig.BuildNameToCertificate()
	return tlsConfig, nil
}
