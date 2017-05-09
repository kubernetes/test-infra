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
	"bytes"
	"flag"
	"io/ioutil"
	"net/http"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

var (
	address = flag.String("address", "http://localhost:8888/hook", "Where to send the fake hook.")
	hmac    = flag.String("hmac", "abcde12345", "HMAC token to sign payload with.")
	event   = flag.String("event", "ping", "Type of event to send, such as pull_request.")
	payload = flag.String("payload", "", "File to send as payload. If unspecified, sends \"{}\".")
)

func main() {
	flag.Parse()

	var body []byte
	if *payload == "" {
		body = []byte("{}")
	} else {
		d, err := ioutil.ReadFile(*payload)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read payload file.")
		}
		body = d
	}

	req, err := http.NewRequest(http.MethodPost, *address, bytes.NewBuffer(body))
	if err != nil {
		logrus.WithError(err).Fatal("Could not make request.")
	}
	req.Header.Set("X-GitHub-Event", *event)
	req.Header.Set("X-Hub-Signature", github.PayloadSignature(body, []byte(*hmac)))
	req.Header.Set("content-type", "application/json")

	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		logrus.WithError(err).Fatal("Error making HTTP request.")
	}
	defer resp.Body.Close()
	rb, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logrus.WithError(err).Fatal("Error reading response body.")
	}
	logrus.WithFields(logrus.Fields{
		"code": resp.StatusCode,
		"body": string(bytes.TrimSpace(rb)),
	}).Info("HTTP response.")
}
