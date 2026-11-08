package midea

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"syscall"
	"time"
)

// Connection errors.
var (
	ErrTimeout     = errors.New("the device does not answer")
	ErrBusy        = errors.New("the device is busy: it accepts only one local connection (Midea app or another controller connected?)")
	ErrRefused     = errors.New("the device refuses the connection (old address or another app holds it)")
	ErrUnreachable = errors.New("the device is not reachable on the network")
	ErrClosed      = errors.New("the device closed the connection")
	ErrBudget      = errors.New("the device did not answer in time (time limit reached)")
	ErrNoPath      = errors.New("no way to reach the device is configured")
)

// ErrOutcomeUnknown wraps an error that happened after a control frame was
// sent: whether the change arrived is unknown.
type ErrOutcomeUnknown struct{ Err error }

func (e *ErrOutcomeUnknown) Error() string {
	return "connection lost after sending: " + e.Err.Error() + " – whether the change arrived is unclear"
}

func (e *ErrOutcomeUnknown) Unwrap() error { return e.Err }

const (
	authLifetime     = 12 * time.Hour
	busyEvidence     = 5 * time.Minute
	maxExtraFrames   = 8
	minRetryBudget   = time.Second
	afterLoginPause  = 500 * time.Millisecond
	retryPause       = 1500 * time.Millisecond
	firstReadTimeout = 4 * time.Second
)

// LANClient is a blocking TCP connection to the device (port 6444) with V3
// login. Not safe for concurrent use: Device serialises all calls.
type LANClient struct {
	Host     string
	Port     int
	DeviceID uint64
	Token    []byte
	Key      []byte
	// Deadline is the latest point in time for the running job (zero = none).
	Deadline time.Time
	// Debugf receives protocol traces (may be nil).
	Debugf func(format string, args ...any)

	conn              net.Conn
	codec             V3Codec
	authAt            time.Time
	pending           [][]byte
	readTimeoutsInRow int
	wroteFrame        bool
	failedAfterWrite  bool
	lastReachable     time.Time
	closedByPeer      bool
	buf               [4096]byte
}

func (c *LANClient) debugf(format string, args ...any) {
	if c.Debugf != nil {
		c.Debugf(format, args...)
	}
}

// FailedAfterWrite reports whether the last final error came after the frame
// was written.
func (c *LANClient) FailedAfterWrite() bool { return c.failedAfterWrite }

// Connected reports whether a socket is open.
func (c *LANClient) Connected() bool { return c.conn != nil }

// Close drops the connection (the device accepts only one at a time).
func (c *LANClient) Close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.debugf("disconnected from %s", c.Host)
	}

	c.conn = nil
	c.authAt = time.Time{}
	c.codec = V3Codec{}
	c.pending = nil
	c.readTimeoutsInRow = 0
}

func (c *LANClient) authenticated() bool {
	return c.conn != nil && !c.authAt.IsZero() && time.Since(c.authAt) < authLifetime
}

func (c *LANClient) recentlyReachable() bool {
	return !c.lastReachable.IsZero() && time.Since(c.lastReachable) < busyEvidence
}

// budgeted shortens a wait to the rest of the job's budget.
func (c *LANClient) budgeted(t time.Duration) (time.Duration, error) {
	if c.Deadline.IsZero() {
		return t, nil
	}

	rest := time.Until(c.Deadline)
	if rest <= 50*time.Millisecond {
		return 0, ErrBudget
	}

	return min(t, rest), nil
}

// Send sends a command frame and returns all answer frames (including
// unsolicited reports). expect is the expected answer id (byte 10, e.g.
// 0xC0), 0 = any.
func (c *LANClient) Send(frame []byte, retries int, expect byte) ([][]byte, error) {
	c.failedAfterWrite = false
	return c.send(frame, retries, expect, false)
}

