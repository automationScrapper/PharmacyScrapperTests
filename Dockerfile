# Multi-stage build: Go server + Playwright runtime for Render

# 1) Build Go server
FROM golang:1.21-bookworm AS go-builder
WORKDIR /src

# Cache modules first
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags='-s -w' -o /out/server ./cmd/server


# 2) Install Node deps (including Playwright JS lib)
FROM node:20-bookworm AS node-deps
WORKDIR /app/automation
COPY automation/package*.json ./
# Install devDependencies too (playwright is a devDependency here)
RUN npm ci --no-audit --no-fund


# 3) Final runtime with Playwright browsers preinstalled
# Use a Playwright base matching the version in automation/package-lock.json (1.56.1)
FROM mcr.microsoft.com/playwright:v1.56.1-jammy
WORKDIR /app

# Reuse preinstalled browsers in this image
ENV PLAYWRIGHT_BROWSERS_PATH=/ms-playwright \
    NODE_ENV=production \
    PORT=8080

# Create runtime dirs and ensure permissions for non-root user
RUN mkdir -p /app/automation/downloads /app/automation/data && \
    chown -R pwuser:pwuser /app

# Copy server binary
COPY --from=go-builder /out/server /app/server

# Copy Playwright project files and node_modules
COPY automation /app/automation
COPY --from=node-deps /app/automation/node_modules /app/automation/node_modules

# Optional: avoid unnecessary root privileges
USER pwuser

EXPOSE 8080

CMD ["/app/server"]
