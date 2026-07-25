package remarkable

import (
	"encoding/json"
	"strconv"
)

// Content is xochitl's <uuid>.content sidecar file. PageCount/Pages are left
// empty — xochitl paginates and rewrites this itself (pulling title/author/
// publisher straight from the EPUB) the moment it rescans on restart.
type Content struct {
	CoverPageNumber    int            `json:"coverPageNumber"`
	DocumentMetadata   map[string]any `json:"documentMetadata"`
	DummyDocument      bool           `json:"dummyDocument"`
	ExtraMetadata      map[string]any `json:"extraMetadata"`
	FileType           string         `json:"fileType"`
	FontName           string         `json:"fontName"`
	FormatVersion      int            `json:"formatVersion"`
	LineHeight         int            `json:"lineHeight"`
	Margins            int            `json:"margins"`
	Orientation        string         `json:"orientation"`
	OriginalPageCount  int            `json:"originalPageCount"`
	PageCount          int            `json:"pageCount"`
	Pages              []string       `json:"pages"`
	RedirectionPageMap []int          `json:"redirectionPageMap"`
	SizeInBytes        string         `json:"sizeInBytes"`
	TextAlignment      string         `json:"textAlignment"`
	TextScale          int            `json:"textScale"`
}

func BuildContent(sizeBytes int64) Content {
	return Content{
		CoverPageNumber:    -1,
		DocumentMetadata:   map[string]any{},
		DummyDocument:      false,
		ExtraMetadata:      map[string]any{},
		FileType:           "epub",
		FontName:           "",
		FormatVersion:      1,
		LineHeight:         -1,
		Margins:            180,
		Orientation:        "portrait",
		OriginalPageCount:  -1,
		PageCount:          0,
		Pages:              []string{},
		RedirectionPageMap: []int{},
		SizeInBytes:        strconv.FormatInt(sizeBytes, 10),
		TextAlignment:      "left",
		TextScale:          1,
	}
}

func (c Content) JSON() ([]byte, error) {
	return json.MarshalIndent(c, "", "    ")
}
