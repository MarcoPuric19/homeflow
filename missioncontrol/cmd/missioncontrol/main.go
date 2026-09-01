// Command missioncontrol runs the local cockpit: cluster dependencies and the
// ticket exchange between the assistants working on HomeFlow.
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
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"missioncontrol/internal/cluster"
	"missioncontrol/internal/deploy"
	"missioncontrol/internal/httpd"
	"missioncontrol/internal/repo"
	"missioncontrol/internal/tickets"
)

// Mission Control versions itself. It is deliberately not tied to HomeFlow's
// VERSION: the cockpit is a separate product with its own lifecycle, and its
// tickets carry this version, not HomeFlow's.
const version = "0.14.0"

func main() {
	if err := run(); err != nil {
		slog.New(slog.NewTextHandler(os.Stderr, nil)).Error("mission control gestoppt", "error", err)
		os.Exit(1)
	}
}

func run() error {
	defaultRoot := repositoryRoot()
	address := flag.String("addr", "127.0.0.1:7777", "Adresse; bewusst nur Loopback, es gibt keine Anmeldung")
	namespaces := flag.String("namespaces", "homeflow,homeflow-monitoring", "Namespaces, kommagetrennt")
	interval := flag.Duration("interval", 15*time.Second, "Abstand zwischen zwei Cluster-Abfragen")
	ticketDir := flag.String("tickets", filepath.Join(defaultRoot, "missioncontrol", "tickets"), "Verzeichnis der Ticket-Dateien")
	webRoot := flag.String("web", filepath.Join(defaultRoot, "missioncontrol", "web"), "Verzeichnis der Oberfläche")
	kubectlPath := flag.String("kubectl", "kubectl", "Pfad zu kubectl")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig; leer = Standard des Benutzers")
	kubeContext := flag.String("context", "", "kubectl-Kontext; leer = der aktuelle")
	gitPath := flag.String("git", "git", "Pfad zu git")
	makePath := flag.String("make", "make", "Pfad zu make")
	enableDeploy := flag.Bool("deploy", true, "Deploy-Dienst anbieten; --deploy=false schaltet ihn ab")
	flag.Parse()

	if !strings.HasPrefix(*address, "127.0.0.1:") && !strings.HasPrefix(*address, "localhost:") {
		return fmt.Errorf("weigere mich, auf %q zu lauschen: Mission Control hat keine Anmeldung und bleibt auf dem Loopback", *address)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	store, err := tickets.NewStore(*ticketDir, map[string]string{
		tickets.ProjectMission:  version,
		tickets.ProjectHomeFlow: homeflowVersion(defaultRoot),
	})
	if err != nil {
		return err
	}
	// Der Deploy-Dienst ist die einzige schreibende Fähigkeit. Er lässt sich
	// abschalten, ohne dass der Rest des Cockpits davon berührt wird.
	var deployer *deploy.Runner
	if *enableDeploy {
		deployer = deploy.NewRunner(defaultRoot, *makePath, *kubectlPath)
		deployer.ProcessStarted = time.Now()
	}
	server := httpd.New(logger, store, cluster.KubectlRunner(*kubectlPath, *kubeconfig, *kubeContext),
		repo.GitRunner(*gitPath, defaultRoot), deployer, httpd.Config{
			Namespaces: splitAndTrim(*namespaces),
			Interval:   *interval,
			WebRoot:    *webRoot,
			Version:    version,
			RepoRoot:   defaultRoot,
		})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go server.Poll(ctx)

	httpServer := &http.Server{
		Addr:              *address,
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 * 1024,
	}
	errs := make(chan error, 1)
	go func() {
		logger.Info("mission control gestartet", "adresse", *address, "version", version,
			"namespaces", *namespaces, "tickets", *ticketDir)
		errs <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdown)
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// homeflowVersion reads HomeFlow's release from the Makefile rather than
// keeping a copy here. Mission Control has its own version; it only needs to
// *know* HomeFlow's to stamp HF tickets correctly.
func homeflowVersion(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, found := strings.CutPrefix(line, "VERSION ?= "); found {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

func splitAndTrim(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// repositoryRoot walks up from the binary's working directory looking for the
// repository marker, so the service works no matter where systemd starts it.
func repositoryRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}
