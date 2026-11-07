// Command sharksend publishes one frame to a Shark robot vacuum's broker. It is
// the scriptable twin of the TUI's publish pane.
//
//	go run ./cmd/sharksend -host 192.168.1.79 play
//	go run ./cmd/sharksend -host 192.168.1.79 -topic /viper/wifiIn WIFISecondaryCloudUp:false
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

func main() {
	host := flag.String("host", "192.168.1.79", "broker host")
	port := flag.Int("port", 1883, "broker port")
	topic := flag.String("topic", qfeel.TopicInput, "topic to publish on")
	flag.Parse()

	payload := flag.Arg(0)
	if a, ok := qfeel.ActionByKey(payload); ok {
		payload = a.Payload
	}
	if payload == "" {
		fmt.Fprintln(os.Stderr, "usage: sharksend [-host H] <pause|play|dock|heartbeat-a|heartbeat-b|raw payload>")
		os.Exit(2)
	}

	c := broker.New(broker.Options{Addr: fmt.Sprintf("%s:%d", *host, *port)})
	if err := c.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer c.Close()

	if err := c.Publish(*topic, []byte(payload)); err != nil {
		fmt.Fprintln(os.Stderr, "publish:", err)
		os.Exit(1)
	}
	fmt.Printf("sent %q to %s\n", payload, *topic)

	// Watch the robot's own field 4 for a few seconds: there is no ack.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range c.Snapshot() {
			if m.Topic != qfeel.TopicOutput {
				continue
			}
			if t, err := qfeel.Decode(string(m.Payload)); err == nil {
				fmt.Printf("  state now %d (%s)\n", uint32(t.State), t.State.Label())
				deadline = time.Time{}
			}
		}
		if deadline.IsZero() {
			break
		}
		time.Sleep(150 * time.Millisecond)
	}
}
