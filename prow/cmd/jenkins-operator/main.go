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
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	m "k8s.io/test-infra/prow/metrics"
)

var (
	configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	selector   = flag.String("label-selector", kube.EmptySelector, "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")

	jenkinsURL             = flag.String("jenkins-url", "http://jenkins-proxy", "Jenkins URL")
	jenkinsUserName        = flag.String("jenkins-user", "jenkins-trigger", "Jenkins username")
	jenkinsTokenFile       = flag.String("jenkins-token-file", "", "Path to the file containing the Jenkins API token.")
	jenkinsBearerTokenFile = flag.String("jenkins-bearer-token-file", "", "Path to the file containing the Jenkins API bearer token.")
	certFile               = flag.String("cert-file", "", "Path to a PEM-encoded certificate file.")
	keyFile                = flag.String("key-file", "", "Path to a PEM-encoded key file.")
	caCertFile             = flag.String("ca-cert-file", "", "Path to a PEM-encoded CA certificate file.")
	csrfProtect            = flag.Bool("csrf-protect", false, "Request a CSRF protection token from Jenkins that will be used in all subsequent requests to Jenkins.")

	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
	dryRun          = flag.Bool("dry-run", true, "Whether or not to make mutating API calls to GitHub.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "jenkins-operator"}),
	)

	if _, err := labels.Parse(*selector); err != nil {
		logrus.WithError(err).Fatal("Error parsing label selector.")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	ac := &jenkins.AuthConfig{
		CSRFProtect: *csrfProtect,
	}
	if *jenkinsTokenFile != "" {
		token, err := loadToken(*jenkinsTokenFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read token file.")
		}
		ac.Basic = &jenkins.BasicAuthConfig{
			User:  *jenkinsUserName,
			Token: token,
		}
	} else if *jenkinsBearerTokenFile != "" {
		token, err := loadToken(*jenkinsBearerTokenFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read bearer token file.")
		}
		ac.BearerToken = &jenkins.BearerTokenAuthConfig{
			Token: token,
		}
	} else {
		logrus.Fatal("An auth token for basic or bearer token auth must be supplied.")
	}
	var tlsConfig *tls.Config
	if *certFile != "" && *keyFile != "" {
		config, err := loadCerts(*certFile, *keyFile, *caCertFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read certificate files.")
		}
		tlsConfig = config
	}
	metrics := jenkins.NewMetrics()
	jc, err := jenkins.NewClient(*jenkinsURL, tlsConfig, ac, nil, metrics.ClientMetrics)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not setup Jenkins client.")
	}

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read Github oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		logrus.WithError(err).Fatal("Must specify a valid --github-endpoint URL.")
	}

	var ghc *github.Client
	if *dryRun {
		ghc = github.NewDryRunClient(oauthSecret, *githubEndpoint)
	} else {
		ghc = github.NewClient(oauthSecret, *githubEndpoint)
	}

	c := jenkins.NewController(kc, jc, ghc, nil, configAgent, *selector)

	// Push metrics to the configured prometheus pushgateway endpoint.
	pushGateway := configAgent.Config().PushGateway
	if pushGateway.Endpoint != "" {
		go m.PushMetrics("jenkins-operator", pushGateway.Endpoint, pushGateway.Interval)
	}
	// Serve Jenkins logs here and proxy deck to use this endpoint
	// instead of baking agent-specific logic in deck. This func also
	// serves prometheus metrics.
	go serve(jc)
	// gather metrics for the jobs handled by the jenkins controller.
	go gather(c)

	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			if err := c.Sync(); err != nil {
				logrus.WithError(err).Error("Error syncing.")
			}
			duration := time.Since(start)
			logrus.WithField("duration", fmt.Sprintf("%v", duration)).Info("Synced")
			metrics.ResyncPeriod.Observe(duration.Seconds())
		case <-sig:
			logrus.Info("Jenkins operator is shutting down...")
			return
		}
	}
}

func loadToken(file string) (string, error) {
	raw, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSpace(raw)), nil
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

// serve starts a http server and serves Jenkins logs
// and prometheus metrics. Meant to be called inside
// a goroutine.
func serve(jc *jenkins.Client) {
	http.Handle("/", gziphandler.GzipHandler(handleLog(jc)))
	http.Handle("/metrics", promhttp.Handler())
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

// gather metrics from the jenkins controller.
// Meant to be called inside a goroutine.
func gather(c *jenkins.Controller) {
	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			c.SyncMetrics()
			logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
		case <-sig:
			logrus.Debug("Jenkins operator gatherer is shutting down...")
			return
		}
	}
}
