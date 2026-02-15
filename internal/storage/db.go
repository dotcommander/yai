package storage

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gofrs/flock"
)

var (
	// ErrNoMatches is returned when no conversations match the query.
	ErrNoMatches = errors.New("no conversations found")
	// ErrManyMatches is returned when multiple conversations match the query.
	ErrManyMatches = errors.New("multiple conversations matched the input")
)

const (
	indexFileName      = "index.jsonl"
	compactMinOps      = 256
	compactScaleFactor = 4
)

type convoEvent struct {
	Op           string        `json:"op"`
	ID           string        `json:"id,omitempty"`
	Conversation *Conversation `json:"conversation,omitempty"`
}

// Open loads the conversation metadata store from the given datasource.
//
// The datasource is usually a directory path. The special value ":memory:"
// creates a temporary store (primarily used for tests).
func Open(ds string) (*DB, error) {
	dir, cleanupDir, err := resolveStoreDir(ds)
	if err != nil {
		return nil, fmt.Errorf("could not resolve store path: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("could not create store directory: %w", err)
	}

	c := &DB{
		indexPath:      filepath.Join(dir, indexFileName),
		lock:           flock.New(filepath.Join(dir, "index.lock")),
		conversations:  make(map[string]Conversation),
		cleanupTempDir: cleanupDir,
	}
	if err := c.load(); err != nil {
		return nil, err
	}

	return c, nil
}

// DB is an append-only JSONL-backed conversation metadata index.
type DB struct {
	mu             sync.RWMutex
	indexPath      string
	lock           *flock.Flock
	conversations  map[string]Conversation
	ops            int
	cleanupTempDir string
}

// Conversation in the database.
type Conversation struct {
	ID        string    `db:"id"`
	Title     string    `db:"title"`
	UpdatedAt time.Time `db:"updated_at"`
	API       *string   `db:"api"`
	Model     *string   `db:"model"`
}

// Close releases temporary resources (used for :memory: stores).
func (c *DB) Close() error {
	if c.cleanupTempDir == "" {
		return nil
	}
	if err := os.RemoveAll(c.cleanupTempDir); err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}

// Save upserts a conversation metadata record.
func (c *DB) Save(id, title, api, model string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("Save: %w", errors.New("empty id"))
	}
	if strings.TrimSpace(title) == "" {
		return fmt.Errorf("Save: %w", errors.New("empty title"))
	}

	now := time.Now().UTC()
	apiCopy := api
	modelCopy := model
	convo := Conversation{
		ID:        id,
		Title:     title,
		UpdatedAt: now,
		API:       &apiCopy,
		Model:     &modelCopy,
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.conversations[id] = convo
	if err := c.appendEventLocked(convoEvent{Op: "upsert", Conversation: &convo}); err != nil {
		return fmt.Errorf("Save: %w", err)
	}
	if err := c.compactIfNeededLocked(); err != nil {
		return fmt.Errorf("Save: %w", err)
	}

	return nil
}

// Delete removes a conversation record by ID.
func (c *DB) Delete(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("Delete: %w", errors.New("empty id"))
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, ok := c.conversations[id]; !ok {
		return nil
	}
	delete(c.conversations, id)

	if err := c.appendEventLocked(convoEvent{Op: "delete", ID: id}); err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	if err := c.compactIfNeededLocked(); err != nil {
		return fmt.Errorf("Delete: %w", err)
	}
	return nil
}

// ListOlderThan returns conversations older than the given duration.
func (c *DB) ListOlderThan(t time.Duration) []Conversation {
	cutoff := time.Now().Add(-t)

	c.mu.RLock()
	convos := make([]Conversation, 0, len(c.conversations))
	for _, convo := range c.conversations {
		if convo.UpdatedAt.Before(cutoff) {
			convos = append(convos, convo)
		}
	}
	c.mu.RUnlock()

	sortConversationsByUpdatedAtDesc(convos)
	return convos
}

// FindHEAD returns the most recently updated conversation.
func (c *DB) FindHEAD() (*Conversation, error) {
	list := c.List()
	if len(list) == 0 {
		return nil, fmt.Errorf("FindHead: %w", ErrNoMatches)
	}
	head := list[0]
	return &head, nil
}

// Completions returns shell completion candidates for IDs and titles.
func (c *DB) Completions(in string) []string {
	resultSet := make(map[string]struct{})

	c.mu.RLock()
	for _, convo := range c.conversations {
		if strings.HasPrefix(convo.ID, in) {
			displayID := convo.ID
			if len(in) < SHA1Short && len(convo.ID) > SHA1Short {
				displayID = convo.ID[:SHA1Short]
			}
			resultSet[fmt.Sprintf("%s\t%s", displayID, convo.Title)] = struct{}{}
		}
		if strings.HasPrefix(convo.Title, in) {
			displayID := convo.ID
			if len(convo.ID) > SHA1Short {
				displayID = convo.ID[:SHA1Short]
			}
			resultSet[fmt.Sprintf("%s\t%s", convo.Title, displayID)] = struct{}{}
		}
	}
	c.mu.RUnlock()

	result := make([]string, 0, len(resultSet))
	for value := range resultSet {
		result = append(result, value)
	}
	sort.Strings(result)

	return result
}

// Find resolves a conversation by ID prefix or exact title.
func (c *DB) Find(in string) (*Conversation, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	conversations := make([]Conversation, 0, len(c.conversations))
	if len(in) < SHA1MinLen {
		for _, convo := range c.conversations {
			if convo.Title == in {
				conversations = append(conversations, convo)
			}
		}
	} else {
		for _, convo := range c.conversations {
			if strings.HasPrefix(convo.ID, in) || convo.Title == in {
				conversations = append(conversations, convo)
			}
		}
	}

	if len(conversations) > 1 {
		return nil, fmt.Errorf("%w: %s", ErrManyMatches, in)
	}
	if len(conversations) == 1 {
		return &conversations[0], nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNoMatches, in)
}

// List returns conversations sorted by most recently updated.
func (c *DB) List() []Conversation {
	c.mu.RLock()
	convos := make([]Conversation, 0, len(c.conversations))
	for _, convo := range c.conversations {
		convos = append(convos, convo)
	}
	c.mu.RUnlock()

	sortConversationsByUpdatedAtDesc(convos)
	return convos
}

func resolveStoreDir(ds string) (dir string, cleanupDir string, err error) {
	if ds == ":memory:" {
		tempDir, err := os.MkdirTemp("", "yai-conversations-*")
		if err != nil {
			return "", "", fmt.Errorf("could not create temp conversations directory: %w", err)
		}
		return tempDir, tempDir, nil
	}

	if filepath.Ext(ds) == ".db" {
		return filepath.Dir(ds), "", nil
	}

	return ds, "", nil
}

func (c *DB) load() error {
	if c.lock != nil {
		if err := c.lock.Lock(); err != nil {
			return fmt.Errorf("could not lock index file: %w", err)
		}
		defer func() { _ = c.lock.Unlock() }()
	}

	file, err := os.Open(c.indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("could not open index file: %w", err)
	}
	defer file.Close() //nolint:errcheck

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var evt convoEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return fmt.Errorf("could not parse index event: %w", err)
		}
		if err := c.applyEvent(&evt); err != nil {
			return err
		}
		c.ops++
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("could not scan index file: %w", err)
	}

	return nil
}

