package midea

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Vectors from Klimacontrol (Tests/MideaKitTests/vectors.json, MIT), generated
// with msmart-ng.
type vectors struct {
	GetState, GetCapabilities, GetGroup4, ToggleDisplay, GetOutSilent, SetOutSilentOn string
	SetIECOOn, SetSelfCleanOn, SetSoundOff, GetProps3, SetStateIonFrost               string
	SetState                                                                          []struct {
		Power            bool
		Temp             float64
		Mode, Fan, Swing int
		Eco, Turbo, Beep bool
		Frame            string
	}
	V2 struct {
		DeviceID uint64 `json:"deviceId"`
		Frame    string
		Epoch    float64
		Packet   string
	}
	V3 struct {
		LocalKey string
		PacketID int
		Data     string
		Packet   string
	}
	Handshake struct{ Key, Response, LocalKey string }
	UDPID     struct {
		DeviceID uint64 `json:"deviceId"`
		Little   string
		Big      string
	} `json:"udpid"`
	NethomeSign struct {
		Endpoint string
		Body     map[string]string
		Sign     string
		Pwd      string
	}
	Smarthome struct{ Sign, Pwd, Iam string }
	Discovery string
}

func load(t *testing.T) vectors {
	t.Helper()

	b, err := os.ReadFile("testdata/vectors.json")
	if err != nil {
		t.Fatal(err)
	}

	var v vectors
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}

	return v
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()

	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// with16 builds a frame with message id 0x10 like the vectors.
func with16(f func() []byte) string {
	messageID.Store(0x0F)
	return hex.EncodeToString(f())
}

func TestFrames(t *testing.T) {
	v := load(t)
	cases := map[string][2]string{
		"getState":       {with16(FrameGetState), v.GetState},
		"getCaps":        {with16(FrameGetCapabilities), v.GetCapabilities},
		"getGroup4":      {with16(func() []byte { return FrameGetGroup(4) }), v.GetGroup4},
		"toggleDisplay":  {with16(func() []byte { return FrameToggleDisplay(true) }), v.ToggleDisplay},
		"getOutSilent":   {with16(func() []byte { return FrameGetProperties(PropOutSilent) }), v.GetOutSilent},
		"setOutSilentOn": {with16(func() []byte { return FrameSetProperty(PropOutSilent, true) }), v.SetOutSilentOn},
		"setIECOOn":      {with16(func() []byte { return FrameSetProperty(PropIECO, true) }), v.SetIECOOn},
		"setSelfCleanOn": {with16(func() []byte { return FrameSetProperty(PropSelfClean, true) }), v.SetSelfCleanOn},
		"setSoundOff":    {with16(func() []byte { return FrameSetProperty(PropSound, false) }), v.SetSoundOff},
		"getProps3":      {with16(func() []byte { return FrameGetProperties(PropIECO, PropOutSilent, PropSelfClean) }), v.GetProps3},
		"setStateIonFrost": {with16(func() []byte {
			return FrameSetState(State{Power: true, TargetTemperature: 21, Mode: ModeHeat, FanSpeed: 40, FreezeProtection: true, Purifier: true}, true)
		}), v.SetStateIonFrost},
	}

	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s:\n got %s\nwant %s", name, c[0], c[1])
		}
	}

	for i, s := range v.SetState {
		st := State{Power: s.Power, TargetTemperature: s.Temp, Mode: Mode(s.Mode), FanSpeed: s.Fan, Eco: s.Eco, Turbo: s.Turbo, Swing: s.Swing}
		if got := with16(func() []byte { return FrameSetState(st, s.Beep) }); got != s.Frame {
			t.Errorf("setState %d:\n got %s\nwant %s", i, got, s.Frame)
		}
	}
}

func TestV2V3(t *testing.T) {
	v := load(t)
	frame := unhex(t, v.V2.Frame)
	sec := int64(v.V2.Epoch)
	ts := time.Unix(sec, int64((v.V2.Epoch-float64(sec))*1e9+0.5))

	p, err := EncodeV2(v.V2.DeviceID, frame, ts)
	if err != nil {
		t.Fatal(err)
	}

	if hex.EncodeToString(p) != v.V2.Packet {
		t.Fatalf("v2:\n got %x\nwant %s", p, v.V2.Packet)
	}

	back, err := DecodeV2(p)
	if err != nil || hex.EncodeToString(back) != v.V2.Frame {
		t.Fatalf("v2 decode: %v %x", err, back)
	}

	c := V3Codec{LocalKey: unhex(t, v.V3.LocalKey), Random: func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = 0xAB
		}

		return b
	}}
	for range v.V3.PacketID {
		c.EncodeHandshake(nil)
	}

	enc, err := c.EncodeEncrypted(unhex(t, v.V3.Data), v3EncryptedRequest)
	if err != nil {
		t.Fatal(err)
	}

	if hex.EncodeToString(enc) != v.V3.Packet {
		t.Fatalf("v3:\n got %x\nwant %s", enc, v.V3.Packet)
	}

	// An encrypted answer has the same layout, type 0x3.
	enc[5] = enc[5]&0xF0 | 0x3
	enc = enc[:len(enc)-32]
	payload := append([]byte{}, enc[:6]...)
	plain, _ := decryptCBCRaw(c.LocalKey, enc[6:])
	payload = append(payload, plain...)
	enc = append(enc, sha256Sum(payload)...)

	typ, data, err := c.Decode(enc)
	if err != nil || typ != v3EncryptedResponse || hex.EncodeToString(data) != v.V3.Data {
		t.Fatalf("v3 decode: %v %d", err, typ)
	}

	lk, err := LocalKeyFromHandshake(unhex(t, v.Handshake.Key), unhex(t, v.Handshake.Response))
	if err != nil || hex.EncodeToString(lk) != v.Handshake.LocalKey {
		t.Fatalf("handshake: %v %x", err, lk)
	}
}

