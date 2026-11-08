package midea

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// CloudKind names the Midea cloud flavour.
type CloudKind string

// Cloud flavours: MSmartHome (meiju schema) and NetHome Plus.
const (
	CloudSmartHome   CloudKind = "MSmartHome"
	CloudNetHomePlus CloudKind = "NetHome Plus"
)

// CloudKinds in the order they are tried.
var CloudKinds = []CloudKind{CloudSmartHome, CloudNetHomePlus}

// CloudError is an error answer of the Midea cloud.
type CloudError struct {
	Code int
	Msg  string
}

func (e *CloudError) Error() string {
	switch {
	case e.Code == 3123:
		return "the device is switched off or without power according to the Midea cloud (3123)"
	case e.Code == 3101 || e.Code == 3102 || strings.Contains(strings.ToLower(e.Msg), "password"):
		return fmt.Sprintf("e-mail or password of the Midea account are wrong (%d)", e.Code)
	case e.Code == 9999:
		return "the Midea cloud does not hand out credentials right now (9999)"
	}

	msg := e.Msg
	if msg == "" {
		msg = "no text"
	}

	return fmt.Sprintf("Midea cloud error: %s (%d)", msg, e.Code)
}

// IsSessionError reports cloud errors that a new login fixes.
func (e *CloudError) IsSessionError() bool {
	switch e.Code {
	case 3004, 3106, 40002, 44005:
		return true
	}

	m := strings.ToLower(e.Msg)

	return strings.Contains(m, "token") || strings.Contains(m, "session") || strings.Contains(m, "sign") ||
		strings.Contains(m, "not login") || strings.Contains(m, "unauthorized")
}

// Cloud errors without a code.
var (
	ErrCloudNetwork     = errors.New("the Midea cloud is not reachable")
	ErrCloudBadReply    = errors.New("the Midea cloud answered unexpectedly")
	ErrCloudNoToken     = errors.New("the cloud has no token for this device")
	ErrCloudNotInAcct   = errors.New("the device is not in this Midea account")
	ErrCloudSessionKeys = errors.New("cloud session keys missing or wrong")
	ErrTokenRejected    = errors.New("the device rejects the token from the cloud – is it set up in exactly this account?")
)

// IsApplianceOffline reports cloud error 3123.
func IsApplianceOffline(err error) bool {
	var ce *CloudError
	return errors.As(err, &ce) && ce.Code == 3123
}

// CloudAppliance is a device of the account.
type CloudAppliance struct {
	DeviceID uint64 `json:"device_id,string"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Online   bool   `json:"online"`
}

// CloudSession is a saved login (reused so the account is not logged in on
// every start; every login makes Midea show "account logged in elsewhere").
type CloudSession struct {
	Kind        CloudKind `json:"kind"`
	AccessToken string    `json:"access_token"`
	AESKey      string    `json:"aes_key,omitempty"`
	AESIV       string    `json:"aes_iv,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	SavedAt     time.Time `json:"saved_at"`
}

// cloudAPI is one cloud flavour.
type cloudAPI interface {
	kind() CloudKind
	login(ctx context.Context) error
	canControl() bool
	token(ctx context.Context, udpid string, applianceID uint64) (token, key string, err error)
	appliances(ctx context.Context) ([]map[string]any, error)
	transparentSend(ctx context.Context, frame []byte, deviceID uint64, applianceID string) ([][]byte, error)
	session() *CloudSession
	restore(s *CloudSession) bool
}

var cloudHTTP = &http.Client{Timeout: 15 * time.Second}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)

	return hex.EncodeToString(b)
}

func stamp(t time.Time) string { return t.UTC().Format("20060102150405") }

func sha256Hex(s string) string { return hexs(sha256Sum([]byte(s))) }

func md5Hex(s string) string { return hexs(md5Sum([]byte(s))) }

func postJSON(ctx context.Context, req *http.Request) (map[string]any, error) {
	resp, err := cloudHTTP.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCloudNetwork, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCloudNetwork, err)
	}

	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrCloudNetwork, resp.StatusCode)
	}

	var out map[string]any

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()

	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%w (HTTP %d)", ErrCloudBadReply, resp.StatusCode)
	}

	return out, nil
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

