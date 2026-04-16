## Email features

What is still not implemented:
- Blocking login or account access until email is verified.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

## Account features

- Change email address securely

## Security

- Consider adding an application-level pepper stored outside the database for defense in depth.
- Review email verification token. Is SHA-256 OK?
- Audit code
