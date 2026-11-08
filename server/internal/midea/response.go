package midea

import (
	"fmt"
	"math"
)

// State is the device state as it reports it (answer 0xC0) plus extras read
// from properties and group 4. JSON names are the API contract.
type State struct {
	Power             bool     `json:"power"`
	Mode              Mode     `json:"-"`
	ModeName          string   `json:"mode"`
	TargetTemperature float64  `json:"target_temperature"`
	FanSpeed          int      `json:"fan_speed"`
	Swing             int      `json:"-"`
	SwingOn           bool     `json:"swing"`
	Eco               bool     `json:"eco"`
	Turbo             bool     `json:"turbo"`
	Sleep             bool     `json:"sleep"`
	Fahrenheit        bool     `json:"fahrenheit"`
	FollowMe          bool     `json:"follow_me"`
	Purifier          bool     `json:"purifier"`
	FreezeProtection  bool     `json:"freeze_protection"`
	DisplayOn         bool     `json:"display_on"`
	FilterAlert       bool     `json:"filter_alert"`
	IndoorTemperature *float64 `json:"indoor_temperature"`
	OutdoorTemp       *float64 `json:"outdoor_temperature"`
	ErrorCode         int      `json:"error_code"`
	OutSilent         *bool    `json:"outdoor_silent"`
	SelfClean         *bool    `json:"self_clean"`
	IECO              *bool    `json:"ieco"`
	Sound             *bool    `json:"sound"`
	RatePercent       *int     `json:"rate_percent"`
	OnTimerMinutes    *int     `json:"on_timer_minutes"`
	OffTimerMinutes   *int     `json:"off_timer_minutes"`
	PowerWatts        *float64 `json:"power_watts"`
	TotalEnergyKWh    *float64 `json:"total_energy_kwh"`
	RunEnergyKWh      *float64 `json:"run_energy_kwh"`
}

// Normalize fills the derived JSON fields.
func (s *State) Normalize() {
	s.ModeName = s.Mode.String()
	s.SwingOn = s.Swing != SwingOff
}

// standbyPlaceholder is the outdoor temperature the PortaSplit reports while
// switched off instead of a measurement.
const standbyPlaceholder = 10.0

// Response kinds.
const (
	RespState      = 0xC0
	RespGroup      = 0xC1
	RespProperties = 0xB1
	RespSetProps   = 0xB0
	RespCaps       = 0xB5
)

// FramePayload checks a frame (checksum, device type) and returns its
// payload (bytes 10..len-2).
func FramePayload(f []byte) ([]byte, error) {
	if len(f) < 13 {
		return nil, fmt.Errorf("%w: frame too short", ErrProtocol)
	}

	if checksum(f[1:len(f)-1]) != f[len(f)-1] {
		return nil, fmt.Errorf("%w: bad checksum", ErrProtocol)
	}

	if f[2] != deviceTypeAC {
		return nil, fmt.Errorf("%w: device type 0x%x", ErrProtocol, f[2])
	}

	return f[10 : len(f)-2], nil
}

// ResponseID is byte 10 of a frame (0 if too short).
func ResponseID(f []byte) byte {
	if len(f) > 10 {
		return f[10]
	}

	return 0
}

