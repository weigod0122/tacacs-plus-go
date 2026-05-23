// Session heartbeat — checks /check-session on user activity, with backoff
// when the tab is hidden. Far cheaper than a fixed 60s interval.

import { api } from "./api.js";
import { toast } from "./components/toast.js";
import { t } from "./i18n.js";

const ACTIVE_INTERVAL = 90 * 1000;       // every 90s while user is active
const HIDDEN_INTERVAL = 5 * 60 * 1000;   // every 5min while hidden
const ACTIVITY_DEBOUNCE = 5 * 1000;

let lastCheck = 0;
let lastActivity = Date.now();
let timer;

function schedule() {
  clearTimeout(timer);
  const interval = document.hidden ? HIDDEN_INTERVAL : ACTIVE_INTERVAL;
  timer = setTimeout(tick, interval);
}

async function tick() {
  if (document.hidden && Date.now() - lastActivity > 30 * 60 * 1000) {
    schedule();
    return;
  }
  try {
    const res = await api.get("/check-session");
    lastCheck = Date.now();
    if (res && res.expired) {
      toast.warning(t("app.session.expired"));
      setTimeout(() => { window.location.href = "/login"; }, 1200);
      return;
    }
  } catch {
    // ignored — api.js already handles 401
  }
  schedule();
}

function onActivity() {
  const now = Date.now();
  if (now - lastActivity < ACTIVITY_DEBOUNCE) return;
  lastActivity = now;
  if (now - lastCheck > ACTIVE_INTERVAL) tick();
}

export function startSessionWatch() {
  ["mousedown", "keydown", "touchstart"].forEach((evt) =>
    document.addEventListener(evt, onActivity, { passive: true })
  );
  document.addEventListener("visibilitychange", () => {
    if (!document.hidden) onActivity();
    schedule();
  });
  schedule();
}
