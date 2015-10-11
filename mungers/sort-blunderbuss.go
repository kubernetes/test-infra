/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package mungers

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/kubernetes/pkg/util/yaml"
)

var (
	_ = fmt.Print
)

func (b *BlunderbussMunger) doNormalizeBlunderbuss() error {
	file, err := os.Open(b.blunderbussConfigFile)
	if err != nil {
		glog.Fatalf("Failed to load blunderbuss config: %v", err)
	}
	defer file.Close()

	b.config = &BlunderbussConfig{}
	if err := yaml.NewYAMLToJSONDecoder(file).Decode(b.config); err != nil {
		glog.Fatalf("Failed to load blunderbuss config: %v", err)
	}
	glog.V(4).Infof("Loaded config from %s", b.blunderbussConfigFile)

	out := new(bytes.Buffer)

	var paths []string
	for k := range b.config.PrefixMap {
		paths = append(paths, k)
	}
	sort.Strings(paths)

	for i, p := range paths {
		if i != 0 {
			out.WriteString("\n")
		} else {
			out.WriteString("prefixMap:\n")
		}
		out.WriteString(fmt.Sprintf("  %s:\n", p))
		var names []string
		for _, n := range b.config.PrefixMap[p] {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			out.WriteString(fmt.Sprintf("    - %s\n", n))
		}
	}

	f, err := os.Create(b.blunderbussConfigFile)
	if err != nil {
		glog.Fatalf("unable to open file for write: %v", err)
	}
	defer f.Close()

	f.Write(out.Bytes())
	return nil
}

func (b *BlunderbussMunger) addBlunderbussCommand(root *cobra.Command) {
	normalizeBlunderbuss := &cobra.Command{
		Use:   "blunderbuss-normalize",
		Short: "alphabetize the blunderbuss config file",
		RunE: func(_ *cobra.Command, _ []string) error {
			return b.doNormalizeBlunderbuss()
		},
	}
	root.AddCommand(normalizeBlunderbuss)
}
