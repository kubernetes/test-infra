/*
Copyright 2016 The Kubernetes Authors.

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
	"fmt"
	"io"

	"github.com/golang/protobuf/proto"
	"k8s.io/test-infra/testgrid/config"
	"sigs.k8s.io/yaml"
)

// Config includes config and defaults to apply on unspecified values.
type Config struct {
	config        *config.Configuration
	defaultConfig *config.DefaultConfiguration
}

// MissingFieldError is an error that includes the missing field.
type MissingFieldError struct {
	Field string
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("field missing or unset: %s", e.Field)
}

// ReconcileTestGroup sets unfilled currentTestGroup fields to the corresponding defaultTestGroup value.
func ReconcileTestGroup(currentTestGroup *config.TestGroup, defaultTestGroup *config.TestGroup) {
	if currentTestGroup.DaysOfResults == 0 {
		currentTestGroup.DaysOfResults = defaultTestGroup.DaysOfResults
	}

	if currentTestGroup.TestsNamePolicy == config.TestGroup_TESTS_NAME_MIN {
		currentTestGroup.TestsNamePolicy = defaultTestGroup.TestsNamePolicy
	}

	if currentTestGroup.IgnorePending == false {
		currentTestGroup.IgnorePending = defaultTestGroup.IgnorePending
	}

	if currentTestGroup.ColumnHeader == nil {
		currentTestGroup.ColumnHeader = defaultTestGroup.ColumnHeader
	}

	if currentTestGroup.NumColumnsRecent == 0 {
		currentTestGroup.NumColumnsRecent = defaultTestGroup.NumColumnsRecent
	}

	if currentTestGroup.AlertStaleResultsHours == 0 {
		currentTestGroup.AlertStaleResultsHours = defaultTestGroup.AlertStaleResultsHours
	}

	if currentTestGroup.NumFailuresToAlert == 0 {
		currentTestGroup.NumFailuresToAlert = defaultTestGroup.NumFailuresToAlert
	}
	if currentTestGroup.CodeSearchPath == "" {
		currentTestGroup.CodeSearchPath = defaultTestGroup.CodeSearchPath
	}
	if currentTestGroup.NumPassesToDisableAlert == 0 {
		currentTestGroup.NumPassesToDisableAlert = defaultTestGroup.NumPassesToDisableAlert
	}
	// is_external and user_kubernetes_client should always be true
	currentTestGroup.IsExternal = true
	currentTestGroup.UseKubernetesClient = true
}

// ReconcileDashboardTab sets unfilled currentTab fields to the corresponding defaultTab value.
func ReconcileDashboardTab(currentTab *config.DashboardTab, defaultTab *config.DashboardTab) {
	if currentTab.BugComponent == 0 {
		currentTab.BugComponent = defaultTab.BugComponent
	}

	if currentTab.CodeSearchPath == "" {
		currentTab.CodeSearchPath = defaultTab.CodeSearchPath
	}

	if currentTab.NumColumnsRecent == 0 {
		currentTab.NumColumnsRecent = defaultTab.NumColumnsRecent
	}

	if currentTab.OpenTestTemplate == nil {
		currentTab.OpenTestTemplate = defaultTab.OpenTestTemplate
	}

	if currentTab.FileBugTemplate == nil {
		currentTab.FileBugTemplate = defaultTab.FileBugTemplate
	}

	if currentTab.AttachBugTemplate == nil {
		currentTab.AttachBugTemplate = defaultTab.AttachBugTemplate
	}

	if currentTab.ResultsText == "" {
		currentTab.ResultsText = defaultTab.ResultsText
	}

	if currentTab.ResultsUrlTemplate == nil {
		currentTab.ResultsUrlTemplate = defaultTab.ResultsUrlTemplate
	}

	if currentTab.CodeSearchUrlTemplate == nil {
		currentTab.CodeSearchUrlTemplate = defaultTab.CodeSearchUrlTemplate
	}

	if currentTab.AlertOptions == nil {
		currentTab.AlertOptions = defaultTab.AlertOptions
	}
}

// updateDefaults reads any default configuration from yamlData and updates the
// defaultConfig in c.
//
// Returns an error if the defaultConfig remains unset.
func (c *Config) updateDefaults(yamlData []byte) error {
	newDefaults := &config.DefaultConfiguration{}
	err := yaml.Unmarshal(yamlData, newDefaults)
	if err != nil {
		return err
	}

	if c.defaultConfig == nil {
		c.defaultConfig = newDefaults
	} else {
		if newDefaults.DefaultTestGroup != nil {
			c.defaultConfig.DefaultTestGroup = newDefaults.DefaultTestGroup
		}
		if newDefaults.DefaultDashboardTab != nil {
			c.defaultConfig.DefaultDashboardTab = newDefaults.DefaultDashboardTab
		}
	}

	if c.defaultConfig.DefaultTestGroup == nil {
		return MissingFieldError{"DefaultTestGroup"}
	}
	if c.defaultConfig.DefaultDashboardTab == nil {
		return MissingFieldError{"DefaultDashboardTab"}
	}
	return nil
}

// Update reads the config in yamlData and updates the config in c.
// If yamlData does not contain any defaults, the defaults from a
// previous call to Update are used instead.
func (c *Config) Update(yamlData []byte) error {
	if err := c.updateDefaults(yamlData); err != nil {
		return err
	}

	curConfig := &config.Configuration{}
	if err := yaml.Unmarshal(yamlData, curConfig); err != nil {
		return err
	}

	if c.config == nil {
		c.config = &config.Configuration{}
	}

	for _, testgroup := range curConfig.TestGroups {
		ReconcileTestGroup(testgroup, c.defaultConfig.DefaultTestGroup)
		c.config.TestGroups = append(c.config.TestGroups, testgroup)
	}

	for _, dashboard := range curConfig.Dashboards {
		// validate dashboard tabs
		for _, dashboardtab := range dashboard.DashboardTab {
			ReconcileDashboardTab(dashboardtab, c.defaultConfig.DefaultDashboardTab)
		}
		c.config.Dashboards = append(c.config.Dashboards, dashboard)
	}

	for _, dashboardGroup := range curConfig.DashboardGroups {
		c.config.DashboardGroups = append(c.config.DashboardGroups, dashboardGroup)
	}

	return nil
}

// validate checks that a configuration is well-formed, having test groups and dashboards set.
func (c *Config) validate() error {
	if c.config == nil {
		return errors.New("Configuration unset")
	}
	if len(c.config.TestGroups) == 0 {
		return MissingFieldError{"TestGroups"}
	}
	if len(c.config.Dashboards) == 0 {
		return MissingFieldError{"Dashboards"}
	}

	return nil
}

// MarshalText writes a text version of the parsed configuration to the supplied io.Writer.
// Returns an error if config is invalid or writing failed.
func (c *Config) MarshalText(w io.Writer) error {
	if err := c.validate(); err != nil {
		return err
	}
	return proto.MarshalText(w, c.config)
}

// MarshalBytes returns the wire-encoded protobuf data for the parsed configuration.
// Returns an error if config is invalid or encoding failed.
func (c *Config) MarshalBytes() ([]byte, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return proto.Marshal(c.config)
}

// Raw returns the raw protocol buffer for the parsed configuration after validation.
// Returns an error if validation fails.
func (c *Config) Raw() (*config.Configuration, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	return c.config, nil
}