// ParseState parses a 0xC0 payload.
func ParseState(p []byte) (State, bool) {
	var s State
	if len(p) < 17 || p[0] != RespState {
		return s, false
	}

	s.Power = p[1]&0x1 != 0
	half := 0.0
	if p[2]&0x10 != 0 {
		half = 0.5
	}

	s.TargetTemperature = float64(p[2]&0xF) + 16 + half
	s.Mode = Mode(int(p[2]>>5) & 0x7)
	if !s.Mode.Valid() {
		s.Mode = ModeAuto
	}

	s.FanSpeed = int(p[3] & 0x7F)
	s.OnTimerMinutes, s.OffTimerMinutes = decodeDeviceTimer(p[4], p[5], p[6])
	if sw := int(p[7] & 0xF); sw == SwingVertical {
		s.Swing = SwingVertical
	}

	s.Turbo = p[8]&0x20 != 0 || p[10]&0x2 != 0
	s.FollowMe = p[8]&0x80 != 0
	s.Eco = p[9]&0x10 != 0
	s.Purifier = p[9]&0x20 != 0
	s.Sleep = p[10]&0x1 != 0
	s.Fahrenheit = p[10]&0x4 != 0
	s.IndoorTemperature = decodeTemperature(p[11], float64(p[15]&0xF)/10, s.Fahrenheit)
	s.OutdoorTemp = decodeTemperature(p[12], float64(p[15]>>4)/10, s.Fahrenheit)

	if !s.Power && s.OutdoorTemp != nil && *s.OutdoorTemp == standbyPlaceholder {
		s.OutdoorTemp = nil
	}

	if alt := p[13] & 0x1F; alt != 0 {
		s.TargetTemperature = float64(alt) + 12 + half
	}

	s.FilterAlert = p[13]&0x20 != 0
	s.DisplayOn = p[14] != 0x70
	s.ErrorCode = int(p[16])

	if len(p) >= 22 {
		s.FreezeProtection = p[21]&0x80 != 0
	}

	s.Normalize()

	return s, true
}

func decodeTemperature(raw byte, decimals float64, fahrenheit bool) *float64 {
	if raw == 0xFF {
		return nil
	}

	t := (float64(raw) - 50) / 2
	whole := math.Trunc(t)
	sign := 1.0

	if t < 0 {
		sign = -1
	}

	var v float64

	switch {
	case !fahrenheit && decimals != 0:
		v = whole + sign*decimals
	case decimals >= 0.5:
		v = whole + sign*0.5
	default:
		v = t
	}

	v = math.Round(v*10) / 10

	return &v
}

// ParseProperties parses a 0xB0/0xB1 payload: per entry ID (2 byte LE),
// result, length, data. Entries with an error flag are skipped.
func ParseProperties(p []byte) map[PropertyID][]byte {
	out := map[PropertyID][]byte{}
	if len(p) < 2 {
		return out
	}

	i := 2
	for range int(p[1]) {
		if i+4 > len(p) {
			break
		}

		id := PropertyID(uint16(p[i]) | uint16(p[i+1])<<8)
		size := int(p[i+3])
		end := min(len(p), i+4+size)

		if size > 0 && p[i+2]&0x10 == 0 && i+4 < end {
			out[id] = append([]byte{}, p[i+4:end]...)
		}

		i += 4 + size
	}

	return out
}

// counter reads a 32 bit big endian counter (group 4).
func counter(p []byte, i int) int {
	return int(p[i])<<24 | int(p[i+1])<<16 | int(p[i+2])<<8 | int(p[i+3])
}

// ParseEnergy parses group 4. The PortaSplit counts binary, not BCD: bytes
// 4–7 total counter, 12–15 current run, one step = 0.01 kWh; bytes 16–18 =
// power / 10 W. Counter 0 or 0xFFFFFFFF counts as unknown, never as 0 kWh.
func ParseEnergy(p []byte) (watts, totalKWh, runKWh *float64) {
	if len(p) <= 19 {
		return nil, nil, nil
	}

	raw := counter(p, 4)
	run := counter(p, 12)
	power := float64(int(p[16])<<16|int(p[17])<<8|int(p[18])) / 10

	if raw == 0 && run == 0 && power == 0 {
		return nil, nil, nil
	}

	w := math.Round(power*10) / 10
	watts = &w

	if raw != 0 && raw != 0xFFFFFFFF {
		t := float64(raw) / 100
		totalKWh = &t
	}

	if run != 0xFFFFFFFF {
		r := float64(run) / 100
		runKWh = &r
	}

	return watts, totalKWh, runKWh
}

