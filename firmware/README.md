# Reference firmware

*[Читать по-русски](README.ru.md)*

Reference implementation of the [PhoneGyro Hardware Protocol](../docs/PROTOCOL.md)
for Arduino Nano (ATmega328) + GY-521 (MPU-6050), I2C address `0x68`.

## `reference_nano_gy521/`

Single-file sketch, no external MPU-6050 library dependency (Wire only), so
it can be read top to bottom as the protocol reference it's meant to be.

- Wakes the MPU-6050, configures it for ±250 °/s / ±2 g / 200 Hz internal
  sample rate (`SMPLRT_DIV` = 4 with the DLPF enabled).
- Sends one metadata frame (`TYPE=0x00`) at startup declaring that
  configuration, then streams data frames (`TYPE=0x01`) at 200 Hz.
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