func codeOf(v any) int {
	n, err := strconv.Atoi(strings.TrimSpace(str(v)))
	if err != nil {
		return -1
	}

	return n
}

func tokenFrom(result map[string]any, udpid string) (string, string, bool) {
	list, _ := result["tokenlist"].([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if str(m["udpId"]) == udpid && str(m["token"]) != "" && str(m["key"]) != "" {
			return str(m["token"]), str(m["key"]), true
		}
	}

	return "", "", false
}

// csv encodes bytes as signed decimals joined by commas.
func csv(b []byte) string {
	parts := make([]string, len(b))
	for i, c := range b {
		parts[i] = strconv.Itoa(int(int8(c)))
	}

	return strings.Join(parts, ",")
}

func fromCSV(s string) ([]byte, bool) {
	var out []byte

	for _, part := range strings.Split(s, ",") {
		v, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || v < -128 || v > 255 {
			return nil, false
		}

		out = append(out, byte(v))
	}

	return out, len(out) > 0
}

// cloudPacket wraps a frame for transparent send (unencrypted frame in a V2 header).
func cloudPacket(deviceID uint64, frame []byte, now time.Time) []byte {
	p := v2Header(deviceID, now)
	p = append(p, frame...)
	length := len(p) + 16
	p[4], p[5] = byte(length), byte(length>>8)

	return append(p, sign(p)...)
}

func cloudPacketDecode(d []byte) ([]byte, bool) {
	if len(d) < 56 {
		return nil, false
	}

	inner := d[40:]
	if inner[0] != 0xAA || len(inner) <= int(inner[1]) {
		return nil, false
	}

	return inner[:int(inner[1])+1], true
}

func innerFrame(text string) ([][]byte, error) {
	decoded, ok := fromCSV(text)
	if !ok {
		return nil, ErrCloudSessionKeys
	}

	inner, ok := cloudPacketDecode(decoded)
	if !ok {
		return nil, fmt.Errorf("%w: answer of the device unreadable", ErrCloudBadReply)
	}

	return [][]byte{inner}, nil
}

// ---- MSmartHome --------------------------------------------------------

const (
	smartHomeAppID    = "1010"
	smartHomeBase     = "https://mp-prod.appsmb.com"
	smartHomeHMACKey  = "PROD_VnoClJI9aikS8dyy"
	smartHomeIOTKey   = "meicloud"
	smartHomeLoginKey = "ac21b9f9cbfe4ca5a88562ef25e2b768"
)

type smartHome struct {
	account, password, clientID string

	mu          sync.Mutex
	accessToken string
	aesKey      []byte
	aesIV       []byte
}

func (s *smartHome) kind() CloudKind { return CloudSmartHome }

func (s *smartHome) canControl() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.accessToken != "" && len(s.aesKey) > 0 && len(s.aesIV) > 0
}

func (s *smartHome) session() *CloudSession {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.accessToken == "" {
		return nil
	}

	return &CloudSession{Kind: CloudSmartHome, AccessToken: s.accessToken, AESKey: hexs(s.aesKey), AESIV: hexs(s.aesIV), SavedAt: time.Now()}
}

func (s *smartHome) restore(cs *CloudSession) bool {
	k, err1 := hex.DecodeString(cs.AESKey)
	iv, err2 := hex.DecodeString(cs.AESIV)

	if cs.AccessToken == "" || err1 != nil || err2 != nil || len(k) == 0 || len(iv) == 0 {
		return false
	}

	s.mu.Lock()
	s.accessToken, s.aesKey, s.aesIV = cs.AccessToken, k, iv
	s.mu.Unlock()

	return true
}

func (s *smartHome) body(extra map[string]any) map[string]any {
	b := map[string]any{
		"appId": smartHomeAppID, "src": smartHomeAppID, "format": 2, "clientType": 1,
		"language": "en_US", "deviceId": s.clientID, "stamp": stamp(time.Now()), "reqId": randomHex(16),
	}
	for k, v := range extra {
		b[k] = v
	}

	return b
}

func smartHomeSign(js, random string) string {
	m := hmac.New(sha256.New, []byte(smartHomeHMACKey))
	m.Write([]byte(smartHomeIOTKey + js + random))

	return hexs(m.Sum(nil))
}

