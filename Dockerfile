FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /datey ./main.go

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata su-exec && \
    adduser -D -u 1000 datey
WORKDIR /app
COPY --from=builder /datey .
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENV DATA_DIR=/db
RUN mkdir -p /db && chown datey:datey /db
EXPOSE 6270
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:6270/health || exit 1
ENTRYPOINT ["/entrypoint.sh"]
