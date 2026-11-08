// Command portacontrol-cli administers and controls a PortaControl server.
//
//	portacontrol-cli apikey [--rotate]                 print the API key (local database)
//	portacontrol-cli password [--generate] [--if-unset] [password]
//	portacontrol-cli status                            all devices
//	portacontrol-cli on|off <device|all>
//	portacontrol-cli set <device> mode=cool target=23 fan=auto eco=on ...
//	portacontrol-cli scene <name or id> [device]
//	portacontrol-cli discover [ip]                     search devices on the LAN
//	portacontrol-cli token --account a@b.c --password x <ip>   token/key from the Midea cloud
//	portacontrol-cli update [version]                  install the latest release (root, LXC)
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/api"
	"github.com/mortendnk/portacontrol/server/internal/config"
	"github.com/mortendnk/portacontrol/server/internal/midea"
	"github.com/mortendnk/portacontrol/server/internal/store"
	"github.com/mortendnk/portacontrol/server/internal/version"
)

type globals struct {
	config string
	api    string
	key    string
}

func main() {
	g := globals{}
	fs := flag.NewFlagSet("portacontrol-cli", flag.ExitOnError)
	fs.StringVar(&g.config, "config", envOr("PORTACONTROL_CONFIG", "/etc/portacontrol/config.yaml"), "configuration file (local commands)")
	fs.StringVar(&g.api, "api", envOr("PORTACONTROL_API", "http://127.0.0.1:8080"), "server URL")
	fs.StringVar(&g.key, "api-key", os.Getenv("PORTACONTROL_API_KEY"), "API key (default: from the local database)")
	showVersion := fs.Bool("version", false, "print version")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: portacontrol-cli [flags] <command> [args]\n\ncommands: apikey, password, status, on, off, set, scene, discover, token, update, version\n\nflags:")
		fs.PrintDefaults()
	}

	_ = fs.Parse(os.Args[1:])

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	args := fs.Args()
	if len(args) == 0 {
		fs.Usage()
		os.Exit(2)
	}

	var err error

	switch args[0] {
	case "version":
		fmt.Println(version.String())
	case "apikey":
		err = cmdAPIKey(g, args[1:])
	case "password":
		err = cmdPassword(g, args[1:])
	case "status":
		err = cmdStatus(g)
	case "on", "off":
		err = cmdPower(g, args[0] == "on", args[1:])
	case "set":
		err = cmdSet(g, args[1:])
	case "scene":
		err = cmdScene(g, args[1:])
	case "discover":
		err = cmdDiscover(args[1:])
	case "token":
		err = cmdToken(args[1:])
	case "update":
		err = cmdUpdate(args[1:])
	default:
		fs.Usage()
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}

	return def
}

func openStore(g globals) (*store.Store, error) {
	cfg, err := config.Load(g.config)
	if err != nil {
		return nil, err
	}

	return store.Open(cfg.DataDir)
}

func cmdAPIKey(g globals, args []string) error {
	fs := flag.NewFlagSet("apikey", flag.ExitOnError)
	rotate := fs.Bool("rotate", false, "create a new key (paired apps must be paired again)")
	_ = fs.Parse(args)

	st, err := openStore(g)
	if err != nil {
		return err
	}
	defer st.Close()

	var k string
	if *rotate {
		k, err = api.RotateAPIKey(st)
	} else {
		k, err = api.EnsureAPIKey(st)
	}

	if err != nil {
		return err
	}

	fmt.Println(k)

	return nil
}

func cmdPassword(g globals, args []string) error {
	fs := flag.NewFlagSet("password", flag.ExitOnError)
	gen := fs.Bool("generate", false, "generate a random password and print it")
	ifUnset := fs.Bool("if-unset", false, "do nothing when a password exists")
	_ = fs.Parse(args)

	st, err := openStore(g)
	if err != nil {
		return err
	}
	defer st.Close()

	if *ifUnset && api.PasswordSet(st) {
		fmt.Println("(unchanged – already set)")
		return nil
	}

	pw := fs.Arg(0)

	switch {
	case *gen:
		pw = api.GeneratePassword()
	case pw == "":
		return errors.New("give a password or --generate")
	}

	if err := api.SetPassword(st, pw); err != nil {
		return err
	}

	fmt.Println(pw)

	return nil
}

func (g globals) apiKey() string {
	if g.key != "" {
		return g.key
	}

	if st, err := openStore(g); err == nil {
		defer st.Close()

		if k, err := api.EnsureAPIKey(st); err == nil {
			return k
		}
	}

	return ""
}

