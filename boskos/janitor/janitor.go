/*
Copyright 2017 The Kubernetes Authors.

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
	"os/exec"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"k8s.io/test-infra/boskos/client"
)

var (
	clean = janitorClean
)

type semaphore chan bool

// acquire 1 resources
func (s semaphore) P() error {
	select {
	case s <- true:
	default:
		return errors.New("channel is full")
	}

	return nil
}

// release 1 resources
func (s semaphore) V() {
	<-s
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	boskos := client.NewClient("Janitor", "http://boskos")
	logrus.Info("Initialized boskos client!")
	sem := make(semaphore, 10)
	errChan := make(chan error)
	for range time.Tick(time.Minute * 10) {
		if err := update(boskos, sem, errChan); err != nil {
			logrus.WithError(err).Error("Update has error!")
		}
	}
}

// Clean by janitor script
func janitorClean(proj string) error {
	script := "../../jenkins/janitor.py"
	return exec.Command(fmt.Sprintf("%s --project=%s --hour=0", script, proj)).Run()
}

type boskosClient interface {
	Acquire(rtype string, state string, dest string) (string, error)
	ReleaseOne(name string, dest string) error
}

func janitor(c boskosClient, proj string, sem semaphore, errChan chan error) {
	if err := sem.P(); err != nil {
		errChan <- err
		// put it back to dirty

		if err := c.ReleaseOne(proj, "dirty"); err != nil {
			errChan <- err
		}
		return
	}

	dest := "free"
	if err := clean(proj); err != nil {
		errChan <- err
		dest = "dirty"
	}

	if err := c.ReleaseOne(proj, dest); err != nil {
		errChan <- err
	}

	sem.V()
}

func update(c boskosClient, sem semaphore, errChan chan error) error {
	// Try to acquire all dirty projects, until none are available.
	for {
		if proj, err := c.Acquire("project", "dirty", "cleaning"); err != nil {
			errChan <- err
		} else if proj == "" {
			break
		} else {
			go janitor(c, proj, sem, errChan)
		}
	}

	var errstrings []string
CheckError:
	for {
		select {
		case err := <-errChan:
			errstrings = append(errstrings, err.Error())
		default:
			break CheckError
		}
	}

	if len(errstrings) == 0 {
		return nil
	}

	return fmt.Errorf(strings.Join(errstrings, "\n"))
}
