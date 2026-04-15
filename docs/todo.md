## Email feature

What is still not implemented:
- Real SMTP/provider sending. Current sender logs delivery attempts only, which is right for development.
- A resend confirmation email flow.
- Blocking login or account access until email is verified.
- UI text after registration telling the user to check their email.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

The email feature is complete for the planned log-sender account-confirmation slice, but not “production email delivery” yet. The next practical slice would be either SMTP sender or a small UX polish slice: show “check your email” after registration and display verification status on /account.

## Security

- Review use of bcrypt. It seems to be `2a`. Should it be `2b`? Should we switch to Argon2?
- Review email verification token. Is SHA-256 OK?
- Audit code
