# Stage 1: build
FROM golang:1.26 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO off: the SQLite driver (modernc.org/sqlite) is pure Go, so the binary
# is fully static and portable into the slim runtime image.
RUN CGO_ENABLED=0 go build -o mdi ./cmd/medical-device-intelligence-pp-cli

# Stage 2: minimal runtime
FROM debian:stable-slim
# CA certificates for the HTTPS API calls (openFDA, ClinicalTrials.gov, PubMed).
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /app/mdi .
# Render provides the PORT env variable; the serve command reads it.
CMD ["./mdi", "serve"]
