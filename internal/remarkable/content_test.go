package remarkable

import (
	"encoding/json"
	"testing"
)

func TestBuildContent(t *testing.T) {
	c := BuildContent(1547868)

	if c.FileType != "epub" {
		t.Errorf("FileType = %q, want %q", c.FileType, "epub")
	}
	if c.SizeInBytes != "1547868" {
		t.Errorf("SizeInBytes = %q, want %q", c.SizeInBytes, "1547868")
	}
	if c.PageCount != 0 {
		t.Errorf("PageCount = %d, want 0 (xochitl paginates on import)", c.PageCount)
	}
	if len(c.Pages) != 0 {
		t.Errorf("Pages = %v, want empty", c.Pages)
	}
	if c.CoverPageNumber != -1 {
		t.Errorf("CoverPageNumber = %d, want -1", c.CoverPageNumber)
	}
}

func TestContentJSON(t *testing.T) {
	c := BuildContent(42)

	data, err := c.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var round map[string]any
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if round["fileType"] != "epub" {
		t.Errorf("marshaled fileType = %v, want %q", round["fileType"], "epub")
	}
	if round["sizeInBytes"] != "42" {
		t.Errorf("marshaled sizeInBytes = %v, want %q", round["sizeInBytes"], "42")
	}
}
