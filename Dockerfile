FROM golang:1.26-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -o /datey .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates tzdata && \
    adduser -D -u 1000 datey
WORKDIR /app
COPY --from=builder /datey .
RUN mkdir -p /data && chown datey:datey /data
USER datey
EXPOSE 6270
HEALTHCHECK --interval=30s --timeout=3s CMD wget -qO- http://localhost:6270/health || exit 1
ENTRYPOINT ["/app/datey"]
