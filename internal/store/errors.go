package store

import "errors"

// ErrNotFound is returned by store Get*/load methods when a row does not
// exist. The httpapi layer maps it to 404.
var ErrNotFound = errors.New("store: not found")
