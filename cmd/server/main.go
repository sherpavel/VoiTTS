package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"voitts/internal/api"
	"voitts/internal/audio"
	"voitts/internal/profile"
	"voitts/internal/tts"
	"voitts/internal/web"
)

const port = 17890 // Fixed forever!

// version set by scripts/build-release.sh:
//
//	go build -ldflags "-X main.version=v1.2.3"
var version = "dev"

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("voitts-server", version)
		return
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))
	slog.Info("starting", "version", version)

	if err := run(); err != nil {
		slog.Error("server failed", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	sigCtx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	procCtx, stopProcs := context.WithCancel(context.Background())
	defer stopProcs()

	// External dependencies check: Piper, voice model, audio stack
	if err := preflight(sigCtx, os.Stdout); err != nil {
		return err
	}

	// User profiles
	profileStore := profile.NewStore()
	if err := profileStore.Load(""); err != nil {
		return fmt.Errorf("load profiles: %w", err)
	}
	slog.Info("profiles loaded", "path", profileStore.Path())

	// TTS engine
	ttsPiper, err := tts.New("", tts.DefaultModel, 0, 0)
	if err != nil {
		return fmt.Errorf("create piper engine: %w", err)
	}

	// Virtual source, sink and monitor
	mic, err := audio.Open(procCtx, audio.Options{
		Monitor:        true,
		MonitorLatency: audio.DefaultMonitorLatency,
	})
	if err != nil {
		return fmt.Errorf("create audio device: %w", err)
	}
	defer closeAndLog("audio device", mic)
	slog.Info("virtual microphone ready", "source", mic.Source())

	stream, err := mic.OpenStream(procCtx, audio.Format(ttsPiper.Format()))
	if err != nil {
		return fmt.Errorf("create audio stream: %w", err)
	}
	defer closeAndLog("audio stream", stream)

	// Persistent Piper instance, cold start takes ~500ms, inference 20-100ms
	if err := ttsPiper.Start(procCtx, stream); err != nil {
		return fmt.Errorf("start piper tts: %w", err)
	}
	defer closeAndLog("piper tts", ttsPiper)

	// -------------------
	// Web server boundary
	// -------------------

	webHandler, err := web.Handler()
	if err != nil {
		return fmt.Errorf("create webui handler: %w", err)
	}

	ln, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		return listenError(port, err)
	}

	mux := http.NewServeMux()
	apiServer := api.New(profileStore, ttsPiper)
	mux.Handle("/api/", apiServer.Handler())
	mux.Handle("/", webHandler)

	server := &http.Server{
		Handler:           requestLogger(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(ln) }()

	slog.Info("server started", "addr", ln.Addr().String())
	// Display connection QR code
	announce(os.Stdout, ln.Addr())

	// Block until errors or exit
	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-sigCtx.Done():
	}

	// Force close, SIGINT/SIGTERM kill Piper process anyways
	if err := server.Close(); err != nil {
		return fmt.Errorf("close server: %w", err)
	}
	if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// Reports a teardown failure
func closeAndLog(what string, c io.Closer) {
	if err := c.Close(); err != nil {
		slog.Error("Failed to close "+what, "error", err.Error())
	}
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		slog.Info("request",
			"method", r.Method,
			"endpoint", r.URL.Path,
			"status", rec.status,
			"time_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}