func (c *DB) applyEvent(evt *convoEvent) error {
	switch evt.Op {
	case "upsert":
		if evt.Conversation == nil {
			return fmt.Errorf("invalid upsert event: missing conversation")
		}
		if strings.TrimSpace(evt.Conversation.ID) == "" {
			return fmt.Errorf("invalid upsert event: empty id")
		}
		convo := *evt.Conversation
		c.conversations[convo.ID] = convo
	case "delete":
		if strings.TrimSpace(evt.ID) == "" {
			return fmt.Errorf("invalid delete event: empty id")
		}
		delete(c.conversations, evt.ID)
	default:
		return fmt.Errorf("invalid index event op: %q", evt.Op)
	}
	return nil
}

func (c *DB) appendEventLocked(evt convoEvent) error {
	if c.lock != nil {
		if err := c.lock.Lock(); err != nil {
			return fmt.Errorf("lock index: %w", err)
		}
		defer func() { _ = c.lock.Unlock() }()
	}

	file, err := os.OpenFile(c.indexPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer func() { _ = file.Close() }()

	bts, err := json.Marshal(evt)
	if err != nil {
		return fmt.Errorf("marshal index event: %w", err)
	}
	bts = append(bts, '\n')
	if _, err := file.Write(bts); err != nil {
		_ = file.Close()
		return fmt.Errorf("write index event: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync index: %w", err)
	}

	c.ops++
	return nil
}

func (c *DB) compactIfNeededLocked() error {
	if c.ops < compactMinOps {
		return nil
	}
	if len(c.conversations) > 0 && c.ops < len(c.conversations)*compactScaleFactor {
		return nil
	}
	return c.compactLocked()
}

func (c *DB) compactLocked() error {
	if c.lock != nil {
		if err := c.lock.Lock(); err != nil {
			return fmt.Errorf("lock index: %w", err)
		}
		defer func() { _ = c.lock.Unlock() }()
	}

	items := make([]Conversation, 0, len(c.conversations))
	for _, convo := range c.conversations {
		items = append(items, convo)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt.Before(items[j].UpdatedAt)
	})

	tmpPath := c.indexPath + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open compacted index: %w", err)
	}

	enc := json.NewEncoder(file)
	for _, convo := range items {
		event := convoEvent{Op: "upsert", Conversation: &convo}
		if err := enc.Encode(event); err != nil {
			_ = file.Close()
			return fmt.Errorf("write compacted index: %w", err)
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync compacted index: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close compacted index: %w", err)
	}

	if err := os.Rename(tmpPath, c.indexPath); err != nil {
		return fmt.Errorf("replace index with compacted version: %w", err)
	}
	_ = syncDir(filepath.Dir(c.indexPath))

	c.ops = len(c.conversations)
	return nil
}

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}

func sortConversationsByUpdatedAtDesc(convos []Conversation) {
	sort.Slice(convos, func(i, j int) bool {
		if convos[i].UpdatedAt.Equal(convos[j].UpdatedAt) {
			return convos[i].ID < convos[j].ID
		}
		return convos[i].UpdatedAt.After(convos[j].UpdatedAt)
	})
}
