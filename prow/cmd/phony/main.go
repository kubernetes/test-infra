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
	"flag"
	"io/ioutil"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/phony"
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

	if err := phony.SendHook(*address, *event, body, []byte(*hmac)); err != nil {
		logrus.WithError(err).Error("Error sending hook.")
	} else {
		logrus.Info("Hook sent.")
	}
}
