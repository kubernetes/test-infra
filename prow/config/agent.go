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

package config

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/fsnotify.v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/interrupts"
)

// Delta represents the before and after states of a Config change detected by the Agent.
type Delta struct {
	Before, After Config
}

// DeltaChan is a channel to receive config delta events when config changes.
type DeltaChan = chan<- Delta

// Agent watches a path and automatically loads the config stored
// therein.
type Agent struct {
	mut           sync.RWMutex // do not export Lock, etc methods
	c             *Config
	subscriptions []DeltaChan
}

// IsConfigMapMount determines whether the provided directory is a configmap mounted directory
func IsConfigMapMount(path string) (bool, error) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return false, fmt.Errorf("Could not read provided directory %s: %v", path, err)
	}
	for _, file := range files {
		if file.Name() == "..data" {
			return true, nil
		}
	}
	return false, nil
}

// GetCMMountWatcher returns a function that watches a configmap mounted directory and runs the provided "eventFunc" every time
// the directory gets updated and the provided "errFunc" every time it encounters an error.
// Example of a possible eventFunc:
// func() error {
//		value, err := RunUpdate()
//		if err != nil {
//			return err
//		}
//		globalValue = value
//		return nil
// }
// Example of errFunc:
// func(err error, msg string) {
//		logrus.WithError(err).Error(msg)
// }
func GetCMMountWatcher(eventFunc func() error, errFunc func(error, string), path string) (func(ctx context.Context), error) {
	isCMMount, err := IsConfigMapMount(path)
	if err != nil {
		return nil, err
	} else if !isCMMount {
		return nil, fmt.Errorf("Provided directory %s is not a configmap directory", path)
	}
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	err = w.Add(path)
	if err != nil {
		return nil, err
	}
	logrus.Debugf("Watching %s", path)
	dataPath := filepath.Join(path, "..data")
	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				if err := w.Close(); err != nil {
					errFunc(err, fmt.Sprintf("failed to close fsnotify watcher for directory %s", path))
				}
				return
			case event := <-w.Events:
				if event.Name == dataPath && event.Op == fsnotify.Create {
					err := eventFunc()
					if err != nil {
						errFunc(err, fmt.Sprintf("event function for watch directory %s failed", path))
					}
				}
			case err := <-w.Errors:
				errFunc(err, fmt.Sprintf("received fsnotify error for directory %s", path))
			}
		}
	}, nil
}

// GetFileWatcher returns a function that watches the specified file(s), running the "eventFunc" whenever an event for the file(s) occurs
// and the "errFunc" whenever an error is encountered. In this function, the eventFunc has access to the watcher, allowing the eventFunc
// to add new files/directories to be watched as needed.
// Example of a possible eventFunc:
// func(w *fsnotify.Watcher) error {
//		value, err := RunUpdate()
//		if err != nil {
//			return err
//		}
//		globalValue = value
//      newFiles := getNewFiles()
//      for _, file := range newFiles {
//			if err := w.Add(file); err != nil {
//				return err
//			}
// 		}
//		return nil
// }
// Example of errFunc:
// func(err error, msg string) {
//		logrus.WithError(err).Error(msg)
// }
func GetFileWatcher(eventFunc func(*fsnotify.Watcher) error, errFunc func(error, string), files ...string) (func(ctx context.Context), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if err := w.Add(file); err != nil {
			return nil, err
		}
	}
	logrus.Debugf("Watching files: %v", files)
	return func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				if err := w.Close(); err != nil {
					errFunc(err, fmt.Sprintf("failed to close fsnotify watcher for files: %v", files))
				}
				return
			case <-w.Events:
				err := eventFunc(w)
				if err != nil {
					errFunc(err, fmt.Sprintf("event function failed watching files: %v", files))
				}
			case err := <-w.Errors:
				errFunc(err, fmt.Sprintf("received fsnotify error watching files: %v", files))
			}
		}
	}, nil
}

