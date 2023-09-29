package main

import (
	"github.com/fsnotify/fsnotify"
)

// NewWatcher creates a watcher
func NewWatcher(c *CLI) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return watcher, err
	}

	err = watcher.Add(c.CompositeResource)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(c.Composition)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(c.Functions)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(c.ObservedResources)
	if err != nil {
		return watcher, err
	}

	return watcher, nil
}
