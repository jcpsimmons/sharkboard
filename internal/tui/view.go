package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
	"github.com/jcpsimmons/sharkboard/internal/render"
)

const minW, minH = 72, 20

// View draws the whole screen.
func (m Model) View() string {
	w, h := m.w, m.h
	if w < minW {
		w = minW
	}
	if h < minH {
		h = minH
	}

	header := m.header(w)
	footer := m.footer(w)

	body := ""
	switch {
	case m.splash:
		body = m.splashView(w, h-6)
	case m.help:
		body = m.helpView(w, h-6)
	case m.v == viewDash:
		body = m.dashView(w, h-6)
	case m.v == viewTopics:
		body = m.topicsView(w, h-6)
	case m.v == viewMap:
		body = m.mapView(w, h-6)
	case m.v == viewPublish:
		body = m.publishView(w, h-6)
	}

	return strings.Join([]string{header, body, footer}, "\n")
}

func (m Model) header(w int) string {
	left := title.Render("sharkboard") + "  " + addr.Render(m.opts.Addr)

	st := bad.Render("● down")
	switch m.status {
	case broker.StatusUp:
		st = good.Render("● connected")
	case broker.StatusConnecting:
		st = warn.Render("● connecting")
	}

	if dropped := m.client.Dropped(); dropped > 0 {
		st += dim.Render(fmt.Sprintf("  %d dropped", dropped))
	}

	tabs := make([]string, 0, viewCount)
	for i := view(0); i < viewCount; i++ {
		label := fmt.Sprintf("%d %s", int(i)+1, i.title())
		if i == m.v {
			tabs = append(tabs, tabActive.Render(label))
		} else {
			tabs = append(tabs, tab.Render(label))
		}
	}
	tabBar := strings.Join(tabs, "")

	leftW := lipgloss.Width(left) + lipgloss.Width(st) + 6
	gap := w - leftW - lipgloss.Width(tabBar)
	if gap < 1 {
		gap = 1
	}
	return left + "  " + st + strings.Repeat(" ", gap) + tabBar
}

func (m Model) footer(w int) string {
	if m.toast != "" {
		s := toastInfo
		if m.toastErr {
			s = toastErr
		} else {
			s = toastOK
		}
		return s.Render(clip(m.toast, w-2))
	}
	var hints []string
	switch m.v {
	case viewDash:
		hints = []string{"p pause", "s play", "d dock", "H heartbeat", "writes hit ALL robots", "2 topics", "3 map", "4 publish"}
	case viewTopics:
		hints = []string{"j/k pin topic", "a all", "J/K scroll", "f follow", "/ filter"}
	case viewMap:
		hints = []string{"needs a moving robot to fill", "2 topics", "4 publish"}
	case viewPublish:
		hints = []string{"j/k pick", "enter send", "e custom frame", "1 telemetry"}
	}
	if m.opts.NoPublish {
		hints = append([]string{"DRY RUN"}, hints...)
	}
	hints = append(hints, "? help", "q quit")
	return dim.Render(clip(strings.Join(hints, "  ·  "), w-2))
}

