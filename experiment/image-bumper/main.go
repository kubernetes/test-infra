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
	"flag"
	"log"
	"regexp"
	"strings"

	"k8s.io/test-infra/experiment/image-bumper/bumper"
)

type options struct {
	imageRegex string
	files      []string
}

func parseOptions() options {
	var o options
	flag.StringVar(&o.imageRegex, "image-regex", "", "Only touch images matching this regex")
	flag.Parse()
	o.files = flag.Args()
	return o
}

func main() {
	o := parseOptions()
	var imageRegex *regexp.Regexp
	if o.imageRegex != "" {
		var err error
		imageRegex, err = regexp.Compile(o.imageRegex)
		if err != nil {
			log.Fatalf("Failed to parse image-regex: %v\n", err)
		}
	}

	cli := bumper.NewClient()
	for _, f := range o.files {
		if err := cli.UpdateFile(cli.FindLatestTag, f, imageRegex); err != nil {
			log.Printf("Failed to update %s: %v", f, err)
		}
	}
	log.Println("Done.")
	for before, after := range cli.GetReplacements() {
		if strings.Split(before, ":")[1] == after {
			continue
		}
		log.Printf("%s -> %s\n", before, after)
	}
}
