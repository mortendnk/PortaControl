// Command portacontrold is the PortaControl server: it controls Midea
// PortaSplit air conditioners on the LAN and serves the API and web UI.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/mortendnk/portacontrol/server/internal/api"
	"github.com/mortendnk/portacontrol/server/internal/app"
	"github.com/mortendnk/portacontrol/server/internal/config"
	"github.com/mortendnk/portacontrol/server/internal/mqtt"
	"github.com/mortendnk/portacontrol/server/internal/store"
	"github.com/mortendnk/portacontrol/server/internal/version"
	"github.com/mortendnk/portacontrol/server/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "portacontrold:", err)
		os.Exit(1)
	}
}

func run() error {
	cfgPath := flag.String("config", "/etc/portacontrol/config.yaml", "configuration file")
	showVersion := flag.Bool("version", false, "print version and exit")
	simulate := flag.Bool("simulate", false, "start a simulated PortaSplit (demo without hardware)")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return nil
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		return err
	}

	if *simulate {
		cfg.Simulate = true
	}

	level := slog.LevelInfo

	switch strings.ToLower(cfg.Log.Level) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var handler slog.Handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	if cfg.Log.JSON {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	}

	log := slog.New(handler)

	st, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()

	if _, err := api.EnsureAPIKey(st); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := app.New(cfg, st, log)
	bridge := &mqtt.Bridge{App: a, Log: log}
	a.OnState = func(id string) { bridge.PublishState(id) }
	a.OnDevices = bridge.Announce
	a.OnSettings = bridge.Reload

	if err := a.Start(ctx); err != nil {
		return err
	}
	defer a.Close()

	bridge.Reload()
	defer bridge.Close()

	srv := api.New(a, web.Handler())
	srv.MQTTStatus = bridge.Status

	// No WriteTimeout: the SSE event stream is long-lived.
	hs := &http.Server{Addr: cfg.Listen, Handler: srv.Router(), ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout: 2 * time.Minute, MaxHeaderBytes: 64 << 10}

	go func() {
		<-ctx.Done()

		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = hs.Shutdown(sctx)
	}()

	log.Info("portacontrold starting", "listen", cfg.Listen, "public_url", srv.PublicURL(), "data_dir", cfg.DataDir,
		"version", version.String(), "simulate", cfg.Simulate)

	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	return nil
}
