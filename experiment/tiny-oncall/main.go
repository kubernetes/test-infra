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
	"context"
	"errors"
	"flag"
	"fmt"
	"log"

	"k8s.io/test-infra/pkg/io"
)

type options struct {
	configPath string
	output     string
	creds      string
}

func gatherOptions() (options, error) {
	o := options{}
	flag.StringVar(&o.configPath, "config", "", "Path to the oncall config")
	flag.StringVar(&o.creds, "gcp-service-account", "", "Optionally, path to the GCP service account credentials")
	flag.StringVar(&o.output, "output", "-", "Path to the output. stdout if not specified")
	flag.Parse()
	if o.configPath == "" {
		return o, errors.New("--config is mandatory")
	}
	return o, nil
}

func run(o options) error {
	c, err := parseConfig(o.configPath)
	if err != nil {
		return fmt.Errorf("couldn't parse config: %v", err)
	}
	oncall, err := pickOncallers(c)
	if err != nil {
		return fmt.Errorf("couldn't pick oncallers: %v", err)
	}

	output, err := generateOncall(oncall)
	if err != nil {
		return fmt.Errorf("couldn't generate oncall file: %v", err)
	}

	if o.output == "-" || o.output == "" {
		fmt.Println(string(output))
		return nil
	}

	opener, err := io.NewOpener(context.Background(), o.creds)
	if err != nil {
		return fmt.Errorf("couldn't create opener: %v", err)
	}
	writer, err := opener.Writer(context.Background(), o.output)
	if err != nil {
		return fmt.Errorf("couldn't create writer: %v", err)
	}
	if _, err := writer.Write(output); err != nil {
		return fmt.Errorf("couldn't write output file: %v", err)
	}
	writer.Close()
	return nil
}

func main() {
	o, err := gatherOptions()
	if err != nil {
		log.Fatalf("Bad flags: %v.\n", err)
	}
	if err := run(o); err != nil {
		log.Fatalf("tiny-oncall: %v.\n", err)
	}
}
