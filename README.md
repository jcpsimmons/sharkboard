# sharkboard

A terminal console for the local MQTT broker on a Shark Matrix robot vacuum, the same open broker a phone app talks to. It shows the telemetry the robot publishes, decodes the LiDAR floor plan it hands out while it moves, and offers the three captured command frames back.

The broker is mosquitto on `tcp/1883`, with no password, no TLS and no ACL. Nothing here is vendor documentation: the topic names, field numbers and frames come from watching the wire. Provenance and confidence for every label are in [docs/field-labels.md](docs/field-labels.md); the teardown itself is at [jcpsimmons/shark-vacuum-teardown](https://github.com/jcpsimmons/shark-vacuum-teardown).

```
sharkboard  192.168.1.79:1883  ● connected                 1 telemetry  2 topics  3 lidar map  4 publish
╭────────────────────────────────────────────╮╭──────────────────────────────────────────────────────╮
│ robot                                      ││ newest frame (labels are inferred)                   │
│ state (4)     running                      ││ tick_ms      2298131  ms counter, +201 per frame     │
│ battery (9:8) 100%                         ││ model        74  constant; NOT the state             │
│ battery trend ▂▂▂▂▂▂▂▂▂▂▂▂▂▂▂▂▂            ││ state        24 (running)  14 idle / 24 running / …  │
│ progress (13) 50                           ││ scan         scan bundle, grid 169x234               │
│ cadence       201 ms/frame  (201 nominal)  ││ status       battery 100%, charging flag 3           │
│ model (2)     74                           ││ x10          1  constant 1                           │
│ echo (16)     16 {1:{2:75}}                ││ x38          2  only present while active            │
│ map label     none                         ││ x45          2  0 docked, 2 while active             │
│                                            ││                                                      │
│ traffic                                    ││                                                      │
│ arrival       56.4 msg/s  412 total        ││                                                      │
╰────────────────────────────────────────────╯╰──────────────────────────────────────────────────────╯
p pause  ·  s play  ·  d dock  ·  H heartbeat  ·  2 topics  ·  3 map  ·  4 publish  ·  ? help  ·  q quit
```

On launch it shows a card naming the build and linking to [youtube.com/@drjoshcsimmons](https://youtube.com/@drjoshcsimmons) and [drjoshcsimmons.com](https://drjoshcsimmons.com). Any key dismisses it.

## Panes

| Key | Pane | What it is |
|---|---|---|
| `1` | telemetry | Live state, battery, charge flag, run progress, frame cadence, the command echo, and every field of the newest frame with its inferred name and the evidence for it. |
| `2` | topics | Per-topic message rate and the raw message log. Filter with `/`, scroll with `J`/`K`, `f` toggles following. |
| `3` | lidar map | The occupancy grid, downsampled to the pane, with the robot pose marked. Only fills while the robot is off the dock. |
| `4` | publish | The three captured frames, the heartbeat pair, and a free-form topic/payload editor (`e`). |

Writing:

| Key | Frame | Bytes |
|---|---|---|
| `p` | pause | `800108` (`16:8`) |
| `s` | play | `3a040a02104b80010a` (`7{1{2:75}} 16:10`) |
| `d` | dock | `3a040a021032800111` (`7{1{2:50}} 16:17`) |
| `H` | heartbeat | `QgIIAw==`, the app's session keepalive |

## Install

Go 1.25 or newer.

```
git clone https://github.com/jcpsimmons/sharkboard
cd sharkboard
go build -o sharkboard .
./sharkboard
```

Press `q` to leave. `-dry-run` connects and watches but publishes nothing.

## Finding the robot

The vacuum leases a new address from DHCP whenever it feels like it (`.78` one week, `.79` the next), so by default the tool probes the local `/24` for anything answering on `1883` and keeps the first candidate that actually publishes `/qfeel` traffic. That is noisy but it usually lands in a couple of seconds. Point it at a known address when you would rather not sweep:

```
./sharkboard -host 192.168.1.79
```

Flags: `-host`, `-port` (1883), `-interval` (redraw period, default 350ms), `-dry-run`, `-once` (print one decoded frame and exit, no TUI), `-wait` (how long `-once` waits, default 20s), `-version`.

`-once` is the quickest sanity check, and the thing to paste into a bug report:

```
$ ./sharkboard -host 192.168.1.79 -once
14:22:43.229 /qfeel/PbOutput  (76 bytes)
  tick_ms       2293835
  model         74
  state         14 (idle/dock)
  status        battery 100%, charging flag 3
  ...
```

`cmd/mapdump` renders a saved frame's grid in the terminal, which is the fastest way to look at a capture:

```
go run ./cmd/mapdump internal/qfeel/testdata/frame-scan.b64 4
```

`cmd/sharksend` publishes a single frame from a shell, for scripted use:

```
go run ./cmd/sharksend -host 192.168.1.79 play
go run ./cmd/sharksend -host 192.168.1.79 dock
go run ./cmd/sharksend -topic /viper/wifiIn WIFISecondaryCloudUp:false
```

## What the numbers mean, and how sure they are

| Field | Name | Confidence |
|---|---|---|
| 1 | `tick_ms` | High. Steps by exactly 201 ms, about 5 frames a second. It is the frame clock and a check on your own decoding. |
| 2 | `model` | High that it is a constant (74 in every state), low that it is a model or firmware id. **Not** the state, which was the first mistake made here. |
| 4 | `state` | High for 14 / 24 / 7 / 4, each confirmed by watching the robot. 6 and 13 appeared once on 2026-11-07 and are shown as unmapped rather than named. |
| 7 | `scan` | High. Only present while moving; carries the floor plan (7 → 5) and paired LiDAR range arrays. |
| 9 | `status` | `8` is battery percent, high. `1` is a flag seen at 3 while docked; "charging" is inference. `x11` sits at 20 and is unmapped. |
| 13 | `progress` | Probable. Climbed 48 → 51 across one run. |
| 16 | `extra` | Echoes the last accepted command. Useful as confirmation, not as an API. |
| 18, 35, 36, 38, 40, 41, 45 | unmapped | Observed, several only while active. Labelled `xN`, never invented. |

The map is categorical, not probabilistic: `75` free floor, `15` obstacle core, `100` inflated costmap fringe, `0` unseen. The fringe is dilation the planner adds for clearance, so the drawing is a navigation surface, not an architectural plan.

## Behaviour worth knowing before you press a key

- **There is no ack.** Success only shows up as state field 4 moving, or as the command echoed back in field 16. The tool logs what it sent into the message log (marked `▲`) and watches the next frames.
- **The topics are fixed and carry no device id.** Two robots on one network are commanded at the same time by the same publish.
- **Play needs an idle robot.** Sent mid-return, it does nothing; it only takes effect off the dock.
- **A docked robot publishes no map.** Pane 3 stays empty until it is moving.
- Everything written is QoS 0, no retain, and no flash is touched.

## Layout

```
main.go                 flags, LAN discovery, -once mode
internal/broker         MQTT client, bounded message ring, per-topic stats
internal/qfeel          protobuf wire decoder, field labels, command frames
internal/render         grid downsampler and sparkline
internal/tui            bubbletea model, panes, keybinds
cmd/sharksend           one-shot publisher
cmd/grabframe           save one live frame to a file (test fixtures come from this)
cmd/mapdump             print a captured frame's grid as text
docs/field-labels.md    provenance for every label
```

## Tests

```
go test ./...
```

Decoding, frame handling and rendering are tested against real captured frames (the scan run kept two, so a decode rule has to hold across both) in `internal/qfeel/testdata`, so the tests fail if a label or a parse rule drifts. Screens are golden-tested; regenerate with `go test ./internal/tui -update` after an intentional layout change.

## Limits

- Schema names are inferred. The bytes are reliable, the words are not, and unmapped fields stay unmapped.
- Only the local path is covered. The cloud (AWS IoT with a device certificate) is a different door, and the camera in the July 2026 cloud writeup belongs to a different model, the Shark IQ. This one has no camera; it has a laser.
- The map is a partial, dock-adjacent snapshot, not a whole-house plan.
- Nothing here authenticates. Anything on the LAN can do all of this, which is the point of the exercise and also the finding.

MIT licensed.
