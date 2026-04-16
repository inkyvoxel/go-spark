## Email features

What is still not implemented:
- Blocking login or account access until email is verified.
- UI text after registration telling the user to check their email.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.
- Secure forgotten password flow
- Visiting an expire verification link, when signed in, displays a 'Email link expired. This confirmation link is invalid or has expired.' and a 'Sign in' link. If they are already signed in, display the "Go to your account" link instead.

## Account features

- Change email address securely
- Change password securely

## Security

- Rate limiting: on login, on register, on resend verification email, on 'forgotten password' (once implemented), etc.
- Review use of bcrypt. It seems to be `2a`. Should it be `2b`? Should we switch to Argon2?
- Review email verification token. Is SHA-256 OK?
- Audit code
