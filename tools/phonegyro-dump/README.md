# phonegyro-dump

*[Читать по-русски](README.ru.md)*

A minimal conformance/diagnostic tool for the [PhoneGyro Hardware
Protocol](../../docs/PROTOCOL.md). Opens a serial port, decodes the frame
stream, and prints human-readable frames plus link-quality stats (frames/sec,
garbage bytes discarded).

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
phonegyro-dump <port> [seconds]
```

- `<port>`: e.g. `COM3` on Windows, `/dev/ttyUSB0` on Linux.
- `[seconds]`: optional; stop automatically after N seconds. Omit to run
  until Ctrl+C.

Run with no arguments to list detected serial ports.

A healthy device should settle into a steady stream at its declared frame
rate (200 Hz for the reference firmware) with zero or near-zero garbage
bytes. A handful of garbage bytes right when the port opens is normal — the
Arduino bootloader resets the board on connect (DTR), and the decoder
resyncs within a couple of bytes. Garbage that continues throughout the
capture means something is actually wrong (wiring, baud rate, power).
