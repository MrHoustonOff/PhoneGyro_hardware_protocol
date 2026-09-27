// phonegyro-dump is a minimal conformance/diagnostic tool for the PhoneGyro
// Hardware Protocol (see ../../docs/PROTOCOL.md). It opens a serial port,
// decodes the 24-byte frame stream with the same resync + CRC-8 logic the
// real host uses, and prints human-readable frames plus link-quality stats.
//
// A plain serial terminal (Arduino IDE's Serial Monitor, PuTTY, etc.) will
// always show this stream as binary garbage -- that is expected, the
// protocol is intentionally binary, not text. This tool exists so a device
// author can verify their firmware without needing the full host app.
//
// Run with no arguments (e.g. by double-clicking the .exe) and it scans
// every serial port itself, exactly like the real host's auto-discovery
// (protocol Level 4) -- no need to know a COM port name up front. It also
// always waits for Enter before the window closes, so double-clicking never
// just flashes and vanishes.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.bug.st/serial"
)

const (
	frameSize = 24
	magic0    = 0xAA
	magic1    = 0x55
	typeMeta  = 0x00
	typeData  = 0x01
	typeName  = 0x02
	baudRate  = 115200
)

func serialMode() *serial.Mode {
	return &serial.Mode{BaudRate: baudRate, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
}

func crc8(data []byte) byte {
	var crc byte
	for _, b := range data {
		crc ^= b
		for i := 0; i < 8; i++ {
			if crc&0x80 != 0 {
				crc = (crc << 1) ^ 0x07
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

type decoder struct{ buf []byte }

// push feeds newly-read bytes in and returns every whole, CRC-valid frame
// found, plus how many bytes were discarded as garbage/resync noise.
func (d *decoder) push(data []byte) (frames [][]byte, garbage int) {
	d.buf = append(d.buf, data...)
	for {
		if len(d.buf) < 2 {
			break
		}
		if d.buf[0] != magic0 || d.buf[1] != magic1 {
			d.buf = d.buf[1:]
			garbage++
			continue
		}
		if len(d.buf) < frameSize {
			break
		}
		f := d.buf[:frameSize]
		if crc8(f[:frameSize-1]) != f[frameSize-1] {
			d.buf = d.buf[2:]
			garbage += 2
			continue
		}
		frames = append(frames, append([]byte(nil), f...))
		d.buf = d.buf[frameSize:]
	}
	return
}

func le16(b []byte) int16  { return int16(uint16(b[0]) | uint16(b[1])<<8) }
func le32(b []byte) uint32 { return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24 }

// deviceName extracts the optional TYPE=0x02 frame's ASCII name from bytes
// 4..22, trimmed at the first 0x00 (or all 19 bytes if there is none).
func deviceName(f []byte) string {
	b := f[4:23]
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return string(b)
}

func indexByte(b []byte, target byte) int {
	for i, c := range b {
		if c == target {
			return i
		}
	}
	return -1
}

func printFrame(f []byte) {
	typ, seq, ts := f[2], f[3], le32(f[4:8])
	accel := [3]int16{le16(f[8:10]), le16(f[10:12]), le16(f[12:14])}
	gyro := [3]int16{le16(f[14:16]), le16(f[16:18]), le16(f[18:20])}
	temp, buttons := le16(f[20:22]), f[22]

	if typ == typeMeta {
		fmt.Printf("[META]  protocol=%d.%d.%d  accelRange=%dg  gyroRange=%d deg/s  rate=%dHz  caps=0x%02X\n",
			ts>>16, (ts>>8)&0xFF, ts&0xFF, accel[0], accel[1], accel[2], buttons)
		return
	}
	if typ == typeName {
		fmt.Printf("[NAME]  device=%q\n", deviceName(f))
		return
	}
	fmt.Printf("[DATA]  seq=%3d  ts_us=%10d  accel=(%6d,%6d,%6d)  gyro=(%6d,%6d,%6d)  temp=%5d  buttons=0x%02X\n",
		seq, ts, accel[0], accel[1], accel[2], gyro[0], gyro[1], gyro[2], temp, buttons)
}

func status(ok bool) string {
	if ok {
		return "OK"
	}
	return "FAIL"
}

// checklistLine is the tool's pseudo-feedback: a one-line pass/fail readout
// against the protocol's own Level 5 requirements (frames present, minimum
// 100Hz rate, metadata declared, no dropped frames), refreshed every report
// interval so a device author gets an immediate verdict instead of having to
// read raw numbers themselves.
func checklistLine(hz float64, totalFrames int, metaSeen bool, dropped uint64, totalGarbage int) string {
	metaStatus := "OK"
	if !metaSeen {
		metaStatus = "MISSING (defaults applied)"
	}
	lossStatus := "OK"
	if dropped > 0 {
		lossStatus = fmt.Sprintf("WARN (%d dropped)", dropped)
	}
	return fmt.Sprintf("[CHECK] frames=%s  rate=%s (%.0f Hz)  metadata=%s  loss=%s  garbage(total)=%d bytes",
		status(totalFrames > 0), status(hz >= 95), hz, metaStatus, lossStatus, totalGarbage)
}

func listPorts() {
	ports, _ := serial.GetPortsList()
	if len(ports) == 0 {
		fmt.Println("no serial ports detected on this machine")
		return
	}
	fmt.Println("available ports:")
	for _, p := range ports {
		fmt.Println(" ", p)
	}
}

// autoDiscover is the same Level 4 auto-discovery the real host uses: try
// every serial port until one produces a valid PhoneGyro frame. Returns the
// still-open port plus whatever frames/leftover bytes were already read
// during the probe, so nothing seen during discovery is lost on handoff.
func autoDiscover() (name string, port serial.Port, initial [][]byte, pending []byte, err error) {
	ports, lerr := serial.GetPortsList()
	if lerr != nil || len(ports) == 0 {
		return "", nil, nil, nil, fmt.Errorf("no serial ports detected on this machine")
	}
	for _, p := range ports {
		port, err := serial.Open(p, serialMode())
		if err != nil {
			continue
		}
		_ = port.SetReadTimeout(150 * time.Millisecond)
		var dec decoder
		deadline := time.Now().Add(1500 * time.Millisecond)
		buf := make([]byte, 128)
		for time.Now().Before(deadline) {
			n, rerr := port.Read(buf)
			if rerr != nil {
				break
			}
			if n == 0 {
				continue
			}
			if frames, _ := dec.push(buf[:n]); len(frames) > 0 {
				return p, port, frames, dec.buf, nil
			}
		}
		port.Close()
	}
	return "", nil, nil, nil, fmt.Errorf("no PhoneGyro-compatible device found on any port")
}

// pauseBeforeExit keeps the console window open when launched by double-click
// (no parent terminal to keep it visible), instead of it flashing and closing
// the instant the program returns.
func pauseBeforeExit() {
	fmt.Println("\nPress Enter to close...")
	bufio.NewReader(os.Stdin).ReadString('\n')
}

func main() {
	defer pauseBeforeExit()

	var name string
	runFor := time.Duration(0) // 0 = run forever, until Ctrl+C

	args := os.Args[1:]
	if len(args) > 0 {
		name = args[0]
	}
	if len(args) > 1 {
		if secs, err := strconv.Atoi(args[1]); err == nil && secs > 0 {
			runFor = time.Duration(secs) * time.Second
		}
	}

	var port serial.Port
	var initialFrames [][]byte
	var dec decoder

	if name == "" {
		fmt.Println("no port given -- scanning every serial port for a PhoneGyro device (protocol Level 4 auto-discovery)...")
		var err error
		name, port, initialFrames, dec.buf, err = autoDiscover()
		if err != nil {
			fmt.Println(err)
			listPorts()
			return
		}
		fmt.Printf("found a device on %s\n", name)
	} else {
		var err error
		port, err = serial.Open(name, serialMode())
		if err != nil {
			fmt.Printf("failed to open %s: %v\n\n", name, err)
			listPorts()
			return
		}
	}
	defer port.Close()
	_ = port.SetReadTimeout(300 * time.Millisecond)

	fmt.Printf("listening on %s at 115200 8N1", name)
	if runFor > 0 {
		fmt.Printf(" for %s\n", runFor)
	} else {
		fmt.Println(" -- Ctrl+C to stop")
	}

	var totalFrames, totalGarbage int
	var dropped uint64
	var haveSeq bool
	var lastSeq byte
	var metaSeen bool
	var afterMeta bool // previous frame was metadata: data SEQ 0 now means a reboot
	framesSinceReport := 0
	start := time.Now()
	lastReport := start
	lastPrint := time.Time{}
	var lastMeta []byte
	var lastName []byte

	handle := func(f []byte) {
		totalFrames++
		framesSinceReport++
		if f[2] == typeMeta {
			// Protocol v1.1 devices repeat metadata every second (and a
			// reboot sends it again). Print it once per distinct value,
			// not once per repeat, so repeats or a reboot loop can't flood
			// the console into starving the reader and causing its own
			// packet loss.
			if string(f) != string(lastMeta) {
				printFrame(f)
				lastMeta = append([]byte(nil), f...)
			}
			metaSeen = true
			afterMeta = true
			return
		}
		if f[2] == typeName {
			if string(f) != string(lastName) {
				printFrame(f)
				lastName = append([]byte(nil), f...)
			}
			return
		}
		seq := f[3]
		rebooted := afterMeta && seq == 0 // SEQ restart after a boot is not loss
		afterMeta = false
		if haveSeq && !rebooted {
			gap := int(seq) - int(lastSeq)
			if gap < 0 {
				gap += 256
			}
			if gap > 1 {
				dropped += uint64(gap - 1)
			}
		}
		lastSeq, haveSeq = seq, true

		if time.Since(lastPrint) >= 200*time.Millisecond {
			printFrame(f)
			lastPrint = time.Now()
		}
	}

	for _, f := range initialFrames {
		handle(f)
	}

	buf := make([]byte, 256)
	for {
		if runFor > 0 && time.Since(start) >= runFor {
			break
		}
		n, err := port.Read(buf)
		if err != nil {
			fmt.Printf("read error: %v\n", err)
			break
		}
		if n > 0 {
			frames, garbage := dec.push(buf[:n])
			totalGarbage += garbage
			for _, f := range frames {
				handle(f)
			}
		}
		if time.Since(lastReport) >= 2*time.Second {
			hz := float64(framesSinceReport) / time.Since(lastReport).Seconds()
			fmt.Println(checklistLine(hz, totalFrames, metaSeen, dropped, totalGarbage))
			framesSinceReport = 0
			lastReport = time.Now()
		}
	}

	elapsed := time.Since(start).Seconds()
	fmt.Println()
	fmt.Printf("summary: %d valid frames in %.1fs (%.0f/s avg), %d garbage bytes discarded\n",
		totalFrames, elapsed, float64(totalFrames)/elapsed, totalGarbage)
	fmt.Println(checklistLine(float64(totalFrames)/elapsed, totalFrames, metaSeen, dropped, totalGarbage))
	if totalFrames == 0 {
		fmt.Println("no valid PhoneGyro frames seen at all -- check baud rate (must be 115200),")
		fmt.Println("wiring, and that the sketch actually uploaded successfully.")
	} else if totalGarbage > totalFrames {
		fmt.Println("warning: more garbage bytes than valid frames -- link is noisy or partially misaligned.")
	}
}