func (s *smartHome) request(ctx context.Context, endpoint string, body map[string]any) (map[string]any, error) {
	js, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	random := randomHex(16)

	req, err := http.NewRequest(http.MethodPost, smartHomeBase+"/mas/v5/app/proxy?alias="+endpoint, bytes.NewReader(js))
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	token := s.accessToken
	s.mu.Unlock()

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("secretVersion", "1")
	req.Header.Set("sign", smartHomeSign(string(js), random))
	req.Header.Set("random", random)
	req.Header.Set("accessToken", token)

	r, err := postJSON(ctx, req)
	if err != nil {
		return nil, err
	}

	if code := codeOf(r["code"]); code != 0 {
		return nil, &CloudError{Code: code, Msg: CleanText(str(r["msg"]), "", 200)}
	}

	data, _ := r["data"].(map[string]any)
	if data == nil {
		data = map[string]any{}
	}

	return data, nil
}

func (s *smartHome) login(ctx context.Context) error {
	s.mu.Lock()
	s.accessToken, s.aesKey, s.aesIV = "", nil, nil
	s.mu.Unlock()

	r, err := s.request(ctx, "/v1/user/login/id/get", s.body(map[string]any{"loginAccount": s.account}))
	if err != nil {
		return err
	}

	loginID := str(r["loginId"])
	if loginID == "" {
		return fmt.Errorf("%w: login answered without id", ErrCloudBadReply)
	}

	iam := sha256Hex(loginID + md5Hex(md5Hex(s.password)) + smartHomeLoginKey)
	pwd := sha256Hex(loginID + sha256Hex(s.password) + smartHomeLoginKey)
	b := map[string]any{
		"data": map[string]any{"platform": 2, "deviceId": s.clientID},
		"iotData": map[string]any{
			"appId": smartHomeAppID, "src": smartHomeAppID, "clientType": 1, "loginAccount": s.account,
			"iampwd": iam, "password": pwd, "pushToken": randomHex(60),
			"stamp": stamp(time.Now()), "reqId": randomHex(16),
		},
	}

	r, err = s.request(ctx, "/mj/user/login", b)
	if err != nil {
		return err
	}

	mdata, _ := r["mdata"].(map[string]any)

	token := str(mdata["accessToken"])
	if token == "" {
		return fmt.Errorf("%w: login answered without token", ErrCloudBadReply)
	}

	s.mu.Lock()
	s.accessToken = token
	s.mu.Unlock()

	// Session keys for control (meiju: accessToken + randomData).
	encKeyHex, encIVHex := str(r["accessToken"]), str(r["randomData"])
	if encKeyHex != "" && encIVHex != "" {
		digest := sha256Hex(smartHomeLoginKey)
		tmpKey, tmpIV := []byte(digest[:16]), []byte(digest[16:32])
		ek, err1 := hex.DecodeString(encKeyHex)
		ei, err2 := hex.DecodeString(encIVHex)

		if err1 == nil && err2 == nil {
			k, err1 := DecryptCBC(tmpKey, tmpIV, ek)
			iv, err2 := DecryptCBC(tmpKey, tmpIV, ei)

			if err1 == nil && err2 == nil {
				s.mu.Lock()
				s.aesKey, s.aesIV = k, iv
				s.mu.Unlock()
			}
		}
	}

	return nil
}

func (s *smartHome) token(ctx context.Context, udpid string, applianceID uint64) (string, string, error) {
	r, err := s.request(ctx, "/v1/iot/secure/getToken", s.body(map[string]any{"udpid": udpid, "applianceCodes": strconv.FormatUint(applianceID, 10)}))
	if err != nil {
		return "", "", err
	}

	t, k, ok := tokenFrom(r, udpid)
	if !ok {
		return "", "", ErrCloudNoToken
	}

	return t, k, nil
}

func (s *smartHome) appliances(ctx context.Context) ([]map[string]any, error) {
	r, err := s.request(ctx, "/v1/appliance/user/list/get", s.body(map[string]any{"homegroupId": "0"}))
	if err != nil {
		return nil, err
	}

	return listOf(r["list"]), nil
}

func listOf(v any) []map[string]any {
	arr, _ := v.([]any)
	out := make([]map[string]any, 0, len(arr))

	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}

	return out
}

