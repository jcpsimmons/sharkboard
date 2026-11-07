package broker

import (
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// fakeMessage is the minimum mqtt.Message the ring cares about.
type fakeMessage struct {
	topic   string
	payload []byte
}

func (f fakeMessage) Duplicate() bool   { return false }
func (f fakeMessage) Qos() byte         { return 0 }
func (f fakeMessage) Retained() bool    { return false }
func (f fakeMessage) Topic() string     { return f.topic }
func (f fakeMessage) MessageID() uint16 { return 0 }
func (f fakeMessage) Payload() []byte   { return f.payload }
func (f fakeMessage) Ack()              {}

var _ mqtt.Message = fakeMessage{}

func feed(c *Client, topic string, n int) {
	for i := 0; i < n; i++ {
		c.onMessage(nil, fakeMessage{topic: topic, payload: []byte("x")})
	}
}

func TestRingKeepsNewestAndBounded(t *testing.T) {
	c := New(Options{Addr: "x:1883", Buffer: 4})
	feed(c, "/qfeel/PbOutput", 10)

	got := c.Snapshot()
	if len(got) != 4 {
		t.Fatalf("snapshot = %d messages, want the 4-message buffer", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Seq != got[i-1].Seq+1 {
			t.Fatalf("snapshot is not in arrival order: %d then %d", got[i-1].Seq, got[i].Seq)
		}
	}
	if last := got[len(got)-1].Seq; last != 10 {
		t.Errorf("newest seq = %d, want 10", last)
	}
}

func TestPublishedIsFlagged(t *testing.T) {
	c := New(Options{Addr: "x:1883", Buffer: 4})
	c.Published("/qfeel/PbInput", []byte("gAEI"))
	got := c.Snapshot()
	if len(got) != 1 {
		t.Fatalf("snapshot = %d, want 1", len(got))
	}
	if !got[0].Local {
		t.Error("published message is not marked local")
	}
}

func TestStatsPerTopic(t *testing.T) {
	c := New(Options{Addr: "x:1883"})
	feed(c, "/qfeel/PbOutput", 5)
	feed(c, "/viper/wifiIn", 2)
	feed(c, "/qfeel/PbInput", 3)

	stats := c.Stats()
	if len(stats) != 3 {
		t.Fatalf("stats = %d topics, want 3", len(stats))
	}
	if stats[0].Topic != "/qfeel/PbOutput" || stats[0].Count != 5 {
		t.Errorf("first stat = %+v, want the busiest topic first", stats[0])
	}
	for _, s := range stats {
		if s.Rate <= 0 {
			t.Errorf("%s rate = %v, want > 0 while messages are arriving", s.Topic, s.Rate)
		}
	}
	msgs, rate := c.Totals()
	if msgs != 10 {
		t.Errorf("total = %d, want 10", msgs)
	}
	if rate <= 0 {
		t.Errorf("rate = %v, want > 0", rate)
	}
}

func TestStatsFiltersOutOldTraffic(t *testing.T) {
	c := New(Options{Addr: "x:1883"})
	c.mu.Lock()
	c.stats["/old"] = &TopicStat{Topic: "/old", Count: 1, recent: []time.Time{time.Now().Add(-time.Minute)}}
	c.mu.Unlock()
	feed(c, "/new", 2)

	for _, s := range c.Stats() {
		if s.Topic == "/old" && s.Rate != 0 {
			t.Errorf("stale topic rate = %v, want 0", s.Rate)
		}
	}
}

func TestPublishWithoutConnection(t *testing.T) {
	c := New(Options{Addr: "x:1883"})
	if err := c.Publish("/qfeel/PbInput", []byte("gAEI")); err == nil {
		t.Error("publish on a cold client succeeded")
	}
}

func TestStatusStartsDown(t *testing.T) {
	c := New(Options{Addr: "x:1883"})
	if s, _ := c.Status(); s != StatusDown {
		t.Errorf("status = %v, want down", s)
	}
	if got := StatusUp.String(); got != "connected" {
		t.Errorf("StatusUp.String() = %q", got)
	}
}

func TestStartRejectsBadAddress(t *testing.T) {
	c := New(Options{Addr: "nonsense"})
	if err := c.Start(); err == nil {
		t.Error("Start accepted an address with no port")
	}
}
