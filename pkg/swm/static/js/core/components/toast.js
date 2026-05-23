// Toast stack — single instance, supports stacking, auto-dismiss, manual close.

import { h, qs } from "../dom.js";
import { t } from "../i18n.js";

const DEFAULT_TIMEOUT = 20000;       // 20s before auto-dismiss
const LEAVE_ANIM_MS = 240;           // matches toastOut keyframe (--motion-base)

function ensureStack() {
  let stack = qs(".toast-stack");
  if (stack) return stack;
  stack = h("div", { class: "toast-stack", role: "status", "aria-live": "polite" });
  document.body.appendChild(stack);
  return stack;
}

function show(kind, message, opts = {}) {
  const stack = ensureStack();
  const el = h("div", { class: ["toast", `toast--${kind}`], role: "alert" }, [
    h("span", { class: "toast__msg" }, String(message ?? "")),
    h("button", {
      class: "toast__close",
      type: "button",
      "aria-label": t("app.aria.close"),
      onclick: (e) => { e.stopPropagation(); dismiss(el); },
    }, "×"),
  ]);
  stack.appendChild(el);

  const timeout = opts.timeout ?? DEFAULT_TIMEOUT;
  if (timeout > 0) {
    el._autoTimer = setTimeout(() => dismiss(el), timeout);
  }
  return el;
}

function dismiss(el) {
  if (!el || !el.isConnected || el.classList.contains("is-leaving")) return;
  if (el._autoTimer) {
    clearTimeout(el._autoTimer);
    el._autoTimer = null;
  }
  el.classList.add("is-leaving");
  // animationend is the happy path; setTimeout is a hard fallback in case
  // reduced-motion zeroes the duration or the browser skips the event.
  setTimeout(() => { if (el.isConnected) el.remove(); }, LEAVE_ANIM_MS);
}

export const toast = {
  success: (msg, opts) => show("success", msg, opts),
  error:   (msg, opts) => show("error",   msg, opts),
  info:    (msg, opts) => show("info",    msg, opts),
  warning: (msg, opts) => show("warning", msg, opts),
};