func (m Model) dashView(w, h int) string {
	colW := w * 2 / 5
	stateStr := dim.Render("no frame yet")
	if m.latest != nil {
		label := m.latest.State.ShortLabel()
		stateStr = base.Render(label)
		switch m.latest.State {
		case qfeel.StateRunning, qfeel.StateDocking:
			stateStr = warn.Render("▶ " + label)
		case qfeel.StateIdleDock:
			stateStr = good.Render("● " + label)
		case qfeel.StateStopped:
			stateStr = bad.Render("■ " + label)
		}
	}

	batt := dim.Render("n/a")
	if m.latest != nil && m.latest.HasBattery {
		batt = base.Render(fmt.Sprintf("%d%%", m.latest.Battery))
		if m.latest.Battery <= 20 {
			batt = bad.Render(fmt.Sprintf("%d%%", m.latest.Battery))
		}
	}
	battHist := dim.Render("")
	if len(m.batteryHst) > 3 {
		battHist = addr.Render(render.Sparkline(m.batteryHst, min(len(m.batteryHst), 28)))
	}

	cadence := dim.Render("waiting for two frames")
	if m.cadenceMs > 0 {
		txt := fmt.Sprintf("%.0f ms/frame", m.cadenceMs)
		if m.cadenceOK {
			cadence = good.Render(txt + "  (201 nominal)")
		} else {
			cadence = bad.Render(txt + "  (201 nominal, dropped frames?)")
		}
	}

	progress := dim.Render("n/a")
	if m.latest != nil && m.latest.Progress > 0 {
		progress = base.Render(fmt.Sprintf("%d", m.latest.Progress))
	}
	echo := dim.Render("none")
	if m.latest != nil && m.latest.EchoedCmd != "" {
		echo = local.Render(m.latest.EchoedCmd)
	}
	mapID := dim.Render("none")
	if m.latest != nil && m.latest.MapID != "" {
		mapID = base.Render(m.latest.MapID)
	}

	kv := func(k, v string) string {
		return dim.Render(padRight(k, 14)) + clip(v, colW-22)
	}

	left := panelTitle.Render("robot") + "\n" +
		kv("state (4)", stateStr) + "\n" +
		kv("battery (9:8)", batt) + "\n" +
		kv("battery trend", battHist) + "\n" +
		kv("progress (13)", progress) + "\n" +
		kv("cadence", cadence) + "\n" +
		kv("model (2)", modelStr(m)) + "\n" +
		kv("echo (16)", echo) + "\n" +
		kv("map label", mapID)

	total, rate := m.client.Totals()
	rateLine := dim.Render("idle")
	if rate > 0 {
		rateLine = addr.Render(fmt.Sprintf("%.1f msg/s", rate)) + dim.Render(fmt.Sprintf("  %d total", total))
	}
	spark := dim.Render("")
	if len(m.rateHst) > 2 {
		spark = addr.Render(render.Sparkline(m.rateHst, min(len(m.rateHst), 30)))
	}
	topics := m.statTopics()
	topicRows := make([]string, 0, 4)
	for i, s := range topics {
		if i >= 4 {
			break
		}
		topicRows = append(topicRows, dim.Render(padRight(clip(s.Topic, 22), 23))+fmt.Sprintf("%.1f/s", s.Rate))
	}
	if len(topicRows) == 0 {
		topicRows = append(topicRows, dim.Render("no traffic yet"))
	}

	right := panelTitle.Render("traffic") + "\n" +
		kv("arrival", rateLine) + "\n" +
		kv("history", spark) + "\n" +
		strings.Join(topicRows, "\n")

	// Field dump of the newest frame, labelled, with unmapped ones named xN.
	var dumpLines []string
	if m.latest != nil {
		for _, line := range m.latest.Snapshot().Lines {
			name := dim.Render(padRight(line.Name, 13))
			valW := max(10, (w-colW)/4)
			val := base.Render(clip(line.Value, valW))
			if !line.Mapped {
				name = faint.Render(padRight(line.Name, 13))
			}
			row := name + val
			if line.Note != "" {
				rest := w - colW - 8 - 13 - valW
				if rest > 8 {
					row += faint.Render("  " + clip(line.Note, rest))
				}
			}
			dumpLines = append(dumpLines, row)
		}
	} else {
		dumpLines = []string{dim.Render("waiting for a /qfeel/PbOutput frame")}
	}

	boxH := max(4, h-2)
	leftBox := panel.Width(colW - 4).Height(boxH).Render(left + "\n\n" + right)
	rightBox := panel.Width(w - colW - 4).Height(boxH).Render(panelTitle.Render("newest frame (labels are inferred)") + "\n" + strings.Join(dumpLines, "\n"))

	row := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
	return fit(row, h)
}

func modelStr(m Model) string {
	if m.latest == nil {
		return dim.Render("n/a")
	}
	return base.Render(fmt.Sprintf("%d", m.latest.Model))
}

