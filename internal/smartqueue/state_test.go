package smartqueue_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/bandesz/printago-buddy/internal/smartqueue"
)

func TestStateMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smart-queue.json")

	got, err := smartqueue.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Enabled = true, want false")
	}
	if got.HasMergingSnapshot {
		t.Errorf("HasMergingSnapshot = true, want false")
	}
	if len(got.PrioritizedJobIDs) != 0 {
		t.Errorf("PrioritizedJobIDs = %#v, want none", got.PrioritizedJobIDs)
	}
}

func TestStateSaveThenLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "smart-queue.json")
	cases := []smartqueue.State{
		{
			Enabled:            true,
			HasMergingSnapshot: true,
			AutomaticMerging:   false,
			PrioritizedJobIDs:  []string{"job-2", "job-1"},
		},
		{
			Enabled:            false,
			HasMergingSnapshot: false,
			AutomaticMerging:   true,
			PrioritizedJobIDs:  []string{"job-9"},
		},
	}

	for _, want := range cases {
		if err := smartqueue.SaveState(path, want); err != nil {
			t.Fatalf("SaveState() error = %v", err)
		}

		got, err := smartqueue.LoadState(path)
		if err != nil {
			t.Fatalf("LoadState() error = %v", err)
		}
		if got.Enabled != want.Enabled ||
			got.HasMergingSnapshot != want.HasMergingSnapshot ||
			got.AutomaticMerging != want.AutomaticMerging ||
			!slices.Equal(got.PrioritizedJobIDs, want.PrioritizedJobIDs) {
			t.Fatalf("LoadState() = %+v, want %+v", got, want)
		}

		raw, err := os.ReadFile(path) //nolint:gosec // path is the temp state file this test just wrote
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if doc["enabled"] != want.Enabled ||
			doc["hasMergingSnapshot"] != want.HasMergingSnapshot ||
			doc["automaticMerging"] != want.AutomaticMerging {
			t.Fatalf("state file = %#v, want enabled=%v hasMergingSnapshot=%v automaticMerging=%v",
				doc, want.Enabled, want.HasMergingSnapshot, want.AutomaticMerging)
		}
		ids, ok := doc["prioritizedJobIds"].([]any)
		if !ok || len(ids) != len(want.PrioritizedJobIDs) {
			t.Fatalf("prioritizedJobIds = %#v, want %#v", doc["prioritizedJobIds"], want.PrioritizedJobIDs)
		}
		for i, id := range want.PrioritizedJobIDs {
			if ids[i] != id {
				t.Fatalf("prioritizedJobIds = %#v, want %#v", doc["prioritizedJobIds"], want.PrioritizedJobIDs)
			}
		}
	}
}

func TestStateDirectoryCannotBeCreated(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	path := filepath.Join(blocker, "smart-queue.json")
	err := smartqueue.SaveState(path, smartqueue.State{
		Enabled:            true,
		HasMergingSnapshot: true,
		AutomaticMerging:   true,
		PrioritizedJobIDs:  []string{"job-1"},
	})
	if err == nil {
		t.Fatal("SaveState() error = nil, want an error")
	}

	got, loadErr := smartqueue.LoadState(path)
	if loadErr == nil && got.Enabled {
		t.Fatalf("LoadState() = %+v, nil; failed save left a file that loads as enabled", got)
	}

	data, err := os.ReadFile(blocker) //nolint:gosec // path is the temp blocker file this test created
	if err != nil {
		t.Fatalf("ReadFile(blocker) error = %v", err)
	}
	if string(data) != "blocker" {
		t.Fatalf("blocker content = %q, want unchanged", data)
	}
}
