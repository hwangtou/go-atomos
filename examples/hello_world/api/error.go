package api

import "errors"

var (
	// RemoteBooth Errors
	ErrRemoteUnsupportedWatcher = errors.New("remote unsupported watcher")
	ErrRemoteUnwatchedWatcher = errors.New("remote unwatched watcher")
)
