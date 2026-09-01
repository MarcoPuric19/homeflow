# Eigenständige Ziele für Mission Control.
#
# Seit 2026-09-01 gibt es dieselben Ziele auch in der Wurzel-Makefile als
# `mission-control-*` (und `mission-control-ci` läuft in `make ci` mit). Diese
# Datei bleibt die Quelle: die Wurzel ruft nur `$(MAKE) -C missioncontrol` auf,
# damit es keine zwei Wahrheiten gibt.
SHELL := /bin/sh
REPO := $(CURDIR)/..
GO_CACHE := $(REPO)/.cache/go-build
GO_MOD_CACHE := $(REPO)/.cache/go-mod
GO := GOCACHE=$(GO_CACHE) GOMODCACHE=$(GO_MOD_CACHE) go

.DEFAULT_GOAL := help
.PHONY: help fmt fmt-check lint test build run ci install status logs stop clean

help: ## Zeigt die verfügbaren Ziele
	@awk 'BEGIN {FS = ":.*## "; printf "Usage: make <target>\n\n"} /^[a-zA-Z_-]+:.*## / {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

fmt: ## Formatiert den Go-Code
	$(GO)fmt -w .

fmt-check: ## Meldet Dateien, die gofmt umschreiben würde
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then echo "gofmt würde umschreiben:" >&2; echo "$$files" >&2; exit 1; fi; \
	echo "gofmt: nichts zu tun"

lint: ## Statische Analyse
	$(GO) vet ./...

test: ## Unit-Tests
	$(GO) test -race -cover ./...

build: ## Baut das Binary nach bin/missioncontrol
	$(GO) build -trimpath -o bin/missioncontrol ./cmd/missioncontrol

run: build ## Startet das Cockpit im Vordergrund
	./bin/missioncontrol -tickets $(CURDIR)/tickets -web $(CURDIR)/web

ci: fmt-check lint test build ## Alles, was vor einem Commit laufen sollte
	@echo "mission control: alle Prüfungen bestanden"

install: ## Richtet den systemd-Benutzerdienst ein (Autostart)
	./install.sh

status: ## Zeigt den Dienststatus
	systemctl --user status mission-control.service --no-pager || true

logs: ## Folgt dem Dienstprotokoll
	journalctl --user -u mission-control.service -f

stop: ## Stoppt den Dienst und entfernt den Autostart
	systemctl --user disable --now mission-control.service

clean: ## Entfernt das Binary
	rm -f bin/missioncontrol
