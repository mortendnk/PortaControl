package midea

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

// Errors of the packet layer.
var (
	ErrProtocol      = errors.New("midea: protocol error")
	ErrAuth          = errors.New("midea: token or key do not match the device")
	ErrDeviceRefused = errors.New("midea: the device reports an error (token expired or connection busy)")
)

// timestamp as msmart-ng: centiseconds, sec, min, hour, day, month, year%100, year/100 (UTC).
func timestamp(t time.Time) []byte {
	t = t.UTC()
	y := t.Year()
	ms := (t.UnixNano() + 500_000) / 1_000_000 % 1000

	return []byte{
		byte(ms / 10), byte(t.Second()), byte(t.Minute()), byte(t.Hour()),
		byte(t.Day()), byte(t.Month()), byte(y % 100), byte(y / 100),
	}
}

// v2Header builds the 40 byte header of a 0x5A5A packet; the length field is
// filled in by the caller.
func v2Header(deviceID uint64, now time.Time) []byte {
	h := []byte{0x5A, 0x5A, 0x01, 0x11, 0, 0, 0x20, 0x00, 0, 0, 0, 0}
	h = append(h, timestamp(now)...)
	h = binary.LittleEndian.AppendUint64(h, deviceID)
	h = append(h, make([]byte, 12)...)

	return h
}

// EncodeV2 wraps a command frame into an encrypted, signed 0x5A5A packet.
func EncodeV2(deviceID uint64, frame []byte, now time.Time) ([]byte, error) {
	enc, err := EncryptECB(encKey, frame)
	if err != nil {
		return nil, err
	}

	p := v2Header(deviceID, now)
	length := 40 + len(enc) + 16
	p[4], p[5] = byte(length), byte(length>>8)
	p = append(p, enc...)

	return append(p, sign(p)...), nil
}

// DecodeV2 checks and decrypts a 0x5A5A packet and returns the command frame.
func DecodeV2(p []byte) ([]byte, error) {
	if len(p) < 6 {
		return nil, fmt.Errorf("%w: packet too short", ErrProtocol)
	}

	if p[0] != 0x5A || p[1] != 0x5A {
		return nil, fmt.Errorf("%w: unknown packet % x", ErrProtocol, p[:min(8, len(p))])
	}

	length := int(p[4]) | int(p[5])<<8
	if len(p) < length || length < 56 || (length-56)%16 != 0 {
		return nil, fmt.Errorf("%w: bad packet length", ErrProtocol)
	}

	body := p[:length]
	if !bytes.Equal(sign(body[:length-16]), body[length-16:]) {
		return nil, fmt.Errorf("%w: bad MD5 signature", ErrProtocol)
	}

	return DecryptECB(encKey, body[40:length-16])
}

// V3 packet types (low nibble of byte 5).
const (
	v3HandshakeRequest  = 0x0
	v3HandshakeResponse = 0x1
	v3EncryptedResponse = 0x3
	v3EncryptedRequest  = 0x6
	v3Error             = 0xF
)

// maxV3Packet is the largest 8370 packet accepted; real answers are < 300 bytes.
const maxV3Packet = 4096

// V3Codec frames the 0x8370 transport: handshake with the token, then
// AES-CBC with the negotiated session key.
type V3Codec struct {
	LocalKey []byte
	packetID uint16
	buf      []byte
	// Random returns n random bytes (padding); replaceable in tests.
	Random func(n int) []byte
}

func (c *V3Codec) random(n int) []byte {
	if c.Random != nil {
		return c.Random(n)
	}

	b := make([]byte, n)
	_, _ = rand.Read(b)

	return b
}

func v3Header(length int, extra byte) []byte {
	return []byte{0x83, 0x70, byte(length >> 8), byte(length), 0x20, extra}
}

func (c *V3Codec) bump() { c.packetID = (c.packetID + 1) & 0xFFF }

// EncodeHandshake builds the handshake request carrying the token.
func (c *V3Codec) EncodeHandshake(token []byte) []byte {
	p := v3Header(len(token), v3HandshakeRequest)
	p = binary.BigEndian.AppendUint16(p, c.packetID)
	p = append(p, token...)
	c.bump()

	return p
}

