FROM golang:1.27-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app ./cmd/app

FROM alpine:3.24
RUN apk add --no-cache ca-certificates tzdata sqlite
RUN addgroup -S app && adduser -S app -G app
WORKDIR /app
COPY --from=builder /app ./
RUN mkdir -p /data && chown app:app /data
USER app
EXPOSE 8080
ENTRYPOINT ["./app"]
CMD ["serve"]
