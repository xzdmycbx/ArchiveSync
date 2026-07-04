// Package storage defines the Backend interface for backup destinations and a
// registry through which concrete backends (s3, local) register themselves.
package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"archivesync/internal/models"
)

// Object is a stored archive object.
type Object struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

// Backend is a storage destination able to store, list and delete archives.
type Backend interface {
	// Put stores r under key. size may be -1 if unknown.
	Put(ctx context.Context, key string, r io.Reader, size int64) error
	// Get opens the object at key for reading.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// List returns objects whose key starts with prefix.
	List(ctx context.Context, prefix string) ([]Object, error)
	// Delete removes the object at key (no error if absent).
	Delete(ctx context.Context, key string) error
	// Ping verifies connectivity / credentials.
	Ping(ctx context.Context) error
	// Kind returns the backend type identifier.
	Kind() string
}

// Factory constructs a Backend from a channel definition.
type Factory func(ch models.Channel) (Backend, error)

var registry = map[models.ChannelType]Factory{}

// Register makes a backend factory available to New. Called from impls' init().
func Register(t models.ChannelType, f Factory) { registry[t] = f }

// New constructs the backend for the given channel.
func New(ch models.Channel) (Backend, error) {
	f, ok := registry[ch.Type]
	if !ok {
		return nil, fmt.Errorf("storage: unknown channel type %q", ch.Type)
	}
	return f(ch)
}

// Types returns the registered backend type identifiers.
func Types() []models.ChannelType {
	out := make([]models.ChannelType, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	return out
}
