FROM golang:1.21-alpine

WORKDIR /app

# 1. Copy go.mod first
COPY go.mod ./

# 2. CRITICAL: Copy source code BEFORE downloading dependencies
# This ensures 'go mod tidy' can see the imports in main.go
COPY *.go ./

# 3. Resolve dependencies
RUN go mod tidy
RUN go mod download

# 4. Build
RUN go build -o /sentinel-engine

EXPOSE 8080
CMD [ "/sentinel-engine" ]