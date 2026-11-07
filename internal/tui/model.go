package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

// view is one top-level pane.
type view int

// Panes.
const (
	viewDash view = iota
	viewTopics
	viewMap
	viewPublish
	viewCount
)

func (v view) title() string {
	switch v {
	case viewDash:
		return "telemetry"
	case viewTopics:
		return "topics"
	case viewMap:
		return "lidar map"
	case viewPublish:
		return "publish"
	}
	return "?"
}

// Options configures a Model.
type Options struct {
	Addr      string
	Interval  time.Duration
	NoPublish bool
	Version   string
	ClientID  string
}

type tickMsg time.Time
type reconnectMsg error
type sentMsg struct {
	text string
	err  bool
}

const (
	histLen   = 120
	toastLife = 4 * time.Second
)

// Model is the whole UI state.
type Model struct {
	opts   Options
	client *broker.Client

	w, h   int
	v      view
	help   bool
	quit   bool
	splash bool

	status  broker.Status
	errText string

	lastSeq    int64
	latest     *qfeel.Telemetry
	frames     int64
	prevTick   uint64
	cadenceMs  float64
	cadenceOK  bool
	batteryHst []float64
	rateHst    []float64
	grid       *qfeel.Grid

	topicCursor   int
	topicSelected bool
	filter        string
	filterMode    bool
	followLog     bool
	logOffset     int

	actionCursor int
	inflight     bool
	editing      int // 0 none, 1 topic field, 2 payload field
	topicInput   textinput.Model
	payloadInput textinput.Model

	toast    string
	toastErr bool
	toastAt  time.Time
}

// New builds a Model. Init dials the broker.
func New(opts Options) Model {
	ti := textinput.New()
	ti.SetValue(qfeel.TopicInput)
	ti.CharLimit = 128
	ti.Prompt = "topic  "
	ti.Width = 40

	pi := textinput.New()
	pi.SetValue(qfeel.CmdPause)
	pi.CharLimit = 4096
	pi.Prompt = "b64    "
	pi.Width = 40

	m := Model{
		opts:         opts,
		client:       broker.New(broker.Options{Addr: opts.Addr, ClientID: opts.ClientID}),
		followLog:    true,
		splash:       true,
		topicInput:   ti,
		payloadInput: pi,
		status:       broker.StatusDown,
	}
	return m
}

func (m *Model) connect() tea.Cmd {
	client := m.client
	clientID := m.opts.ClientID
	return func() tea.Msg {
		err := client.Start()
		if err == nil && clientID == "" {
			// nothing else to do; the id was already generated on the client
		}
		return reconnectMsg(err)
	}
}

// Init starts the refresh ticker and dials the broker.
func (m Model) Init() tea.Cmd {
	return tea.Batch(tick(m.opts.Interval), m.connect())
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height

	case reconnectMsg:
		if err := error(msg); err != nil {
			m.setToast(err.Error(), true)
		}

	case sentMsg:
		m.inflight = false
		m.setToast(msg.text, msg.err)

	case tickMsg:
		m.refresh()
		cmds = append(cmds, tick(m.opts.Interval))

	case tea.KeyMsg:
		if m.splash {
			// The card is informational; any key moves on.
			m.splash = false
			return m, nil
		}
		if m.help {
			m.help = false
			return m, nil
		}
		if m.editing > 0 {
			cmds = append(cmds, m.updateEditor(msg))
			return m, tea.Batch(cmds...)
		}
		if m.filterMode {
			cmds = append(cmds, m.updateFilter(msg))
			return m, tea.Batch(cmds...)
		}
		cmds = append(cmds, m.onKey(msg))
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) onKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "q", "ctrl+c":
		m.quit = true
		m.client.Close()
		return tea.Quit
	case "?":
		m.help = true
	case "1":
		m.v = viewDash
	case "2":
		m.v = viewTopics
	case "3":
		m.v = viewMap
	case "4":
		m.v = viewPublish
	case "tab":
		m.v = (m.v + 1) % viewCount
	case "shift+tab":
		m.v = (m.v + viewCount - 1) % viewCount
	case "r":
		m.setToast("reconnecting to "+m.opts.Addr, false)
		return m.connect()
	}

	switch m.v {
	case viewDash:
		return m.dashKey(k)
	case viewTopics:
		return m.topicKey(k)
	case viewPublish:
		return m.publishKey(k)
	}
	return nil
}

func (m *Model) dashKey(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "p":
		return m.fire("pause")
	case "s":
		return m.fire("play")
	case "d":
		return m.fire("dock")
	case "H":
		return m.fire("heartbeat-a")
	}
	return nil
}

func (m *Model) topicKey(k tea.KeyMsg) tea.Cmd {
	stats := m.statTopics()
	switch k.String() {
	case "j", "down":
		if m.topicCursor < len(stats)-1 {
			m.topicCursor++
			m.topicSelected = true
			m.logOffset = 0
		}
	case "k", "up":
		if m.topicCursor > 0 {
			m.topicCursor--
			m.topicSelected = true
			m.logOffset = 0
		}
	case "a":
		m.topicSelected = false
		m.logOffset = 0
	case "J":
		m.logOffset++
		m.followLog = false
	case "K":
		if m.logOffset > 0 {
			m.logOffset--
		}
	case "g":
		m.logOffset = 0
		m.followLog = true
	case "f":
		m.followLog = !m.followLog
	case "/":
		m.filterMode = true
	}
	return nil
}

