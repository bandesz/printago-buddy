# ---- build stage --------------------------------------------------------
FROM golang:1.26-alpine AS builder

WORKDIR /build

# Cache dependency downloads separately from source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Produce a fully static binary; -trimpath removes local path info.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /printago-buddy ./cmd/printago-buddy

# ---- runtime stage ------------------------------------------------------
FROM alpine:3

# CA certificates are required for outbound HTTPS calls to api.printago.io.
RUN apk --no-cache add ca-certificates

# Run as a dedicated non-root user.
RUN addgroup -S app && adduser -S app -G app
USER app

COPY --from=builder /printago-buddy /printago-buddy

ENTRYPOINT ["/printago-buddy"]