func (m Model) topicsView(w, h int) string {
	stats := m.statTopics()
	listW := max(28, w/3)

	rows := make([]string, 0, len(stats))
	nameW := max(8, listW-18)
	for i, s := range stats {
		name := clip(s.Topic, nameW)
		plain := padRight(name, nameW) +
			fmt.Sprintf("%6.1f/s", s.Rate) +
			dim.Render(padLeft(fmt.Sprint(s.LastBytes), 6))
		if m.topicSelected && i == m.topicCursor {
			rows = append(rows, rowSel.Render(padRight(plain, listW-6)))
		} else {
			rows = append(rows, base.Render(plain))
		}
	}
	if len(rows) == 0 {
		rows = []string{dim.Render("no topics matching")}
	}

	filterLine := dim.Render("filter: ") + base.Render(m.filter)
	if m.filterMode {
		filterLine = addr.Render("filter: ") + base.Render(m.filter) + addr.Render("▏")
	}
	left := panel.Width(listW - 2).Render(panelTitle.Render("topics by traffic") + "\n" + filterLine + "\n\n" + strings.Join(rows, "\n"))

	rightW := w - listW - 2
	log := m.logView(rightW-ie(), h-6)
	head := "messages"
	if topic := m.selectedTopic(); topic != "" {
		head = "messages on " + clip(topic, rightW-20)
	}
	right := panel.Width(rightW - 2).Render(panelTitle.Render(head) + "\n" + log)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) mapView(w, h int) string {
	if m.grid == nil {
		box := panel.Width(w - 4).Render(
			panelTitle.Render("lidar map") + "\n\n" +
				base.Render("No map on the wire.") + "\n\n" +
				dim.Render("A docked robot publishes no scan bundle at all. The grid lives in") + "\n" +
				dim.Render("PbOutput field 7 -> 5, and only arrives while the robot is moving.") + "\n" +
				dim.Render("Send play (dash pane, s) or dock (d) and this fills in."),
		)
		return fit(box, h)
	}

	innerW, innerH := w-6, h-6
	lines := render.Grid(m.grid, innerW, innerH)
	counts := map[qfeel.CellKind]int{}
	for _, c := range m.grid.Cells {
		counts[qfeel.Classify(c)]++
	}
	ex, ey := m.grid.ExtentMetres()

	meta := fmt.Sprintf("%dx%d cells · %s/cell · %s x %s m · free %d · wall %d · fringe %d · unseen %d",
		m.grid.Width, m.grid.Height,
		rate3(m.grid.Resolution), rate1(ex), rate1(ey),
		counts[qfeel.CellFree], counts[qfeel.CellBlocked], counts[qfeel.CellFringe], counts[qfeel.CellUnseen],
	)
	if m.grid.LengthMismatch {
		meta += "  ·  LENGTH MISMATCH: stride is a guess"
	}
	if m.grid.HasPose {
		meta += fmt.Sprintf("  ·  pose %.2f,%.2f", m.grid.PoseX, m.grid.PoseY)
	}
	if n := len(m.latest.Ranges); n > 0 && m.latest != nil {
		meta += fmt.Sprintf("  ·  %d range arrays", n)
	}

	body := panelTitle.Render("lidar map") + dim.Render("  (live, unauthenticated, 5 Hz while moving)") + "\n" +
		dim.Render(clip(meta, w-4)) + "\n" +
		strings.Join(lines, "\n") + "\n" +
		dim.Render(render.Legend())

	return fit(panel.Width(w-2).Render(body), h)
}

func (m Model) publishView(w, h int) string {
	acts := qfeel.Actions()
	innerW := w - 8
	rows := make([]string, 0, len(acts))
	for i, a := range acts {
		mark := "  "
		if a.Risky {
			mark = warn.Render("▲ ")
		}
		line := mark + padRight(a.Title, 18) + dim.Render(clip(a.Hint, max(10, innerW-20)))
		// Pad to the panel's inner width so the highlight is a clean bar rather
		// than a ragged run of text, and so no hint wraps under a border.
		line = padRight(line, innerW)
		if i == m.actionCursor {
			rows = append(rows, rowSel.Render(line))
		} else {
			rows = append(rows, base.Render(line))
		}
	}

	note := dim.Render("QoS 0, no retain, base64 text on ") + addr.Render(clip(qfeel.TopicInput, 24)) + "\n" +
		dim.Render("No ack, and no device id in the frame: every robot on the LAN takes it.") + "\n" +
		dim.Render("Confirmation is the state field (4) moving, or the echo in field 16.")

	editor := ""
	if m.editing > 0 {
		editor = "\n\n" + panelTitle.Render("custom frame") + "\n" +
			m.topicInput.View() + "\n" + m.payloadInput.View() + "\n" +
			dim.Render("tab switch · ctrl+s send · esc cancel")
	}

	body := panelTitle.Render("commands") + dim.Render("  ▲ moves the robot") + "\n\n" +
		strings.Join(rows, "\n") + "\n\n" + note + editor
	return fit(panel.Width(w-2).Render(body), h)
}

