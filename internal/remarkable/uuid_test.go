package remarkable

import (
	"regexp"
	"testing"
)

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUID(t *testing.T) {
	id, err := NewUUID()
	if err != nil {
		t.Fatalf("NewUUID() error: %v", err)
	}
	if !uuidV4Pattern.MatchString(id) {
		t.Errorf("NewUUID() = %q, does not match UUIDv4 format", id)
	}
}

func TestNewUUIDIsRandom(t *testing.T) {
	id1, _ := NewUUID()
	id2, _ := NewUUID()
	if id1 == id2 {
		t.Errorf("NewUUID() returned the same value twice: %q", id1)
	}
}
