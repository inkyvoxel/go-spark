## Email features

What is still not implemented:
- Blocking login or account access until email is verified.
- Admin/ops visibility for failed outbox rows.
- Separate worker process mode.
- HTML email styling beyond the simple message body.

## Account features

- Change email address securely

## Security

- Audit code

## HTMX adoption plan

Goal: add SPA-like interactivity while keeping server-rendered pages, progressive enhancement, and existing auth/security behavior.

### Principles
- Keep full-page SSR routes as canonical behavior.
- Add HTMX as progressive enhancement only; no JavaScript-only critical path.
- Prefer partial updates for forms and status blocks over whole-page swaps.
- Preserve CSRF, auth/anonymous route guards, rate limiting, and anti-enumeration messaging.
- Use PRG for full-page flow; allow fragment responses for `HX-Request`.

### Phase 1: account resend verification (low risk, high impact)
- Add a small status fragment on `/account` for resend-verification messages.
- Update `POST /account/resend-verification` handler:
  - For normal requests: keep existing redirect behavior.
  - For HTMX requests: return only the status/button fragment with success/error state.
- Update `account.html`:
  - Put resend section in a target container.
  - Add `hx-post`, `hx-target`, and `hx-swap` on resend button/form.
  - Keep normal `method/action` attributes for non-HTMX fallback.
- Tests:
  - Existing redirect tests stay passing.
  - Add HTMX request test asserting fragment response and status text.

### Phase 2: anonymous email forms (forgot/resend public)
- Apply same pattern to:
  - `POST /forgot-password`
  - `POST /resend-verification`
- For HTMX requests:
  - Return form fragment with inline errors or generic success/error status.
- For normal requests:
  - Keep current redirect/query-status behavior.
- Tests:
  - Verify both regular and HTMX branches.
  - Ensure generic messaging remains unchanged.

### Phase 3: login/register inline error UX
- Add HTMX enhancement to login and registration forms for inline validation and auth errors.
- Keep success transitions as standard redirects (login -> account, register -> account).
- For HTMX success, use response headers/redirect pattern that navigates to destination.
- Tests:
  - Invalid credentials/validation return form fragment with field errors.
  - Success still establishes session and navigates correctly.

### Phase 4: reset-password and account change-password forms
- Enhance password forms to render inline errors without full-page refresh.
- Keep security-sensitive success flows unchanged:
  - reset password -> login status message
  - account change password -> logout + login status message
- Tests:
  - HTMX invalid state fragment rendering.
  - Success behavior unchanged (session handling + redirects).

### Shared implementation tasks
- Add `isHXRequest(r *http.Request) bool` helper (`HX-Request: true`).
- Create partial templates for reusable form/status fragments to avoid duplicated HTML.
- Keep template field names and error keys identical to existing handlers.
- Add minimal loading affordances (`hx-indicator`) and button disabled state.

### Out of scope for initial rollout
- Full-page HTMX navigation (`hx-boost` everywhere).
- Client-side state stores or SPA routers.
- Realtime/push updates.

### Done criteria
- All enhanced forms work with and without HTMX.
- Existing security posture unchanged.
- Route test suite covers both full-page and HTMX fragment paths.
- No regression in redirect-based flows for non-HTMX clients.
