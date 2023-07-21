package internal

import (
	"sync"
	"time"
)

type Stage struct {
	stage int
	started bool
	startedAt time.Time
	mut sync.RWMutex
}

const (
	IDLE int = iota
	PROCESSING
	DELIVERING
	WAITING
	SUBMERGING
	UPDATING
	MERGING
	DONE
)

func CreateStage(stage int) Stage {
	return Stage{
		stage: stage,
		started: false,
		startedAt: time.Now(),
	}
}

func (s *Stage) Request(stage int) bool {
	s.mut.Lock()
	defer s.mut.Unlock()
	if stage < s.stage {
		return false
	}
	s.stage = stage
	return true
}

func (s *Stage) Start() bool {
	s.mut.Lock()
	defer s.mut.Unlock()
	if s.started {
		return false
	} else {
		s.started = true
		s.startedAt = time.Now()
		return true
	}
}

func (s *Stage) StartedAt() time.Time {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.startedAt
}

func (s *Stage) Value() int {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.stage
}

func (s *Stage) Is(stage int) bool {
	s.mut.RLock()
	defer s.mut.RUnlock()
	return s.stage == stage
}


func (s *Stage) Release() {
	s.mut.Lock()
	defer s.mut.Unlock()
	s.started = false
}
