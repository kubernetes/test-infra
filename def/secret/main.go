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

// secret will write a secret to --output from --data=name=/path/to/source
package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"k8s.io/api/core/v1"
	"sigs.k8s.io/yaml"
)

type options struct {
	data      multiKeyValue
	labels    multiKeyValue
	name      string
	namespace string
	output    string
}

// multiKeyValue allows --key=value --key=value2
type multiKeyValue map[string]string

func (mkv *multiKeyValue) String() string {
	var b bytes.Buffer
	if mkv == nil {
		return ""
	}
	for k, v := range *mkv {
		if b.Len() > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%s=%s", k, v)
	}
	return b.String()
}

func (mkv *multiKeyValue) Set(v string) error {
	p := strings.SplitN(v, "=", 2)
	if len(p) != 2 {
		return fmt.Errorf("%s does not match label=value", v)
	}
	if mkv == nil {
		mkv = &multiKeyValue{
			p[0]: p[1],
		}
	} else {
		(*mkv)[p[0]] = p[1]
	}
	return nil
}

func flags() *options {
	opt := options{
		data:   multiKeyValue{},
		labels: multiKeyValue{},
	}
	flag.StringVar(&opt.output, "output", "", "Write secret here instead of stdout")
	flag.StringVar(&opt.name, "name", "", "Name of resource")
	flag.StringVar(&opt.namespace, "namespace", "", "Namespace for resource")
	flag.Var(&opt.labels, "label", "Add a key=value label (repeat flag)")
	flag.Var(&opt.data, "data", "Add a key=/path/to/file configmap source (repeat flag)")
	flag.Parse()
	return &opt
}

func buildSecret(name, namespace string, labels map[string]string, data map[string]string) (*v1.Secret, error) {

	var s v1.Secret
	s.TypeMeta.Kind = "Secret"
	s.TypeMeta.APIVersion = "v1"
	s.ObjectMeta.Name = name
	s.ObjectMeta.Namespace = namespace
	s.ObjectMeta.Labels = labels
	if len(data) > 0 {
		s.Data = map[string][]byte{}
		for key, value := range data {
			buf, err := ioutil.ReadFile(value)
			if err != nil {
				wd, _ := os.Getwd()
				return nil, fmt.Errorf("could not read %s/%s: %v", wd, value, err)
			}
			//hongkailiu: did not make this work, so string as unnecessary intermediate step
			//dst := make([]byte, base64.StdEncoding.EncodedLen(len(buf)))
			//l, err := base64.StdEncoding.Decode(dst, buf)
			s.Data[key] = []byte(base64.StdEncoding.EncodeToString(buf))
		}
	}
	return &s, nil
}

func main() {
	opt := flags()
	if opt.name == "" {
		log.Fatal("Non-empty --name required")
	}
	s, err := buildSecret(opt.name, opt.namespace, opt.labels, opt.data)
	if err != nil {
		log.Fatalf("Failed to create %s: %v", opt.name, err)
	}
	buf, err := yaml.Marshal(s)
	if err != nil {
		log.Fatalf("Failed to serialize %s: %v", opt.name, err)
	}
	if opt.output == "" {
		fmt.Print(string(buf))
		return
	}
	err = ioutil.WriteFile(opt.output, buf, 0644)
	if err != nil {
		log.Fatalf("Failed to write %s: %v", opt.output, err)
	}
}
