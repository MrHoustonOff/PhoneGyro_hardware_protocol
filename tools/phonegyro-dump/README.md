# phonegyro-dump

*[Читать по-русски](README.ru.md)*

A minimal conformance/diagnostic tool for the [PhoneGyro Hardware
Protocol](../../docs/PROTOCOL.md). It opens a serial port, decodes the frame
stream with the same resync + CRC-8 logic the real host uses, and prints:

- human-readable frames (metadata and data),
- a live pass/fail checklist against the protocol's own requirements,
- link-quality stats (frames/sec, dropped frames, garbage bytes).

A plain serial terminal (Arduino IDE's Serial Monitor, PuTTY, etc.) will
always show this stream as unreadable binary — the protocol is intentionally
binary, not text. Use this tool instead when you need to actually read the
values or verify a device implements the contract correctly.

## Build

```
cd tools/phonegyro-dump
go build -o phonegyro-dump.exe .
```

## Use

```
phonegyro-dump [port] [seconds]
```

Both arguments are optional:

- **No arguments at all** (including just double-clicking the `.exe`): the
  tool scans every serial port on the machine itself, exactly like the real
  host's auto-discovery (protocol Level 4) — it doesn't need to be told a COM
  port name. It also always waits for Enter before the window closes, so a
  double-click never just flashes and vanishes.
- `[port]`: e.g. `COM3` on Windows, `/dev/ttyUSB0` on Linux. Give this to skip
  scanning and connect to a specific port directly.
- `[seconds]`: only used together with `[port]`; stop automatically after N
  seconds instead of running until Ctrl+C. Handy for scripted checks.

## Reading the output

```
no port given -- scanning every serial port for a PhoneGyro device (protocol Level 4 auto-discovery)...
found a device on COM3
listening on COM3 at 115200 8N1 -- Ctrl+C to stop
[META]  protocol=1.0.0  accelRange=2g  gyroRange=250 deg/s  rate=200Hz  caps=0x00
[DATA]  seq= 40  ts_us=    200880  accel=(    -8, -8860, 14448)  gyro=(  -229,    53,    33)  temp= 3742  buttons=0x00
[CHECK] frames=OK  rate=OK (199 Hz)  metadata=OK  loss=OK  garbage(total)=16 bytes
```

- `[META]` — printed once (and again only if it ever actually changes);
  shows the device's declared protocol version and sensor range.
- `[DATA]` — a sampled data frame, throttled to about 5 lines/sec so real
  200 Hz traffic stays readable; raw register values, not physical units.
- `[CHECK]` — the pass/fail verdict, refreshed every ~2 seconds and again as
  a final summary:
  - `frames` — any valid frame seen at all.
  - `rate` — measured Hz is at least the protocol's own minimum (100 Hz).
  - `metadata` — a real metadata frame was seen (`MISSING` means safe
    defaults were applied instead — check the device actually sends one).
  - `loss` — no gaps in the SEQ counter (`WARN (N dropped)` otherwise).
  - `garbage(total)` — bytes discarded while resyncing, accumulated since
    start.

A handful of garbage bytes right when the port opens is normal — opening the
port often resets the Arduino (DTR), so the board reboots mid-frame; the
decoder resyncs within a couple of bytes. Protocol v1.1 devices also repeat
their metadata every second; `[META]` is still printed only when it changes. Garbage or `loss` warnings that continue throughout the
capture mean something is actually wrong (wiring, baud rate, power).