// splashView is the load card: what this is, who made it, and clickable links.
func (m Model) splashView(w, h int) string {
	card := splashBox.Render(
		title.Render("sharkboard "+m.opts.Version) + "\n" +
			dim.Render("a wire console for a Shark robot vacuum") + "\n\n" +
			base.Render("another fine release by DrJ") + "\n\n" +
			dim.Render("youtube  ") + hyperlink("https://youtube.com/@drjoshcsimmons") + "\n" +
			dim.Render("web      ") + hyperlink("https://drjoshcsimmons.com") + "\n\n" +
			faint.Render("press any key"),
	)

	lines := strings.Split(card, "\n")
	cardW := 0
	for _, l := range lines {
		if n := lipgloss.Width(l); n > cardW {
			cardW = n
		}
	}
	padX := max(0, (w-cardW)/2)
	padY := max(0, (h-len(lines))/2)

	var b strings.Builder
	for i := 0; i < padY; i++ {
		b.WriteByte('\n')
	}
	for _, l := range lines {
		b.WriteString(strings.Repeat(" ", padX) + l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// hyperlink wraps a URL in an OSC 8 hyperlink where the terminal supports it and
// prints the URL as the label either way, so the card is useful in a plain pty.
func hyperlink(url string) string {
	return "\x1b]8;;" + url + "\x1b\\" + addr.Render(url) + "\x1b]8;;\x1b\\"
}

func (m Model) logView(w, h int) string {
	msgs := m.client.Snapshot()
	topic := m.selectedTopic()

	// Filter first, then take only the rows that fit, then decode. Decoding the
	// whole 4096-message ring on every keystroke is what makes this pane slow
	// once the buffer fills.
	filtered := make([]broker.Msg, 0, len(msgs))
	for _, msg := range msgs {
		if topic != "" && msg.Topic != topic {
			continue
		}
		filtered = append(filtered, msg)
	}

	if !m.followLog {
		end := len(filtered) - m.logOffset
		if end < 0 {
			end = 0
		}
		filtered = filtered[:end]
	}
	if len(filtered) > h {
		filtered = filtered[len(filtered)-h:]
	}

	rows := make([]string, 0, len(filtered))
	for _, msg := range filtered {
		rows = append(rows, m.logLine(msg, w))
	}
	if len(rows) == 0 {
		return dim.Render("nothing here yet")
	}
	return strings.Join(rows, "\n")
}

func (m Model) logLine(msg broker.Msg, w int) string {
	ts := msg.At.Format("15:04:05.000")
	tag := dim.Render(ts) + " "
	if msg.Local {
		tag = local.Render(ts+" ▲") + " "
	}
	topic := clip(msg.Topic, 22)
	head := tag + addr.Render(padRight(topic, 22)) + dim.Render(padLeft(fmt.Sprint(len(msg.Payload)), 6)) + " "

	prev, show := preview(msg, w-lipgloss.Width(head)-2)
	return head + prev + show
}

// preview makes a message body readable: telemetry is summarised by its
// labelled fields, everything else is quoted and clipped.
func preview(msg broker.Msg, budget int) (string, string) {
	if budget < 12 {
		budget = 12
	}
	if msg.Topic == qfeel.TopicOutput {
		if t, err := qfeel.Decode(string(msg.Payload)); err == nil {
			s := fmt.Sprintf("state=%d(%s) battery=%d tick=%d", uint32(t.State), t.State.ShortLabel(), t.Battery, t.Tick)
			if t.Grid != nil {
				s += fmt.Sprintf(" grid=%dx%d", t.Grid.Width, t.Grid.Height)
			}
			return base.Render(clip(s, budget)), ""
		}
		return bad.Render(clip(string(msg.Payload), budget)), ""
	}
	body := string(msg.Payload)
	if msg.Topic == qfeel.TopicInput {
		return local.Render(clip(body, budget)), ""
	}
	return dim.Render(clip(body, budget)), ""
}

func (m Model) helpView(w, h int) string {
	rows := [][2]string{
		{"1-4 / tab", "switch pane"},
		{"? ", "toggle this help"},
		{"q", "quit"},
		{"", ""},
		{"dash pane", ""},
		{"p / s / d", "pause / play / dock the robot"},
		{"H", "send one heartbeat frame"},
		{"", ""},
		{"topics pane", ""},
		{"j / k", "move topic cursor"},
		{"J / K", "scroll the log"},
		{"f / g", "toggle following / jump to newest"},
		{"/", "filter topics"},
		{"", ""},
		{"publish pane", ""},
		{"enter", "send selected frame"},
		{"e", "custom topic and payload"},
		{"", ""},
		{"r", "reconnect"},
	}
	var b strings.Builder
	b.WriteString(panelTitle.Render("keys") + "\n\n")
	for _, r := range rows {
		if r[0] == "" && r[1] == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(dim.Render(padRight(r[0], 14)) + base.Render(r[1]) + "\n")
	}
	b.WriteString("\n" + dim.Render("Payload labels are inferred from observation, not vendor docs."))
	return fit(panel.Width(min(58, w-4)).Render(b.String()), h)
}

// fit trims or pads a block to exactly h lines.
func fit(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-lipgloss.Width(s))
}

func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

// ie is the panel's inner-edge padding budget.
func ie() int { return 2 }

func rate3(f float32) string { return fmt.Sprintf("%.2f m", f) }
func rate1(f float64) string { return fmt.Sprintf("%.1f", f) }

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
