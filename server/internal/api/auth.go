package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mortendnk/portacontrol/server/internal/store"
)

// Roles.
const (
	RoleNone  = 0
	RoleGuest = 1 // household: control on the home network
	RoleAdmin = 2 // settings, setup, secrets
)

const sessionCookie = "pc_session"

// EnsureAPIKey returns the API key, creating it on first use.
func EnsureAPIKey(st *store.Store) (string, error) {
	var k string
	if err := st.GetKV("api_key", &k); err == nil && k != "" {
		return k, nil
	}

	b := make([]byte, 24)
	_, _ = rand.Read(b)
	k = "pc_" + hex.EncodeToString(b)

	return k, st.SetKV("api_key", k)
}

// RotateAPIKey creates a new API key (old app pairings stop working).
func RotateAPIKey(st *store.Store) (string, error) {
	_ = st.DeleteKV("api_key")
	return EnsureAPIKey(st)
}

// PasswordSet reports whether an admin password exists.
func PasswordSet(st *store.Store) bool {
	var h string
	return st.GetKV("admin_password", &h) == nil && h != ""
}

// SetPassword stores a new admin password (bcrypt) and ends all sessions.
func SetPassword(st *store.Store, pw string) error {
	if len(pw) < 6 {
		return errors.New("password must have at least 6 characters")
	}

	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	st.DeleteAllSessions()

	return st.SetKV("admin_password", string(h))
}

// CheckPassword compares pw with the stored hash.
func CheckPassword(st *store.Store, pw string) bool {
	var h string
	if st.GetKV("admin_password", &h) != nil || h == "" {
		return false
	}

	return bcrypt.CompareHashAndPassword([]byte(h), []byte(pw)) == nil
}

// GeneratePassword returns a readable random password.
func GeneratePassword() string {
	const alphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	b := make([]byte, 14)
	_, _ = rand.Read(b)

	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}

	return string(b[:4]) + "-" + string(b[4:9]) + "-" + string(b[9:])
}

var privateNets = func() []*net.IPNet {
	var out []*net.IPNet

	for _, c := range []string{"127.0.0.0/8", "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
		"100.64.0.0/10", "::1/128", "fc00::/7", "fe80::/10"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}

	return out
}()

func clientIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}

	return net.ParseIP(host)
}

func isPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}

	for _, n := range privateNets {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

func (s *Server) apiKeyFrom(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}

	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
		return strings.TrimPrefix(a, "Bearer ")
	}

	return r.URL.Query().Get("key")
}

// role determines what the caller may do.
func (s *Server) role(r *http.Request) int {
	if k := s.apiKeyFrom(r); k != "" {
		if want, err := EnsureAPIKey(s.App.Store); err == nil && subtle.ConstantTimeCompare([]byte(k), []byte(want)) == 1 {
			return RoleAdmin
		}
	}

	if c, err := r.Cookie(sessionCookie); err == nil && s.App.Store.ValidSession(c.Value) {
		return RoleAdmin
	}

	private := isPrivate(clientIP(r))

	// First start without password: the home network may set everything up.
	if private && !PasswordSet(s.App.Store) {
		return RoleAdmin
	}

	if private && s.App.Store.Settings().GuestControl {
		return RoleGuest
	}

	return RoleNone
}

func (s *Server) need(min int, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := s.role(r)
		if role >= min {
			next(w, r)
			return
		}

		if role == RoleNone {
			writeErr(w, http.StatusUnauthorized, "login required")
			return
		}

		writeErr(w, http.StatusForbidden, "only the administrator may do this – please log in")
	}
}

// loginLimiter slows down password guessing per IP.
type loginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	var recent []time.Time

	for _, t := range l.fails[ip] {
		if time.Since(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}

	l.fails[ip] = recent

	return len(recent) >= 10
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	l.fails[ip] = append(l.fails[ip], time.Now())
	l.mu.Unlock()
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}

	if !readJSON(w, r, &req) {
		return
	}

	ip := clientIP(r).String()
	if s.limiter.blocked(ip) {
		writeErr(w, http.StatusTooManyRequests, "too many failed logins – try again in 15 minutes")
		return
	}

	if !CheckPassword(s.App.Store, req.Password) {
		s.limiter.fail(ip)
		time.Sleep(700 * time.Millisecond)
		writeErr(w, http.StatusUnauthorized, "wrong password")

		return
	}

	tok, err := s.App.Store.CreateSession(30 * 24 * time.Hour)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		MaxAge: 30 * 24 * 3600, Secure: r.TLS != nil})
	writeJSON(w, map[string]any{"ok": true, "role": "admin"})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.App.Store.DeleteSession(c.Value)
	}

	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	writeJSON(w, map[string]any{"ok": true})
}

func roleName(r int) string {
	switch r {
	case RoleAdmin:
		return "admin"
	case RoleGuest:
		return "guest"
	}

	return "none"
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{
		"role": roleName(s.role(r)), "password_set": PasswordSet(s.App.Store),
		"guest_control": s.App.Store.Settings().GuestControl, "private_network": isPrivate(clientIP(r)),
	})
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}

	if !readJSON(w, r, &req) {
		return
	}

	if err := SetPassword(s.App.Store, req.Password); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	// Keep the caller logged in.
	tok, _ := s.App.Store.CreateSession(30 * 24 * time.Hour)
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600})
	writeJSON(w, map[string]any{"ok": true})
}
