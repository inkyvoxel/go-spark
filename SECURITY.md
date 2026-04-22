# Security Policy

## Reporting a Security Issue

If you find a security issue in this starter template, please open a GitHub issue.

Do not include secrets, private keys, session tokens, production database files, or sensitive user data in the issue. If a report requires sharing sensitive details, open a brief issue first and ask the maintainer how they would like to receive the details.

## Supported Versions

This project is a starter template. Security fixes are expected to land on the main branch. Projects cloned from this template should review and apply relevant fixes manually.

## Security Scope

This starter includes basic server-rendered authentication, sessions, CSRF protection, and SQLite persistence. It is intended as a foundation, not a complete production security program.

Before deploying a project based on this template:

* Serve the application over HTTPS.
* Set `APP_COOKIE_SECURE=true` when TLS is terminated before the Go process.
* Keep secrets and database files out of Git.
* Review deployment-specific headers, logging, backups, and access controls.
* Keep Go modules and browser assets up to date.
* Review and tune auth rate-limit settings (`RATE_LIMIT_*`) for your traffic profile.
* Set `AUTH_PASSWORD_PEPPER` in production and treat it as a secret outside the database.
