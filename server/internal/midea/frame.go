package midea

import (
	"math"
	"sync/atomic"
)

// Mode is the operational mode of the air conditioner.
type Mode int

// Modes as the device encodes them.
const (
	ModeAuto Mode = 1
	ModeCool Mode = 2
	ModeDry  Mode = 3
	ModeHeat Mode = 4
	ModeFan  Mode = 5
)

// Valid reports whether m is a mode the device knows.
func (m Mode) Valid() bool { return m >= ModeAuto && m <= ModeFan }

// String is the API name of the mode.
func (m Mode) String() string {
	switch m {
	case ModeAuto:
		return "auto"
	case ModeCool:
		return "cool"
	case ModeDry:
		return "dry"
	case ModeHeat:
		return "heat"
	case ModeFan:
		return "fan"
	}

	return "unknown"
}

// ParseMode is the inverse of Mode.String.
func ParseMode(s string) (Mode, bool) {
	for m := ModeAuto; m <= ModeFan; m++ {
		if m.String() == s {
			return m, true
		}
	}

	return 0, false
}

// Fan speeds: 1–100 percent stepless, the presets below, 102 = auto.
const (
	FanSilent = 20
	FanLow    = 40
	FanMedium = 60
	FanHigh   = 80
	FanMax    = 100
	FanAuto   = 102
)

// Swing modes.
const (
	SwingOff      = 0x00
	SwingVertical = 0x0C
)

// Frame types (byte 9).
const (
	FrameControl = 0x02
	FrameQuery   = 0x03
	FrameReport  = 0x04
)

// Property IDs of the 0xB0/0xB1 property commands.
type PropertyID uint16

// Known properties.
const (
	PropSelfClean  PropertyID = 0x0039
	PropRateSelect PropertyID = 0x0048
	PropOutSilent  PropertyID = 0x00CD
	PropIECO       PropertyID = 0x00E3
	PropSound      PropertyID = 0x022C
	PropIdentifier PropertyID = 0x00AB
	PropIndoorT2   PropertyID = 0x00E0
)

// Encode encodes a boolean property value like msmart-ng.
func (p PropertyID) Encode(on bool) []byte {
	b := func(v bool, t, f byte) byte {
		if v {
			return t
		}

		return f
	}

	switch p {
	case PropOutSilent:
		return []byte{b(on, 3, 0)}
	case PropIECO:
		return append([]byte{0, 1, b(on, 1, 0)}, make([]byte, 10)...)
	case PropRateSelect:
		return []byte{b(on, 100, 50)}
	default:
		return []byte{b(on, 1, 0)}
	}
}

// Decode decodes a boolean property value; ok is false when unreadable.
func (p PropertyID) Decode(d []byte) (on, ok bool) {
	switch p {
	case PropOutSilent:
		if len(d) > 0 {
			return d[0] == 3, true
		}
	case PropIECO:
		if len(d) >= 2 {
			return d[1] != 0, true
		}
	case PropRateSelect:
		if len(d) > 0 {
			return d[0] >= 100, true
		}
	default:
		if len(d) > 0 {
			return d[0] != 0, true
		}
	}

	return false, false
}

const (
	deviceTypeAC  = 0xAC
	controlSource = 0x02
)

var messageID atomic.Uint32

func checksum(b []byte) byte {
	sum := 0
	for _, c := range b {
		sum += int(c)
	}

	return byte(-sum)
}

// BuildFrame builds a command frame with message id, CRC8 and checksum.
// id < 0 picks the next running message id.
func BuildFrame(typ byte, payload []byte, id int) []byte {
	mid := byte(id)
	if id < 0 {
		mid = byte(messageID.Add(1))
	}

	body := append(append([]byte{}, payload...), mid)
	body = append(body, crc8(body))
	f := []byte{0xAA, byte(len(body) + 10), deviceTypeAC, 0, 0, 0, 0, 0, 0, typ}
	f = append(f, body...)

	return append(f, checksum(f[1:]))
}

// FrameGetState queries the basic state (answer 0xC0).
func FrameGetState() []byte {
	return BuildFrame(FrameQuery, []byte{0x41, 0x81, 0x00, 0xFF, 0x03, 0xFF, 0x00, 0x02,
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x03}, -1)
}

// FrameGetCapabilities queries the capabilities (answer 0xB5).
func FrameGetCapabilities() []byte { return BuildFrame(FrameQuery, []byte{0xB5, 0x01, 0x00}, -1) }

