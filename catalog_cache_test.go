package frictionax

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCatalogCache_Load(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) string // returns file path
		wantErr     bool
		wantVersion string
	}{
		{
			name: "file does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.json")
			},
			wantVersion: "",
		},
		{
			name: "empty file",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "empty.json")
				if err := os.WriteFile(p, []byte(""), 0600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantVersion: "",
		},
		{
			name: "invalid json",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "bad.json")
				if err := os.WriteFile(p, []byte("{not valid json"), 0600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantVersion: "", // invalid json is silently ignored
		},
		{
			name: "valid catalog",
			setup: func(t *testing.T) string {
				p := filepath.Join(t.TempDir(), "catalog.json")
				cat := CatalogData{
					Version: "v1",
					Commands: []CommandMapping{
						{Pattern: "stauts", Target: "status"},
					},
				}
				data, _ := json.Marshal(cat)
				if err := os.WriteFile(p, data, 0600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantVersion: "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup(t)
			cc := newCatalogCache(p)
			err := cc.Load()

			if tt.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := cc.Version(); got != tt.wantVersion {
				t.Errorf("Version() = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}

func TestCatalogCache_Load_ReadError(t *testing.T) {
	// directory instead of file triggers a read error
	dir := t.TempDir()
	cc := newCatalogCache(dir) // path is a directory, ReadFile will fail
	err := cc.Load()
	if err == nil {
		t.Error("expected error when reading a directory as file")
	}
}

func TestCatalogCache_Save(t *testing.T) {
	t.Run("nil catalog is no-op", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "catalog.json")
		cc := newCatalogCache(p)
		if err := cc.Save(nil); err != nil {
			t.Fatalf("Save(nil) error: %v", err)
		}
		// file should not exist
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("file should not exist after Save(nil)")
		}
	})

	t.Run("creates directory and writes file", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "sub", "dir", "catalog.json")
		cc := newCatalogCache(p)
		cat := &CatalogData{
			Version: "v2",
			Tokens:  []TokenMapping{{Pattern: "foo", Target: "bar"}},
		}
		if err := cc.Save(cat); err != nil {
			t.Fatalf("Save error: %v", err)
		}

		// verify file exists and is valid json
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("ReadFile error: %v", err)
		}
		var loaded CatalogData
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}
		if loaded.Version != "v2" {
			t.Errorf("loaded version = %q, want %q", loaded.Version, "v2")
		}
	})

	t.Run("atomic write leaves no temp file", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "catalog.json")
		cc := newCatalogCache(p)
		cat := &CatalogData{Version: "v3"}
		if err := cc.Save(cat); err != nil {
			t.Fatalf("Save error: %v", err)
		}

		// temp file should not remain
		tmpFile := p + ".tmp"
		if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
			t.Error("temp file should not exist after successful save")
		}
	})
}

