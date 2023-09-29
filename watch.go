package main

import (
	"github.com/fsnotify/fsnotify"
)

// NewWatcher creates a watcher
func NewWatcher(r *Rendered) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return watcher, err
	}

	err = watcher.Add(r.CompositeResource)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(r.Composition)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(r.Functions)
	if err != nil {
		return watcher, err
	}
	err = watcher.Add(r.ObservedResources)
	if err != nil {
		return watcher, err
	}

	return watcher, nil
}
