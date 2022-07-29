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

package common

import (
	"errors"
	"fmt"
	"io/ioutil"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

// ValidateConfig validates config with existing resources
// In: boskosConfig - a boskos config defining resources
// Out: nil on success, error on failure
func ValidateConfig(config *BoskosConfig) error {
	if len(config.Resources) == 0 {
		return errors.New("empty config")
	}
	resourceNames := map[string]bool{}
	resourceTypes := map[string]bool{}
	resourcesNeeds := map[string]int{}
	actualResources := map[string]int{}

	var errs []error
	for idx, e := range config.Resources {
		if e.Type == "" {
			errs = append(errs, fmt.Errorf(".%d.type: must be set", idx))
		}

		if resourceTypes[e.Type] {
			errs = append(errs, fmt.Errorf(".%d.type.%s already exists", idx, e.Type))
		}

		names := e.Names
		if e.IsDRLC() {
			// Dynamic Resource
			if e.MaxCount == 0 {
				errs = append(errs, fmt.Errorf(".%d.max-count: must be >0", idx))
			}
			if e.MinCount > e.MaxCount {
				errs = append(errs, fmt.Errorf(".%d.min-count: must be <= .%d.max-count", idx, idx))
			}
			for i := 0; i < e.MaxCount; i++ {
				name := GenerateDynamicResourceName()
				names = append(names, name)
			}

			// Updating resourceNeeds
			for k, v := range e.Needs {
				resourcesNeeds[k] += v * e.MaxCount
			}

		} else {
			if e.MinCount != 0 {
				errs = append(errs, fmt.Errorf(".%d.min-count must be unset when the names property is set", idx))
			}
			if e.MaxCount != 0 {
				errs = append(errs, fmt.Errorf(".%d.max-count must be unset when the names property is set", idx))
			}
		}
		actualResources[e.Type] += len(names)
		for nameIdx, name := range names {
			validationErrs := validation.IsDNS1123Subdomain(name)
			if len(validationErrs) != 0 {
				errs = append(errs, fmt.Errorf(".%d.names.%d(%s) is invalid: %v", idx, nameIdx, name, validationErrs))
			}

			if _, ok := resourceNames[name]; ok {
				errs = append(errs, fmt.Errorf(".%d.names.%d(%s) is a duplicate", idx, nameIdx, name))
				continue
			}
			resourceNames[name] = true
		}
	}

	for rType, needs := range resourcesNeeds {
		actual, ok := actualResources[rType]
		if !ok {
			errs = append(errs, fmt.Errorf("need for resource %s that does not exist", rType))
		}
		if needs > actual {
			errs = append(errs, fmt.Errorf("not enough resource of type %s for provisioning", rType))
		}
	}
	return utilerrors.NewAggregate(errs)
}

// ParseConfig reads in configPath and returns a list of resource objects
// on success.
func ParseConfig(configPath string) (*BoskosConfig, error) {
	file, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var data BoskosConfig
	if err := yaml.Unmarshal(file, &data); err != nil {
		return nil, err
	}
	return &data, nil
}