func TestCloudAndDiscoveryVectors(t *testing.T) {
	v := load(t)
	if got := UDPID(v.UDPID.DeviceID, false); got != v.UDPID.Little {
		t.Errorf("udpid little %s", got)
	}

	if got := UDPID(v.UDPID.DeviceID, true); got != v.UDPID.Big {
		t.Errorf("udpid big %s", got)
	}

	if got := netHomeSign(v.NethomeSign.Endpoint, v.NethomeSign.Body); got != v.NethomeSign.Sign {
		t.Errorf("nethome sign %s", got)
	}

	if got := sha256Hex("LOGIN-ID" + sha256Hex("geheim") + netHomeAppKey); got != v.NethomeSign.Pwd {
		t.Errorf("nethome pwd %s", got)
	}

	if got := smartHomeSign(`{"a":1}`, "00ff"); got != v.Smarthome.Sign {
		t.Errorf("smarthome sign %s", got)
	}

	if got := sha256Hex("LOGIN-ID" + sha256Hex("geheim") + smartHomeLoginKey); got != v.Smarthome.Pwd {
		t.Errorf("smarthome pwd %s", got)
	}

	if got := sha256Hex("LOGIN-ID" + md5Hex(md5Hex("geheim")) + smartHomeLoginKey); got != v.Smarthome.Iam {
		t.Errorf("smarthome iam %s", got)
	}

	d, err := ParseDiscovery(unhex(t, v.Discovery), "192.168.1.40")
	if err != nil {
		t.Fatal(err)
	}

	if d.Name != "net_ac_1A2B" || d.Type != 0xAC || d.Version != 3 || d.Port != 6444 || d.DeviceID != 20015998343868 {
		t.Fatalf("discovery %+v", d)
	}
}

func TestStateRoundTrip(t *testing.T) {
	p := []byte{0xC0, 0x01, 0x46, 0x66, 0x7F, 0x7F, 0x00, 0x3C, 0x00, 0x10, 0x00, 0x5D, 0x5F, 0x00, 0x00, 0x35, 0x00}
	s, ok := ParseState(p)

	if !ok || !s.Power || s.Mode != ModeCool || s.TargetTemperature != 22 || s.FanSpeed != FanAuto || !s.Eco || s.Swing != SwingVertical {
		t.Fatalf("%+v", s)
	}

	if *s.IndoorTemperature != 21.5 || *s.OutdoorTemp != 22.3 {
		t.Fatalf("temps %v %v", *s.IndoorTemperature, *s.OutdoorTemp)
	}

	on, off := 90, 15
	a, b, c := encodeDeviceTimer(&on, &off)
	gotOn, gotOff := decodeDeviceTimer(a, b, c)

	if *gotOn != 90 || *gotOff != 15 {
		t.Fatalf("timer %d %d", *gotOn, *gotOff)
	}
}

func TestDiscoveryReplyRoundTrip(t *testing.T) {
	r, err := BuildDiscoveryReply(nil, 6444, 123456789, "SN123", "net_ac_ABCD")
	if err != nil {
		t.Fatal(err)
	}

	d, err := ParseDiscovery(r, "10.0.0.5")
	if err != nil || d.DeviceID != 123456789 || d.Name != "net_ac_ABCD" || d.Port != 6444 {
		t.Fatalf("%v %+v", err, d)
	}
}

func TestCSV(t *testing.T) {
	b := []byte{0, 1, 0x7F, 0x80, 0xFF}
	if s := csv(b); s != "0,1,127,-128,-1" {
		t.Fatal(s)
	}

	back, ok := fromCSV("0,1,127,-128,-1")
	if !ok || hex.EncodeToString(back) != "00017f80ff" {
		t.Fatal(back)
	}
}
