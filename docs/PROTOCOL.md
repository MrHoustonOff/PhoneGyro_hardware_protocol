# PhoneGyro Hardware Protocol — Specification v1.0 (draft)

*[Читать по-русски](PROTOCOL.ru.md)*

## 0. Philosophy

This repository defines a **data contract**, not a list of supported boards.
Any microcontroller, any gyroscope/accelerometer, any hand-built board is
compatible with the PhoneGyro host application if — and only if — it
implements Levels 1–5 below.

The message to a third-party hardware builder should read:

> "Your device must speak protocol X. If it does, the app finds and connects
> it automatically, no matter what it's built from."

Not:

> "Buy an Arduino Nano and a GY-521 sensor."

The Nano + GY-521 combination is only the **reference implementation** in
`firmware/` — an example to copy, not a requirement.

---

## 1. Level 1 — Transport

The device must appear to the host OS as a **Virtual COM Port (Serial-over-USB)**.
This is the lowest common denominator supported by virtually every
USB-capable microcontroller (native USB or through a USB-UART bridge).

- **Baud rate:** `115200` is the mandatory minimum every implementation must
  support — every common USB-UART bridge (CH340, CP210x, FT232, native USB
  CDC) handles it reliably. Higher rates (`250000`, `460800`) may be offered
  as an option but must never be the *only* supported rate, or cheap bridge
  clones will silently fail.
- **Framing:** 8 data bits, no parity, 1 stop bit (8N1) — the default;
  deviating from it is not allowed.
- The device must start sending frames (see Level 2) immediately after the
  port opens, without a handshake and without waiting for a command from the
  host. The host is a passive listener.

---

## 2. Level 2 — Frame Format

Data is sent as **fixed-length binary frames**, not variable-length text
lines. This is a hard requirement, not a style preference:

- Text lines (`"12.45, -3.12\n"`) require variable-length parsing, waste
  bandwidth on characters instead of data, and introduce unpredictable jitter
  exactly where stable sample timing matters most.
- A fixed-size binary frame can be read by the host in one operation, with no
  allocation, and lets the host resynchronize instantly if the stream is
  interrupted (e.g. the cable is bumped).

### Frame layout — 24 bytes, little-endian

```
+-------+------+-----+---------------+---------------+---------------+------+---------+------+
| MAGIC | TYPE | SEQ | TIMESTAMP_US  | ACCEL (X,Y,Z) | GYRO (X,Y,Z)  | TEMP | BUTTONS | CRC8 |
| 2 B   | 1 B  | 1 B | 4 B           | 6 B           | 6 B           | 2 B  | 1 B     | 1 B  |
+-------+------+-----+---------------+---------------+---------------+------+---------+------+
```

| Offset | Size | Type | Field | Purpose |
|---|---|---|---|---|
| 0–1 | 2 B | `0xAA 0x55` | **MAGIC** | Fixed start-of-frame signature. Lets the host find frame boundaries in a byte stream and resynchronize within one cycle after any glitch. |
| 2 | 1 B | `uint8` | **TYPE** | Payload version/kind. `0x00` = metadata frame (see Level 3). `0x01` = sensor data frame (current version). Future protocol revisions (e.g. sending a device-computed quaternion instead of raw data) get a new value, without breaking old parsers. |
| 3 | 1 B | `uint8` | **SEQ** | Incrementing frame counter (wraps around). Used by the host to count dropped frames and report link quality. |
| 4–7 | 4 B | `uint32` | **TIMESTAMP_US** | Device hardware timestamp (microseconds since boot). Critical for sensor-fusion filters (e.g. Madgwick/Mahony) — the device's own clock is not subject to host-OS or network jitter. |
| 8–13 | 6 B | `int16[3]` | **ACCEL X,Y,Z** | Raw accelerometer readings on all three axes, exactly as they sit in the sensor's registers. Units and conversion range are sensor-specific and are *not* declared by this level — that's Level 3's job (metadata frame), or safe defaults. |
| 14–19 | 6 B | `int16[3]` | **GYRO X,Y,Z** | Raw gyroscope readings, same convention. |
| 20–21 | 2 B | `int16` | **TEMP** | Sensor die temperature, if available. `0` if not available. |
| 22 | 1 B | `uint8` | **BUTTONS** | Bitmask of physical buttons on the device. Bit 0 is reserved for "reset centering/calibration" and must be supported if the device has at least one physical button. Bits 1–7 are free for arbitrary extra triggers. |
| 23 | 1 B | `uint8` | **CRC8** | CRC-8 (polynomial `0x07`, init `0x00`, no reflection, no final XOR — the classic "CRC-8/SMBUS" variant) over bytes 0–22. The host must discard any frame that fails this check. |

Total size: **24 bytes**. The frame is self-contained and requires no state
between calls.

> **Why CRC-8 and not a simple XOR checksum:** an XOR checksum misses whole
> classes of common errors — e.g. two flipped bits in the same column across
> different bytes cancel each other out, and byte reordering is invisible to
> it entirely. Since independent third-party firmware is expected to produce
> frames the host must trust, a real CRC costs the same negligible amount of
> CPU time on an 8-bit MCU but catches far more real corruption.

---

## 3. Level 3 — Metadata

The protocol deliberately transmits **raw sensor register values**, not
physical units (rad/s, g): different sensors have different ADC resolution
and different sensitivity scale (`LSB/°/s`, `LSB/g`), and converting to
physical units is the host's job, not the device's.

