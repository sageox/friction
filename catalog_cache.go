package frictionax

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// catalogCache manages the friction catalog cache on disk.
type catalogCache struct {
	mu       sync.RWMutex
	catalog  *CatalogData
	filePath string
}

// newCatalogCache creates a new catalog cache that stores data at the given file path.
func newCatalogCache(filePath string) *catalogCache {
	return &catalogCache{
		filePath: filePath,
	}
}

// Load reads the catalog from disk if it exists.
func (c *catalogCache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			c.catalog = nil
			return nil
		}
		return fmt.Errorf("read catalog cache: %w", err)
	}

	if len(data) == 0 {
		c.catalog = nil
		return nil
	}

	var cat CatalogData
	if err := json.Unmarshal(data, &cat); err != nil {
		c.catalog = nil
		return nil
	}

	c.catalog = &cat
	return nil
}

// Save writes the catalog to disk.
func (c *catalogCache) Save(cat *CatalogData) error {
	if cat == nil {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(c.filePath), 0755); err != nil {
		return fmt.Errorf("create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(cat, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}

	tmpFile := c.filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := os.Rename(tmpFile, c.filePath); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("rename temp file: %w", err)
	}

	c.catalog = cat
	return nil
}

// Version returns the current cached catalog version.
func (c *catalogCache) Version() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.catalog == nil {
		return ""
	}
	return c.catalog.Version
}

// Data returns the current catalog data.
func (c *catalogCache) Data() *CatalogData {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.catalog == nil {
		return nil
	}

	catalogCopy := *c.catalog
	if c.catalog.Commands != nil {
		catalogCopy.Commands = make([]CommandMapping, len(c.catalog.Commands))
		copy(catalogCopy.Commands, c.catalog.Commands)
	}
	if c.catalog.Tokens != nil {
		catalogCopy.Tokens = make([]TokenMapping, len(c.catalog.Tokens))
		copy(catalogCopy.Tokens, c.catalog.Tokens)
	}

	return &catalogCopy
}

// Update updates the catalog cache if the new version differs from current.
func (c *catalogCache) Update(cat *CatalogData) (bool, error) {
	if cat == nil {
		return false, nil
	}

	c.mu.RLock()
	currentVersion := ""
	if c.catalog != nil {
		currentVersion = c.catalog.Version
	}
	c.mu.RUnlock()

	if currentVersion == cat.Version {
		return false, nil
	}

	if err := c.Save(cat); err != nil {
		return false, err
	}

	return true, nil
}

// Clear removes the catalog cache from disk and memory.
func (c *catalogCache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.catalog = nil

	if err := os.Remove(c.filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove catalog cache: %w", err)
	}

	return nil
}

// FilePath returns the path to the catalog cache file.
func (c *catalogCache) FilePath() string {
	return c.filePath
}
