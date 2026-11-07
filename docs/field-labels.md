# Field labels and where they come from

Every label in this tool is an inference pinned to an observation, and the tool
prints the evidence next to the value. This page collects that provenance in one
place so nobody has to guess how much weight a name deserves.

The device is a Shark Matrix, model class RV2320EDUS, on the LAN, reached as
`tcp/1883` running mosquitto 2.0.15 with no password and no ACL. Work was done
from the owner's own network on 2026-11-04, with a follow-up read on 2026-11-07.
The full teardown is at
[jcpsimmons/shark-vacuum-teardown](https://github.com/jcpsimmons/shark-vacuum-teardown).

## Terms

- **Confirmed**: the robot was seen doing the thing. Acts.
- **High**: many consistent frames, no contradiction.
- **Probable**: one observation, plausible, not retested.
- **Unmapped**: a field number exists, nothing is claimed about it, and it is
  displayed as `xN`.

## Topics

| Topic | Direction | Encoding | Confidence |
|---|---|---|---|
| `/qfeel/PbOutput` | robot → anyone | base64 of protobuf, 5 Hz | Confirmed |
| `/qfeel/PbInput` | anyone → robot | base64 of protobuf; commands and an app-session heartbeat | Confirmed by replay |
| `/viper/wifiIn` | both | plaintext `Key:Value` | High that it exists, low that it controls anything |

The topic set is fixed. No frame carries a device id, a serial or a nonce, so
two robots on one network answer to the same publish.

`/viper/wifiIn` accepts publishes, but sending the opposite of the advertised
cloud flag did not drop the cloud link and barely moved the input rate
(8 frames in 20 s, then 10 in 30 s, then 9 in 25 s). Treat it as an echo.

## PbOutput fields

| # | Label | Confidence | Evidence |
|---|---|---|---|
| 1 | `tick_ms` | Confirmed | Steps by exactly 201 per frame, ~5 Hz, in every capture since. |
| 2 | `model` | High it is constant, low what it is | Held 74 through docked, cleaning, paused and docking frames. Was wrongly read as the state at first. |
| 4 | `state` | Confirmed for 14/24/7/4 | 14 idle on dock as the baseline and the value it returns to after docking; 24 only while the wheels move; 7 during commanded and manual docks; 4 after a pause, held 33 consecutive frames with no motion. 6 and 13 were each seen once on 2026-11-07 and are left unmapped. |
| 7 | `scan` | Confirmed | Appears only while the robot moves; 20–170 KB frames against 40–120 bytes parked. Holds the map at 7 → 5. |
| 9 | `status` | Mixed | `8` is battery percent (100 on the dock). `1` = 3 while docked is a flag; "charging" is inference. `9` unmapped. `11` sits at 20, unmapped. |
| 10 | `x10` | High | Constant 1. |
| 13 | `progress` | Probable | Climbed 48 → 51 across one run and only appears while active. Run percentage is a reading, not a fact. |
| 16 | `extra` | High | Echoes the last accepted command, e.g. `16 {1 {2:75}}` after a play. Sparse while idle. |
| 18 | `x18` | Unmapped | Seen as `{5:5}` during a driven run. |
| 35, 36 | `x35`, `x36` | High they are constant | Both 2. |
| 38, 40, 41 | unmapped | Unmapped | Only present while active. |
| 45 | `x45` | High | 0 docked, 2 while active. |

## Map record (field 7 → field 5)

| Subfield | Meaning | Confidence |
|---|---|---|
| 1 | resolution, metres per cell (0.06) | High. Extent matches the declared cell count. |
| 2 | origin x, y in metres, negative | High. Puts the grid inside a house-sized area. |
| 3, 4 | width, height in cells (169 × 234) | High. `width × height` equals the byte length exactly. |
| 6 | the grid, row-major, one byte per cell | High. 169 × 234 = 39,546 bytes. |
| 9 | robot pose x, y | High that it is a position, medium that the frame is grid-local. |
| 17 | flags | Unmapped, seen as 1. |

Cell values are categorical, not probabilistic: 75 free, 15 obstacle core, 100
inflation fringe, 0 unseen, 92 stray. The fringe is costmap dilation, so the
drawing shows a navigation surface rather than a floor plan with clean edges.

A short rendering mistake is worth knowing about: drawing field 5's bytes from
offset 0 rather than from field 6 shifts every row by the header length, and the
image appears to shear off the right edge and wrap. `width × height` against the
byte length catches it immediately.

## Command frames

Captured from the vendor app while it drove the robot, then replayed from the
LAN with the app closed. The robot obeyed all three, and each produced the
expected state change plus its own echo.

| Action | Payload (base64 text on `/qfeel/PbInput`) | Decoded |
|---|---|---|
| pause | `gAEI` | `16:8` |
| play | `OgQKAhBLgAEK` | `7 {1 {2:75}} 16:10` |
| dock | `OgQKAhAygAER` | `7 {1 {2:50}} 16:17` |

Field 16 is the opcode, field 7 a mode selector where 75 = clean and 50 = dock.
QoS 0, no retain. Opcodes 1–30 were swept once without movement, so anything
outside 8/10/17 is unmapped rather than "unsupported".

`play` needs an idle robot. Sent while it was already returning, the state held
at 7 for the whole attempt. It did cancel an in-progress dock once, which is
where a wrong "play cancels a dock" reading came from.

## What has not been mapped

Directional driving (the app's joystick), spot cleaning, room and zone cleans,
suction level, schedules. The map record carries a label and an id
(`unnamed`, `79F28BD6`) that are probably the handle for zone cleans.

The tool never invents a name for these. Unmapped fields show as `xN` and their
values print raw.
