// PhoneGyro Hardware Protocol -- reference firmware
// Board:  Arduino Nano (ATmega328)
// Sensor: GY-521 breakout (MPU-6050), I2C address 0x68 (AD0 -> GND)
//
// Implements protocol v1.0 (see ../../docs/PROTOCOL.md):
//   - one metadata frame (TYPE=0x00) sent once at startup
//   - a continuous stream of data frames (TYPE=0x01) at 200 Hz
//   - 24-byte fixed frame, little-endian, CRC-8/SMBUS trailer
//   - raw register values only -- no on-device sensor fusion
//
// No external MPU-6050 library is used on purpose: the point of a
// *reference* implementation is that anyone can read it top to bottom
// without chasing a dependency. Wire is the only requirement.

#include <Wire.h>

// ---- MPU-6050 registers ----------------------------------------------
static const uint8_t MPU_ADDR         = 0x68; // AD0 tied to GND
static const uint8_t REG_SMPLRT_DIV   = 0x19;
static const uint8_t REG_CONFIG       = 0x1A;
static const uint8_t REG_GYRO_CONFIG  = 0x1B;
static const uint8_t REG_ACCEL_CONFIG = 0x1C;
static const uint8_t REG_PWR_MGMT_1   = 0x6B;
static const uint8_t REG_ACCEL_XOUT_H = 0x3B; // accel(6) + temp(2) + gyro(6) = 14B burst

// ---- Protocol constants ------------------------------------------------
static const uint8_t  FRAME_MAGIC0 = 0xAA;
static const uint8_t  FRAME_MAGIC1 = 0x55;
static const uint8_t  TYPE_META    = 0x00;
static const uint8_t  TYPE_DATA    = 0x01;
static const uint8_t  FRAME_SIZE   = 24;

static const uint16_t SAMPLE_RATE_HZ  = 200;
static const uint16_t GYRO_RANGE_DPS  = 250;  // matches GYRO_CONFIG FS_SEL=0 below
static const uint16_t ACCEL_RANGE_G   = 2;    // matches ACCEL_CONFIG AFS_SEL=0 below
static const uint32_t PROTOCOL_VERSION = (1UL << 16) | (0UL << 8) | 0UL; // 1.0.0

// This reference build has no physical reset button (see DEVICE_SPEC.md
// in the host repository) -- BUTTONS stays 0 in every frame, and bit 0
// of the metadata capability mask stays 0 to say so honestly.
static const uint8_t CAPABILITIES = 0x00;

#pragma pack(push, 1)
struct PhoneGyroFrame {
  uint8_t  magic0;
  uint8_t  magic1;
  uint8_t  type;
  uint8_t  seq;
  uint32_t timestampUs;
  int16_t  a[3];
  int16_t  g[3];
  int16_t  temp;
  uint8_t  buttons;
  uint8_t  crc8;
};
#pragma pack(pop)

static_assert(sizeof(PhoneGyroFrame) == FRAME_SIZE, "frame must be exactly 24 bytes");

uint8_t frameSeq = 0;

// ---- CRC-8/SMBUS: poly 0x07, init 0x00, no reflect, no final xor -------
uint8_t crc8(const uint8_t *data, uint8_t len) {
  uint8_t crc = 0x00;
  for (uint8_t i = 0; i < len; i++) {
    crc ^= data[i];
    for (uint8_t bit = 0; bit < 8; bit++) {
      crc = (crc & 0x80) ? (uint8_t)((crc << 1) ^ 0x07) : (uint8_t)(crc << 1);
    }
  }
  return crc;
}

void sendFrame(PhoneGyroFrame &f) {
  f.crc8 = crc8(reinterpret_cast<uint8_t *>(&f), FRAME_SIZE - 1);
  // One write call for the whole 24-byte buffer, per protocol Level 5:
  // the frame must not be split by the host OS.
  Serial.write(reinterpret_cast<uint8_t *>(&f), FRAME_SIZE);
}