func call(g globals, method, path string, body, out any) error {
	var rd io.Reader

	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, strings.TrimRight(g.api, "/")+"/api/v1"+path, rd)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Client", "CLI")

	if k := g.apiKey(); k != "" {
		req.Header.Set("X-API-Key", k)
	}

	c := &http.Client{Timeout: 90 * time.Second}

	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}

		_ = json.Unmarshal(data, &e)

		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error)
	}

	if out != nil {
		return json.Unmarshal(data, out)
	}

	return nil
}

type deviceView struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	IP     string       `json:"ip"`
	Online bool         `json:"online"`
	Path   string       `json:"path"`
	State  *midea.State `json:"state"`
}

func cmdStatus(g globals) error {
	var list []deviceView
	if err := call(g, http.MethodGet, "/devices", nil, &list); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tONLINE\tPOWER\tMODE\tTARGET\tINDOOR\tWATTS")

	for _, d := range list {
		power, mode, target, indoor, watts := "-", "-", "-", "-", "-"

		if s := d.State; s != nil {
			power = map[bool]string{true: "on", false: "off"}[s.Power]
			mode = s.ModeName
			target = fmt.Sprintf("%.0f°C", s.TargetTemperature)

			if s.IndoorTemperature != nil {
				indoor = fmt.Sprintf("%.1f°C", *s.IndoorTemperature)
			}

			if s.PowerWatts != nil {
				watts = fmt.Sprintf("%.0f", *s.PowerWatts)
			}
		}

		fmt.Fprintf(tw, "%s\t%s\t%v\t%s\t%s\t%s\t%s\t%s\n", d.ID, d.Name, d.Online, power, mode, target, indoor, watts)
	}

	return tw.Flush()
}

func resolve(g globals, name string) ([]string, error) {
	var list []deviceView
	if err := call(g, http.MethodGet, "/devices", nil, &list); err != nil {
		return nil, err
	}

	var out []string

	for _, d := range list {
		if name == "all" || d.ID == name || strings.EqualFold(d.Name, name) {
			out = append(out, d.ID)
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no device %q", name)
	}

	return out, nil
}

func cmdPower(g globals, on bool, args []string) error {
	name := "all"
	if len(args) > 0 {
		name = args[0]
	}

	ids, err := resolve(g, name)
	if err != nil {
		return err
	}

	for _, id := range ids {
		if err := call(g, http.MethodPost, "/devices/"+id+"/control", store.Control{Power: &on}, nil); err != nil {
			return err
		}
	}

	return nil
}

func cmdSet(g globals, args []string) error {
	if len(args) < 2 {
		return errors.New("usage: set <device> key=value ... (power, mode, target, fan, swing, eco, turbo, sleep, purifier, freeze, outdoor_silent, ieco, display, sound, rate, off_timer, on_timer)")
	}

	ids, err := resolve(g, args[0])
	if err != nil {
		return err
	}

	var c store.Control

	for _, kv := range args[1:] {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("expected key=value, got %q", kv)
		}

		b := v == "on" || v == "1" || v == "true"
		boolp := &b

		switch k {
		case "power":
			c.Power = boolp
		case "mode":
			c.Mode = &v
		case "target":
			f, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return err
			}

			c.Target = &f
		case "fan":
			n := midea.FanAuto
			if v != "auto" {
				if n, err = strconv.Atoi(v); err != nil {
					return err
				}
			}

			c.Fan = &n
		case "swing":
			c.Swing = boolp
		case "eco":
			c.Eco = boolp
		case "turbo":
			c.Turbo = boolp
		case "sleep":
			c.Sleep = boolp
		case "purifier":
			c.Purifier = boolp
		case "freeze":
			c.FreezeProtection = boolp
		case "outdoor_silent":
			c.OutdoorSilent = boolp
		case "ieco":
			c.IECO = boolp
		case "display":
			c.Display = boolp
		case "sound":
			c.Sound = boolp
		case "self_clean":
			c.SelfClean = boolp
		case "rate":
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			c.Rate = &n
		case "off_timer", "on_timer":
			n, err := strconv.Atoi(v)
			if err != nil {
				return err
			}

			if k == "off_timer" {
				c.OffTimerMinutes = &n
			} else {
				c.OnTimerMinutes = &n
			}
		default:
			return fmt.Errorf("unknown key %q", k)
		}
	}

	for _, id := range ids {
		if err := call(g, http.MethodPost, "/devices/"+id+"/control", c, nil); err != nil {
			return err
		}
	}

	return nil
}

