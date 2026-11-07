// Command grabframe saves one live MQTT frame to a file, which is where the
// test fixtures in internal/qfeel/testdata came from.
//
//	go run ./cmd/grabframe -host 192.168.1.79 /qfeel/PbOutput /tmp/frame.b64 20000
//
// The optional trailing argument is a minimum payload size, so a scan bundle can
// be waited for without picking up a 60-byte status frame first.
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
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: grabframe [-host H] <topic> <outfile> [min bytes]")
		os.Exit(2)
	}
	topic, out := flag.Arg(0), flag.Arg(1)
	minBytes := 0
	if flag.NArg() > 2 {
		fmt.Sscan(flag.Arg(2), &minBytes)
	}
	c := broker.New(broker.Options{Addr: fmt.Sprintf("%s:%d", *host, *port)})
	if err := c.Start(); err != nil {
		panic(err)
	}
	defer c.Close()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range c.Snapshot() {
			if m.Topic != topic || len(m.Payload) < minBytes {
				continue
			}
			os.WriteFile(out, m.Payload, 0o644)
			fmt.Printf("wrote %s %d bytes\n", out, len(m.Payload))
			if topic == qfeel.TopicOutput {
				if t, err := qfeel.Decode(string(m.Payload)); err == nil {
					fmt.Println("state", t.State, "grid?", t.Grid != nil)
				}
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	panic("timeout")
}
