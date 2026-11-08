// Package fakeac simulates a Midea PortaSplit on the LAN (protocol V3 on TCP
// 6444, discovery on UDP 6445). It is used by tests and by
// `portacontrol-fakeac` for development without hardware.
package fakeac

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/midea"
)

// AC is the simulated device.
type AC struct {
	DeviceID uint64
	Name     string
	SN       string
	Token    []byte // 64 bytes
	Key      []byte // 32 bytes
	Log      *slog.Logger

	mu        sync.Mutex
	st        sim
	busy      bool
	tcp       net.Listener
	udp       []*net.UDPConn
	lastTick  time.Time
	closed    bool
	Port      int
	Discovery int
}

type sim struct {
	power, eco, turbo, sleep, purifier, freeze, fahrenheit, swing bool
	display, filter                                               bool
	outSilent, selfClean, ieco, sound                             bool
	mode                                                          midea.Mode
	target                                                        float64
	fan                                                           int
	indoor, outdoor                                               float64
	watts                                                         float64
	totalKWh, runKWh                                              float64
	rate                                                          int
	onTimer, offTimer                                             time.Time
	errorCode                                                     int
}

// New creates a simulated AC with random credentials.
func New(deviceID uint64) *AC {
	tok := make([]byte, 64)
	key := make([]byte, 32)
	_, _ = rand.Read(tok)
	_, _ = rand.Read(key)

	return &AC{
		DeviceID: deviceID, Name: "net_ac_" + strconv.FormatUint(deviceID%10000, 10), SN: "000000P0000000Q1FAKEPORTASPLIT",
		Token: tok, Key: key, Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		st: sim{mode: midea.ModeCool, target: 23, fan: midea.FanAuto, indoor: 26.5, outdoor: 31, display: true,
			sound: true, rate: 100, totalKWh: 123.45},
	}
}

// TokenHex and KeyHex return the credentials for setup.
func (a *AC) TokenHex() string { return hex.EncodeToString(a.Token) }

// KeyHex returns the key as hex.
func (a *AC) KeyHex() string { return hex.EncodeToString(a.Key) }

// Listen starts the TCP server on addr (e.g. "127.0.0.1:0") and, if
// discoveryAddr is not empty, the UDP discovery responder.
func (a *AC) Listen(addr, discoveryAddr string) error {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	a.tcp = l
	a.Port = l.Addr().(*net.TCPAddr).Port
	a.lastTick = time.Now()

	go a.accept()

	if discoveryAddr != "" {
		host, port, _ := net.SplitHostPort(discoveryAddr)
		p, _ := strconv.Atoi(port)

		ports := []int{p}
		if p == midea.DiscoveryPorts[0] {
			ports = midea.DiscoveryPorts
		}

		for _, p := range ports {
			u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(host), Port: p})
			if err != nil {
				return err
			}

			if a.Discovery == 0 {
				a.Discovery = u.LocalAddr().(*net.UDPAddr).Port
			}

			a.udp = append(a.udp, u)

			go a.discovery(u)
		}
	}

	return nil
}

// Close stops the simulator.
func (a *AC) Close() {
	a.mu.Lock()
	a.closed = true
	a.mu.Unlock()

	if a.tcp != nil {
		_ = a.tcp.Close()
	}

	for _, u := range a.udp {
		_ = u.Close()
	}
}

func (a *AC) discovery(u *net.UDPConn) {
	buf := make([]byte, 2048)

	for {
		n, from, err := u.ReadFromUDP(buf)
		if err != nil {
			return
		}

		if !midea.IsDiscoveryRequest(buf[:n]) {
			continue
		}

		local := net.IPv4(127, 0, 0, 1)
		reply, err := midea.BuildDiscoveryReply(local, a.Port, a.DeviceID, a.SN, a.Name)

		if err == nil {
			_, _ = u.WriteToUDP(reply, from)
		}
	}
}

func (a *AC) accept() {
	for {
		c, err := a.tcp.Accept()
		if err != nil {
			return
		}

		a.mu.Lock()
		busy := a.busy
		if !busy {
			a.busy = true
		}
		a.mu.Unlock()

		if busy {
			// Like the real device: only one local connection at a time.
			_ = c.Close()
			continue
		}

		go func() {
			defer func() {
				a.mu.Lock()
				a.busy = false
				a.mu.Unlock()
			}()

			a.serve(c)
		}()
	}
}

