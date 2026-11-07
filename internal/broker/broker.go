// Package broker wraps an MQTT client for a Shark robot vacuum's local
// broker: anonymous TCP, no TLS, no ACL, QoS 0.
//
// The broker is a bulletin board anyone on the network can read and write, so
// this client subscribes to the whole tree and keeps a bounded recent-message
// ring for the UI to draw. It never retries a publish and never invents an ack:
// the only confirmation that a command landed is the robot's own telemetry
// changing, or the command coming back in the field 16 echo.
package broker

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Status is the link state shown in the UI header.
type Status int

// Link states.
const (
	StatusDown Status = iota
	StatusConnecting
	StatusUp
)

// String names the state for the header.
func (s Status) String() string {
	switch s {
	case StatusUp:
		return "connected"
	case StatusConnecting:
		return "connecting"
	}
	return "down"
}

// Msg is one received publish.
type Msg struct {
	Seq     int64
	Topic   string
	Payload []byte
	At      time.Time
	Local   bool // written by us rather than received
}

// TopicStat is the per-topic rollup the topic pane draws.
type TopicStat struct {
	Topic     string
	Count     int
	Bytes     int
	Last      time.Time
	LastBytes int
	Rate      float64 // messages per second over the last 10s

	recent []time.Time
}

// Options configures a Client.
type Options struct {
	Addr      string        // host:port
	ClientID  string        // empty picks a random-ish id
	KeepAlive time.Duration // 0 uses 15s
	Buffer    int           // recent-message ring size, 0 uses 4096
}

// Client is a live subscription plus a bounded message ring.
type Client struct {
	opts Options

	mu      sync.Mutex
	ring    []Msg
	next    int
	seq     int64
	full    bool
	dropped int64
	stats   map[string]*TopicStat
	recent  []time.Time
	status  Status
	errText string

	paho mqtt.Client
	done chan struct{}
	once sync.Once
}

// ErrClosed reports a publish attempted after Close.
var ErrClosed = errors.New("broker: client closed")

// New builds a client. Nothing connects until Start.
func New(opts Options) *Client {
	if opts.Buffer <= 0 {
		opts.Buffer = 4096
	}
	if opts.KeepAlive <= 0 {
		opts.KeepAlive = 15 * time.Second
	}
	return &Client{
		opts:   opts,
		ring:   make([]Msg, opts.Buffer),
		stats:  make(map[string]*TopicStat),
		status: StatusDown,
		done:   make(chan struct{}),
	}
}

// Addr reports the broker address.
func (c *Client) Addr() string { return c.opts.Addr }

// Start dials the broker and subscribes to "#". It returns once the TCP dial
// has been attempted; reconnects are handled by the MQTT library afterwards.
func (c *Client) Start() error {
	host, _, err := net.SplitHostPort(c.opts.Addr)
	if err != nil {
		return fmt.Errorf("broker: bad address %q: %w", c.opts.Addr, err)
	}
	if err := probe(host); err != nil {
		return err
	}

	o := mqtt.NewClientOptions().
		AddBroker("tcp://" + c.opts.Addr).
		SetClientID(c.opts.ClientID).
		SetCleanSession(true).
		SetKeepAlive(c.opts.KeepAlive).
		SetConnectTimeout(5 * time.Second).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(10 * time.Second).
		SetConnectRetry(false).
		SetOrderMatters(false)

	o.SetOnConnectHandler(func(cl mqtt.Client) {
		c.setStatus(StatusUp, "")
		if tok := cl.Subscribe("#", 0, c.onMessage); tok.Wait() && tok.Error() != nil {
			c.setStatus(StatusDown, tok.Error().Error())
		}
	})
	o.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		c.setStatus(StatusDown, err.Error())
	})

	c.paho = mqtt.NewClient(o)
	c.setStatus(StatusConnecting, "")
	tok := c.paho.Connect()
	tok.Wait()
	if err := tok.Error(); err != nil {
		c.setStatus(StatusDown, err.Error())
		return fmt.Errorf("broker: connect %s: %w", c.opts.Addr, err)
	}
	return nil
}

