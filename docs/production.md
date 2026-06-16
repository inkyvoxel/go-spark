# Production Deployment

This guide covers deploying with Docker Compose and Caddy. Caddy obtains and renews TLS certificates automatically via Let's Encrypt — no manual certificate management needed.

## Prerequisites

* Docker and Docker Compose v2 on your server
* A domain name with an A record pointing to your server's public IP
* SMTP credentials for transactional email

## 1. Configure your environment

Copy the example env file and fill in the required values:

```sh
cp .env.example .env
```

Required for deployment:

| Variable | Description |
|---|---|
| `DOMAIN` | Your public domain, e.g. `yourdomain.example` |
| `SECRET_KEY_BASE` | Root signing key — generate with `openssl rand -hex 32` |
| `AUTH_TOTP_KEY` | Root key for TOTP data at rest — generate with `openssl rand -hex 32` |
| `AUTH_PASSWORD_PEPPER` | Password hashing pepper — generate with `openssl rand -hex 32` |
| `AUTH_TOTP_ISSUER` | Issuer name shown in authenticator apps — set to your app's name |
| `AUTH_PASSKEY_RP_ID` | Passkey relying party ID — optional; defaults to the host of `APP_BASE_URL`. Set to the registrable parent domain for multi-subdomain setups |
| `AUTH_PASSKEY_RP_DISPLAY_NAME` | Passkey name shown in some browser/OS prompts — set to your app's name |
| `EMAIL_FROM` | Sender address, e.g. `"My App <hello@yourdomain.example>"` |
| `SMTP_HOST` | SMTP server hostname |
| `SMTP_PORT` | SMTP port (default: `587`) |
| `SMTP_USERNAME` | SMTP username (leave blank if unauthenticated) |
| `SMTP_PASSWORD` | SMTP password (leave blank if unauthenticated) |

`DOMAIN` is used by both the Caddyfile (to obtain a TLS certificate) and the compose file (to set `APP_BASE_URL`). You only need to set it once.

Keep your `.env` outside version control. The `.gitignore` already excludes it.

## 2. Start the app

```sh
docker compose up -d
```

To follow logs:

```sh
docker compose logs -f
```

## How it works

Four services start on `docker compose up`:

| Service | Role |
|---|---|
| `migrate` | Runs `migrate up`, then exits. `app` and `worker` wait for it to succeed. |
| `app` | HTTP server (`serve` mode). Handles web traffic only. |
| `worker` | Background jobs (`worker` mode). Email delivery and cleanup run here. |
| `caddy` | Reverse proxy. Terminates TLS and forwards traffic to `app`. |

`app` and `worker` run as separate containers so a job failure does not take down the web server, and each can be restarted independently. Both mount the `app-data` volume at `/data` and share the same SQLite database file. SQLite WAL mode is enabled, which allows concurrent reads and writes from both processes without lock contention under normal load.

Caddy appends the connecting address to `X-Forwarded-For` on requests it forwards. The compose file configures `TRUSTED_PROXY_IPS=172.16.0.0/12` for `app`, which covers the Docker bridge network range and tells the app to derive the real client IP from that header. The app walks the header from right to left and uses the first hop that is not a trusted proxy, so client-supplied entries (and headers like `X-Real-IP`, which Caddy does not strip) are never trusted. Rate limiting uses the real client IP as a result.

## 3. Pre-launch checklist

Run through this before accepting real traffic.

**Infrastructure**
- [ ] DNS A record for `DOMAIN` resolves to the server's public IP
- [ ] Ports 80 and 443 are open in the server firewall
- [ ] `https://DOMAIN` loads over HTTPS (Caddy obtained a certificate)

