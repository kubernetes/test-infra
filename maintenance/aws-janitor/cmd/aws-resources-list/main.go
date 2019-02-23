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
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go/aws/session"
	"k8s.io/test-infra/maintenance/aws-janitor/account"
	"k8s.io/test-infra/maintenance/aws-janitor/regions"
	"k8s.io/test-infra/maintenance/aws-janitor/resources"
)

var (
	region = flag.String("region", regions.Default, "")
)

func main() {
	flag.Parse()
	resourceKinds := append(resources.RegionalTypeList, resources.GlobalTypeList...)

	session := session.Must(session.NewSession())
	acct, err := account.GetAccount(session, *region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error retrieving account: %v", err)
		os.Exit(1)
	}

	for _, r := range resourceKinds {
		set, err := r.ListAll(session, acct, *region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error listing %T: %v\n", r, err)
			continue
		}

		fmt.Printf("==%T==\n", r)

		for _, s := range set.GetARNs() {
			fmt.Println(s)
		}
	}
}
