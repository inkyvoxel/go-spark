## Email features

What is still not implemented:
- Blocking login or account access until email is verified.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

## Account features

- Change email address securely

## Security

- Review use of bcrypt. It seems to be `2a`. Should it be `2b`? Should we switch to Argon2?
- Review email verification token. Is SHA-256 OK?
- Audit code
