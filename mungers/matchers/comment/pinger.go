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

package comment

import (
	"time"

	"github.com/google/go-github/github"
)

// Pinger checks if it's time to send a ping.
// You can build a pinger for a specific use-case and re-use it when you want.
type Pinger struct {
	keyword     string        // Short description for the ping
	description string        // Long description for the ping
	timePeriod  time.Duration // How often should we ping
	maxCount    int           // Will stop pinging after that many occurences
}

// NewPinger creates a new pinger. `keyword` is the name of the notification.
func NewPinger(keyword string) *Pinger {
	return &Pinger{
		keyword: keyword,
	}
}

// SetDescription is the description that goes along the ping
func (p *Pinger) SetDescription(description string) *Pinger {
	p.description = description

	return p
}

// SetTimePeriod is the time we wait between pings
func (p *Pinger) SetTimePeriod(timePeriod time.Duration) *Pinger {
	p.timePeriod = timePeriod

	return p
}

// SetMaxCount will make the pinger fail when it reaches maximum
func (p *Pinger) SetMaxCount(maxCount int) *Pinger {
	p.maxCount = maxCount

	return p
}

// PingNotification creates a new notification to ping `who`
func (p *Pinger) PingNotification(comments []*github.IssueComment, who string, startDate *time.Time) *Notification {
	if startDate == nil {
		startDate = &time.Time{}
	}

	pings := FilterComments(
		comments,
		And([]Matcher{
			CreatedAfter(*startDate),
			MungerNotificationName(p.keyword),
		}),
	)

	// We have pinged too many times, it's time to try something else
	if p.isMaxReached(pings) {
		return nil
	}

	if !p.shouldPingNow(pings, startDate) {
		return nil
	}

	return &Notification{
		Name:      p.keyword,
		Arguments: who,
		Context:   p.description,
	}
}

// IsMaxReached tells you if you've reached the limit yet
func (p *Pinger) IsMaxReached(comments []*github.IssueComment, startDate *time.Time) bool {
	if startDate == nil {
		startDate = &time.Time{}
	}
	return p.isMaxReached(FilterComments(
		comments,
		And([]Matcher{
			CreatedAfter(*startDate),
			MungerNotificationName(p.keyword),
		}),
	))
}

func (p *Pinger) isMaxReached(pings FilteredComments) bool {
	return p.maxCount != 0 && len(pings) >= p.maxCount
}

func (p *Pinger) shouldPingNow(pings FilteredComments, startDate *time.Time) bool {
	lastEvent := startDate

	if len(pings) != 0 {
		lastEvent = pings[len(pings)-1].CreatedAt
	}

	return time.Since(*lastEvent) > p.timePeriod
}
