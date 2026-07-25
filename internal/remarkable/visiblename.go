package remarkable

import (
	"path/filepath"
	"strings"
)

// VisibleNameFromFilename derives the default "My Files" title from an
// EPUB's filename: base name, extension stripped.
func VisibleNameFromFilename(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
