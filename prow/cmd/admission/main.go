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

package main

import (
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
)

type options struct {
	cert                   string
	privateKey             string
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.StringVar(&o.cert, "tls-cert-file", "", "Path to x509 certificate for HTTPS")
	flags.StringVar(&o.privateKey, "tls-private-key-file", "", "Path to matching x509 private key.")
	o.instrumentationOptions.AddFlags(flags)
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %v", err)
	}
	if len(o.cert) == 0 || len(o.privateKey) == 0 {
		return errors.New("Both --tls-cert-file and --tls-private-key-file are required for HTTPS")
	}
	return nil
}

func main() {
	logrusutil.ComponentInit()

	o := parseOptions()

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)
	health := pjutil.NewHealth()

	http.HandleFunc("/validate", handle)
	s := http.Server{
		Addr: ":8443",
		TLSConfig: &tls.Config{
			ClientAuth: tls.NoClientCert,
		},
	}

	health.ServeReady()
	interrupts.ListenAndServeTLS(&s, o.cert, o.privateKey, 5*time.Second)
}
