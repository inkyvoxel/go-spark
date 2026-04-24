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
* `CSRF_SIGNING_KEY=<long-random-secret>`

Strongly recommended:

* `EMAIL_PROVIDER=smtp`, and configure `SMTP_*` settings
* `AUTH_EMAIL_VERIFICATION_REQUIRED=true`
* `EMAIL_LOG_BODY=false`
* set a real sender address in `EMAIL_FROM`

Also set your production database path (for example `DATABASE_PATH=/var/lib/go-spark/app.db`) and SMTP settings when SMTP is enabled.

## 3. Run migrations before first start

```sh
DATABASE_PATH=/var/lib/go-spark/app.db ./bin/app migrate up
```

Migrations are embedded into the binary, so this command does not depend on a local `migrations/` directory.

## 4. Run as a systemd service

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

## 5. Put Caddy in front

Minimal Caddy config:

```caddy
your-domain.example {
	reverse_proxy 127.0.0.1:8080
}
```

Keep app listen address on loopback (for example `APP_ADDR=127.0.0.1:8080`) so only Caddy is internet-facing.
