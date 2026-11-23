# Build stage
FROM golang:1.23-alpine AS builder
RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /build

# Download dependencies first (better layer caching)
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
# Static binary for the host architecture (works on x86 and Apple Silicon)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /build/api-server .

# Runtime stage
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
RUN addgroup -g 1000 appuser && adduser -D -u 1000 -G appuser appuser
WORKDIR /app
COPY --from=builder /build/api-server /app/api-server
COPY application.yaml /app/application.yaml
# data/ holds users.json and the simulated meter keys (HSM)
RUN mkdir -p /app/data/meters && chown -R appuser:appuser /app/data
USER appuser
EXPOSE 3000
ENTRYPOINT ["/app/api-server"]
