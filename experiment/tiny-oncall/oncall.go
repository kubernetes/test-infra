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
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	"sigs.k8s.io/yaml"
)

type config struct {
	Oncallers           []string `json:"oncallers"`
	Shadows             []string `json:"shadows"`
	ChangeDay           int      `json:"change_day"`
	ChangeHour          int      `json:"change_hour"`
	GenerateSecondaries bool     `json:"generate_secondaries"`
}

type legacyOncall struct {
	TestInfra string `json:"testinfra"`
}

type oncall struct {
	Primary   string   `json:"primary"`
	Secondary string   `json:"secondary,omitempty"`
	Shadows   []string `json:"shadows"`
	Next      *oncall  `json:"next,omitempty"`
}

type currentOncall struct {
	// legacy version
	LegacyOncall legacyOncall `json:"Oncall"`
	Oncall       oncall       `json:"oncall"`
}

func pickOncallers(c config) (oncall, error) {
	if len(c.Oncallers) == 0 {
		return oncall{}, errors.New("at least one oncaller is required")
	}
	t := time.Now().UTC().AddDate(0, 0, -(c.ChangeDay - 1)).Add(time.Duration(-c.ChangeHour) * time.Hour)
	_, week := t.ISOWeek()
	l := len(c.Oncallers)
	if c.Shadows == nil {
		c.Shadows = []string{}
	}
	o := oncall{
		Primary:   c.Oncallers[week%l],
		Secondary: c.Oncallers[(week+1)%l],
		Shadows:   c.Shadows,
		Next: &oncall{
			Primary:   c.Oncallers[(week+1)%l],
			Secondary: c.Oncallers[(week+2)%l],
			Shadows:   c.Shadows,
		},
	}
	if !c.GenerateSecondaries || o.Primary == o.Secondary {
		o.Secondary = ""
	}
	if !c.GenerateSecondaries || o.Next.Primary == o.Next.Secondary {
		o.Next.Secondary = ""
	}
	return o, nil
}

func generateOncall(o oncall) ([]byte, error) {
	c := currentOncall{
		Oncall: o,
		LegacyOncall: legacyOncall{
			TestInfra: o.Primary,
		},
	}
	return json.Marshal(c)
}

func parseConfig(path string) (config, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	c := config{}
	if err := yaml.UnmarshalStrict(b, &c); err != nil {
		return config{}, err
	}
	return c, nil
}
