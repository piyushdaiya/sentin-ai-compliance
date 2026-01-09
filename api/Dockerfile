FROM golang:1.21-alpine

WORKDIR /app

# 1. Copy go.mod first
COPY go.mod ./

# 2. Copy the source code (CRITICAL FIX: This was missing before the tidy step)
COPY *.go ./

# 3. Now run tidy. It will scan main.go, find the missing libs, and download them.
RUN go mod tidy

# 4. Build
RUN go build -o /sentinel-engine

EXPOSE 8080
CMD [ "/sentinel-engine" ]