func (c *LANClient) send(frame []byte, retries int, expect byte, sentBefore bool) ([][]byte, error) {
	hadSession := c.authenticated()

	frames, err := c.sendOnce(frame, expect)
	if err == nil {
		c.readTimeoutsInRow = 0
		c.lastReachable = time.Now()

		return frames, nil
	}

	// A timeout in an existing session keeps the session the first time (the
	// late answer is dropped before the next send); else reconnect.
	if errors.Is(err, ErrTimeout) && hadSession && c.readTimeoutsInRow == 0 {
		c.readTimeoutsInRow++
	} else {
		c.Close()
	}

	final := retries <= 0 || errors.Is(err, ErrAuth)
	if !c.Deadline.IsZero() && time.Until(c.Deadline) < minRetryBudget {
		final = true
	}

	if final {
		c.failedAfterWrite = sentBefore || c.wroteFrame
		return nil, err
	}

	c.debugf("%s: %v – retrying", c.Host, err)

	if c.wroteFrame {
		if d, e := c.budgeted(retryPause); e == nil {
			time.Sleep(d)
		}
	}

	return c.send(frame, retries-1, expect, sentBefore || c.wroteFrame)
}

func containsID(frames [][]byte, id byte) bool {
	for _, f := range frames {
		if ResponseID(f) == id {
			return true
		}
	}

	return false
}

func (c *LANClient) sendOnce(frame []byte, expect byte) ([][]byte, error) {
	c.wroteFrame = false

	if err := c.ensureSession(); err != nil {
		return nil, err
	}

	c.debugf("→ %s %s % x", c.Host, FrameLabel(frame), frame)

	if err := c.drain(); err != nil {
		return nil, err
	}

	pkt, err := EncodeV2(c.DeviceID, frame, time.Now())
	if err != nil {
		return nil, err
	}

	enc, err := c.codec.EncodeEncrypted(pkt, v3EncryptedRequest)
	if err != nil {
		return nil, err
	}

	if err := c.write(enc); err != nil {
		return nil, err
	}

	c.wroteFrame = true

	wait, err := c.budgeted(firstReadTimeout)
	if err != nil {
		return nil, err
	}

	first, err := c.readPacket(wait)
	if err != nil {
		return nil, err
	}

	f, err := c.decodeFrame(first)
	if err != nil {
		return nil, err
	}

	frames := [][]byte{f}

	if expect == 0 || !containsID(frames, expect) {
		for range maxExtraFrames {
			more, err := c.readPacket(100 * time.Millisecond)
			if err != nil {
				break
			}

			if f, err := c.decodeFrame(more); err == nil {
				frames = append(frames, f)
			}
		}
	}

	if expect != 0 && !containsID(frames, expect) {
		for range 4 {
			wait, err := c.budgeted(2 * time.Second)
			if err != nil {
				break
			}

			more, err := c.readPacket(wait)
			if err != nil {
				break
			}

			if f, err := c.decodeFrame(more); err == nil {
				frames = append(frames, f)
				if ResponseID(f) == expect {
					break
				}
			}
		}
	}

	for _, f := range frames {
		c.debugf("← %s %s % x", c.Host, FrameLabel(f), f)
	}

	return frames, nil
}

func (c *LANClient) decodeFrame(pkt []byte) ([]byte, error) {
	typ, payload, err := c.codec.Decode(pkt)
	if err != nil {
		return nil, err
	}

	if typ != v3EncryptedResponse {
		return nil, fmt.Errorf("%w: unexpected handshake answer", ErrProtocol)
	}

	return DecodeV2(payload)
}

// drain drops everything that is already in the socket (late answers).
func (c *LANClient) drain() error {
	c.pending = nil
	if c.conn == nil {
		return nil
	}

	dropped := 0

	for {
		_ = c.conn.SetReadDeadline(time.Now().Add(time.Millisecond))

		n, err := c.conn.Read(c.buf[:])
		if n > 0 {
			dropped += len(c.codec.Feed(c.buf[:n]))
		}

		if err != nil {
			if isTimeout(err) {
				break
			}

			if errors.Is(err, io.EOF) {
				return ErrClosed
			}

			return err
		}
	}

	if dropped > 0 {
		c.debugf("%s: dropped %d late frames", c.Host, dropped)
	}

	return nil
}

