package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ConfigAgent agent for fetch tasks with watching and hot reload.
type ConfigAgent[T any] struct {
	path   string
	config T
	mu     sync.RWMutex
}

// WatchConfig monitors a file for changes and sends a message on the channel when the file changes
func (c *ConfigAgent[T]) WatchConfig(ctx context.Context, interval time.Duration, onChangeHandler func(f string) error) {
	var lastMod time.Time
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			logrus.Debug("ticker")
			info, err := os.Stat(c.path)
			if err != nil {
				fmt.Printf("Error getting file info: %v\n", err)
			} else if modTime := info.ModTime(); modTime.After(lastMod) {
				lastMod = modTime
				onChangeHandler(c.path)
			}
		}
	}
}

// Reload read and update config data.
func (c *ConfigAgent[T]) Reload(file string) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("could no load config file %s: %w", file, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	switch path.Ext(file) {
	case ".json":
		if err := json.Unmarshal(data, &c.config); err != nil {
			return fmt.Errorf("could not unmarshal JSON config: %w", err)
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &c.config); err != nil {
			return fmt.Errorf("could not unmarshal YAML config: %w", err)
		}
	default:
		return errors.New("only support file with `.json` or `.yaml` extension")
	}

	return nil
}

func (c *ConfigAgent[T]) Data() T {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.config
}
