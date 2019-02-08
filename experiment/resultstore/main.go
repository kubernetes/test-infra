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

// Resultstore converts --build=gs://prefix/JOB/NUMBER from prow's pod-utils to a ResultStore invocation suite, which it optionally will --upload=gcp-project.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/testgrid/resultstore"
	"k8s.io/test-infra/testgrid/util/gcs"
)

var re = regexp.MustCompile(`( ?|^)\[[^]]+\]( |$)`)

// Converts "[k8s.io] hello world [foo]" into "hello world", []string{"k8s.io", "foo"}
func stripTags(str string) (string, []string) {
	tags := re.FindAllString(str, -1)
	for i, w := range tags {
		w = strings.TrimSpace(w)
		tags[i] = w[1 : len(w)-1]
	}
	var reals []string
	for _, p := range re.Split(str, -1) {
		if p == "" {
			continue
		}
		reals = append(reals, p)
	}
	return strings.Join(reals, " "), tags
}

type options struct {
	path    gcs.Path
	account string
	project string
	secret  string
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.Var(&o.path, "build", "The gs://bucket/to/job/build-1234/ url")
	flags.StringVar(&o.account, "service-account", "", "Authenticate with the service account at specified path")
	flags.StringVar(&o.project, "upload", "", "Upload results to specified gcp project instead of stdout")
	flags.StringVar(&o.secret, "secret", "", "Use the specified secret guid instead of randomly generating one.")
	flags.Parse(args)
	return nil
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		log.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func main() {
	if err := run(parseOptions()); err != nil {
		log.Fatalf("Failed: %v", err)
	}
}

func str(inv interface{}) string {
	buf, err := yaml.Marshal(inv)
	if err != nil {
		panic(err)
	}
	return string(buf)
}

func print(inv ...interface{}) {
	for _, i := range inv {
		fmt.Println(str(i))
	}
}

func trailingSlash(s string) string {
	if strings.HasSuffix(s, "/") {
		return s
	}
	return s + "/"
}

func run(opt options) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	result, err := download(ctx, opt)
	if err != nil {
		return fmt.Errorf("download %s: %v", opt.path, err)
	}

	inv, target, test := convert(
		opt.project,
		"hello again",
		opt.path,
		*result,
	)
	print(inv.To(), test.To())

	if opt.project == "" {
		return nil
	}

	conn, err := resultstore.Connect(ctx, opt.account)
	if err != nil {
		return fmt.Errorf("resultstore connection: %v", err)
	}
	rsClient := resultstore.NewClient(conn).WithContext(ctx)
	if opt.secret != "" {
		rsClient = rsClient.WithSecret(resultstore.Secret(opt.secret))
	} else {
		secret := resultstore.NewSecret()
		fmt.Println("Secret:", secret)
		rsClient = rsClient.WithSecret(secret)
	}

	targetID := test.Name
	const configID = resultstore.Default
	invName, err := rsClient.Invocations().Create(inv)
	if err != nil {
		return fmt.Errorf("create invocation: %v", err)
	}
	targetName, err := rsClient.Targets(invName).Create(targetID, target)
	if err != nil {
		fmt.Println(resultstore.URL(invName))
		return fmt.Errorf("create target: %v", err)
	}
	fmt.Println(resultstore.URL(targetName))
	_, err = rsClient.Configurations(invName).Create(configID)
	if err != nil {
		return fmt.Errorf("create configuration: %v", err)
	}
	ctName, err := rsClient.ConfiguredTargets(targetName, configID).Create()
	if err != nil {
		return fmt.Errorf("create configured target: %v", err)
	}
	_, err = rsClient.Actions(ctName).Create("primary", test)
	if err != nil {
		return fmt.Errorf("create action: %v", err)
	}
	return nil
}
