// Command sharkboard is a terminal console for a Shark robot vacuum's local
// MQTT broker: it watches what the robot publishes and offers the three
// captured command frames back.
//
// The broker is anonymous, unencrypted and has no ACL, so the tool assumes
// nothing about auth and everything about the device being on the same LAN.
// See https://github.com/jcpsimmons/shark-vacuum-teardown for the protocol.
package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
	"github.com/jcpsimmons/sharkboard/internal/tui"
)

const version = "0.1.0"

func main() {
	var (
		host     = flag.String("host", "", "broker address or IP (default: probe the LAN for port 1883)")
		port     = flag.Int("port", 1883, "broker port")
		interval = flag.Duration("interval", 350*time.Millisecond, "redraw interval")
		dryRun   = flag.Bool("dry-run", false, "watch only, publish nothing")
		once     = flag.Bool("once", false, "print one decoded frame and exit (no TUI)")
		timeout  = flag.Duration("wait", 20*time.Second, "how long to wait for a frame in -once mode")
		showVer  = flag.Bool("version", false, "print version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("sharkboard", version)
		return
	}

	addr := *host
	if addr != "" && !strings.Contains(addr, ":") {
		addr = net.JoinHostPort(addr, fmt.Sprint(*port))
	}

	if addr == "" {
		fmt.Fprintln(os.Stderr, "probing the LAN for an open 1883...")
		found, err := discover(4 * time.Second)
		if err != nil {
			fmt.Fprintln(os.Stderr, "no broker found:", err)
			os.Exit(1)
		}
		addr = found
		fmt.Fprintln(os.Stderr, "found broker at", addr)
	}

	if *once {
		if err := printOne(addr, *timeout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	m, cmd := tui.New(tui.Options{
		Addr:      addr,
		Interval:  *interval,
		NoPublish: *dryRun,
		Version:   version,
	})
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "sharkboard:", err)
		os.Exit(1)
	}
	_ = cmd
}

// discover scans the local /24s for anything answering on 1883 and keeps the
// first candidate that actually publishes /qfeel traffic. The vacuum has DHCP'd
// to .78 and .79 on different days, so a hardcoded IP rots fast.
func discover(perHost time.Duration) (string, error) {
	hosts := localSubnetHosts()
	if len(hosts) == 0 {
		return "", fmt.Errorf("no IPv4 interface with a /24")
	}

	var (
		mu   sync.Mutex
		open []string
		wg   sync.WaitGroup
	)
	sem := make(chan struct{}, 64)
	for _, h := range hosts {
		wg.Add(1)
		go func(h string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			c, err := net.DialTimeout("tcp", net.JoinHostPort(h, "1883"), perHost)
			if err != nil {
				return
			}
			_ = c.Close()
			mu.Lock()
			open = append(open, h)
			mu.Unlock()
		}(h)
	}
	wg.Wait()

	// Prefer hosts that speak /qfeel over hosts that merely have 1883 open.
	for _, h := range open {
		if speaks(addr(h), 3*time.Second) {
			return addr(h), nil
		}
	}
	if len(open) > 0 {
		sort.Strings(open)
		return addr(open[0]), nil
	}
	return "", fmt.Errorf("nothing answered on port 1883")
}

func addr(h string) string { return net.JoinHostPort(h, "1883") }

func speaks(a string, wait time.Duration) bool {
	c := broker.New(broker.Options{Addr: a})
	defer c.Close()
	if err := c.Start(); err != nil {
		return false
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		for _, m := range c.Snapshot() {
			if m.Topic == qfeel.TopicOutput || m.Topic == qfeel.TopicInput {
				return true
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

// localSubnetHosts lists .1-.254 for every non-loopback IPv4 /24.
func localSubnetHosts() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil || len(ipnet.Mask) != 4 || ipnet.Mask[3] != 0 {
				continue
			}
			if seen[ip4.String()] {
				continue
			}
			seen[ip4.String()] = true
			base := ip4.Mask(ipnet.Mask)
			for i := 1; i < 255; i++ {
				out = append(out, net.IPv4(base[0], base[1], base[2], byte(i)).String())
			}
		}
	}
	return out
}

// printOne connects, waits for one telemetry frame, dumps it labelled, and
// exits. Useful for a smoke test without a terminal.
func printOne(a string, wait time.Duration) error {
	c := broker.New(broker.Options{Addr: a})
	if err := c.Start(); err != nil {
		return err
	}
	defer c.Close()

	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		for _, m := range c.Snapshot() {
			if m.Topic != qfeel.TopicOutput {
				continue
			}
			t, err := qfeel.Decode(string(m.Payload))
			if err != nil {
				continue
			}
			fmt.Printf("%s %s  (%d bytes)\n", m.At.Format("15:04:05.000"), m.Topic, len(m.Payload))
			for _, line := range t.Snapshot().Lines {
				name := line.Name
				if !line.Mapped {
					name += "  (unmapped)"
				}
				fmt.Printf("  %-13s %s\n", name, line.Value)
			}
			if t.Grid != nil {
				fmt.Printf("  grid %dx%d, %d cells, mismatch=%v\n", t.Grid.Width, t.Grid.Height, len(t.Grid.Cells), t.Grid.LengthMismatch)
			}
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("no /qfeel/PbOutput frame within %s", wait)
}