func (s *smartHome) transparentSend(ctx context.Context, frame []byte, deviceID uint64, applianceID string) ([][]byte, error) {
	if !s.canControl() {
		return nil, ErrCloudSessionKeys
	}

	s.mu.Lock()
	key, iv := s.aesKey, s.aesIV
	s.mu.Unlock()

	enc, err := EncryptCBC(key, iv, []byte(csv(cloudPacket(deviceID, frame, time.Now()))))
	if err != nil {
		return nil, err
	}

	r, err := s.request(ctx, "/v1/appliance/transparent/send", s.body(map[string]any{
		"order": hexs(enc), "funId": "0000", "applianceCode": applianceID, "waitResp": true,
	}))
	if err != nil {
		return nil, err
	}

	rd, err := hex.DecodeString(str(r["reply"]))
	if err != nil || len(rd) == 0 {
		return nil, fmt.Errorf("%w: no answer from the device", ErrCloudBadReply)
	}

	plain, err := DecryptCBC(key, iv, rd)
	if err != nil {
		return nil, ErrCloudSessionKeys
	}

	return innerFrame(string(plain))
}

// ---- NetHome Plus --------------------------------------------------------

const (
	netHomeAppID  = "1017"
	netHomeBase   = "https://mapp.appsmb.com"
	netHomeAppKey = "3742e9e5842d4ad59c2db887e12449f9"
)

type netHomePlus struct {
	account, password, clientID string

	mu          sync.Mutex
	sessionID   string
	accessToken string
}

func (n *netHomePlus) kind() CloudKind { return CloudNetHomePlus }

func (n *netHomePlus) canControl() bool {
	n.mu.Lock()
	defer n.mu.Unlock()

	return n.sessionID != "" && n.accessToken != ""
}

func (n *netHomePlus) session() *CloudSession {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.sessionID == "" {
		return nil
	}

	return &CloudSession{Kind: CloudNetHomePlus, AccessToken: n.accessToken, SessionID: n.sessionID, SavedAt: time.Now()}
}

func (n *netHomePlus) restore(cs *CloudSession) bool {
	if cs.SessionID == "" || cs.AccessToken == "" {
		return false
	}

	n.mu.Lock()
	n.sessionID, n.accessToken = cs.SessionID, cs.AccessToken
	n.mu.Unlock()

	return true
}

func (n *netHomePlus) body(extra map[string]string) map[string]string {
	n.mu.Lock()
	sid := n.sessionID
	n.mu.Unlock()

	b := map[string]string{
		"appId": netHomeAppID, "src": netHomeAppID, "format": "2", "clientType": "1", "language": "en_US",
		"deviceId": n.clientID, "stamp": stamp(time.Now()), "sessionId": sid,
	}
	for k, v := range extra {
		b[k] = v
	}

	return b
}

func netHomeSign(endpoint string, fields map[string]string) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + fields[k]
	}

	return sha256Hex(endpoint + strings.Join(parts, "&") + netHomeAppKey)
}

