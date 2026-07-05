package dropbox

import (
	"errors"
	"testing"
	"time"
)

type fakeLister struct {
	entries []Entry
	err     error
	calls   int
}

func (f *fakeLister) ListFolder(path string, recursive bool) ([]Entry, error) {
	f.calls++
	return f.entries, f.err
}

func TestCachedListerCachesWithinTTL(t *testing.T) {
	fake := &fakeLister{entries: []Entry{{Path: "/a.md"}}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCachedLister(fake, 10*time.Minute)
	c.Now = func() time.Time { return now }

	for range 3 {
		entries, err := c.ListFolder("/vault", true)
		if err != nil {
			t.Fatalf("ListFolder: %v", err)
		}
		if len(entries) != 1 || entries[0].Path != "/a.md" {
			t.Fatalf("unexpected entries: %+v", entries)
		}
	}

	if fake.calls != 1 {
		t.Fatalf("expected underlying lister to be called once, got %d", fake.calls)
	}
}

func TestCachedListerRefreshesAfterTTL(t *testing.T) {
	fake := &fakeLister{entries: []Entry{{Path: "/a.md"}}}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := NewCachedLister(fake, 10*time.Minute)
	c.Now = func() time.Time { return now }

	if _, err := c.ListFolder("/vault", true); err != nil {
		t.Fatalf("ListFolder: %v", err)
	}

	now = now.Add(11 * time.Minute)
	if _, err := c.ListFolder("/vault", true); err != nil {
		t.Fatalf("ListFolder: %v", err)
	}

	if fake.calls != 2 {
		t.Fatalf("expected underlying lister to be called again after TTL, got %d calls", fake.calls)
	}
}

func TestCachedListerCachesPerPathIndependently(t *testing.T) {
	fake := &fakeLister{entries: []Entry{{Path: "/a.md"}}}
	c := NewCachedLister(fake, 10*time.Minute)

	if _, err := c.ListFolder("/vault", true); err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	if _, err := c.ListFolder("/vault/Clippings", true); err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	if _, err := c.ListFolder("/vault", true); err != nil {
		t.Fatalf("ListFolder: %v", err)
	}

	if fake.calls != 2 {
		t.Fatalf("expected 2 underlying calls (one per distinct path), got %d", fake.calls)
	}
}

func TestCachedListerDoesNotCacheErrors(t *testing.T) {
	fake := &fakeLister{err: errors.New("dropbox unavailable")}
	c := NewCachedLister(fake, 10*time.Minute)

	if _, err := c.ListFolder("/vault", true); err == nil {
		t.Fatal("expected error to propagate")
	}
	if _, err := c.ListFolder("/vault", true); err == nil {
		t.Fatal("expected error to propagate on retry too")
	}
	if fake.calls != 2 {
		t.Fatalf("expected a failed lookup to retry rather than being cached, got %d calls", fake.calls)
	}
}
