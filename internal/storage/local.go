package storage

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"archivesync/internal/models"
)

func init() {
	Register(models.ChannelLocal, newLocal)
}

// localBackend is a storage.Backend that stores archives on the local
// filesystem beneath a configured base directory.
type localBackend struct {
	base string
}

// newLocal constructs a local filesystem backend from a channel definition.
// It requires ch.Config.BasePath.
func newLocal(ch models.Channel) (Backend, error) {
	base := strings.TrimSpace(ch.Config.BasePath)
	if base == "" {
		return nil, fmt.Errorf("storage/local: base_path is required")
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return nil, fmt.Errorf("storage/local: resolve base_path %q: %w", base, err)
	}
	return &localBackend{base: abs}, nil
}

// resolve maps a slash-separated object key to an absolute filesystem path
// under base, rejecting keys that would escape the base directory.
func (b *localBackend) resolve(key string) (string, error) {
	// Normalise to forward slashes and clean against a virtual root so that
	// any ".." segments can never climb above the base directory.
	slash := filepath.ToSlash(key)
	clean := path.Clean("/" + slash)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "" || rel == "." {
		return "", fmt.Errorf("storage/local: invalid key %q", key)
	}
	full := filepath.Join(b.base, filepath.FromSlash(rel))
	// Defensive containment check in case of platform-specific edge cases.
	if full != b.base && !strings.HasPrefix(full, b.base+string(os.PathSeparator)) {
		return "", fmt.Errorf("storage/local: key %q escapes base path", key)
	}
	return full, nil
}

// Put stores r under key, creating any missing parent directories. size is
// ignored; the full stream is written.
func (b *localBackend) Put(ctx context.Context, key string, r io.Reader, size int64) error {
	full, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("storage/local: create dir for %q: %w", key, err)
	}
	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("storage/local: create %q: %w", key, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return fmt.Errorf("storage/local: write %q: %w", key, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("storage/local: close %q: %w", key, err)
	}
	return nil
}

// Get opens the object at key for reading.
func (b *localBackend) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := b.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		return nil, fmt.Errorf("storage/local: open %q: %w", key, err)
	}
	return f, nil
}

// List walks the base directory and returns objects whose slash-separated key
// starts with prefix.
func (b *localBackend) List(ctx context.Context, prefix string) ([]Object, error) {
	prefix = filepath.ToSlash(prefix)
	var out []Object
	err := filepath.WalkDir(b.base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// The base dir not existing yet simply means there is nothing to list.
			if os.IsNotExist(err) && p == b.base {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(b.base, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out = append(out, Object{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage/local: list %q: %w", prefix, err)
	}
	return out, nil
}

// Delete removes the object at key. A missing file is not an error.
func (b *localBackend) Delete(ctx context.Context, key string) error {
	full, err := b.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage/local: delete %q: %w", key, err)
	}
	return nil
}

// Ping ensures the base directory exists and is writable.
func (b *localBackend) Ping(ctx context.Context) error {
	if err := os.MkdirAll(b.base, 0o755); err != nil {
		return fmt.Errorf("storage/local: ensure base path %q: %w", b.base, err)
	}
	f, err := os.CreateTemp(b.base, ".archivesync-ping-*")
	if err != nil {
		return fmt.Errorf("storage/local: base path %q not writable: %w", b.base, err)
	}
	name := f.Name()
	f.Close()
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("storage/local: cleanup ping file: %w", err)
	}
	return nil
}

// Kind returns the backend type identifier.
func (b *localBackend) Kind() string { return "local" }