**Configuration**
- [ ] `docker compose logs app` shows no startup warnings — check for insecure cookies, non-HTTPS base URL, log-only email, default sender
- [ ] `AUTH_TOTP_ISSUER` is set to your app name — this value is baked into QR codes when users enrol 2FA; changing it later breaks existing authenticator apps
- [ ] `APP_BASE_URL` uses your real HTTPS domain — passkeys are bound to this origin, so existing passkeys stop working if the domain (relying party ID) changes
- [ ] `SECRET_KEY_BASE`, `AUTH_TOTP_KEY`, and `AUTH_PASSWORD_PEPPER` are not committed to version control

**Email**
- [ ] SPF, DKIM, and DMARC records are in DNS (see [SMTP deliverability](#6-smtp-deliverability))
- [ ] A test registration email arrives in the inbox, not spam

**Backups**
- [ ] A backup has been taken and a restore has been completed successfully on a test copy (see [Backups](#5-backups))

## 4. Health endpoints

The app exposes two plain-text health endpoints:

* `GET /healthz` — returns `200 OK` with body `ok`
* `GET /readyz` — returns `200 OK` / `ok` when ready, `503` / `not ready` otherwise

Responses do not include internal details such as database state, migration versions, or build identifiers.

## 5. Backups

The `app-data` Docker volume contains the SQLite database and its WAL sidecar files (`app.db`, `app.db-wal`, `app.db-shm`). Do not copy `app.db` directly while the database is running — you may capture an inconsistent state mid-write. Use the SQLite online backup API instead, which checkpoints the WAL and produces a clean, self-contained snapshot without stopping the app.

The Docker image includes `sqlite3` for this purpose.

**Taking a backup:**

```sh
BACKUP="backups/app-$(date +%Y%m%d-%H%M%S).db"
mkdir -p backups
docker compose exec app sqlite3 /data/app.db ".backup /data/backup.db"
docker compose cp app:/data/backup.db "$BACKUP"
docker compose exec app rm /data/backup.db
```

The resulting file is a portable SQLite database with no WAL dependency. Copy it off the server and verify it opens cleanly:

```sh
sqlite3 "$BACKUP" "PRAGMA integrity_check"
```

**Restoring from a backup:**

```sh
docker compose stop app worker
docker compose cp "$BACKUP" app:/data/app.db
# Remove any stale WAL files from the previous session
docker compose run --rm --no-deps app sh -c "rm -f /data/app.db-wal /data/app.db-shm"
docker compose start app worker
```

**Practice restores.** A backup you have never restored is a backup of unknown quality. Run through the restore procedure on a copy of your data periodically — before an incident, not during one.

A future version of this template will add Litestream for continuous, automated SQLite replication.

## 6. SMTP deliverability

The app sends transactional email for registration, password reset, and email change. Poor deliverability means these land in spam or bounce silently.

**Use a dedicated transactional email provider.** Self-hosted SMTP (Postfix, etc.) is hard to deliver reliably. Prefer a provider with a reputation system: Postmark, Resend, SendGrid, AWS SES, and similar services all work. Configure `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, and `SMTP_PASSWORD` in your `.env`.

**SMTP transport notes.** The sender connects in plaintext and upgrades with STARTTLS when `SMTP_TLS=true` (the usual setup on port 587). Implicit TLS (port 465) and auth mechanisms other than `PLAIN` are not supported; every major transactional provider supports STARTTLS with `PLAIN` auth on 587.

**Set DNS records.** Your email provider's dashboard will supply the values.

| Record | Purpose |
|---|---|
| SPF | Authorises your provider's servers to send on behalf of your domain |
| DKIM | Cryptographic signature that proves the message was not tampered with |
| DMARC | Policy that tells receiving servers what to do with messages that fail SPF/DKIM |

A minimal DMARC record to start (monitoring only, no rejection):

```
v=DMARC1; p=none; rua=mailto:dmarc@yourdomain.example
```

Tighten to `p=quarantine` or `p=reject` once you are confident all legitimate senders are covered.

**`EMAIL_FROM` must match your domain.** Sending from `hello@yourdomain.example` when your SPF and DKIM records cover `yourdomain.example` is consistent. Mismatches cause deliverability failures.

**Test before launch.** Send a registration email and check it with [mail-tester.com](https://www.mail-tester.com) or a similar inbox placement tool. Confirm SPF, DKIM, and DMARC all pass before accepting real users.

## 7. Secret rotation

Keys are split across two roots by rotation cost: `SECRET_KEY_BASE` signs ephemeral state and is cheap to rotate, while `AUTH_TOTP_KEY` and `AUTH_PASSWORD_PEPPER` protect data at rest and are expensive to rotate. See [Secret Key Derivation](components.md#secret-key-derivation) for the full map.

**`SECRET_KEY_BASE`** is the root for signing and ephemeral state: CSRF tokens, flash cookies, and the TOTP/passkey ceremony cookies. Rotating it:

* does **not** sign users out — session tokens are random values stored as unkeyed hashes, independent of this key
* does **not** affect 2FA — TOTP secrets and backup codes derive from `AUTH_TOTP_KEY`, not this one
* invalidates any in-flight CSRF tokens — forms submitted at the moment of rotation will fail once with a CSRF error, then succeed on retry, and any half-finished TOTP/passkey ceremony must be restarted

It is safe to rotate at any time if you suspect it is compromised. Generate a new value, update `.env`, and restart:

```sh
openssl rand -hex 32   # generate new value, put it in .env
docker compose up -d
```

**`AUTH_TOTP_KEY`** protects TOTP data at rest — it encrypts the shared secret and hashes backup codes. Rotating it breaks both factors for every user with 2FA enabled: stored secrets become undecryptable *and* stored backup-code hashes no longer match. Those users cannot complete the second-factor step and will need an out-of-band 2FA reset.

Only rotate this if you have confirmed it was compromised (e.g. a database backup leaked). There is no graceful migration path without forcing re-enrolment. If you do need to rotate:

1. Generate a new value (`openssl rand -hex 32`) and update `.env`
2. Restart the app
3. Reset 2FA for affected users and notify them to re-enrol

**`AUTH_PASSWORD_PEPPER`** is prepended to every password before Argon2id hashing. Rotating it means every stored hash no longer matches any user's real password — all users are effectively locked out until they reset their password via email.

Only rotate this if you have confirmed the pepper was compromised. There is no graceful migration path without a user-facing password reset campaign. If you do need to rotate:

1. Generate a new pepper and update `.env`
2. Restart the app
3. Notify users that they must reset their password

**SMTP credentials** are independent of the above. Rotate them by updating `SMTP_USERNAME` and `SMTP_PASSWORD` in `.env` and restarting only the worker:

```sh
docker compose up -d worker
```

## 8. Upgrading

To deploy updated application code:

```sh
docker compose build
docker compose up -d
```

`migrate` always runs on start and is idempotent, so new migrations are applied automatically.

To update the Caddy image:

```sh
docker compose pull caddy
docker compose up -d caddy
```

## 9. Scaling beyond a single server

This template is designed for single-server deployment. SQLite is the database, and the rate limiter is an in-memory counter — both are scoped to one machine. These two constraints are coupled: if you need to run the web process across multiple servers, you need to address both at the same time.

| Concern | Single server | Multiple servers |
|---|---|---|
| Database | SQLite (file on disk) | Postgres or similar |
| Rate limiting | In-memory (per process) | Shared store (e.g. Redis) |

**Rate limiter.** The `rateLimitStore` interface in `internal/server` is the intended extension point. Swap in a shared-store implementation via `Options.RateLimiter` if you need limits that hold across instances.

**Database.** Migrating away from SQLite means replacing `internal/platform/sqlite`, the migration setup in `cmd/app/main.go`, and regenerating queries with a different sqlc driver. That is a deliberate rearchitecture step, not a config change.

For most apps this template targets — a single VPS, low-to-moderate traffic, straightforward ops — single-server SQLite is a good fit and these limits are not reached in practice.
