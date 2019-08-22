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
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/flagutil"
)

var re = regexp.MustCompile(`^([^@]+)@(.+)\.iam\.gserviceaccount\.com$`)

// ensureGloud ensures gcloud on path or prints a note of how to install.
func ensureGcloud() error {
	const binary = "gcloud"
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("%s: %s", binary, "https://cloud.google.com/sdk/gcloud")
	}
	return nil
}

type options struct {
	project               string
	serviceAccount        string
	addRoles              flagutil.Strings
	removeRoles           flagutil.Strings
	adds                  sets.String
	removes               sets.String
	serviceAccountPrefix  string
	serviceAccountProject string
}

func (o options) validate() error {
	if o.project == "" {
		return errors.New("empty --project")
	}
	if o.serviceAccount == "" {
		return errors.New("empty --service-account")
	}
	adds := o.addRoles.Strings()
	removes := o.removeRoles.Strings()
	if len(adds)+len(removes) == 0 {
		return errors.New("--add or --remove required")
	}

	o.adds = sets.NewString(adds...)
	o.removes = sets.NewString(removes...)
	if both := o.adds.Intersection(o.removes); len(both) > 0 {
		return fmt.Errorf("Cannot both add and remove roles: %v", both.List())
	}
	mat := re.FindStringSubmatch(o.serviceAccount)
	if mat != nil {
		o.serviceAccountPrefix = mat[1]
		o.serviceAccountProject = mat[2]
	}
	return nil
}

func addFlags(fs *flag.FlagSet) *options {
	var o options
	fs.StringVar(&o.project, "project", "", "GCP project to change roles on")
	fs.StringVar(&o.serviceAccount, "service-account", "", "Service account member to change")
	fs.Var(&o.addRoles, "add", "Append to the list of roles to add")
	fs.Var(&o.removeRoles, "remove", "Append to the list of roles to remove")
	return &o
}

// gcloud iam service-accounts create erick-test --project=fejta-prod
func create(project, prefix string) error {
	create := exec.Command("gcloud", "iam", "service-accounts", "create", "-f", "--project="+project, prefix)
	create.Stderr = os.Stderr
	if err := create.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	return create.Wait()
}

// gcloud iam service-accounts describe erick-test2@fejta-prod.iam.gserviceaccount.com --project=fejta-prod
func describe(user string) error {
	desc := exec.Command("gcloud", "iam", "service-accounts", "describe", user)
	desc.Stderr = os.Stderr
	if err := desc.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	return desc.Wait()
}

// gcloud projects fejta-prod add-iam-policy-binding --member=serviceAccount:erick-test2@fejta-prod.iam.gserviceaccount.com --role=ROLE
func addPolicy(project, member, role string) error {
	add := exec.Command("gcloud", "projects", project, "add-iam-policy-binding", "--member="+member, "--role="+role)
	add.Stderr = os.Stderr
	if err := add.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	return add.Wait()
}

// gcloud projects fejta-prod remove-iam-policy-binding --member=serviceAccount:erick-test2@fejta-prod.iam.gserviceaccount.com --role=ROLE
func removePolicy(project, member, role string) error {
	remove := exec.Command("gcloud", "projects", project, "remove-iam-policy-binding", "--member="+member, "--role="+role)
	remove.Stderr = os.Stderr
	if err := remove.Start(); err != nil {
		return fmt.Errorf("start: %v", err)
	}
	return remove.Wait()
}

func main() {
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	opt := addFlags(fs)
	fs.Parse(os.Args[1:])
	if err := opt.validate(); err != nil {
		logrus.WithError(err).Fatal("Bad flags")
	}
	if err := run(*opt); err != nil {
		logrus.WithError(err).Fatal("Failed")
	}
}

func run(o options) error {
	if err := ensureGcloud(); err != nil {
		fmt.Println("gcloud is required, please install:")
		fmt.Println("  *", err)
		return errors.New("missing gcloud")
	}

	user := o.serviceAccount
	if err := describe(user); err != nil {
		if o.serviceAccountProject == "" {
			logrus.WithField("serviceAccount", user).Warn("Cannot parse prefix and project from service account")
			return fmt.Errorf("validate account pre-existence: %v", err)
		}
		if cerr := create(o.serviceAccountProject, o.serviceAccountPrefix); err != nil {
			return fmt.Errorf("create account: %v", cerr)
		}
	}
	if err := describe(user); err != nil {
		return fmt.Errorf("validate account: %v", err)
	}

	member := "serviceAccount:" + user
	project := o.project

	var addErrors []error
	var removeErrors []error
	for role := range o.adds {
		if err := addPolicy(project, member, role); err != nil {
			logrus.WithFields(logrus.Fields{
				"project": project,
				"member":  member,
				"role":    role,
			}).Warn("Could not add policy")
			addErrors = append(addErrors, err)
		}
	}

	if n := len(addErrors); n > 0 {
		return fmt.Errorf("%d add errors: %v", n, addErrors)
	}

	for role := range o.removes {
		if err := removePolicy(project, member, role); err != nil {
			logrus.WithFields(logrus.Fields{
				"project": project,
				"member":  member,
				"role":    role,
			}).Warn("Could not remove policy")
			removeErrors = append(removeErrors, err)
		}
	}
	if n := len(removeErrors); n > 0 {
		return fmt.Errorf("%d remove errors: %v", n, removeErrors)
	}
	return nil

}
