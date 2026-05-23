// Shared formatting / mapping helpers used across pages.

import { h } from "./dom.js";
import { t } from "./i18n.js";

// Each entry: id → { i18nKey, className }. We look up the label at render
// time so locale switching takes effect on the next paint.
const APPROVAL_STATUS = {
  0: { key: "status.approval.0", className: "badge--closed" },
  1: { key: "status.approval.1", className: "badge--timeout" },
  2: { key: "status.approval.2", className: "badge--rejected" },
  3: { key: "status.approval.3", className: "badge--pending" },
  4: { key: "status.approval.4", className: "badge--approved" },
};

export function approvalStatusBadge(value) {
  const meta = APPROVAL_STATUS[value];
  const label = meta ? t(meta.key) : String(value);
  const cls = meta ? meta.className : "";
  return h("span", { class: ["badge", cls] }, label);
}

// Maps Chinese status strings (the canonical, backend-supplied value) to
// localized label + badge class. Backend always sends Chinese; we translate
// for display only. This means the *value* compared in code stays "使用中",
// "暂停使用", "已停用" everywhere — only the user-facing text varies.
const USER_STATUS = {
  "使用中":   { key: "status.user.active",   className: "badge--approved" },
  "暂停使用": { key: "status.user.paused",   className: "badge--pending" },
  "已停用":   { key: "status.user.disabled", className: "badge--closed" },
};

export function userStatusBadge(value) {
  const meta = USER_STATUS[value];
  const label = meta ? t(meta.key) : String(value ?? "");
  const cls = meta ? meta.className : "";
  return h("span", { class: ["badge", cls] }, label);
}

export function fmtDateTime(value) {
  if (typeof value !== "string" || !value) return "";
  // Go 的 time.Time 零值序列化为 "0001-01-01T00:00:00Z"；MySQL/PG 的零值常见
  // 形式为 "0000-00-00 ..."。这两类都视为"未设置"，返回空串而不是把一串 0
  // 暴露给用户。
  if (value.startsWith("0001-01-01") || value.startsWith("0000-")) return "";
  const tIdx = value.indexOf("T");
  if (tIdx === -1) return value;
  return (value.slice(0, tIdx) + " " + value.slice(tIdx + 1).slice(0, 8));
}

// Password-rule list shown in form hints. Computed on-demand so the locale
// switch updates the strings on next render.
export const PASSWORD_RULES_KEYS = [
  "fmt.passwordRule.length",
  "fmt.passwordRule.case",
  "fmt.passwordRule.expire",
  "fmt.passwordRule.reuse",
];

export function passwordRules() {
  return PASSWORD_RULES_KEYS.map((k) => t(k));
}

/** Returns 0–4 score based on length + character classes. */
export function passwordStrength(pw) {
  if (!pw) return 0;
  let score = 0;
  if (pw.length >= 8) score++;
  if (/[a-z]/.test(pw) && /[A-Z]/.test(pw)) score++;
  if (/\d/.test(pw)) score++;
  if (/[^A-Za-z0-9]/.test(pw)) score++;
  return Math.min(4, score);
}

/**
 * validatePassword(pw) — null if it satisfies all enforceable rules,
 * otherwise a human-readable error message naming the first failing rule.
 * The 90-day expiry / no-reuse-of-last-3 rules are checked server-side.
 */
export function validatePassword(pw) {
  if (!pw) return t("fmt.password.empty");
  if (pw.length < 8) return t("fmt.password.short");
  if (!/[a-z]/.test(pw) || !/[A-Z]/.test(pw)) return t("fmt.password.case");
  if (!/\d/.test(pw)) return t("fmt.password.digit");
  if (!/[^A-Za-z0-9]/.test(pw)) return t("fmt.password.special");
  return null;
}

/** validateEmail(s) — null if format looks like a valid email, else a message. */
export function validateEmail(s) {
  const v = (s || "").trim();
  if (!v) return t("fmt.email.empty");
  if (v.length > 128) return t("fmt.email.long");
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v)) return t("fmt.email.format");
  return null;
}

/**
 * validatePhone(s) — accepts digits with optional + - space ( ) separators;
 * requires at least 7 digits. Returns null if valid, else a message.
 */
export function validatePhone(s) {
  const v = (s || "").trim();
  if (!v) return t("fmt.phone.empty");
  if (v.length > 32) return t("fmt.phone.long");
  if (!/^[\d\s\-+()]+$/.test(v)) return t("fmt.phone.invalid");
  const digits = (v.match(/\d/g) || []).length;
  if (digits < 7) return t("fmt.phone.short");
  return null;
}

/**
 * generatePassword(length) — produces a cryptographically random password
 * that satisfies validatePassword (>=8 chars, mixed case, digit, special).
 * Uses crypto.getRandomValues; default length 14.
 */
export function generatePassword(length = 14) {
  const lower   = "abcdefghijkmnpqrstuvwxyz"; // skip l, o for readability
  const upper   = "ABCDEFGHJKMNPQRSTUVWXYZ"; // skip I, L, O
  const digit   = "23456789";                // skip 0, 1
  const special = "!@#$%^&*-_+=";
  const all = lower + upper + digit + special;
  const len = Math.max(8, length | 0);

  const pick = (set) => set[randInt(set.length)];
  const required = [pick(lower), pick(upper), pick(digit), pick(special)];
  const rest = [];
  for (let i = required.length; i < len; i++) rest.push(pick(all));

  const arr = [...required, ...rest];
  for (let i = arr.length - 1; i > 0; i--) {
    const j = randInt(i + 1);
    [arr[i], arr[j]] = [arr[j], arr[i]];
  }
  return arr.join("");
}

function randInt(max) {
  if (max <= 0) return 0;
  // Use rejection sampling to avoid modulo bias on very small `max`.
  const limit = Math.floor(0xffffffff / max) * max;
  const buf = new Uint32Array(1);
  while (true) {
    crypto.getRandomValues(buf);
    if (buf[0] < limit) return buf[0] % max;
  }
}
