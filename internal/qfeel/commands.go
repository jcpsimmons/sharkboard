package qfeel

// The three command frames, captured from the vendor app on the wire and
// replayed from the LAN with the app closed. The robot obeyed all three.
//
// The payload is base64 TEXT on a fixed topic, QoS 0, no retain. Neither frame
// carries a device id or a nonce, so every robot on the network is commanded at
// once. There is no ack: confirmation only comes from watching state field 4
// change.
//
// Field 16 is the opcode, field 7 a mode selector (75 clean, 50 dock).
const (
	CmdPause = "gAEI"         // 16: 8
	CmdPlay  = "OgQKAhBLgAEK" // 7 {1 {2: 75}} 16: 10
	CmdDock  = "OgQKAhAygAER" // 7 {1 {2: 50}} 16: 17
)

// Heartbeat pair the app trades on the command channel about every 3 s. Skip
// them when looking for a real command, and harmless to interleave with one.
const (
	HeartbeatA = "QgIIAw==" // 8 {1: 3}
	HeartbeatB = "SAE="     // 9: 1
)

// Action is a named write the UI can offer.
type Action struct {
	Key     string
	Title   string
	Payload string
	Hint    string
	// Risky marks writes that move the physical robot.
	Risky bool
}

// Actions returns the mapped command set plus the heartbeat pair.
func Actions() []Action {
	return []Action{
		{"pause", "Pause", CmdPause, "16:8 - stops the robot where it is, no return", true},
		{"play", "Play", CmdPlay, "16:10 - starts a clean run; needs an idle robot", true},
		{"dock", "Dock", CmdDock, "16:17 - drives it home", true},
		{"heartbeat-a", "Heartbeat 8{1:3}", HeartbeatA, "session keepalive, no motion", false},
		{"heartbeat-b", "Heartbeat 9:1", HeartbeatB, "the other half of the keepalive pair", false},
	}
}

// ActionByKey looks an action up by its short key.
func ActionByKey(k string) (Action, bool) {
	for _, a := range Actions() {
		if a.Key == k {
			return a, true
		}
	}
	return Action{}, false
}

// Canned WiFi-channel payloads. The channel is plaintext Key:Value, and the one
// observed value is echoed status rather than a working switch: publishing the
// opposite did not drop the cloud link.
const (
	WiFiCloudUp   = "WIFISecondaryCloudUp:true"
	WiFiCloudDown = "WIFISecondaryCloudUp:false"
)