func (c *LANClient) ensureSession() error {
	if c.authenticated() {
		return nil
	}

	c.Close()
	c.debugf("connecting to %s:%d (TCP, V3)", c.Host, c.Port)

	wait, err := c.budgeted(5 * time.Second)
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(c.Host, strconv.Itoa(c.Port)), wait)
	if err != nil {
		switch {
		case errors.Is(err, syscall.ECONNREFUSED):
			if c.recentlyReachable() {
				return ErrBusy
			}

			return ErrRefused
		default:
			return fmt.Errorf("%w: %v", ErrUnreachable, err)
		}
	}

	c.conn = conn

	if err := c.write(c.codec.EncodeHandshake(c.Token)); err != nil {
		return err
	}

	c.closedByPeer = false

	wait, err = c.budgeted(firstReadTimeout)
	if err != nil {
		return err
	}

	resp, err := c.readPacket(wait)
	if err != nil {
		switch {
		case errors.Is(err, ErrTimeout):
			if c.recentlyReachable() {
				return ErrBusy
			}

			return ErrTimeout
		case c.closedByPeer:
			// Accepted and closed right away: the device holds another connection.
			return ErrBusy
		}

		return err
	}

	typ, payload, err := c.codec.Decode(resp)
	if err != nil {
		if errors.Is(err, ErrDeviceRefused) {
			return fmt.Errorf("%w (%v)", ErrAuth, err)
		}

		return err
	}

	if typ != v3HandshakeResponse {
		return fmt.Errorf("%w: unexpected packet type", ErrAuth)
	}

	lk, err := LocalKeyFromHandshake(c.Key, payload)
	if err != nil {
		return err
	}

	c.codec.LocalKey = lk
	c.authAt = time.Now()
	c.lastReachable = c.authAt
	c.debugf("handshake ok with %s, session key negotiated", c.Host)
	// msmart-ng waits shortly after the login before sending commands.
	time.Sleep(afterLoginPause)

	return nil
}

func (c *LANClient) write(b []byte) error {
	if c.conn == nil {
		return ErrClosed
	}

	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, err := c.conn.Write(b)

	return err
}

func isTimeout(err error) bool {
	var ne net.Error

	return errors.As(err, &ne) && ne.Timeout()
}

// readPacket reads until one complete 8370 packet is there.
func (c *LANClient) readPacket(timeout time.Duration) ([]byte, error) {
	if len(c.pending) > 0 {
		p := c.pending[0]
		c.pending = c.pending[1:]

		return p, nil
	}

	deadline := time.Now().Add(timeout)

	for {
		if c.conn == nil {
			return nil, ErrClosed
		}

		if time.Until(deadline) <= 0 {
			return nil, ErrTimeout
		}

		_ = c.conn.SetReadDeadline(deadline)

		n, err := c.conn.Read(c.buf[:])
		if n > 0 {
			c.pending = append(c.pending, c.codec.Feed(c.buf[:n])...)
		}

		if len(c.pending) > 0 {
			p := c.pending[0]
			c.pending = c.pending[1:]

			return p, nil
		}

		if err != nil {
			if isTimeout(err) {
				return nil, ErrTimeout
			}

			if errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET) {
				c.closedByPeer = true
				return nil, ErrClosed
			}

			return nil, err
		}
	}
}

// FrameLabel is a short human name of a frame for traces.
func FrameLabel(f []byte) string {
	if len(f) <= 13 {
		return "?"
	}

	var typ string

	switch f[9] {
	case 0x02:
		typ = "set"
	case 0x03:
		typ = "query"
	case 0x04, 0x05:
		typ = "report"
	case 0x06:
		typ = "fault"
	default:
		typ = fmt.Sprintf("type %02X", f[9])
	}

	var what string

	switch f[10] {
	case 0x40:
		what = "set state"
	case 0x41:
		if f[11] == 0x21 {
			what = fmt.Sprintf("group %d", f[13]&0xF)
		} else {
			what = "state"
		}
	case 0xB0:
		what = "set properties"
	case 0xB1:
		what = "properties"
	case 0xB5:
		what = "capabilities"
	case 0xC0:
		what = "state"
	case 0xC1:
		what = fmt.Sprintf("group %d", f[13]&0xF)
	default:
		what = fmt.Sprintf("0x%02X", f[10])
	}

	return "[" + typ + " · " + what + "]"
}
