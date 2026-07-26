package remarkable

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateEpub(t *testing.T) {
	dir := t.TempDir()

	epubPath := filepath.Join(dir, "book.epub")
	if err := os.WriteFile(epubPath, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateEpub(epubPath); err != nil {
		t.Errorf("validateEpub(%q) = %v, want nil", epubPath, err)
	}

	txtPath := filepath.Join(dir, "book.txt")
	if err := os.WriteFile(txtPath, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateEpub(txtPath); err == nil {
		t.Errorf("validateEpub(%q) = nil, want error (wrong extension)", txtPath)
	}

	missing := filepath.Join(dir, "missing.epub")
	if err := validateEpub(missing); err == nil {
		t.Errorf("validateEpub(%q) = nil, want error (missing file)", missing)
	}
}
