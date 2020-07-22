/*
Copyright 2020 The Kubernetes Authors.

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

package githubeventserver

import (
	"flag"
	"fmt"
	"strings"
)

// Options holds the endpoint and port information that can be used
// to create a new github event server
type Options struct {
	// Metrics will be used to expose prometheus metrics from the
	// github event server operations.
	Metrics *Metrics

	// endpoint is the main url path that the github event server will be served.
	endpoint string
	// port will be used to start an http server to listen to.
	port int
}

// DefaultAndValidate validates the option's values and defaults them if they are empty.
func (o *Options) DefaultAndValidate() error {
	if !strings.HasPrefix(o.endpoint, "/") {
		return fmt.Errorf("endpoint %s is not a valid url path", o.endpoint)
	}

	if o.Metrics == nil {
		o.Metrics = NewMetrics()
	}

	return nil
}

// Bind binds the flags into the given flagset.
func (o *Options) Bind(fs *flag.FlagSet) {
	fs.StringVar(&o.endpoint, "endpoint", "/hook", "The endpoint path where the http server will listen to")
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
}
