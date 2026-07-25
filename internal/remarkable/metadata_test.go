package remarkable

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestBuildMetadata(t *testing.T) {
	now := time.Date(2026, 7, 25, 20, 39, 0, 0, time.UTC)
	m := BuildMetadata("Golden Son", now)

	if m.VisibleName != "Golden Son" {
		t.Errorf("VisibleName = %q, want %q", m.VisibleName, "Golden Son")
	}
	if m.Type != "DocumentType" {
		t.Errorf("Type = %q, want %q", m.Type, "DocumentType")
	}
	if m.Parent != "" {
		t.Errorf("Parent = %q, want empty string (root folder)", m.Parent)
	}
	if m.Deleted || m.Pinned || m.Modified || m.MetadataModified || m.Synced {
		t.Errorf("expected all boolean flags false for a fresh import, got %+v", m)
	}

	wantMs := strconv.FormatInt(now.UnixMilli(), 10)
	if m.LastModified != wantMs {
		t.Errorf("LastModified = %q, want %q", m.LastModified, wantMs)
	}
}

func TestMetadataJSON(t *testing.T) {
	m := BuildMetadata("Golden Son", time.Now())

	data, err := m.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var round map[string]any
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if round["visibleName"] != "Golden Son" {
		t.Errorf("marshaled visibleName = %v, want %q", round["visibleName"], "Golden Son")
	}
	if round["type"] != "DocumentType" {
		t.Errorf("marshaled type = %v, want %q", round["type"], "DocumentType")
	}
}