func (a *AC) serve(c net.Conn) {
	defer c.Close()

	var codec midea.V3Codec

	buf := make([]byte, 4096)

	for {
		_ = c.SetReadDeadline(time.Now().Add(2 * time.Minute))

		n, err := c.Read(buf)
		if err != nil {
			return
		}

		for _, pkt := range codec.Feed(buf[:n]) {
			out, err := a.handle(&codec, pkt)
			if err != nil {
				a.Log.Debug("fakeac: drop packet", "err", err)
				return
			}

			for _, o := range out {
				if _, err := c.Write(o); err != nil {
					return
				}
			}
		}
	}
}

var errBadToken = errors.New("bad token")

func (a *AC) handle(codec *midea.V3Codec, pkt []byte) ([][]byte, error) {
	if pkt[5]&0xF == 0 { // handshake
		_, tok, err := codec.Decode(pkt)
		if err != nil {
			return nil, err
		}

		if string(tok) != string(a.Token) {
			// Real devices answer with an error packet.
			return [][]byte{{0x83, 0x70, 0x00, 0x00, 0x20, 0x0F, 0x00, 0x00}}, nil
		}

		plain := make([]byte, 32)
		_, _ = rand.Read(plain)

		enc, err := encryptCBC(a.Key, plain)
		if err != nil {
			return nil, err
		}

		sum := sha(plain)
		resp := []byte{0x83, 0x70, 0x00, 0x40, 0x20, 0x01, 0x00, 0x00}
		resp = append(resp, enc...)
		resp = append(resp, sum...)

		lk := make([]byte, 32)
		for i := range plain {
			lk[i] = plain[i] ^ a.Key[i]
		}

		codec.LocalKey = lk

		return [][]byte{resp}, nil
	}

	_, payload, err := codec.Decode(pkt)
	if err != nil {
		return nil, err
	}

	frame, err := midea.DecodeV2(payload)
	if err != nil {
		return nil, err
	}

	var out [][]byte

	for _, r := range a.process(frame) {
		v2, err := midea.EncodeV2(a.DeviceID, r, time.Now())
		if err != nil {
			return nil, err
		}

		enc, err := codec.EncodeEncrypted(v2, 0x3)
		if err != nil {
			return nil, err
		}

		out = append(out, enc)
	}

	return out, nil
}

// Process answers one command frame (exported for cloud tests).
func (a *AC) Process(frame []byte) [][]byte { return a.process(frame) }

func (a *AC) process(frame []byte) [][]byte {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.tick()

	if len(frame) < 13 {
		return nil
	}

	typ := frame[9]
	p := frame[10 : len(frame)-2]

	switch p[0] {
	case 0x40:
		a.applyControl(p)
		return [][]byte{midea.BuildFrame(typ, a.statePayload(), int(frame[len(frame)-3]))}
	case 0x41:
		if len(p) > 3 && p[1] == 0x21 {
			return [][]byte{midea.BuildFrame(typ, a.groupPayload(p[3]&0xF), -1)}
		}

		if len(p) > 4 && p[4] == 0x02 {
			a.st.display = !a.st.display
		}

		return [][]byte{midea.BuildFrame(typ, a.statePayload(), -1)}
	case 0xB0:
		if len(p) >= 6 {
			id := midea.PropertyID(uint16(p[2]) | uint16(p[3])<<8)
			a.setProperty(id, p[5:5+min(int(p[4]), len(p)-5)])
		}

		return [][]byte{midea.BuildFrame(typ, a.propertiesPayload(0xB0, []midea.PropertyID{}), -1)}
	case 0xB1:
		var ids []midea.PropertyID
		for i := 0; i+1 < len(p)-2 && len(ids) < int(p[1]); i += 2 {
			ids = append(ids, midea.PropertyID(uint16(p[2+i])|uint16(p[3+i])<<8))
		}

		return [][]byte{midea.BuildFrame(typ, a.propertiesPayload(0xB1, ids), -1)}
	case 0xB5:
		return [][]byte{midea.BuildFrame(typ, []byte{0xB5, 0x02, 0x14, 0x02, 0x01, 0x01, 0x16, 0x02, 0x01, 0x01}, -1)}
	}

	return nil
}

func (a *AC) setProperty(id midea.PropertyID, v []byte) {
	if len(v) == 0 {
		return
	}

	switch id {
	case midea.PropOutSilent:
		// Like the PortaSplit: only while cooling or heating.
		if a.st.mode == midea.ModeCool || a.st.mode == midea.ModeHeat {
			a.st.outSilent = v[0] == 3
		}
	case midea.PropSelfClean:
		a.st.selfClean = v[0] != 0
	case midea.PropIECO:
		if len(v) >= 3 {
			a.st.ieco = v[2] != 0
		}
	case midea.PropSound:
		a.st.sound = v[0] != 0
	case midea.PropRateSelect:
		// The PortaSplit knows only 50 and 100 %.
		if v[0] == 50 || v[0] == 100 {
			a.st.rate = int(v[0])
		}
	}
}

