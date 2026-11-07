package main

import (
	"fmt"
	"os"
	"time"

	"github.com/jcpsimmons/sharkboard/internal/broker"
	"github.com/jcpsimmons/sharkboard/internal/qfeel"
)

func main() {
	topic := os.Args[1]
	out := os.Args[2]
	minBytes := 0
	fmt.Sscan(os.Args[3], &minBytes)
	c := broker.New(broker.Options{Addr: "192.168.1.79:1883"})
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