To declare its scale, the device **must** send exactly one **metadata frame**
before starting the `TYPE=0x01` data stream. This uses the *same* 24-byte
layout as a regular frame — the host never needs a second parser, and frame
resynchronization logic stays identical for both frame kinds:

```
MAGIC        = 0xAA 0x55  (as always)
TYPE         = 0x00
SEQ          = 0
TIMESTAMP_US = protocol version the firmware implements,
               packed as (major << 16 | minor << 8 | patch)
ACCEL[0]     = accelerometer full-scale range, in g       (int16, e.g. 2/4/8/16)
ACCEL[1]     = gyroscope full-scale range, in °/s         (int16, e.g. 250/500/1000/2000)
ACCEL[2]     = device's target frame rate, in Hz          (int16)
GYRO[0..2]   = reserved, must be 0
TEMP         = reserved, must be 0
BUTTONS      = capability bitmask (bit 0 = device has a reset button, bits 1-7 reserved)
CRC8         = as always, over bytes 0-22
```

If the host does not see a valid metadata frame within the first ~500 ms of
the stream, it **must** fall back to safe defaults matching the most common
cheap 6-axis sensor on the market (±250 °/s gyro range, ±2 g accel range,
200 Hz) and must not treat this as an error.

---

## 4. Level 4 — Auto-discovery

The host application **must** be able to find compatible devices on its own,
without asking the user to manually pick a COM port from a list:

1. At startup (and on a user-triggered "rescan"), the host enumerates all
   available serial ports.
2. For each port, it opens a connection at `115200` baud and listens to the
   incoming stream for a short timeout (1–2 seconds recommended).
3. If a valid frame appears in the stream (`0xAA 0x55` signature + a passing
   CRC8) — the port is marked as a compatible device and offered to the user
   (or connected automatically if exactly one device is found).
4. Ports that produced no valid frame within the timeout are silently
   ignored — this is not an error, it may simply be an unrelated USB device.

The host **must not** filter ports by USB vendor/product ID before probing.
Doing so would silently exclude third-party devices built on a different
USB-UART bridge or a microcontroller with native USB, defeating the entire
point of a hardware-agnostic contract.

---

## 5. Level 5 — Responsibilities

### Device (microcontroller) responsibilities

- Start sending frames immediately after the port opens, without waiting for
  a command.
- Send exactly one metadata frame (Level 3) before the first data frame.
- Maintain a stable frame rate. Minimum acceptable: 100 Hz. Recommended: 200 Hz.
- Guarantee that every frame is sent whole (not split by the host OS —
  normally satisfied by one write call for the entire 24-byte buffer).
- If a physical reset button exists, set BUTTONS bit 0 for the duration it is held.
- Never require an initialization command from the host to start operating.

### Host (application) responsibilities

- Never rely on a specific device model, chip, or sensor — work with any
  source implementing the protocol.
- Verify the CRC8 of every incoming frame and discard invalid ones without
  dropping the connection.
- On loss of stream sync (data doesn't start with `0xAA 0x55`), search for
  the signature again in the byte stream instead of tearing down the connection.
- Use the device's `TIMESTAMP_US` field, not host system time, wherever it
  affects orientation-computation accuracy.
- Treat BUTTONS bit 0 as a universal re-centering trigger, identical to the
  software calibration path used for other sources (phones).
- If no metadata frame (Level 3) arrives within ~500 ms, apply the safe
  defaults described in Level 3 rather than failing.

---

## 6. Level 6 — Reference implementation

`firmware/` in this repository contains a **reference** implementation on an
Arduino Nano (ATmega328) + MPU-6050 (GY-521 breakout) — the most accessible
and common combination to get started with. It is not the only supported
option — it's a starting point.

Anyone can implement the same contract on:
- A different microcontroller (ESP32, STM32 Blue Pill, Raspberry Pi Pico,
  etc.) — including using its wireless capabilities, as long as the data is
  ultimately relayed to the host as a virtual COM port or another
  transport the host agrees on;
- A different sensor (ICM-42688, BMI160, LSM6DS3, etc.) — with the
  corresponding Level 3 metadata adjusted;
- A custom PCB instead of a breadboard build.

The only requirement is conformance to Levels 1–5 of this document.

---

## 7. Deliberately out of scope for v1.0

**Temperature compensation is not part of this protocol.** The `TEMP` field
exists so a device *may* apply its own compensation internally (still
sending register-equivalent, just corrected, raw values — this does not
violate the "raw register values" principle of Level 3, since bias
correction is not unit conversion). Whether and how a specific device does
this is entirely up to that device's implementation and is out of scope
here. The host must never assume temperature compensation is present.

---

## 8. Forward compatibility

The **TYPE** field exists specifically so the protocol can evolve without
breaking existing implementations:

- `0x00` — metadata frame (Level 3).
- `0x01` — current version, raw accelerometer/gyroscope data (described above).
- Future values (`0x02`, `0x03`, ...) may introduce a different payload
  (e.g. a device-computed orientation quaternion, for MCUs powerful enough
  to run their own fusion filter) while keeping the same
  MAGIC/TYPE/SEQ/CRC8 skeleton.

A host must explicitly treat unknown TYPE values as "unsupported in this
app version" — not as a protocol error — and may suggest the user update the
application.

---

## 9. Conformance tooling

`tools/phonegyro-dump` in this repository decodes and pretty-prints a live
frame stream plus link-quality stats (frames/sec, garbage bytes discarded),
verified against the reference firmware on real hardware. A stricter
pass/fail conformance report per contract level is still a possible future
addition; the dump tool already covers the practical need of validating a
device without installing the full host application.