func TestCatalogCache_Version(t *testing.T) {
	tests := []struct {
		name    string
		catalog *CatalogData
		want    string
	}{
		{name: "nil catalog", catalog: nil, want: ""},
		{name: "loaded catalog", catalog: &CatalogData{Version: "v42"}, want: "v42"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := newCatalogCache(filepath.Join(t.TempDir(), "c.json"))
			if tt.catalog != nil {
				if err := cc.Save(tt.catalog); err != nil {
					t.Fatal(err)
				}
			}
			if got := cc.Version(); got != tt.want {
				t.Errorf("Version() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCatalogCache_Data(t *testing.T) {
	t.Run("nil catalog returns nil", func(t *testing.T) {
		cc := newCatalogCache(filepath.Join(t.TempDir(), "c.json"))
		if got := cc.Data(); got != nil {
			t.Errorf("Data() = %v, want nil", got)
		}
	})

	t.Run("returns deep copy", func(t *testing.T) {
		cc := newCatalogCache(filepath.Join(t.TempDir(), "c.json"))
		original := &CatalogData{
			Version:  "v1",
			Commands: []CommandMapping{{Pattern: "a", Target: "b"}},
			Tokens:   []TokenMapping{{Pattern: "x", Target: "y"}},
		}
		if err := cc.Save(original); err != nil {
			t.Fatal(err)
		}

		// modify the returned copy
		copy1 := cc.Data()
		copy1.Version = "modified"
		copy1.Commands[0].Pattern = "mutated"
		copy1.Tokens[0].Pattern = "mutated"

		// original should be unchanged
		copy2 := cc.Data()
		if copy2.Version != "v1" {
			t.Errorf("cached version was mutated: got %q", copy2.Version)
		}
		if copy2.Commands[0].Pattern != "a" {
			t.Errorf("cached commands were mutated: got %q", copy2.Commands[0].Pattern)
		}
		if copy2.Tokens[0].Pattern != "x" {
			t.Errorf("cached tokens were mutated: got %q", copy2.Tokens[0].Pattern)
		}
	})
}

func TestCatalogCache_Update(t *testing.T) {
	tests := []struct {
		name        string
		initial     *CatalogData
		update      *CatalogData
		wantChanged bool
		wantVersion string
	}{
		{
			name:        "nil input returns false",
			update:      nil,
			wantChanged: false,
			wantVersion: "",
		},
		{
			name:        "same version returns false",
			initial:     &CatalogData{Version: "v1"},
			update:      &CatalogData{Version: "v1"},
			wantChanged: false,
			wantVersion: "v1",
		},
		{
			name:        "new version saves and returns true",
			initial:     &CatalogData{Version: "v1"},
			update:      &CatalogData{Version: "v2"},
			wantChanged: true,
			wantVersion: "v2",
		},
		{
			name:        "first update from empty",
			update:      &CatalogData{Version: "v1"},
			wantChanged: true,
			wantVersion: "v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := newCatalogCache(filepath.Join(t.TempDir(), "c.json"))
			if tt.initial != nil {
				if err := cc.Save(tt.initial); err != nil {
					t.Fatal(err)
				}
			}

			changed, err := cc.Update(tt.update)
			if err != nil {
				t.Fatalf("Update error: %v", err)
			}
			if changed != tt.wantChanged {
				t.Errorf("Update() changed = %v, want %v", changed, tt.wantChanged)
			}
			if got := cc.Version(); got != tt.wantVersion {
				t.Errorf("Version() = %q, want %q", got, tt.wantVersion)
			}
		})
	}
}

func TestCatalogCache_Clear(t *testing.T) {
	t.Run("removes file and clears memory", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "c.json")
		cc := newCatalogCache(p)
		if err := cc.Save(&CatalogData{Version: "v1"}); err != nil {
			t.Fatal(err)
		}

		if err := cc.Clear(); err != nil {
			t.Fatalf("Clear error: %v", err)
		}
		if cc.Version() != "" {
			t.Error("version should be empty after Clear")
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("file should not exist after Clear")
		}
	})

	t.Run("no error when file does not exist", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "nonexistent.json")
		cc := newCatalogCache(p)
		if err := cc.Clear(); err != nil {
			t.Fatalf("Clear error on nonexistent file: %v", err)
		}
	})
}

func TestCatalogCache_FilePath(t *testing.T) {
	want := "/some/path/catalog.json"
	cc := newCatalogCache(want)
	if got := cc.FilePath(); got != want {
		t.Errorf("FilePath() = %q, want %q", got, want)
	}
}

func TestCatalogCache_ConcurrentAccess(t *testing.T) {
	p := filepath.Join(t.TempDir(), "concurrent.json")
	cc := newCatalogCache(p)

	var wg sync.WaitGroup

	// concurrent writes
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			cat := &CatalogData{Version: "v" + string(rune('a'+v))}
			cc.Save(cat)
		}(i)
	}

	// concurrent reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cc.Version()
			cc.Data()
		}()
	}

	wg.Wait()
	// no panics or data races is the assertion (run with -race)
}
