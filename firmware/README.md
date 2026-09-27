# Reference firmware

*[Читать по-русски](README.ru.md)*

Reference implementation of the [PhoneGyro Hardware Protocol](../docs/PROTOCOL.md)
for Arduino Nano (ATmega328) + GY-521 (MPU-6050), I2C address `0x68`.

## `reference_nano_gy521/`

Single-file sketch, no external MPU-6050 library dependency (Wire only), so
it can be read top to bottom as the protocol reference it's meant to be.

- Wakes the MPU-6050, configures it for ±2000 °/s / ±2 g / 200 Hz internal
  sample rate (`SMPLRT_DIV` = 4 with the DLPF enabled). The gyro range is
  deliberately wide, not the tightest/most sensitive option the chip offers:
  ±250 °/s clips on an ordinary fast flick or spin, silently under-reporting
  real rotation and drifting the integrated orientation further with every
  fast movement (verified live). ±2000 °/s matches what real DS4/DualSense
  controllers use for exactly this reason; the coarser resolution it trades
  away is not perceptible in practice.
- Sends a metadata frame (`TYPE=0x00`) at startup declaring that
  configuration, then an optional device name frame (`TYPE=0x02`), then
  streams data frames (`TYPE=0x01`) at 200 Hz, repeating the metadata and
  name frames every second (protocol v1.1) so a host that opens the port
  without rebooting the board still learns the range.
- Every frame is CRC-8/SMBUS-checked and written in a single `Serial.write`
  call, per protocol Level 5.
- `BUTTONS` is always `0` and the metadata capability mask advertises no
  reset button — this reference build has none wired.

### Wiring (from `DEVICE_SPEC.md` in the host repository)

| GY-521 | → | Nano |
|---|---|---|
| VCC | → | 5V |
| GND | → | GND |
| SCL | → | A5 |
| SDA | → | A4 |
| AD0 | → | GND |

### Flashing

Open `reference_nano_gy521.ino` in the Arduino IDE, select "Arduino Nano"
(pick the old bootloader if upload fails on a clone board), select the CH340
or FT232 COM port, upload. No extra libraries to install.
