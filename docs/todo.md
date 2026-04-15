## Email feature

What is still not implemented:
- A public resend confirmation flow for users who are not signed in.
- Blocking login or account access until email is verified.
- UI text after registration telling the user to check their email.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

## Security

- Review use of bcrypt. It seems to be `2a`. Should it be `2b`? Should we switch to Argon2?
- Review email verification token. Is SHA-256 OK?
- Audit code
