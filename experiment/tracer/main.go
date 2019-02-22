/*
Copyright 2018 The Kubernetes Authors.

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
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/NYTimes/gziphandler"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
)

type options struct {
	selector  string
	namespace string

	dryRun   bool
	headless bool

	kubernetes prowflagutil.ExperimentalKubernetesOptions
}

func (o *options) Validate() error {
	if err := o.kubernetes.Validate(o.dryRun); err != nil {
		return err
	}

	if o.selector == "" {
		return fmt.Errorf("you need to specify a label selector")
	}
	if _, err := labels.Parse(o.selector); err != nil {
		return err
	}
	if o.namespace == "" {
		return fmt.Errorf("you need to specify a namespace")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.selector, "label-selector", "", "Label selector to select prow pods for log tracing. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.StringVar(&o.namespace, "namespace", "", "Namespace where prow runs")

	fs.BoolVar(&o.headless, "headless", false, "Run on headless mode")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	o.kubernetes.AddFlags(fs)

	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}
	logrus.SetFormatter(logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "tracer"}))

	client, err := o.kubernetes.InfrastructureClusterClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
	}

	mux := http.NewServeMux()
	if !o.headless {
		mux.Handle("/", gziphandler.GzipHandler(http.FileServer(http.Dir("/static"))))
	}
	mux.Handle("/trace", gziphandler.GzipHandler(handleTrace(o.selector, client.CoreV1().Pods(o.namespace))))

	server := &http.Server{
		Addr:         ":8080",
		Handler:      mux,
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
	}
	logrus.Fatal(server.ListenAndServe())
}