// FrameGetGroup queries diagnostic group n (answer 0xC1).
func FrameGetGroup(group byte) []byte {
	p := make([]byte, 20)
	p[0], p[1], p[2], p[3] = 0x41, 0x21, 0x01, 0x40|group

	return BuildFrame(FrameQuery, p, -1)
}

// FrameToggleDisplay toggles the display (the device only knows "toggle").
func FrameToggleDisplay(beep bool) []byte {
	src := byte(controlSource)
	if beep {
		src |= 0x40
	}

	return BuildFrame(FrameQuery, []byte{0x41, src, 0x00, 0xFF, 0x02,
		0x00, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, -1)
}

// FrameGetProperties reads properties (answer 0xB1).
func FrameGetProperties(props ...PropertyID) []byte {
	p := []byte{0xB1, byte(len(props))}
	for _, id := range props {
		p = append(p, byte(id), byte(id>>8))
	}

	return BuildFrame(FrameQuery, p, -1)
}

// FrameSetPropertyRaw writes one property with raw value bytes.
func FrameSetPropertyRaw(id PropertyID, value []byte) []byte {
	p := []byte{0xB0, 1, byte(id), byte(id >> 8), byte(len(value))}

	return BuildFrame(FrameControl, append(p, value...), -1)
}

// FrameSetProperty writes a boolean property.
func FrameSetProperty(id PropertyID, on bool) []byte { return FrameSetPropertyRaw(id, id.Encode(on)) }

// FrameSetRate sets the power limit: 100 = none, else 75 or 50 percent.
func FrameSetRate(percent int) []byte {
	return FrameSetPropertyRaw(PropRateSelect, []byte{byte(percent)})
}

// FrameSetState builds the control frame (0x40) for a target state.
func FrameSetState(s State, beep bool) []byte {
	target := s.TargetTemperature
	if math.IsNaN(target) || math.IsInf(target, 0) {
		target = 24
	}

	target = math.Min(30, math.Max(16, target))
	integral := int(math.Floor(target))
	half := target-float64(integral) > 0

	var temp, tempAlt byte
	if integral >= 17 && integral <= 30 {
		temp = byte((integral - 16) & 0xF)
	} else {
		tempAlt = byte((integral - 12) & 0x1F)
	}

	if half {
		temp |= 0x10
	}

	mode := byte((int(s.Mode) & 0x7) << 5)
	// Always send existing timers along – 0x7F 0x7F 0x00 would delete them.
	t0, t1, t2 := encodeDeviceTimer(s.OnTimerMinutes, s.OffTimerMinutes)

	bit := func(v bool, b byte) byte {
		if v {
			return b
		}

		return 0
	}

	payload := []byte{
		0x40,
		controlSource | bit(beep, 0x40) | bit(s.Power, 0x01),
		temp | mode,
		byte(s.FanSpeed & 0x7F),
		t0, t1, t2,
		0x30 | byte(s.Swing&0x3F),
		bit(s.FollowMe, 0x80) | bit(s.Turbo, 0x20),
		bit(s.Eco, 0x80) | bit(s.Purifier, 0x20),
		bit(s.Sleep, 0x01) | bit(s.Turbo, 0x02) | bit(s.Fahrenheit, 0x04),
		0, 0, 0, 0, 0, 0, 0,
		tempAlt,
		40,
		0x00,
		bit(s.FreezeProtection, 0x80),
		0x00,
		0x00,
	}

	return BuildFrame(FrameControl, payload, -1)
}

// MaxTimerMinutes is the longest device timer.
const MaxTimerMinutes = 24 * 60

// decodeDeviceTimer reads the timers in state bytes 4–6 (midea-beautiful-air):
// byte 4/5 = 0x80 active | hours << 2 | quarters, byte 6 = rest minutes
// on (high nibble) / off (low nibble). Values over 24 h read as "no timer".
func decodeDeviceTimer(on, off, rest byte) (*int, *int) {
	minutes := func(b, r byte) *int {
		if b&0x80 == 0 {
			return nil
		}

		m := int((b&0x7C)>>2)*60 + int(b&0x3)*15 + int(r)
		if m < 1 || m > MaxTimerMinutes {
			return nil
		}

		return &m
	}

	return minutes(on, rest>>4), minutes(off, rest&0xF)
}

func encodeDeviceTimer(on, off *int) (byte, byte, byte) {
	parts := func(m *int) (byte, byte) {
		if m == nil || *m <= 0 {
			return 0x7F, 0
		}

		total := min(*m, MaxTimerMinutes)
		hours := byte(min(31, total/60))
		quarters := byte((total % 60) / 15)

		return 0x80 | hours<<2 | quarters, byte(total % 15)
	}

	onB, onR := parts(on)
	offB, offR := parts(off)

	return onB, offB, onR<<4 | offR
}

var crcTable = [256]byte{
	0x00, 0x5E, 0xBC, 0xE2, 0x61, 0x3F, 0xDD, 0x83, 0xC2, 0x9C, 0x7E, 0x20, 0xA3, 0xFD, 0x1F, 0x41,
	0x9D, 0xC3, 0x21, 0x7F, 0xFC, 0xA2, 0x40, 0x1E, 0x5F, 0x01, 0xE3, 0xBD, 0x3E, 0x60, 0x82, 0xDC,
	0x23, 0x7D, 0x9F, 0xC1, 0x42, 0x1C, 0xFE, 0xA0, 0xE1, 0xBF, 0x5D, 0x03, 0x80, 0xDE, 0x3C, 0x62,
	0xBE, 0xE0, 0x02, 0x5C, 0xDF, 0x81, 0x63, 0x3D, 0x7C, 0x22, 0xC0, 0x9E, 0x1D, 0x43, 0xA1, 0xFF,
	0x46, 0x18, 0xFA, 0xA4, 0x27, 0x79, 0x9B, 0xC5, 0x84, 0xDA, 0x38, 0x66, 0xE5, 0xBB, 0x59, 0x07,
	0xDB, 0x85, 0x67, 0x39, 0xBA, 0xE4, 0x06, 0x58, 0x19, 0x47, 0xA5, 0xFB, 0x78, 0x26, 0xC4, 0x9A,
	0x65, 0x3B, 0xD9, 0x87, 0x04, 0x5A, 0xB8, 0xE6, 0xA7, 0xF9, 0x1B, 0x45, 0xC6, 0x98, 0x7A, 0x24,
	0xF8, 0xA6, 0x44, 0x1A, 0x99, 0xC7, 0x25, 0x7B, 0x3A, 0x64, 0x86, 0xD8, 0x5B, 0x05, 0xE7, 0xB9,
	0x8C, 0xD2, 0x30, 0x6E, 0xED, 0xB3, 0x51, 0x0F, 0x4E, 0x10, 0xF2, 0xAC, 0x2F, 0x71, 0x93, 0xCD,
	0x11, 0x4F, 0xAD, 0xF3, 0x70, 0x2E, 0xCC, 0x92, 0xD3, 0x8D, 0x6F, 0x31, 0xB2, 0xEC, 0x0E, 0x50,
	0xAF, 0xF1, 0x13, 0x4D, 0xCE, 0x90, 0x72, 0x2C, 0x6D, 0x33, 0xD1, 0x8F, 0x0C, 0x52, 0xB0, 0xEE,
	0x32, 0x6C, 0x8E, 0xD0, 0x53, 0x0D, 0xEF, 0xB1, 0xF0, 0xAE, 0x4C, 0x12, 0x91, 0xCF, 0x2D, 0x73,
	0xCA, 0x94, 0x76, 0x28, 0xAB, 0xF5, 0x17, 0x49, 0x08, 0x56, 0xB4, 0xEA, 0x69, 0x37, 0xD5, 0x8B,
	0x57, 0x09, 0xEB, 0xB5, 0x36, 0x68, 0x8A, 0xD4, 0x95, 0xCB, 0x29, 0x77, 0xF4, 0xAA, 0x48, 0x16,
	0xE9, 0xB7, 0x55, 0x0B, 0x88, 0xD6, 0x34, 0x6A, 0x2B, 0x75, 0x97, 0xC9, 0x4A, 0x14, 0xF6, 0xA8,
	0x74, 0x2A, 0xC8, 0x96, 0x15, 0x4B, 0xA9, 0xF7, 0xB6, 0xE8, 0x0A, 0x54, 0xD7, 0x89, 0x6B, 0x35,
}

func crc8(b []byte) byte {
	var c byte
	for _, x := range b {
		c = crcTable[c^x]
	}

	return c
}
