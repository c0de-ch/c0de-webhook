FROM golang:1.25-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -ldflags="-s -w" -o c0de-webhook .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 webhook
WORKDIR /app
COPY --from=builder /build/c0de-webhook .
RUN mkdir -p /data && chown webhook:webhook /data
USER webhook
VOLUME /data
EXPOSE 8090
ENTRYPOINT ["./c0de-webhook"]
CMD ["-config", "/etc/c0de-webhook/config.yaml"]