// probe does a plain TCP dial first, because paho's dial error text is vague and
// a wrong IP is by far the most common failure here.
func probe(host string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, "1883"), 3*time.Second)
	if err != nil {
		return fmt.Errorf("broker: %s:1883 unreachable (wrong IP, powered off, or off the network?)", host)
	}
	return conn.Close()
}

func (c *Client) onMessage(_ mqtt.Client, m mqtt.Message) {
	payload := append([]byte(nil), m.Payload()...)
	now := time.Now()

	c.mu.Lock()
	c.seq++
	msg := Msg{Seq: c.seq, Topic: m.Topic(), Payload: payload, At: now}
	c.ring[c.next] = msg
	c.next = (c.next + 1) % len(c.ring)
	if c.next == 0 {
		c.full = true
	}
	st := c.stats[m.Topic()]
	if st == nil {
		st = &TopicStat{Topic: m.Topic()}
		c.stats[m.Topic()] = st
	}
	st.Count++
	st.Bytes += len(payload)
	st.Last = now
	st.LastBytes = len(payload)
	st.recent = append(st.recent, now)
	if len(st.recent) > 256 {
		st.recent = st.recent[len(st.recent)-256:]
	}
	c.recent = append(c.recent, now)
	if len(c.recent) > 2048 {
		c.recent = c.recent[len(c.recent)-2048:]
	}
	c.mu.Unlock()
}

// Publish sends a payload at QoS 0 with no retain, as the app does.
func (c *Client) Publish(topic string, payload []byte) error {
	if c.paho == nil || !c.paho.IsConnectionOpen() {
		return errors.New("broker: not connected")
	}
	select {
	case <-c.done:
		return ErrClosed
	default:
	}
	tok := c.paho.Publish(topic, 0, false, payload)
	tok.Wait()
	return tok.Error()
}

// Published logs a local write into the ring so it shows up beside the traffic
// it causes. The robot does not echo inbound frames on the wire, so without
// this the command pane has nothing to point at.
func (c *Client) Published(topic string, payload []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	c.ring[c.next] = Msg{Seq: c.seq, Topic: topic, Payload: payload, At: time.Now(), Local: true}
	c.next = (c.next + 1) % len(c.ring)
	if c.next == 0 {
		c.full = true
	}
}

// Snapshot returns the ring oldest-first.
func (c *Client) Snapshot() []Msg {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := c.next
	if c.full {
		n = len(c.ring)
	}
	out := make([]Msg, 0, n)
	if c.full {
		out = append(out, c.ring[c.next:]...)
		out = append(out, c.ring[:c.next]...)
	} else {
		out = append(out, c.ring[:n]...)
	}
	return out
}

// Stats returns per-topic rollups sorted by traffic, most first. Rate is
// recomputed per call over the last 10 seconds of arrivals.
func (c *Client) Stats() []TopicStat {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Second)
	out := make([]TopicStat, 0, len(c.stats))
	for _, st := range c.stats {
		cp := *st
		var n int
		for _, at := range st.recent {
			if at.After(cutoff) {
				n++
			}
		}
		cp.Rate = float64(n) / 10
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Topic < out[j].Topic
		}
		return out[i].Count > out[j].Count
	})
	return out
}

// Totals reports total received messages, bytes and the broker-wide arrival
// rate observed over the last 10 seconds.
func (c *Client) Totals() (msgs int64, rate float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := time.Now().Add(-10 * time.Second)
	var n int
	for _, at := range c.recent {
		if at.After(cutoff) {
			n++
		}
	}
	return c.seq, float64(n) / 10
}

// Dropped reports messages lost to ring pressure.
func (c *Client) Dropped() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dropped
}

// Status reports link state and the last error.
func (c *Client) Status() (Status, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status, c.errText
}

func (c *Client) setStatus(s Status, errText string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = s
	if errText != "" {
		c.errText = errText
	}
}

// Close disconnects and stops accepting publishes.
func (c *Client) Close() {
	c.once.Do(func() { close(c.done) })
	if c.paho != nil && c.paho.IsConnectionOpen() {
		c.paho.Disconnect(150)
	}
}
