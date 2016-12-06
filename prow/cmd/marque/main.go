/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/kube"
)

const (
	// Time before generating the first cert, for safety.
	sleepTime = time.Minute
	// Time between renewals.
	renewTime = 12 * time.Hour
)

func main() {
	kc, err := kube.NewClientInCluster("default")
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	root, err := ioutil.TempDir("", "certbot")
	if err != nil {
		logrus.WithError(err).Fatal("Could not create temp dir.")
	}
	http.Handle("/", http.FileServer(http.Dir(root)))
	go http.ListenAndServe(":http", nil)

	logrus.Infof("Sleeping for %v before generating cert.", sleepTime)
	time.Sleep(sleepTime)
	if err := generate(root); err != nil {
		logrus.WithError(err).Fatal("Error getting cert.")
	}
	if err := replaceSecret(kc); err != nil {
		logrus.WithError(err).Fatal("Error updating secrets.")
	}

	for range time.Tick(renewTime) {
		logrus.Info("Renewing.")
		if err := renew(); err != nil {
			logrus.WithError(err).Warning("Error renewing cert.")
		}
		if err := replaceSecret(kc); err != nil {
			logrus.WithError(err).Warning("Error updating secrets.")
		}
	}
}

func generate(root string) error {
	args := []string{
		"certonly",
		"--agree-tos",
		"--email", "spxtr@google.com",
		"--non-interactive",
		"--webroot",
		"-w", root,
		"-d", "prow.k8s.io",
	}
	cmd := exec.Command("certbot", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot error: %v output: %s", err, string(output))
	}
	return nil
}

func renew() error {
	cmd := exec.Command("certbot", "renew")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot error: %v output: %s", err, string(output))
	}
	return nil
}

func replaceSecret(c *kube.Client) error {
	key, err := ioutil.ReadFile("/etc/letsencrypt/live/prow.k8s.io/privkey.pem")
	if err != nil {
		return fmt.Errorf("could not read privkey: %v", err)
	}
	cert, err := ioutil.ReadFile("/etc/letsencrypt/live/prow.k8s.io/fullchain.pem")
	if err != nil {
		return fmt.Errorf("could not read fullchain: %v", err)
	}

	s := kube.Secret{
		Data: map[string]string{
			"tls.crt": string(cert),
			"tls.key": string(key),
		},
	}
	if err := c.ReplaceSecret("prow-k8s-cert", s); err != nil {
		return fmt.Errorf("could not replace secret: %v", err)
	}
	return nil
}
