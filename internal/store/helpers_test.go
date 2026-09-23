package store

import "testing"

// openTestStore opens a Store in a temp directory and closes it when the test
// finishes. Closing through t.Cleanup rather than defer keeps the close error
// visible instead of discarded, which is what errcheck is asking for.
func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("Open() error: %v", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close() error: %v", err)
		}
	})
	return s
}