void sendMetadataFrame() {
  PhoneGyroFrame f;
  memset(&f, 0, sizeof(f));
  f.magic0 = FRAME_MAGIC0;
  f.magic1 = FRAME_MAGIC1;
  f.type = TYPE_META;
  f.seq = 0;
  f.timestampUs = PROTOCOL_VERSION;
  f.a[0] = ACCEL_RANGE_G;
  f.a[1] = GYRO_RANGE_DPS;
  f.a[2] = SAMPLE_RATE_HZ;
  // g[], temp: reserved, already zero from memset.
  f.buttons = CAPABILITIES;
  sendFrame(f);
}

void mpuWrite(uint8_t reg, uint8_t value) {
  Wire.beginTransmission(MPU_ADDR);
  Wire.write(reg);
  Wire.write(value);
  Wire.endTransmission();
}

void setupMPU6050() {
  mpuWrite(REG_PWR_MGMT_1, 0x01);   // wake up, PLL clock referenced to X gyro
  mpuWrite(REG_CONFIG, 0x03);       // DLPF ~44Hz accel / 42Hz gyro, 1kHz internal sample rate
  mpuWrite(REG_GYRO_CONFIG, 0x00);  // FS_SEL=0 -> +/-250 deg/s
  mpuWrite(REG_ACCEL_CONFIG, 0x00); // AFS_SEL=0 -> +/-2 g
  // Sample rate = 1kHz / (1 + SMPLRT_DIV) -> divider 4 gives exactly 200 Hz.
  mpuWrite(REG_SMPLRT_DIV, (1000 / SAMPLE_RATE_HZ) - 1);
}

// Burst-reads ACCEL_XOUT_H..GYRO_ZOUT_L (14 bytes: accel, temp, gyro) in one
// I2C transaction, exactly matching MPU-6050's own register layout, and
// fills the frame's raw fields directly -- no unit conversion happens here,
// that stays the host's job (protocol Level 3).
void readMPU6050(PhoneGyroFrame &f) {
  Wire.beginTransmission(MPU_ADDR);
  Wire.write(REG_ACCEL_XOUT_H);
  Wire.endTransmission(false);
  Wire.requestFrom(MPU_ADDR, (uint8_t)14, (uint8_t) true);

  int16_t raw[7]; // ax, ay, az, temp, gx, gy, gz -- registers are big-endian
  for (uint8_t i = 0; i < 7; i++) {
    uint8_t hi = Wire.read();
    uint8_t lo = Wire.read();
    raw[i] = (int16_t)((hi << 8) | lo);
  }

  f.a[0] = raw[0];
  f.a[1] = raw[1];
  f.a[2] = raw[2];
  f.temp = raw[3]; // raw register value, per Level 3 "raw sensor values" philosophy
  f.g[0] = raw[4];
  f.g[1] = raw[5];
  f.g[2] = raw[6];
}

unsigned long nextFrameAt = 0;
const unsigned long FRAME_INTERVAL_US = 1000000UL / SAMPLE_RATE_HZ;

void setup() {
  Serial.begin(115200);
  Wire.begin();
  Wire.setClock(400000); // MPU-6050 supports I2C fast mode
  setupMPU6050();

  // Level 1: start sending immediately, no handshake. First frame out is
  // the mandatory metadata frame (Level 3), then the data stream begins.
  sendMetadataFrame();
  nextFrameAt = micros();
}

void loop() {
  unsigned long now = micros();
  if ((long)(now - nextFrameAt) < 0) {
    return; // not time for the next sample yet
  }
  nextFrameAt += FRAME_INTERVAL_US;

  PhoneGyroFrame f;
  f.magic0 = FRAME_MAGIC0;
  f.magic1 = FRAME_MAGIC1;
  f.type = TYPE_DATA;
  f.seq = frameSeq++;
  f.timestampUs = now;
  f.buttons = 0; // no physical button on this reference build

  readMPU6050(f);
  sendFrame(f);
}
