// Thin fetch wrapper — adds CSRF, JSON encoding, error normalization,
// loading bar tick, and a global 401-redirect.

import { getCsrfToken } from "./csrf.js";
import { startLoading, stopLoading } from "./loading.js";
import { t } from "./i18n.js";

const SAFE_METHODS = new Set(["GET", "HEAD", "OPTIONS"]);

class ApiError extends Error {
  constructor(message, status, body) {
    super(message);
    this.status = status;
    this.body = body;
  }
}

async function request(method, path, { body, signal, query } = {}) {
  const url = query ? `${path}?${new URLSearchParams(query)}` : path;
  const headers = { Accept: "application/json", "X-Requested-With": "XMLHttpRequest" };

  let payload;
  if (body !== undefined && body !== null) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  if (!SAFE_METHODS.has(method)) {
    headers["X-CSRF-Token"] = getCsrfToken();
  }

  startLoading();
  try {
    const res = await fetch(url, {
      method,
      headers,
      body: payload,
      credentials: "same-origin",
      signal,
    });

    if (res.status === 401) {
      window.location.href = "/login";
      throw new ApiError(t("app.session.lost"), 401, null);
    }

    const text = await res.text();
    let data = null;
    if (text) {
      try { data = JSON.parse(text); }
      catch { data = { raw: text }; }
    }

    if (!res.ok) {
      const msg = (data && (data.msg || data.message)) || `HTTP ${res.status}`;
      throw new ApiError(msg, res.status, data);
    }
    return data;
  } finally {
    stopLoading();
  }
}

export const api = {
  get:  (path, opts) => request("GET",    path, opts),
  post: (path, body, opts) => request("POST",   path, { ...opts, body }),
  put:  (path, body, opts) => request("PUT",    path, { ...opts, body }),
  del:  (path, body, opts) => request("DELETE", path, { ...opts, body }),
};

export { ApiError };
