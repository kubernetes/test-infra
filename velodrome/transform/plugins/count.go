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

package plugins

import (
	"github.com/spf13/cobra"
)

// NewCountPlugin counts events and number of issues in given state, and for how long.
func NewCountPlugin(runner func(Plugin) error) *cobra.Command {
	stateCounter := &StatePlugin{}
	eventCounter := &EventCounterPlugin{}
	commentsAsEvents := NewFakeCommentPluginWrapper(eventCounter)
	commentCounter := &CommentCounterPlugin{}
	authorLoggable := NewMultiplexerPluginWrapper(
		commentsAsEvents,
		commentCounter,
	)
	authorLogged := NewAuthorLoggerPluginWrapper(authorLoggable)
	fullMultiplex := NewMultiplexerPluginWrapper(authorLogged, stateCounter)

	fakeOpen := NewFakeOpenPluginWrapper(fullMultiplex)
	typeFilter := NewTypeFilterWrapperPlugin(fakeOpen)
	authorFilter := NewAuthorFilterPluginWrapper(typeFilter)

	cmd := &cobra.Command{
		Use:   "count",
		Short: "Count events and number of issues in given state, and for how long",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := eventCounter.CheckFlags(); err != nil {
				return err
			}
			if err := stateCounter.CheckFlags(); err != nil {
				return err
			}
			if err := typeFilter.CheckFlags(); err != nil {
				return err
			}
			if err := commentCounter.CheckFlags(); err != nil {
				return err
			}
			return runner(authorFilter)
		},
	}

	eventCounter.AddFlags(cmd)
	stateCounter.AddFlags(cmd)
	commentCounter.AddFlags(cmd)
	typeFilter.AddFlags(cmd)
	authorFilter.AddFlags(cmd)
	authorLogged.AddFlags(cmd)

	return cmd
}
