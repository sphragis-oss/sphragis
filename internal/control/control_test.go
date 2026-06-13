package control

import (
	"os"
	"testing"
)

func TestStateRoundTrip(t *testing.T) {
	t.Setenv("SPHRAGIS_HOME", t.TempDir())
	if err := SaveState(State{AutoAnchor: true, Interval: "6h"}); err != nil {
		t.Fatal(err)
	}
	st := LoadState()
	if !st.AutoAnchor || st.Interval != "6h" {
		t.Fatalf("round-trip failed: %+v", st)
	}
}

func TestLoadStateDefault(t *testing.T) {
	t.Setenv("SPHRAGIS_HOME", t.TempDir())
	st := LoadState()
	if st.AutoAnchor || st.Interval != "24h" {
		t.Fatalf("defaults wrong: %+v", st)
	}
}

func TestPIDAndRunning(t *testing.T) {
	t.Setenv("SPHRAGIS_HOME", t.TempDir())
	if _, ok := Running(); ok {
		t.Fatal("nothing should be running")
	}
	if err := WritePID(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	pid, ok := Running()
	if !ok || pid != os.Getpid() {
		t.Fatalf("expected current process running, got %d %v", pid, ok)
	}
	RemovePID()
	if _, ok := Running(); ok {
		t.Fatal("removed pid should not be running")
	}
}
