# PhoneGyro Hardware Protocol

*[Читать по-русски](README.ru.md)*

An open data contract for motion-sensor controllers. Any microcontroller and
any gyroscope/accelerometer combination is compatible with the
[PhoneGyro](https://github.com/MrHoustonOff/iphone-gyro-controller) host
application — no vendor-specific driver code needed — as long as the device
implements the levels described in [`docs/PROTOCOL.md`](docs/PROTOCOL.md).

This is the same philosophy PhoneGyro already uses for phones: any device
that speaks the browser's standard `devicemotion` API just works, regardless
of whether it's an iPhone, an Android flagship, or a budget tablet. This
repository extends that principle to physical hardware.

> Note: the project (host app + this protocol) is transitioning its brand
> from "GyroBridge" to "PhoneGyro". You may still see the old name in a few
> places until that rename is finished — treat "PhoneGyro" as the current,
> authoritative name.

## Contents

- [`docs/PROTOCOL.md`](docs/PROTOCOL.md) — the full specification (English).
- [`docs/PROTOCOL.ru.md`](docs/PROTOCOL.ru.md) — the full specification (Russian).
- `firmware/` — reference implementation for Arduino Nano + GY-521 (MPU-6050).
  Coming next; this is a working example, not the only supported hardware.

## Status

Draft v1.0. Being developed alongside the host application. The wire format
and semantics are considered stable enough to build against, but not yet
frozen — check this repository's tags/releases once the first reference
firmware has been validated on real hardware.

## License

MIT — see [`LICENSE`](LICENSE). Build whatever you want against this contract.
