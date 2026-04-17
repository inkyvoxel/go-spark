# Email Templates

This directory contains all transactional email templates for the app.

Each email uses three files:

- `<name>.subject.txt` for the subject line
- `<name>.text.txt` for the plain-text body
- `<name>.html.tmpl` for the HTML body

## Available emails

- `account_confirmation`
  - Variables: `{{ .ConfirmationURL }}`
- `password_reset`
  - Variables: `{{ .ResetURL }}`
