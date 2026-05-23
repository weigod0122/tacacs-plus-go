// CSRF helper: read the csrf_token cookie set by the server.
// The cookie is intentionally non-HttpOnly so the SPA can echo it back as
// X-CSRF-Token, satisfying double-submit verification.

export function getCsrfToken() {
  const match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/);
  return match ? decodeURIComponent(match[1]) : "";
}