func (m *Model) publishKey(k tea.KeyMsg) tea.Cmd {
	acts := qfeel.Actions()
	switch k.String() {
	case "j", "down":
		if m.actionCursor < len(acts)-1 {
			m.actionCursor++
		}
	case "k", "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "enter":
		if len(acts) > 0 {
			return m.fire(acts[m.actionCursor].Key)
		}
	case "e":
		m.editing = 1
		m.topicInput.Focus()
		m.payloadInput.Blur()
	}
	return nil
}

// updateEditor drives the free-form publish editor. Tab moves between the topic
// and payload fields, esc leaves, ctrl+s sends.
func (m *Model) updateEditor(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.editing = 0
		return nil
	case "tab":
		if m.editing == 1 {
			m.editing = 2
			m.topicInput.Blur()
			m.payloadInput.Focus()
		} else {
			m.editing = 1
			m.payloadInput.Blur()
			m.topicInput.Focus()
		}
		return nil
	case "ctrl+s":
		m.editing = 0
		return m.send(m.topicInput.Value(), []byte(m.payloadInput.Value()))
	}
	var cmd tea.Cmd
	if m.editing == 1 {
		m.topicInput, cmd = m.topicInput.Update(k)
	} else {
		m.payloadInput, cmd = m.payloadInput.Update(k)
	}
	return cmd
}

func (m *Model) updateFilter(k tea.KeyMsg) tea.Cmd {
	switch k.String() {
	case "esc":
		m.filterMode = false
	case "enter":
		m.filterMode = false
	case "backspace":
		// Runes, not bytes, so a non-ASCII filter does not shred into garbage.
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
	default:
		if len(k.Runes) > 0 {
			m.filter += string(k.Runes)
		}
	}
	return nil
}

// fire publishes one named action.
func (m *Model) fire(key string) tea.Cmd {
	a, ok := qfeel.ActionByKey(key)
	if !ok {
		return nil
	}
	// One frame at a time: paho publishes asynchronously at QoS 0, and holding a
	// key otherwise queues a burst of identical frames at the robot.
	if m.inflight {
		m.setToast("still sending, wait a beat", true)
		return nil
	}
	if m.opts.NoPublish {
		m.setToast("dry run: "+a.Title+" not sent", true)
		return nil
	}
	return m.send(a.Target(), []byte(a.Payload))
}

func (m *Model) send(topic string, payload []byte) tea.Cmd {
	m.inflight = true
	client := m.client
	t, p := topic, string(payload)
	return func() tea.Msg {
		if err := client.Publish(t, []byte(p)); err != nil {
			return sentMsg{text: fmt.Sprintf("publish failed: %v", err), err: true}
		}
		client.Published(t, []byte(p))
		return sentMsg{text: fmt.Sprintf("sent %s to %s, watching state field 4", clip(p, 24), t), err: false}
	}
}

func (m *Model) setToast(text string, isErr bool) {
	m.toast = text
	m.toastErr = isErr
	m.toastAt = time.Now()
}

// refresh pulls link state and traffic, decodes any new telemetry frame, and
// appends to the short history windows the panes draw.
func (m *Model) refresh() {
	m.status, m.errText = m.client.Status()
	total, rate := m.client.Totals()
	m.frames = total

	if m.toast != "" && time.Since(m.toastAt) > toastLife {
		m.toast = ""
	}

	msgs := m.client.Snapshot()
	var newest *broker.Msg
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Seq <= m.lastSeq {
			break
		}
		if msgs[i].Topic == qfeel.TopicOutput && newest == nil {
			m := msgs[i]
			newest = &m
		}
	}
	if len(msgs) > 0 && msgs[len(msgs)-1].Seq > m.lastSeq {
		m.lastSeq = msgs[len(msgs)-1].Seq
	}

	if newest != nil {
		if t, err := qfeel.Decode(string(newest.Payload)); err == nil {
			m.observe(t)
		}
	}

	m.rateHst = appendCapped(m.rateHst, rate)
}

func (m *Model) observe(t *qfeel.Telemetry) {
	if m.prevTick != 0 && t.Tick > m.prevTick {
		d := float64(t.Tick - m.prevTick)
		// A single frame gap is 201 ms. Jitter of a few ms is normal; a much
		// larger step means dropped frames, and a much smaller one means the
		// counter is not the frame clock we think it is.
		m.cadenceOK = d > 190 && d < 212
		m.cadenceMs = d
	}
	if t.Tick != 0 {
		m.prevTick = t.Tick
	}
	if t.HasBattery {
		m.batteryHst = appendCapped(m.batteryHst, float64(t.Battery))
	}
	if t.Grid != nil {
		m.grid = t.Grid
	}
	m.latest = t
}

func appendCapped(v []float64, x float64) []float64 {
	v = append(v, x)
	if len(v) > histLen {
		v = v[len(v)-histLen:]
	}
	return v
}

// statTopics is the filtered, non-empty topic list.
func (m *Model) statTopics() []broker.TopicStat {
	all := m.client.Stats()
	if m.filter == "" {
		return all
	}
	needle := strings.ToLower(m.filter)
	out := make([]broker.TopicStat, 0, len(all))
	for _, s := range all {
		if strings.Contains(strings.ToLower(s.Topic), needle) {
			out = append(out, s)
		}
	}
	return out
}

// selectedTopic is the topic the user pinned the log to, or "" while the log
// shows every topic. The list is sorted by traffic, so a cursor alone would
// silently hide topics as the busiest one changes under it.
func (m *Model) selectedTopic() string {
	if !m.topicSelected {
		return ""
	}
	stats := m.statTopics()
	if m.topicCursor >= 0 && m.topicCursor < len(stats) {
		return stats[m.topicCursor].Topic
	}
	return ""
}

// clip shortens s to at most n runes including the ellipsis, so callers can
// budget a fixed width and land on it exactly.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n || n <= 0 {
		return s
	}
	if n == 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}
