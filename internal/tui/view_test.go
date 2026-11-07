package tui

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

var update = flag.Bool("update", false, "rewrite testdata goldens")

// fixture builds a Model with a real captured frame and no live connection, so
// the panes can be rendered deterministically.
func fixture(t *testing.T, frame string) Model {
	t.Helper()
	m, _ := New(Options{Addr: "192.168.1.79:1883"})
	m.w, m.h = 120, 38
	if frame == "" {
		return m
	}
	b, err := os.ReadFile(filepath.Join("..", "qfeel", "testdata", frame))
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	tel, err := qfeel.Decode(string(b))
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	m.observe(tel)
	return m
}

// golden compares the rendered screen with testdata/<name>, rewriting it under
// -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update): %v", err)
	}
	if string(want) != got {
		t.Errorf("%s drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func TestDashboardRenders(t *testing.T) {
	m := fixture(t, "frame-scan.b64")
	if m.latest == nil {
		t.Fatal("frame did not attach")
	}
	out := m.View()
	golden(t, "dashboard.txt", out)

	for _, want := range []string{"sharkboard", "telemetry", "state (4)", "battery (9:8)", "newest frame"} {
		if !strings.Contains(out, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
}

func TestDashboardRendersWithoutFrames(t *testing.T) {
	m := fixture(t, "")
	out := m.View()
	for _, want := range []string{"no frame yet", "waiting for a /qfeel/PbOutput frame", "down"} {
		if !strings.Contains(out, want) {
			t.Errorf("empty dashboard missing %q", want)
		}
	}
}

func TestMapPaneRendersGrid(t *testing.T) {
	m := fixture(t, "frame-scan.b64")
	m.v = viewMap
	out := m.View()
	golden(t, "map.txt", out)

	if !strings.Contains(out, "lidar map") {
		t.Error("map pane has no title")
	}
	if !strings.Contains(out, "169x234 cells") {
		t.Error("map pane lost the grid dimensions")
	}
	if !strings.Contains(out, "robot ◉") {
		t.Error("map pane lost the legend")
	}
}

func TestMapPaneWithoutGrid(t *testing.T) {
	m := fixture(t, "frame-docked.b64")
	m.v = viewMap
	out := m.View()
	if !strings.Contains(out, "No map on the wire") {
		t.Error("empty map pane does not explain itself")
	}
}

func TestPublishPaneListsEveryAction(t *testing.T) {
	m := fixture(t, "")
	m.v = viewPublish
	out := m.View()
	for _, a := range qfeel.Actions() {
		if !strings.Contains(out, a.Title) {
			t.Errorf("publish pane missing action %q", a.Title)
		}
	}
	if !strings.Contains(out, "no ack") {
		t.Error("publish pane does not warn about the missing ack")
	}
}

func TestHelpOverlay(t *testing.T) {
	m := fixture(t, "")
	m.help = true
	out := m.View()
	if !strings.Contains(out, "keys") || !strings.Contains(out, "reconnect") {
		t.Error("help overlay incomplete")
	}
}

// The footer is the only place dry-run shows; it must not be silent.
func TestDryRunFooter(t *testing.T) {
	m := fixture(t, "")
	m.opts.NoPublish = true
	if !strings.Contains(m.View(), "DRY RUN") {
		t.Error("dry run not surfaced")
	}
}

func TestTabsSwitch(t *testing.T) {
	m := fixture(t, "")
	for key, want := range map[string]view{"2": viewTopics, "3": viewMap, "4": viewPublish, "1": viewDash} {
		nm, _ := m.Update(keyMsg(key))
		m = nm.(Model)
		if m.v != want {
			t.Errorf("key %s left view at %v, want %v", key, m.v, want)
		}
	}
}

func TestDryRunSwallowsPublish(t *testing.T) {
	m := fixture(t, "")
	m.opts.NoPublish = true
	m.client = broker.New(broker.Options{Addr: "127.0.0.1:1"})
	cmd := m.fire("dock")
	if cmd != nil {
		t.Error("dry run produced a publish command")
	}
	if !m.toastErr || !strings.Contains(m.toast, "dry run") {
		t.Errorf("toast = %q (err %v)", m.toast, m.toastErr)
	}
}

// keyMsg is a tiny helper so tests read like keystrokes.
func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}