func cmdScene(g globals, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: scene <name or id> [device]")
	}

	var scenes []store.Scene
	if err := call(g, http.MethodGet, "/scenes", nil, &scenes); err != nil {
		return err
	}

	for _, s := range scenes {
		if s.ID == args[0] || strings.EqualFold(s.Name, args[0]) {
			path := "/scenes/" + s.ID + "/apply"
			if len(args) > 1 {
				path += "?device=" + args[1]
			}

			return call(g, http.MethodPost, path, nil, nil)
		}
	}

	return fmt.Errorf("no scene %q", args[0])
}

func cmdDiscover(args []string) error {
	var targets []string
	if len(args) > 0 {
		targets = args
	}

	list, err := midea.Scan(targets, 4*time.Second, nil)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "IP\tPORT\tDEVICE ID\tNAME\tTYPE\tVERSION")

	for _, d := range list {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t0x%X\tV%d\n", d.IP, d.Port, d.DeviceID, d.Name, d.Type, d.Version)
	}

	return tw.Flush()
}

func cmdToken(args []string) error {
	fs := flag.NewFlagSet("token", flag.ExitOnError)
	account := fs.String("account", "", "Midea account (MSmartHome / NetHome Plus e-mail)")
	password := fs.String("password", "", "password")
	_ = fs.Parse(args)

	if *account == "" || *password == "" || fs.NArg() != 1 {
		return errors.New("usage: token --account <mail> --password <pw> <device-ip>")
	}

	list, err := midea.Scan([]string{fs.Arg(0)}, 4*time.Second, nil)
	if err != nil {
		return err
	}

	if len(list) == 0 {
		return errors.New("no Midea device answered at this address")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	acc := midea.NewCloudAccount(*account, *password, "", nil)

	cfg, err := midea.FetchConfig(ctx, acc, list[0])
	if err != nil {
		return err
	}

	fmt.Printf("device_id %d\nip        %s\nport      %d\ntoken     %s\nkey       %s\n", cfg.DeviceID, cfg.IP, cfg.Port, cfg.Token, cfg.Key)

	return nil
}

// cmdUpdate replaces the binaries with a release from GitHub and restarts
// the service (inside the LXC container, as root).
func cmdUpdate(args []string) error {
	if runtime.GOOS != "linux" {
		return errors.New("update works on Linux only")
	}

	tag := "latest"
	if len(args) > 0 {
		tag = args[0]
	}

	base := "https://github.com/mortendnk/PortaControl/releases/latest/download"
	if tag != "latest" {
		base = "https://github.com/mortendnk/PortaControl/releases/download/" + tag
	}

	asset := "portacontrol-linux-" + runtime.GOARCH + ".tar.gz"

	tmp, err := os.MkdirTemp("", "portacontrol-update")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	get := func(url, dst string) error {
		resp, err := http.Get(url) //nolint:gosec,noctx // fixed GitHub URL
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
		}

		f, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(f, resp.Body)

		return err
	}

	fmt.Println("downloading", base+"/"+asset)

	bundle := filepath.Join(tmp, asset)
	if err := get(base+"/"+asset, bundle); err != nil {
		return err
	}

	if err := get(base+"/"+asset+".sha256", bundle+".sha256"); err != nil {
		return err
	}

	want, _ := os.ReadFile(bundle + ".sha256")
	data, _ := os.ReadFile(bundle)
	sum := sha256.Sum256(data)

	if !strings.HasPrefix(strings.TrimSpace(string(want)), hex.EncodeToString(sum[:])) {
		return errors.New("checksum mismatch")
	}

	if out, err := exec.Command("tar", "-xzf", bundle, "-C", tmp).CombinedOutput(); err != nil {
		return fmt.Errorf("unpack: %v %s", err, out)
	}

	for _, f := range []string{"portacontrold", "portacontrol-cli"} {
		src := filepath.Join(tmp, f)
		dst := "/usr/local/bin/" + f

		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}

		if err := os.WriteFile(dst+".new", b, 0o755); err != nil { //nolint:gosec // executable
			return err
		}

		if err := os.Rename(dst+".new", dst); err != nil {
			return err
		}
	}

	if out, err := exec.Command("systemctl", "restart", "portacontrold").CombinedOutput(); err != nil {
		return fmt.Errorf("restart: %v %s", err, out)
	}

	out, _ := exec.Command("/usr/local/bin/portacontrold", "-version").Output()
	fmt.Printf("updated to %s", out)

	return nil
}