// ListCMsAndDirs returns a 2 sets of strings containing the paths of configmapped directories and standard
// directories respectively starting from the provided path. This can be used to watch a large number of
// files, some of which may be populated via configmaps
func ListCMsAndDirs(path string) (cms sets.String, dirs sets.String, err error) {
	cms = sets.NewString()
	dirs = sets.NewString()
	err = filepath.Walk(path, func(path string, info os.FileInfo, _ error) error {
		// We only need to watch directories as creation, deletion, and writes
		// for files in a directory trigger events for the directory
		if info != nil && info.IsDir() {
			if isCM, err := IsConfigMapMount(path); err != nil {
				return fmt.Errorf("Failed to check is path %s is configmap mounted: %v", path, err)
			} else if isCM {
				cms.Insert(path)
				// configmaps can't have nested directories
				return filepath.SkipDir
			} else {
				dirs.Insert(path)
				return nil
			}
		}
		return nil
	})
	return cms, dirs, err
}

func watchConfigs(ca *Agent, prowConfig, jobConfig string, supplementalProwConfigDirs []string, supplementalProwConfigsFileName string, additionals ...func(*Config) error) error {
	cmEventFunc := func() error {
		c, err := Load(prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileName, additionals...)
		if err != nil {
			return err
		}
		ca.Set(c)
		return nil
	}
	// We may need to add more directories to be watched
	dirsEventFunc := func(w *fsnotify.Watcher) error {
		c, err := Load(prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileName, additionals...)
		if err != nil {
			return err
		}
		ca.Set(c)
		// TODO(AlexNPavel): Is there a chance that a ConfigMap mounted directory may appear without making a new pod? If yes, handle that.
		_, dirs, err := ListCMsAndDirs(jobConfig)
		if err != nil {
			return err
		}
		for _, supplementalProwConfigDir := range supplementalProwConfigDirs {
			_, additionalDirs, err := ListCMsAndDirs(supplementalProwConfigDir)
			if err != nil {
				return err
			}
			dirs.Insert(additionalDirs.UnsortedList()...)
		}
		for dir := range dirs {
			// Adding a file or directory that already exists in fsnotify is a no-op, so it is safe to always run Add
			if err := w.Add(dir); err != nil {
				return err
			}
		}
		return nil
	}
	errFunc := func(err error, msg string) {
		logrus.WithField("prowConfig", prowConfig).
			WithField("jobConfig", jobConfig).
			WithError(err).Error(msg)
	}
	cms := sets.NewString()
	dirs := sets.NewString()
	// TODO(AlexNPavel): allow empty jobConfig till fully migrate config to subdirs
	if jobConfig != "" {
		stat, err := os.Stat(jobConfig)
		if err != nil {
			return err
		}
		// TODO(AlexNPavel): allow single file jobConfig till fully migrate config to subdirs
		if stat.IsDir() {
			var err error
			// jobConfig points to directories of configs that may be nested
			cms, dirs, err = ListCMsAndDirs(jobConfig)
			if err != nil {
				return err
			}
		} else {
			// If jobConfig is a single file, we handle it identically to how prowConfig is handled
			if jobIsCMMounted, err := IsConfigMapMount(filepath.Dir(jobConfig)); err != nil {
				return err
			} else if jobIsCMMounted {
				cms.Insert(filepath.Dir(jobConfig))
			} else {
				dirs.Insert(jobConfig)
			}
		}
	}
	// The prow config is always a single file
	if prowIsCMMounted, err := IsConfigMapMount(filepath.Dir(prowConfig)); err != nil {
		return err
	} else if prowIsCMMounted {
		cms.Insert(filepath.Dir(prowConfig))
	} else {
		dirs.Insert(prowConfig)
	}
	var runFuncs []func(context.Context)
	for cm := range cms {
		runFunc, err := GetCMMountWatcher(cmEventFunc, errFunc, cm)
		if err != nil {
			return err
		}
		runFuncs = append(runFuncs, runFunc)
	}
	if len(dirs) > 0 {
		runFunc, err := GetFileWatcher(dirsEventFunc, errFunc, dirs.UnsortedList()...)
		if err != nil {
			return err
		}
		runFuncs = append(runFuncs, runFunc)
	}
	for _, runFunc := range runFuncs {
		interrupts.Run(runFunc)
	}
	return nil
}

