// Package remarkable sends an EPUB to a reMarkable tablet over SFTP and
// registers it in xochitl's document store so it shows up in "My Files"
// (not just SFTP'd next to it) — see main-remarkable.md in the
// 26-remarkable-tablet vault project for how this schema was reverse
// engineered from a live device.
package remarkable

import (
	"encoding/json"
	"strconv"
	"time"
)

// Metadata is xochitl's <uuid>.metadata sidecar file.
type Metadata struct {
	Deleted          bool   `json:"deleted"`
	LastModified     string `json:"lastModified"`
	MetadataModified bool   `json:"metadatamodified"`
	Modified         bool   `json:"modified"`
	Parent           string `json:"parent"`
	Pinned           bool   `json:"pinned"`
	Synced           bool   `json:"synced"`
	Type             string `json:"type"`
	Version          int    `json:"version"`
	VisibleName      string `json:"visibleName"`
}

// BuildMetadata builds the metadata for a freshly imported document, placed
// in the root folder (Parent == "").
func BuildMetadata(visibleName string, now time.Time) Metadata {
	return Metadata{
		Deleted:          false,
		LastModified:     strconv.FormatInt(now.UnixMilli(), 10),
		MetadataModified: false,
		Modified:         false,
		Parent:           "",
		Pinned:           false,
		Synced:           false,
		Type:             "DocumentType",
		Version:          0,
		VisibleName:      visibleName,
	}
}

func (m Metadata) JSON() ([]byte, error) {
	return json.MarshalIndent(m, "", "    ")
}
