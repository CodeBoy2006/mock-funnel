# Build stage
FROM golang:1.22.9-alpine AS builder

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mock-funnel ./cmd/mock-funnel

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/mock-funnel .

# Expose port
EXPOSE 8080

# Set environment variables
ENV ADDR=:8080

# Run the application
CMD ["./mock-funnel"]
