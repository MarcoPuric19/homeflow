# syntax=docker/dockerfile:1
FROM node:22-alpine AS web
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY backend/go.mod backend/go.sum* ./
RUN go mod download
COPY backend/ ./
ARG TARGETOS=linux
ARG TARGETARCH
# VERSION kommt aus der Makefile. Ohne diesen Wert meldet die Anwendung
# "unbekannt" - was ehrlich ist, aber die Frage "was laeuft hier eigentlich"
# unbeantwortet laesst (HF-0105).
ARG VERSION=unbekannt
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/homeflow ./cmd/homeflow

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/homeflow /homeflow
COPY --from=web --chown=65532:65532 /src/dist /web
ENV HOMEFLOW_WEB_ROOT=/web
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/homeflow"]