func (a *AC) propertiesPayload(kind byte, ids []midea.PropertyID) []byte {
	if kind == 0xB0 {
		ids = []midea.PropertyID{midea.PropOutSilent, midea.PropSelfClean, midea.PropIECO, midea.PropSound, midea.PropRateSelect}
	}

	p := []byte{kind, 0}

	for _, id := range ids {
		var data []byte

		switch id {
		case midea.PropOutSilent:
			data = id.Encode(a.st.outSilent)
		case midea.PropSelfClean:
			data = id.Encode(a.st.selfClean)
		case midea.PropIECO:
			data = []byte{0, b2(a.st.ieco)}
		case midea.PropSound:
			data = id.Encode(a.st.sound)
		case midea.PropRateSelect:
			data = []byte{byte(a.st.rate)}
		case midea.PropIdentifier:
			data = []byte("00AC000111171206000046091012")
		case midea.PropIndoorT2:
			v := int(a.st.indoor*100) + 20
			data = []byte{6, 0, 0, byte(v), byte(v >> 8), 0, 0, 0, 0, 0, 0, 0, 0}
		default:
			p = append(p, byte(id), byte(id>>8), 0x10, 0)
			p[1]++

			continue
		}

		p = append(p, byte(id), byte(id>>8), 0, byte(len(data)))
		p = append(p, data...)
		p[1]++
	}

	return p
}

func b2(v bool) byte {
	if v {
		return 1
	}

	return 0
}

func (a *AC) applyControl(p []byte) {
	if len(p) < 22 {
		return
	}

	wasOn := a.st.power
	a.st.power = p[1]&0x1 != 0
	a.st.mode = midea.Mode(int(p[2]>>5) & 0x7)
	t := float64(p[2]&0xF) + 16

	if alt := p[18] & 0x1F; alt != 0 {
		t = float64(alt) + 12
	}

	if p[2]&0x10 != 0 {
		t += 0.5
	}

	a.st.target = t
	a.st.fan = int(p[3] & 0x7F)
	a.st.swing = p[7]&0x0F == midea.SwingVertical
	a.st.turbo = p[8]&0x20 != 0 || p[10]&0x02 != 0
	a.st.eco = p[9]&0x80 != 0
	a.st.purifier = p[9]&0x20 != 0
	a.st.sleep = p[10]&0x01 != 0
	a.st.fahrenheit = p[10]&0x04 != 0
	a.st.freeze = p[21]&0x80 != 0 && a.st.mode == midea.ModeHeat

	now := time.Now()
	a.st.onTimer = timerAt(now, p[4], p[6]>>4)
	a.st.offTimer = timerAt(now, p[5], p[6]&0xF)

	if a.st.power && !wasOn {
		a.st.runKWh = 0
	}
}

func timerAt(now time.Time, b, rest byte) time.Time {
	if b&0x80 == 0 {
		return time.Time{}
	}

	m := int((b&0x7C)>>2)*60 + int(b&0x3)*15 + int(rest)
	if m <= 0 {
		return time.Time{}
	}

	return now.Add(time.Duration(m) * time.Minute)
}

func timerBytes(now, at time.Time) (byte, byte) {
	if at.IsZero() || !at.After(now) {
		return 0x7F, 0
	}

	m := int(math.Ceil(at.Sub(now).Minutes()))
	m = min(m, midea.MaxTimerMinutes)

	return 0x80 | byte(min(31, m/60))<<2 | byte((m%60)/15), byte(m % 15)
}

