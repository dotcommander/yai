// Package cache provides a simple in-file cache implementation.
package cache

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Type represents the type of cache being used.
type Type string

// Cache types for different purposes.
const (
	ConversationCache Type = "conversations"
	TemporaryCache    Type = "temp"
)

const (
	cacheExt       = ".json"
	shardPrefixLen = 2
)

var errInvalidID = errors.New("invalid id")

// Cache is a generic cache implementation that stores data in files.
type Cache[T any] struct {
	baseDir string
	cType   Type
}

// New creates a new cache instance with the specified base directory and cache type.
func New[T any](baseDir string, cacheType Type) (*Cache[T], error) {
	dir := filepath.Join(baseDir, string(cacheType))
	if err := os.MkdirAll(dir, os.ModePerm); err != nil { //nolint:gosec
		return nil, fmt.Errorf("create cache directory: %w", err)
	}
	return &Cache[T]{
		baseDir: baseDir,
		cType:   cacheType,
	}, nil
}

func (c *Cache[T]) dir() string {
	return filepath.Join(c.baseDir, string(c.cType))
}

func (c *Cache[T]) filePath(id string) string {
	if !c.isSharded() || len(id) < shardPrefixLen {
		return c.legacyFilePath(id)
	}
	return filepath.Join(c.dir(), id[:shardPrefixLen], id+cacheExt)
}

func (c *Cache[T]) legacyFilePath(id string) string {
	return filepath.Join(c.dir(), id+cacheExt)
}

func (c *Cache[T]) isSharded() bool {
	return c.cType == ConversationCache
}

func (c *Cache[T]) Read(id string, readFn func(io.Reader) error) error {
	if id == "" {
		return fmt.Errorf("read: %w", errInvalidID)
	}
	file, err := os.Open(c.filePath(id))
	if err != nil {
		if c.isSharded() && errors.Is(err, os.ErrNotExist) {
			file, err = os.Open(c.legacyFilePath(id))
		}
	}
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	defer file.Close() //nolint:errcheck

	if err := readFn(file); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	return nil
}

func (c *Cache[T]) Write(id string, writeFn func(io.Writer) error) error {
	if id == "" {
		return fmt.Errorf("write: %w", errInvalidID)
	}

	path := c.filePath(id)
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil { //nolint:gosec
		return fmt.Errorf("write: %w", err)
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := writeFn(tmp); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// Delete removes a cached item by its ID.
func (c *Cache[T]) Delete(id string) error {
	if id == "" {
		return fmt.Errorf("delete: %w", errInvalidID)
	}
	err := os.Remove(c.filePath(id))
	if c.isSharded() && errors.Is(err, os.ErrNotExist) {
		err = os.Remove(c.legacyFilePath(id))
	}
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	return nil
}
