package tui

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

var update = flag.Bool("update", false, "rewrite testdata goldens")

// stampRE matches the clock column of a log line.
var stampRE = regexp.MustCompile(`\d\d:\d\d:\d\d\.\d\d\d`)

// fixture builds a Model with a real captured frame and no live connection, so
// the panes can be rendered deterministically.
func fixture(t *testing.T, frame string) Model {
	t.Helper()
	m := New(Options{Addr: "192.168.1.79:1883", Version: "9.9.9"})
	m.w, m.h = 120, 38
	m.splash = false // the card is tested on its own
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
	// Message timestamps are wall-clock; normalise them so a golden does not
	// expire the moment it is written.
	if stampRE.MatchString(got) || stampRE.MatchString(string(want)) {
		got = stampRE.ReplaceAllString(got, "00:00:00.000")
		want = []byte(stampRE.ReplaceAllString(string(want), "00:00:00.000"))
	}
	if string(want) != got {
		t.Errorf("%s drifted from golden:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// Discovery must not run before the card is up: with no address, the card
// paints and the sweep happens behind it.
func TestSplashShowsWhileProbing(t *testing.T) {
	m := New(Options{Version: "9.9.9", Discover: func() (string, error) { return "10.0.0.5:1883", nil }})
	m.w, m.h = 100, 30
	if !m.splash || !m.probing {
		t.Fatalf("splash=%v probing=%v, want both set before any command runs", m.splash, m.probing)
	}
	if out := m.View(); !strings.Contains(out, "looking for a broker") {
		t.Error("card does not say what it is doing")
	}

	// The init command batch carries the dial; it should only report an
	// address, not connect yet.
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatalf("init did not batch (got %T)", m.Init()())
	}
	var lm linkMsg
	for _, c := range batch {
		out, ok := c().(linkMsg)
		if ok {
			lm = out
		}
	}
	if lm.err != nil || lm.addr != "10.0.0.5:1883" {
		t.Fatalf("linkMsg = %+v", lm)
	}
	nm, _ := m.Update(lm)
	m = nm.(Model)
	if m.probing {
		t.Error("still probing after the address arrived")
	}
	if m.addr != "10.0.0.5:1883" {
		t.Errorf("addr = %q", m.addr)
	}
}

func TestSplashCardOnLoad(t *testing.T) {
	m := New(Options{Addr: "192.168.1.79:1883", Version: "9.9.9"})
	m.w, m.h = 100, 30
	if !m.splash {
		t.Fatal("splash is not shown on load")
	}
	out := m.View()
	golden(t, "splash.txt", out)
	for _, want := range []string{"another fine release by DrJ", "youtube.com/@drjoshcsimmons", "drjoshcsimmons.com", "sharkboard 9.9.9"} {
		if !strings.Contains(out, want) {
			t.Errorf("splash missing %q", want)
		}
	}
	// OSC 8, so the links are clickable where the terminal supports it.
	if !strings.Contains(out, "\x1b]8;;https://youtube.com/@drjoshcsimmons") {
		t.Error("youtube link is not a hyperlink")
	}

	// Any key dismisses it; the next key acts on the pane.
	nm, _ := m.Update(keyMsg("x"))
	m = nm.(Model)
	if m.splash {
		t.Error("splash survived a keypress")
	}
	if !strings.Contains(m.View(), "state (4)") {
		t.Error("dashboard did not come up after the splash")
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

func TestTopicsPaneRenders(t *testing.T) {
	m := fixture(t, "")
	b, err := os.ReadFile(filepath.Join("..", "qfeel", "testdata", "frame-docked.b64"))
	if err != nil {
		t.Fatal(err)
	}
	m.client.Published("/qfeel/PbOutput", b)
	m.client.Published("/viper/wifiIn", []byte("WIFISecondaryCloudUp:true"))
	m.client.Published("/qfeel/PbInput", []byte("gAEI"))
	m.v = viewTopics
	out := m.View()
	golden(t, "topics.txt", out)

	for _, want := range []string{"topics by traffic", "messages", "/qfeel/PbOutput", "WIFISecondaryCloudUp:true"} {
		if !strings.Contains(out, want) {
			t.Errorf("topics pane missing %q", want)
		}
	}
	// The wifi channel is plaintext, so it is shown as text, not decoded.
	if !strings.Contains(out, "state=14") {
		t.Error("telemetry preview is not summarised")
	}
}

// The selected command row has to be a full-width bar, not a ragged highlight.
func TestPublishSelectionIsFullWidth(t *testing.T) {
	m := fixture(t, "")
	m.v = viewPublish
	rows := strings.Split(m.View(), "\n")
	var widths []int
	for _, row := range rows {
		if strings.Contains(row, "Pause") || strings.Contains(row, "Dock") {
			widths = append(widths, lipgloss.Width(row)-lipgloss.Width(strings.TrimSpace(row)))
		}
	}
	if len(widths) < 2 {
		t.Fatalf("found %d command rows", len(widths))
	}
	if widths[0] != widths[1] {
		t.Errorf("selected row padding %d differs from unselected %d", widths[0], widths[1])
	}

	// Every line of the pane has to be the same width, or the bar and the border
	// come out ragged.
	seen := map[int]bool{}
	for _, row := range rows {
		if !strings.HasPrefix(row, "│") {
			continue
		}
		seen[lipgloss.Width(row)] = true
	}
	if len(seen) > 1 {
		t.Errorf("pane rows have %d distinct widths: %v", len(seen), seen)
	}
}

func TestPublishPaneMarksRiskyWrites(t *testing.T) {
	m := fixture(t, "")
	m.v = viewPublish
	out := m.View()
	if !strings.Contains(out, "▲") {
		t.Error("risky actions are not marked")
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
	if !strings.Contains(out, "No ack") {
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
