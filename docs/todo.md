# To-do

1. Add 'delete account' button for authenticated users
2. Add 'flash messages' via session - currently status messages are passed as query params (e.g. ?status=password-changed), which leaks state into the URL and breaks on refresh. Storing flash messages in the session is the idiomatic fix
3. Two-factor authentication (TOTP)