func (n *netHomePlus) request(ctx context.Context, endpoint string, fields map[string]string) (map[string]any, error) {
	form := url.Values{}
	for k, v := range fields {
		form.Set(k, v)
	}

	form.Set("sign", netHomeSign(endpoint, fields))

	req, err := http.NewRequest(http.MethodPost, netHomeBase+endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	r, err := postJSON(ctx, req)
	if err != nil {
		return nil, err
	}

	if code := codeOf(r["errorCode"]); code != 0 {
		return nil, &CloudError{Code: code, Msg: CleanText(str(r["msg"]), "", 200)}
	}

	res, _ := r["result"].(map[string]any)
	if res == nil {
		res = map[string]any{}
	}

	return res, nil
}

func (n *netHomePlus) login(ctx context.Context) error {
	n.mu.Lock()
	n.sessionID, n.accessToken = "", ""
	n.mu.Unlock()

	r, err := n.request(ctx, "/v1/user/login/id/get", n.body(map[string]string{"loginAccount": n.account}))
	if err != nil {
		return err
	}

	loginID := str(r["loginId"])
	if loginID == "" {
		return fmt.Errorf("%w: login answered without id", ErrCloudBadReply)
	}

	pwd := sha256Hex(loginID + sha256Hex(n.password) + netHomeAppKey)

	r, err = n.request(ctx, "/v1/user/login", n.body(map[string]string{"loginAccount": n.account, "password": pwd}))
	if err != nil {
		return err
	}

	sid, token := str(r["sessionId"]), str(r["accessToken"])
	if sid == "" || token == "" {
		return fmt.Errorf("%w: login answered without session", ErrCloudBadReply)
	}

	n.mu.Lock()
	n.sessionID, n.accessToken = sid, token
	n.mu.Unlock()

	return nil
}

func (n *netHomePlus) token(ctx context.Context, udpid string, applianceID uint64) (string, string, error) {
	r, err := n.request(ctx, "/v1/iot/secure/getToken", n.body(map[string]string{"udpid": udpid, "applianceCodes": strconv.FormatUint(applianceID, 10)}))
	if err != nil {
		return "", "", err
	}

	t, k, ok := tokenFrom(r, udpid)
	if !ok {
		return "", "", ErrCloudNoToken
	}

	return t, k, nil
}

func (n *netHomePlus) appliances(ctx context.Context) ([]map[string]any, error) {
	r, err := n.request(ctx, "/v1/appliance/user/list/get", n.body(nil))
	if err != nil {
		return nil, err
	}

	return listOf(r["list"]), nil
}

func netHomeDataKey(accessTokenHex string) ([]byte, error) {
	key := []byte(md5Hex(netHomeAppKey)[:16])

	tok, err := hex.DecodeString(accessTokenHex)
	if err != nil {
		return nil, ErrCloudSessionKeys
	}

	return DecryptECB(key, tok)
}

func (n *netHomePlus) transparentSend(ctx context.Context, frame []byte, deviceID uint64, applianceID string) ([][]byte, error) {
	n.mu.Lock()
	token := n.accessToken
	n.mu.Unlock()

	dataKey, err := netHomeDataKey(token)
	if err != nil {
		return nil, ErrCloudSessionKeys
	}

	enc, err := EncryptECB(dataKey, []byte(csv(cloudPacket(deviceID, frame, time.Now()))))
	if err != nil {
		return nil, err
	}

	r, err := n.request(ctx, "/v1/appliance/transparent/send", n.body(map[string]string{
		"order": hexs(enc), "funId": "0000", "applianceId": applianceID,
	}))
	if err != nil {
		return nil, err
	}

	rd, err := hex.DecodeString(str(r["reply"]))
	if err != nil || len(rd) == 0 {
		return nil, fmt.Errorf("%w: no answer from the device", ErrCloudBadReply)
	}

	plain, err := DecryptECB(dataKey, rd)
	if err != nil {
		return nil, ErrCloudSessionKeys
	}

	return innerFrame(string(plain))
}

// ---- Account (shared session per account) ---------------------------------

// CloudAccount is a logged in (or loggable) Midea account. It keeps one API
// per flavour and logs in lazily; safe for concurrent use.
type CloudAccount struct {
	Account  string
	Password string
	// ClientID is the persistent 16 hex "deviceId" of this installation.
	ClientID string
	// OnSession is called after a successful login with the new session.
	OnSession func(*CloudSession)

	mu      sync.Mutex
	apis    map[CloudKind]cloudAPI
	logged  map[CloudKind]bool
	pausedU time.Time
	fails   int
}

// NewCloudAccount creates the account; saved sessions are restored.
func NewCloudAccount(account, password, clientID string, saved []*CloudSession) *CloudAccount {
	if clientID == "" {
		clientID = randomHex(8)
	}

	a := &CloudAccount{Account: strings.TrimSpace(account), Password: password, ClientID: clientID,
		apis: map[CloudKind]cloudAPI{}, logged: map[CloudKind]bool{}}
	a.apis[CloudSmartHome] = &smartHome{account: a.Account, password: password, clientID: clientID}
	a.apis[CloudNetHomePlus] = &netHomePlus{account: a.Account, password: password, clientID: clientID}

	for _, s := range saved {
		if s == nil {
			continue
		}

		if api, ok := a.apis[s.Kind]; ok && api.restore(s) {
			a.logged[s.Kind] = true
		}
	}

	return a
}

func (a *CloudAccount) ensure(ctx context.Context, k CloudKind, force bool) (cloudAPI, error) {
	a.mu.Lock()
	api := a.apis[k]
	logged := a.logged[k]
	paused := a.pausedU
	a.mu.Unlock()

	if logged && !force {
		return api, nil
	}

	if time.Now().Before(paused) {
		return nil, fmt.Errorf("cloud login paused after failures until %s", paused.Format("15:04"))
	}

	if err := api.login(ctx); err != nil {
		a.mu.Lock()
		a.fails++
		if a.fails >= 3 {
			a.pausedU = time.Now().Add(10 * time.Minute)
			a.fails = 0
		}
		a.mu.Unlock()

		return nil, err
	}

	a.mu.Lock()
	a.logged[k] = true
	a.fails = 0
	a.mu.Unlock()

	if a.OnSession != nil {
		if s := api.session(); s != nil {
			a.OnSession(s)
		}
	}

	return api, nil
}

// ResetPause lets the next call log in again right away (user action).
func (a *CloudAccount) ResetPause() {
	a.mu.Lock()
	a.pausedU = time.Time{}
	a.fails = 0
	a.mu.Unlock()
}

// run executes op with a login, retrying once after a session error.
func (a *CloudAccount) run(ctx context.Context, k CloudKind, op func(cloudAPI) error) error {
	api, err := a.ensure(ctx, k, false)
	if err != nil {
		return err
	}

	err = op(api)

	var ce *CloudError
	if err == nil || !(errors.As(err, &ce) && ce.IsSessionError() || errors.Is(err, ErrCloudSessionKeys)) {
		return err
	}

	if api, err = a.ensure(ctx, k, true); err != nil {
		return err
	}

	return op(api)
}

// Appliances lists the account's devices (first flavour with results).
func (a *CloudAccount) Appliances(ctx context.Context) ([]CloudAppliance, error) {
	lastErr := error(ErrCloudNoToken)

	for _, k := range CloudKinds {
		var raw []map[string]any

		err := a.run(ctx, k, func(api cloudAPI) error {
			var e error
			raw, e = api.appliances(ctx)

			return e
		})
		if err != nil {
			lastErr = err
			continue
		}

		var out []CloudAppliance

		for _, it := range raw {
			idStr := str(it["id"])
			if idStr == "" {
				idStr = str(it["applianceCode"])
			}

			id, err := strconv.ParseUint(strings.TrimSpace(idStr), 10, 64)
			if err != nil || id == 0 {
				continue
			}

			online := str(it["onlineStatus"])
			out = append(out, CloudAppliance{
				DeviceID: id, Name: CleanText(str(it["name"]), "PortaSplit", 60),
				Type: parseType(str(it["type"])), Online: online == "" || online == "1",
			})
		}

		if len(out) > 0 {
			return out, nil
		}
	}

	return nil, lastErr
}

func parseType(s string) int {
	t := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(s)), "0x")
	if t == "" {
		return 0
	}

	if len(t) > 2 {
		t = t[len(t)-2:]
	}

	v, err := strconv.ParseInt(t, 16, 32)
	if err != nil {
		return 0
	}

	return int(v)
}

// Token fetches token and key for udpid from one flavour.
func (a *CloudAccount) Token(ctx context.Context, k CloudKind, udpid string, deviceID uint64) (string, string, error) {
	var tok, key string

	err := a.run(ctx, k, func(api cloudAPI) error {
		var e error
		tok, key, e = api.token(ctx, udpid, deviceID)

		return e
	})

	return tok, key, err
}

// Send sends a frame through the cloud (transparent send), preferring the
// flavour that can control.
func (a *CloudAccount) Send(ctx context.Context, frame []byte, deviceID uint64) ([][]byte, error) {
	var lastErr error

	for _, k := range CloudKinds {
		var frames [][]byte

		err := a.run(ctx, k, func(api cloudAPI) error {
			if !api.canControl() {
				return ErrCloudSessionKeys
			}

			var e error
			frames, e = api.transparentSend(ctx, frame, deviceID, strconv.FormatUint(deviceID, 10))

			return e
		})
		if err == nil {
			return frames, nil
		}

		lastErr = err
		if IsApplianceOffline(err) {
			return nil, err
		}
	}

	return nil, lastErr
}