// tick advances the simulation: temperature, power and energy.
func (a *AC) tick() {
	now := time.Now()
	dt := now.Sub(a.lastTick).Hours()
	a.lastTick = now

	if !a.st.onTimer.IsZero() && !now.Before(a.st.onTimer) {
		a.st.power, a.st.onTimer = true, time.Time{}
	}

	if !a.st.offTimer.IsZero() && !now.Before(a.st.offTimer) {
		a.st.power, a.st.offTimer = false, time.Time{}
	}

	a.st.watts = 0

	if a.st.power {
		switch a.st.mode {
		case midea.ModeCool, midea.ModeDry, midea.ModeAuto:
			diff := a.st.indoor - a.st.target
			a.st.watts = math.Max(90, math.Min(950, 250+diff*180))
		case midea.ModeHeat:
			diff := a.st.target - a.st.indoor
			a.st.watts = math.Max(90, math.Min(1100, 300+diff*200))
		case midea.ModeFan:
			a.st.watts = 25
		}

		if a.st.turbo {
			a.st.watts *= 1.25
		}

		if a.st.eco || a.st.rate == 50 {
			a.st.watts *= 0.7
		}

		// Move the room temperature towards the target (per second of sim time).
		step := dt * 60 * 0.08
		if a.st.mode == midea.ModeFan {
			step = 0
		}

		if a.st.indoor > a.st.target {
			a.st.indoor = math.Max(a.st.target, a.st.indoor-step)
		} else {
			a.st.indoor = math.Min(a.st.target, a.st.indoor+step)
		}
	} else {
		a.st.indoor = math.Min(29, a.st.indoor+dt*0.6)
	}

	kwh := a.st.watts / 1000 * dt
	a.st.totalKWh += kwh
	a.st.runKWh += kwh
}

func tempRaw(t float64) (byte, byte) {
	raw := byte(math.Round(t*2) + 50)
	dec := byte(math.Round(math.Abs(t-math.Trunc(t))*10)) & 0xF

	return raw, dec
}

func (a *AC) statePayload() []byte {
	s := a.st
	p := make([]byte, 24)
	p[0] = 0xC0
	p[1] = b2(s.power)

	t := s.target
	half := t-math.Floor(t) > 0
	p[2] = byte(int(math.Floor(t))-16)&0xF | byte(s.mode)<<5

	if half {
		p[2] |= 0x10
	}

	p[3] = byte(s.fan)
	now := time.Now()
	onB, onR := timerBytes(now, s.onTimer)
	offB, offR := timerBytes(now, s.offTimer)
	p[4], p[5], p[6] = onB, offB, onR<<4|offR
	p[7] = 0x30

	if s.swing {
		p[7] |= midea.SwingVertical
	}

	if s.turbo {
		p[8] |= 0x20
		p[10] |= 0x02
	}

	if s.eco {
		p[9] |= 0x10
	}

	if s.purifier {
		p[9] |= 0x20
	}

	if s.sleep {
		p[10] |= 0x01
	}

	if s.fahrenheit {
		p[10] |= 0x04
	}

	in, inDec := tempRaw(s.indoor)
	out, outDec := tempRaw(s.outdoor)

	if !s.power {
		out, outDec = tempRaw(10)
	}

	p[11], p[12] = in, out
	p[15] = outDec<<4 | inDec

	if s.filter {
		p[13] |= 0x20
	}

	if !s.display {
		p[14] = 0x70
	}

	p[16] = byte(s.errorCode)

	if s.freeze {
		p[21] = 0x80
	}

	return p
}

func (a *AC) groupPayload(g byte) []byte {
	s := a.st
	p := make([]byte, 24)
	p[0], p[1], p[2], p[3] = 0xC1, 0x21, 0x01, 0x40|g

	on := s.power && s.mode != midea.ModeFan
	hz := byte(0)

	if on {
		hz = byte(math.Min(90, s.watts/10))
	}

	switch g {
	case 1:
		p[4], p[5] = hz, hz+2
		p[7], p[8] = byte(s.watts/230), 231
		p[10] = byte(s.indoor*2 + 30)
		p[11] = byte((s.indoor-8)*2 + 30)
		p[12] = byte((s.outdoor+6)*2 + 50)
		p[13] = byte(s.outdoor*2 + 50)
		p[14] = 60 + hz/3
	case 2:
		if s.power {
			p[4], p[5] = 120, 118
		}
	case 3:
		p[11] = 180
	case 4:
		total := int(math.Round(s.totalKWh * 100))
		run := int(math.Round(s.runKWh * 100))
		w := int(math.Round(s.watts * 10))
		p[4], p[5], p[6], p[7] = byte(total>>24), byte(total>>16), byte(total>>8), byte(total)
		p[12], p[13], p[14], p[15] = byte(run>>24), byte(run>>16), byte(run>>8), byte(run)
		p[16], p[17], p[18] = byte(w>>16), byte(w>>8), byte(w)
	case 5:
		p[4] = 48
		if on {
			p[8] = 90
		}
	case 7:
		cw := int(s.watts * 0.85)
		p[10], p[11] = byte(cw), byte(cw>>8)
	}

	return p
}

// SetIndoor sets the simulated room temperature (tests).
func (a *AC) SetIndoor(t float64) {
	a.mu.Lock()
	a.st.indoor = t
	a.mu.Unlock()
}
