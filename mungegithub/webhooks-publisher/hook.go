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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"

	"cloud.google.com/go/pubsub"
	"github.com/golang/glog"
	"github.com/google/go-github/github"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
	yaml "gopkg.in/yaml.v2"
)

type hookFlags struct {
	configFile string
	listenPort int
}

func (flags *hookFlags) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.configFile, "config", "", "Path to config file")
	cmd.Flags().IntVar(&flags.listenPort, "port", 8080, "Listen for webhooks on this port")
	cmd.Flags().AddGoFlagSet(flag.CommandLine)
}

func (flags *hookFlags) Verify() error {
	if flags.configFile == "" {
		return errors.New("No config file specified.")
	}

	return nil
}

type hookConfig struct {
	Project string
	Paths   map[string]struct {
		Secret string
		Topic  string
	}
}

// ParseHookConfig receives the config file and return the struct
func ParseHookConfig(configStream io.Reader) (*hookConfig, error) {
	configFile, err := ioutil.ReadAll(configStream)
	if err != nil {
		return nil, err
	}
	config := hookConfig{}

	err = yaml.Unmarshal(configFile, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

// Message is the format
type Message struct {
	Payload string
	Type    string
}

// Queue the message in the given topic
func (m *Message) Queue(topic *pubsub.Topic) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	msgIds, err := topic.Publish(context.Background(), &pubsub.Message{
		Data: data,
	})
	if err != nil {
		return err
	}

	glog.Infof("Published to topic (%s): %s", topic.String(), msgIds)

	return nil
}

// HookHandler receives webhook events
type HookHandler struct {
	Secret string
	Topic  *pubsub.Topic
}

// ReplyError send the error back to github
func ReplyError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-type", "text/plain")
	w.WriteHeader(http.StatusInternalServerError)
	w.Write([]byte("Failed to process webhook."))
}

// ServeHTTP receives the webhook event and process it
func (h HookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(h.Secret))
	if err != nil {
		ReplyError(w, err)
		glog.Error("Failed to validate event payload: ", err)
		return
	}

	msg := Message{
		Payload: string(payload),
		Type:    r.Header.Get("X-Github-Event"),
	}
	err = msg.Queue(h.Topic)
	if err != nil {
		ReplyError(w, err)
		glog.Errorf("Failed to push event (%s: %s): %s", msg.Type, payload, err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func runProgram(flags *hookFlags) error {
	if err := flags.Verify(); err != nil {
		return err
	}
	f, err := os.Open(flags.configFile)
	if err != nil {
		return err
	}
	config, err := ParseHookConfig(f)
	if err != nil {
		return err
	}

	glog.Infof("Connecting to pubsub (project-id: %s)", config.Project)
	client, err := pubsub.NewClient(context.Background(), config.Project)
	if err != nil {
		return err
	}

	for path, pathConfig := range config.Paths {
		topic := client.Topic(pathConfig.Topic)
		exists, err := topic.Exists(context.Background())
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("Topic doesn't exist: %s", pathConfig.Topic)
		}
		handler := HookHandler{
			Secret: pathConfig.Secret,
			Topic:  topic,
		}
		glog.Infof("Setting up handler for %s: push to %s", path, pathConfig.Secret, pathConfig.Topic)
		http.Handle(path, handler)
	}
	glog.Infof("Listening on port %d ...", flags.listenPort)
	return http.ListenAndServe(fmt.Sprintf(":%d", flags.listenPort), nil)
}

func main() {
	flags := &hookFlags{}
	cmd := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "Receives webhook and sends them to a PubSub queue",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runProgram(flags)
		},
	}

	flags.AddFlags(cmd)

	if err := cmd.Execute(); err != nil {
		glog.Errorf("%v\n", err)
		return
	}
}
