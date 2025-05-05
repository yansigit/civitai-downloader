package storage

import "errors"

// ErrNotFound is returned when an item cannot be found in remote storage.
var ErrNotFound = errors.New("item not found")
