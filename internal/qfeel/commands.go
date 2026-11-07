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
	Topic   string // defaults to TopicInput
	Payload string
	Hint    string
	// Risky marks writes that move the physical robot.
	Risky bool
}

// Target is the topic the action publishes on.
func (a Action) Target() string {
	if a.Topic == "" {
		return TopicInput
	}
	return a.Topic
}

// Actions returns the mapped command set plus the heartbeat pair.
func Actions() []Action {
	return []Action{
		{Key: "pause", Title: "Pause", Payload: CmdPause, Hint: "16:8 - stops the robot where it is, no return", Risky: true},
		{Key: "play", Title: "Play", Payload: CmdPlay, Hint: "16:10 - starts a clean run; needs an idle robot", Risky: true},
		{Key: "dock", Title: "Dock", Payload: CmdDock, Hint: "16:17 - drives it home", Risky: true},
		{Key: "heartbeat-a", Title: "Heartbeat 8{1:3}", Payload: HeartbeatA, Hint: "session keepalive, no motion"},
		{Key: "heartbeat-b", Title: "Heartbeat 9:1", Payload: HeartbeatB, Hint: "the other half of the keepalive pair"},
		{Key: "wifi-up", Title: "wifi cloud up", Topic: TopicWiFi, Payload: WiFiCloudUp, Hint: "plaintext flag, echoed not switched"},
		{Key: "wifi-down", Title: "wifi cloud down", Topic: TopicWiFi, Payload: WiFiCloudDown, Hint: "plaintext flag, echoed not switched"},
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