// StartWatch will begin watching the config files at the provided paths. If the
// first load fails, Start will return the error and abort. Future load failures
// will log the failure message but continue attempting to load.
// This function will replace Start in a future release.
func (ca *Agent) StartWatch(prowConfig, jobConfig string, supplementalProwConfigDirs []string, supplementalProwConfigsFileName string, additionals ...func(*Config) error) error {
	c, err := Load(prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileName, additionals...)
	if err != nil {
		return err
	}
	ca.Set(c)
	watchConfigs(ca, prowConfig, jobConfig, supplementalProwConfigDirs, supplementalProwConfigsFileName, additionals...)
	return nil
}

func lastConfigModTime(prowConfig, jobConfig string) (time.Time, error) {
	// Check if the file changed to see if it needs to be re-read.
	// os.Stat follows symbolic links, which is how ConfigMaps work.
	prowStat, err := os.Stat(prowConfig)
	if err != nil {
		logrus.WithField("prowConfig", prowConfig).WithError(err).Error("Error loading prow config.")
		return time.Time{}, err
	}
	recentModTime := prowStat.ModTime()
	// TODO(krzyzacy): allow empty jobConfig till fully migrate config to subdirs
	if jobConfig != "" {
		jobConfigStat, err := os.Stat(jobConfig)
		if err != nil {
			logrus.WithField("jobConfig", jobConfig).WithError(err).Error("Error loading job configs.")
			return time.Time{}, err
		}

		if jobConfigStat.ModTime().After(recentModTime) {
			recentModTime = jobConfigStat.ModTime()
		}
	}
	return recentModTime, nil
}

// Start will begin polling the config file at the path. If the first load
// fails, Start will return the error and abort. Future load failures will log
// the failure message but continue attempting to load.
func (ca *Agent) Start(prowConfig, jobConfig string, additionalProwConfigDirs []string, supplementalProwConfigsFileName string, additionals ...func(*Config) error) error {
	lastModTime, err := lastConfigModTime(prowConfig, jobConfig)
	if err != nil {
		lastModTime = time.Time{}
	}
	c, err := Load(prowConfig, jobConfig, additionalProwConfigDirs, supplementalProwConfigsFileName, additionals...)
	if err != nil {
		return err
	}
	ca.Set(c)
	go func() {
		// Rarely, if two changes happen in the same second, mtime will
		// be the same for the second change, and an mtime-based check would
		// fail. Reload periodically just in case.
		skips := 0
		for range time.Tick(1 * time.Second) {
			if skips < 600 {
				recentModTime, err := lastConfigModTime(prowConfig, jobConfig)
				if err != nil {
					continue
				}
				if !recentModTime.After(lastModTime) {
					skips++
					continue // file hasn't been modified
				}
				lastModTime = recentModTime
			}
			if c, err := Load(prowConfig, jobConfig, additionalProwConfigDirs, supplementalProwConfigsFileName, additionals...); err != nil {
				logrus.WithField("prowConfig", prowConfig).
					WithField("jobConfig", jobConfig).
					WithError(err).Error("Error loading config.")
			} else {
				skips = 0
				ca.Set(c)
			}
		}
	}()
	return nil
}

// Subscribe registers the channel for messages on config reload.
// The caller can expect a copy of the previous and current config
// to be sent down the subscribed channel when a new configuration
// is loaded.
func (ca *Agent) Subscribe(subscription DeltaChan) {
	ca.mut.Lock()
	defer ca.mut.Unlock()
	ca.subscriptions = append(ca.subscriptions, subscription)
}

// Getter returns the current Config in a thread-safe manner.
type Getter func() *Config

// Config returns the latest config. Do not modify the config.
func (ca *Agent) Config() *Config {
	ca.mut.RLock()
	defer ca.mut.RUnlock()
	return ca.c
}

// Set sets the config. Useful for testing.
// Also used by statusreconciler to load last known config
func (ca *Agent) Set(c *Config) {
	ca.mut.Lock()
	defer ca.mut.Unlock()
	var oldConfig Config
	if ca.c != nil {
		oldConfig = *ca.c
	}
	delta := Delta{oldConfig, *c}
	ca.c = c
	for _, subscription := range ca.subscriptions {
		go func(sub DeltaChan) { // wait a minute to send each event
			end := time.NewTimer(time.Minute)
			select {
			case sub <- delta:
			case <-end.C:
			}
			if !end.Stop() { // prevent new events
				<-end.C // drain the pending event
			}
		}(subscription)
	}
}
