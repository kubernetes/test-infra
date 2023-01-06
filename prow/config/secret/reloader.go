/*
Copyright 2022 The Kubernetes Authors.

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

package secret

import (
	"os"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type parsingSecretReloader[T any] struct {
	lock      sync.RWMutex
	path      string
	rawValue  []byte
	parsed    T
	parsingFN func([]byte) (T, error)
}

func (p *parsingSecretReloader[T]) start(reloadCensor func()) error {
	raw, parsed, err := loadSingleSecretWithParser(p.path, p.parsingFN)
	if err != nil {
		return err
	}
	p.lock.Lock()
	p.rawValue = raw
	p.parsed = parsed
	p.lock.Unlock()
	reloadCensor()

	go p.reloadSecret(reloadCensor)
	return nil
}

func (p *parsingSecretReloader[T]) reloadSecret(reloadCensor func()) {
	var lastModTime time.Time
	logger := logrus.NewEntry(logrus.StandardLogger())

	skips := 0
	for range time.Tick(1 * time.Second) {
		if skips < 600 {
			// Check if the file changed to see if it needs to be re-read.
			secretStat, err := os.Stat(p.path)
			if err != nil {
				logger.WithField("secret-path", p.path).WithError(err).Error("Error loading secret file.")
				continue
			}

			recentModTime := secretStat.ModTime()
			if !recentModTime.After(lastModTime) {
				skips++
				continue // file hasn't been modified
			}
			lastModTime = recentModTime
		}

		raw, parsed, err := loadSingleSecretWithParser(p.path, p.parsingFN)
		if err != nil {
			logger.WithField("secret-path", p.path).WithError(err).Error("Error loading secret.")
			continue
		}

		p.lock.Lock()
		p.rawValue = raw
		p.parsed = parsed
		p.lock.Unlock()
		reloadCensor()

		skips = 0
	}

}

func (p *parsingSecretReloader[T]) getRaw() []byte {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.rawValue
}

func (p *parsingSecretReloader[T]) get() T {
	p.lock.RLock()
	defer p.lock.RUnlock()
	return p.parsed
}
