package midea

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Discovered is a Midea device that answered the discovery broadcast.
// Discovery has no secret: never trust an answer before a handshake with the
// stored token worked.
type Discovered struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	DeviceID uint64 `json:"device_id,string"`
	SN       string `json:"sn"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Version  int    `json:"version"`
}

// IsAirConditioner reports type 0xAC (or 0xCC).
func (d Discovered) IsAirConditioner() bool { return d.Type == 0xAC || d.Type == 0xCC }

var discoveryMessage = []byte{
	0x5a, 0x5a, 0x01, 0x11, 0x48, 0x00, 0x92, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7f, 0x75, 0xbd, 0x6b, 0x3e, 0x4f, 0x8b, 0x76,
	0x2e, 0x84, 0x9c, 0x6e, 0x57, 0x8d, 0x65, 0x90, 0x03, 0x6e, 0x9d, 0x43, 0x42, 0xa5, 0x0f, 0x1f,
	0x56, 0x9e, 0xb8, 0xec, 0x91, 0x8e, 0x92, 0xe5,
}

// DiscoveryPorts are the UDP ports devices listen on.
var DiscoveryPorts = []int{6445, 20086}

// IsDiscoveryRequest reports whether b is the discovery broadcast (simulator).
func IsDiscoveryRequest(b []byte) bool { return bytes.Equal(b, discoveryMessage) }

// CleanText removes control characters and limits the length.
func CleanText(s, fallback string, maxLen int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}

		return r
	}, s))
	if s == "" {
		return fallback
	}

	if r := []rune(s); len(r) > maxLen {
		return string(r[:maxLen])
	}

	return s
}

// ParseDiscovery parses an answer (V2: 0x5A5A, V3: 0x8370 with 8 byte head
// and 16 byte tail). Every length is checked: any host can send anything.
func ParseDiscovery(data []byte, ip string) (Discovered, error) {
	p := data
	version := 2

	if len(p) < 8 {
		return Discovered{}, fmt.Errorf("%w: discovery answer too short", ErrProtocol)
	}

	switch {
	case p[0] == 0x83 && p[1] == 0x70:
		version = 3
		outer := (int(p[2])<<8 | int(p[3])) + 8

		if outer < 8+56+16 || outer > len(p) {
			return Discovered{}, fmt.Errorf("%w: discovery answer too short", ErrProtocol)
		}

		p = p[8 : outer-16]
	case p[0] == 0x5A && p[1] == 0x5A:
	default:
		return Discovered{}, fmt.Errorf("%w: unknown device (V1 is not supported)", ErrProtocol)
	}

	if len(p) < 56 || p[0] != 0x5A || p[1] != 0x5A {
		return Discovered{}, fmt.Errorf("%w: discovery answer too short", ErrProtocol)
	}

	length := int(p[4]) | int(p[5])<<8
	if length < 56+16 || length > len(p) || (length-56)%16 != 0 {
		return Discovered{}, fmt.Errorf("%w: discovery answer with wrong length", ErrProtocol)
	}

	p = p[:length]
	if !bytes.Equal(sign(p[:length-16]), p[length-16:]) {
		return Discovered{}, fmt.Errorf("%w: discovery answer with wrong signature", ErrProtocol)
	}

	var id uint64
	for i, b := range p[20:26] {
		id |= uint64(b) << (8 * i)
	}

	plain, err := DecryptECB(encKey, p[40:length-16])
	if err != nil {
		return Discovered{}, err
	}

	if len(plain) <= 41 {
		return Discovered{}, fmt.Errorf("%w: device profile too short", ErrProtocol)
	}

	nameLen := int(plain[40])
	name := CleanText(string(plain[41:min(len(plain), 41+nameLen)]), "", 60)
	typ := 0

	if parts := strings.Split(name, "_"); len(parts) > 1 {
		if v, err := strconv.ParseInt(parts[1], 16, 32); err == nil {
			typ = int(v)
		}
	}

	return Discovered{
		IP: ip, Port: int(plain[4]) | int(plain[5])<<8, DeviceID: id,
		SN:   CleanText(string(bytes.TrimRight(plain[8:40], "\x00")), "", 40),
		Name: name, Type: typ, Version: version,
	}, nil
}

// BuildDiscoveryReply builds a V3 discovery answer (used by the simulator).
func BuildDiscoveryReply(ip net.IP, port int, deviceID uint64, sn, name string) ([]byte, error) {
	plain := make([]byte, 41)
	if v4 := ip.To4(); v4 != nil {
		// Byte order as real devices: reversed.
		plain[0], plain[1], plain[2], plain[3] = v4[3], v4[2], v4[1], v4[0]
	}

	plain[4], plain[5] = byte(port), byte(port>>8)
	copy(plain[8:40], sn)
	plain[40] = byte(len(name))
	plain = append(plain, name...)
	plain = append(plain, make([]byte, 16)...)

	enc, err := EncryptECB(encKey, plain)
	if err != nil {
		return nil, err
	}

	h := make([]byte, 40)
	h[0], h[1], h[2], h[3] = 0x5A, 0x5A, 0x01, 0x11

	for i := range 6 {
		h[20+i] = byte(deviceID >> (8 * i))
	}

	length := 40 + len(enc) + 16
	h[4], h[5] = byte(length), byte(length>>8)
	inner := append(h, enc...)
	inner = append(inner, sign(inner)...)

	outer := []byte{0x83, 0x70, 0, 0, 0x20, 0x00, 0, 0}
	total := len(outer) + len(inner) + 16
	binary.BigEndian.PutUint16(outer[2:], uint16(total-8))
	out := append(outer, inner...)

	return append(out, make([]byte, 16)...), nil
}

// UDPID is the id for the token request: SHA256 of the 6 byte device id,
// first and second half XORed.
func UDPID(deviceID uint64, bigEndian bool) string {
	b := make([]byte, 6)
	for i := range 6 {
		b[i] = byte(deviceID >> (8 * i))
	}

	if bigEndian {
		for i, j := 0, 5; i < j; i, j = i+1, j-1 {
			b[i], b[j] = b[j], b[i]
		}
	}

	h := sha256Sum(b)
	out := make([]byte, 16)

	for i := range 16 {
		out[i] = h[i] ^ h[i+16]
	}

	return hexs(out)
}

// Scan sends the discovery message to the targets (default: limited and
// directed broadcasts of all interfaces) and collects answers for timeout.
// stop, when set, ends the scan at the first matching device.
func Scan(targets []string, timeout time.Duration, stop func(Discovered) bool) ([]Discovered, error) {
	if len(targets) == 0 {
		targets = BroadcastTargets()
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	sent := false

	var lastErr error

	send := func(t string) {
		addr := net.ParseIP(t)
		if addr == nil {
			return
		}

		for _, port := range DiscoveryPorts {
			if _, err := conn.WriteToUDP(discoveryMessage, &net.UDPAddr{IP: addr, Port: port}); err != nil {
				lastErr = err
			} else {
				sent = true
			}
		}
	}

	found := map[string]Discovered{}
	buf := make([]byte, 2048)

	collect := func(until time.Time) bool {
		for time.Now().Before(until) {
			_ = conn.SetReadDeadline(until)

			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return false
			}

			ipStr := addr.IP.String()
			if _, ok := found[ipStr]; ok {
				continue
			}

			d, err := ParseDiscovery(buf[:n], ipStr)
			if err != nil {
				continue
			}

			found[ipStr] = d
			if stop != nil && stop(d) {
				return true
			}
		}

		return false
	}

	stopped := false

	for i, t := range targets {
		for range 2 {
			send(t)
		}

		// Collect between batches so neither buffer overflows on subnet scans.
		if (i+1)%32 == 0 {
			if collect(time.Now().Add(5 * time.Millisecond)) {
				stopped = true
				break
			}
		}
	}

	if !sent {
		return nil, fmt.Errorf("discovery not possible: %w", lastErr)
	}

	if !stopped {
		collect(time.Now().Add(timeout))
	}

	list := make([]Discovered, 0, len(found))
	for _, d := range found {
		list = append(list, d)
	}

	sort.Slice(list, func(i, j int) bool { return list[i].IP < list[j].IP })

	return list, nil
}

// LocalNetworks returns the IPv4 networks of the up interfaces (no loopback).
func LocalNetworks() []*net.IPNet {
	var nets []*net.IPNet

	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				nets = append(nets, n)
			}
		}
	}

	return nets
}

// BroadcastTargets are 255.255.255.255 plus each interface's directed broadcast.
func BroadcastTargets() []string {
	out := []string{"255.255.255.255"}

	for _, n := range LocalNetworks() {
		ip4 := n.IP.To4()
		mask := net.IP(n.Mask).To4()

		if mask == nil {
			continue
		}

		b := make(net.IP, 4)
		for i := range 4 {
			b[i] = ip4[i] | ^mask[i]
		}

		out = append(out, b.String())
	}

	return out
}

// SubnetTargets lists every host address of the local networks (networks
// larger than /22 are limited to the own /24).
func SubnetTargets() []string {
	seen := map[string]bool{}

	var out []string

	for _, n := range LocalNetworks() {
		ones, _ := n.Mask.Size()
		if ones < 22 {
			ones = 24
		}

		if ones > 30 {
			continue
		}

		own := binary.BigEndian.Uint32(n.IP.To4())
		mask := ^uint32(0) << (32 - ones)
		network := own & mask
		broadcast := network | ^mask

		for v := network + 1; v <= broadcast; v++ {
			if v == own {
				continue
			}

			ip := make(net.IP, 4)
			binary.BigEndian.PutUint32(ip, v)

			if s := ip.String(); !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}

	return out
}

// Find looks for the device with deviceID: broadcast first, then a subnet scan.
func Find(deviceID uint64, timeout time.Duration) (*Discovered, error) {
	match := func(d Discovered) bool { return d.DeviceID == deviceID }

	var firstErr error

	for _, targets := range [][]string{nil, SubnetTargets()} {
		list, err := Scan(targets, timeout, match)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}

			continue
		}

		for _, d := range list {
			if match(d) {
				return &d, nil
			}
		}
	}

	if firstErr != nil {
		return nil, firstErr
	}

	return nil, errors.New("device not found on the network")
}

// IsIPv4 is a strict dotted quad check.
func IsIPv4(s string) bool {
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil && strings.Count(s, ".") == 3
}