// EncodeEncrypted encrypts data (a V2 packet) for the session; typ is
// v3EncryptedRequest for clients and v3EncryptedResponse for the simulator.
func (c *V3Codec) EncodeEncrypted(data []byte, typ byte) ([]byte, error) {
	if c.LocalKey == nil {
		return nil, fmt.Errorf("%w: not logged in", ErrProtocol)
	}

	pad := 0
	if r := (len(data) + 2) % 16; r != 0 {
		pad = 16 - r
	}

	hdr := v3Header(len(data)+pad+32, byte(pad<<4)|typ)
	payload := binary.BigEndian.AppendUint16(nil, c.packetID)
	payload = append(payload, data...)
	payload = append(payload, c.random(pad)...)
	c.bump()

	enc, err := encryptCBCRaw(c.LocalKey, payload)
	if err != nil {
		return nil, err
	}

	out := append(append([]byte{}, hdr...), enc...)

	return append(out, sha256Sum(append(append([]byte{}, hdr...), payload...))...), nil
}

// LocalKeyFromHandshake derives the session key from the 64 byte handshake
// answer (32 encrypted + 32 SHA256).
func LocalKeyFromHandshake(key, resp []byte) ([]byte, error) {
	if len(resp) != 64 {
		return nil, fmt.Errorf("%w: handshake answer has %d instead of 64 bytes", ErrAuth, len(resp))
	}

	plain, err := decryptCBCRaw(key, resp[:32])
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(sha256Sum(plain), resp[32:]) {
		return nil, ErrAuth
	}

	out := make([]byte, len(plain))
	for i := range plain {
		out[i] = plain[i] ^ key[i]
	}

	return out, nil
}

// Feed takes raw socket bytes and returns all complete 8370 packets. A
// header with an absurd length is skipped (resynchronisation).
func (c *V3Codec) Feed(data []byte) [][]byte {
	c.buf = append(c.buf, data...)

	var packets [][]byte

	i := 0
	for i < len(c.buf) {
		start := bytes.IndexByte(c.buf[i:], 0x83)
		if start < 0 {
			i = len(c.buf)
			break
		}

		i += start
		if i+1 >= len(c.buf) {
			break
		}

		if c.buf[i+1] != 0x70 {
			i++
			continue
		}

		if len(c.buf)-i < 6 {
			break
		}

		total := (int(c.buf[i+2])<<8 | int(c.buf[i+3])) + 8
		if total > maxV3Packet {
			i += 2
			continue
		}

		if len(c.buf)-i < total {
			break
		}

		packets = append(packets, append([]byte{}, c.buf[i:i+total]...))
		i += total
	}

	c.buf = append([]byte{}, c.buf[i:]...)

	return packets
}

// Decode returns the type and the payload of one 8370 packet.
func (c *V3Codec) Decode(p []byte) (byte, []byte, error) {
	if len(p) < 8 || p[0] != 0x83 || p[1] != 0x70 {
		return 0, nil, fmt.Errorf("%w: invalid packet start", ErrProtocol)
	}

	if p[4] != 0x20 {
		return 0, nil, fmt.Errorf("%w: invalid marker byte", ErrProtocol)
	}

	typ := p[5] & 0xF
	switch typ {
	case v3HandshakeRequest, v3HandshakeResponse:
		return typ, p[8:], nil
	case v3EncryptedResponse, v3EncryptedRequest:
		if c.LocalKey == nil {
			return 0, nil, fmt.Errorf("%w: not logged in", ErrProtocol)
		}

		if len(p) < 6+32+16 {
			return 0, nil, fmt.Errorf("%w: packet too short", ErrProtocol)
		}

		hdr := p[:6]

		plain, err := decryptCBCRaw(c.LocalKey, p[6:len(p)-32])
		if err != nil {
			return 0, nil, err
		}

		if !bytes.Equal(sha256Sum(append(append([]byte{}, hdr...), plain...)), p[len(p)-32:]) {
			return 0, nil, fmt.Errorf("%w: SHA256 of the answer does not match", ErrProtocol)
		}

		pad := int(p[5] >> 4)
		if len(plain) < 2+pad {
			return 0, nil, fmt.Errorf("%w: padding does not fit", ErrProtocol)
		}

		return typ, plain[2 : len(plain)-pad], nil
	case v3Error:
		return typ, nil, ErrDeviceRefused
	default:
		return 0, nil, fmt.Errorf("%w: unexpected packet type %d", ErrProtocol, typ)
	}
}