// Diagnostics are the values of the group queries (0xC1), mapping after
// msmart-ng; EEV from group 3 byte 11 (Klimacontrol analysis).
type Diagnostics struct {
	CompressorHz       *int              `json:"compressor_hz"`
	CompressorTargetHz *int              `json:"compressor_target_hz"`
	CurrentA           *int              `json:"current_a"`
	VoltageV           *int              `json:"voltage_v"`
	T1IndoorAir        *float64          `json:"t1_indoor_air"`
	T2IndoorCoil       *float64          `json:"t2_indoor_coil"`
	IndoorTemp2        *float64          `json:"indoor_temp2"`
	T3OutdoorCoil      *float64          `json:"t3_outdoor_coil"`
	T4OutdoorAir       *float64          `json:"t4_outdoor_air"`
	TPDischarge        *int              `json:"tp_discharge"`
	IndoorFanRPM       *int              `json:"indoor_fan_rpm"`
	IndoorFanTargetRPM *int              `json:"indoor_fan_target_rpm"`
	CondensatePump     *bool             `json:"condensate_pump"`
	EEVOpening         *int              `json:"eev_opening"`
	OutdoorFanRPM      *int              `json:"outdoor_fan_rpm"`
	Defrost            *bool             `json:"defrost"`
	Humidity           *int              `json:"humidity"`
	CompressorPowerW   *int              `json:"compressor_power_w"`
	CurrentRunKWh      *float64          `json:"current_run_kwh"`
	EnergyCounterRaw   *int              `json:"energy_counter_raw"`
	Identifier         string            `json:"identifier,omitempty"`
	Raw                map[string]string `json:"raw"`
}

// DiagnosticGroups are the groups read for diagnostics.
var DiagnosticGroups = []byte{1, 2, 3, 4, 5, 7}

func ip(v int) *int         { return &v }
func fp(v float64) *float64 { return &v }
func bp(v bool) *bool       { return &v }

// Apply fills the values of one group payload.
func (d *Diagnostics) Apply(group byte, p []byte) {
	if d.Raw == nil {
		d.Raw = map[string]string{}
	}

	d.Raw[fmt.Sprint(group)] = hexs(p)
	if len(p) < 15 {
		return
	}

	switch group {
	case 1:
		d.CompressorHz, d.CompressorTargetHz = ip(int(p[4])), ip(int(p[5]))
		d.CurrentA, d.VoltageV = ip(int(p[7])), ip(int(p[8]))
		d.T1IndoorAir = fp((float64(p[10]) - 30) / 2)
		d.T2IndoorCoil = fp((float64(p[11]) - 30) / 2)
		d.T3OutdoorCoil = fp((float64(p[12]) - 50) / 2)
		d.T4OutdoorAir = fp((float64(p[13]) - 50) / 2)
		d.TPDischarge = ip(int(p[14]))
	case 2:
		d.IndoorFanTargetRPM, d.IndoorFanRPM = ip(int(p[4])*8), ip(int(p[5])*8)
		d.CondensatePump = bp(p[8]&0x10 != 0)
	case 3:
		d.EEVOpening = ip(int(p[11]))
	case 4:
		if len(p) >= 16 {
			d.CurrentRunKWh = fp(float64(counter(p, 12)) / 100)
		}

		if len(p) >= 8 {
			d.EnergyCounterRaw = ip(counter(p, 4))
		}
	case 5:
		if p[4] != 0 {
			d.Humidity = ip(int(p[4]))
		}

		d.OutdoorFanRPM = ip(int(p[8]) * 8)
		d.Defrost = bp(p[10] != 0)
	case 7:
		d.CompressorPowerW = ip(int(p[10]) + 256*int(p[11]))
	}
}

// DropStandbyPlaceholders discards T4 = 10.0 °C while the device is off.
func (d *Diagnostics) DropStandbyPlaceholders(power bool) {
	if !power && d.T4OutdoorAir != nil && *d.T4OutdoorAir == standbyPlaceholder {
		d.T4OutdoorAir = nil
	}
}

// IsGroupReply reports whether f is the 0xC1 answer of exactly this group.
func IsGroupReply(f []byte, group byte) bool {
	return len(f) > 14 && f[10] == RespGroup && f[13]&0x0F == group&0x0F
}
