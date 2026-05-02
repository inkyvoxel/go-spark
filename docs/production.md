# Production Deployment

This guide shows a practical production path for this starter:

* build one Go binary
* run it on Ubuntu Server
* put Caddy in front for HTTPS and reverse proxying

You can deploy differently if your environment needs it. This is a default pattern, not a requirement.

## 1. Build a production binary

From the project root:

```sh
make build-prod
```

Default output:

* `./bin/app`

Override output path or target platform when needed:

```sh
make build-prod PROD_BIN=./bin/myapp PROD_GOOS=linux PROD_GOARCH=arm64
```

## 2. Configure production environment

Create a production env file (for example `/etc/go-spark/go-spark.env`) and set at least:

* `APP_ENV=production`
* `APP_COOKIE_SECURE=true`
* `APP_BASE_URL=https://your-domain.example`
* `AUTH_PASSWORD_PEPPER=<long-random-secret>`
* `SECRET_KEY_BASE=<long-random-secret>` — root signing key; generate with `openssl rand -hex 32`

Strongly recommended:

* `EMAIL_PROVIDER=smtp`, and configure `SMTP_*` settings
* `AUTH_EMAIL_VERIFICATION_REQUIRED=true`
* `EMAIL_LOG_BODY=false`
* `LOG_FORMAT=json` for structured production log ingestion
* set a real sender address in `EMAIL_FROM`

Also set your production database path (for example `DATABASE_PATH=/var/lib/go-spark/app.db`) and SMTP settings when SMTP is enabled.

## 3. Health Endpoints

The app exposes two plain-text health endpoints intended for load balancers and orchestrators:

* `GET /healthz` returns `200 OK` with body `ok`
* `GET /readyz` returns `200 OK` with body `ok` when the app is ready to serve traffic
* `GET /readyz` returns `503 Service Unavailable` with body `not ready` when the app is not ready

Responses are intentionally minimal and do not include internal details such as database metadata, migration versions, environment values, hostnames, queue state, or build identifiers.

## 4. Run migrations before first start

```sh
DATABASE_PATH=/var/lib/go-spark/app.db ./bin/app migrate up
```

Migrations are embedded into the binary, so this command does not depend on a local `migrations/` directory.

## 5. Run as a systemd service

Example `/etc/systemd/system/go-spark.service` using single-process `all` mode:

```ini
[Unit]
Description=Go Spark App
After=network.target

[Service]
Type=simple
User=gospark
Group=gospark
WorkingDirectory=/opt/go-spark
EnvironmentFile=/etc/go-spark/go-spark.env
ExecStart=/opt/go-spark/bin/app all
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Single-process `all` mode is the default recommendation for this starter. If you need stricter isolation, you can run separate `serve` and `worker` services.

## 6. Configure trusted proxy IPs

If you run the app behind a reverse proxy, set `TRUSTED_PROXY_IPS` to the proxy's IP address (or a CIDR range). This tells the app to read the real client IP from the `X-Real-IP` or `X-Forwarded-For` header instead of using `RemoteAddr`, which would always be the proxy's IP.

Without this, all rate limiting collapses to a single bucket and stops working correctly.

```sh
# Single proxy on loopback
TRUSTED_PROXY_IPS=127.0.0.1

# Multiple proxies or a subnet
TRUSTED_PROXY_IPS=10.0.0.1,172.16.0.0/12
```

Only list IPs you control. Any request arriving from a trusted proxy address will have its IP overridden by the header value, so listing untrusted addresses would allow clients to spoof their IP and bypass rate limiting.

## 7. Put Caddy in front

Minimal Caddy config:

```caddy
your-domain.example {
	reverse_proxy 127.0.0.1:8080
}
```

Keep app listen address on loopback (for example `APP_ADDR=127.0.0.1:8080`) so only Caddy is internet-facing.
