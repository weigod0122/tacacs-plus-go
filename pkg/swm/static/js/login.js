// Login page logic — tabs, password strength, signup submission via fetch.

import { qs, qsa } from "./core/dom.js";
import { passwordStrength, validatePassword, validateEmail, validatePhone, validateUsername } from "./core/format.js";
import { toast } from "./core/components/toast.js";
import { t, toggleLocale, onLocaleChange, getLocale } from "./core/i18n.js";

const tabLogin = qs("#tab-login");
const tabSignup = qs("#tab-signup");
const formLogin = qs("#loginForm");
const formSignup = qs("#signupForm");
const langBtn = qs("#langBtn");

// If the server flagged this render as a rate-limited response, surface it
// as a more attention-grabbing toast on top of the inline error banner.
const rateLimitMsg = document.body.dataset.rateLimit;
if (rateLimitMsg) {
  toast.warning(rateLimitMsg, { timeout: 30000 });
}

function applyTranslations() {
  // Generic data-i18n bindings for static markup.
  qsa("[data-i18n]").forEach((el) => {
    const key = el.dataset.i18n;
    if (!key) return;
    const attr = el.dataset.i18nAttr;
    const value = t(key);
    if (attr) {
      el.setAttribute(attr, value);
    } else {
      el.textContent = value;
    }
  });
  document.title = t("login.title");
  if (langBtn) {
    langBtn.textContent = t("app.langToggle");
    langBtn.title = t("app.langToggle.title");
  }
}

if (langBtn) {
  langBtn.addEventListener("click", () => toggleLocale());
}
onLocaleChange(applyTranslations);
applyTranslations();

function showTab(name) {
  const isLogin = name === "login";
  tabLogin.classList.toggle("is-active", isLogin);
  tabSignup.classList.toggle("is-active", !isLogin);
  tabLogin.setAttribute("aria-selected", String(isLogin));
  tabSignup.setAttribute("aria-selected", String(!isLogin));
  formLogin.classList.toggle("is-active", isLogin);
  formSignup.classList.toggle("is-active", !isLogin);
  setTimeout(() => {
    const focus = (isLogin ? formLogin : formSignup).querySelector("input");
    if (focus) focus.focus();
  }, 30);
}

tabLogin.addEventListener("click", () => showTab("login"));
tabSignup.addEventListener("click", () => showTab("signup"));

const pwInput = qs("#signup-password");
const bars = qsa(".password-strength__bar");
const ruleEls = qsa("#ruleList li[data-rule]");

pwInput.addEventListener("input", () => {
  const v = pwInput.value;
  const score = passwordStrength(v);
  bars.forEach((bar, i) => {
    bar.classList.remove("is-on-1", "is-on-2", "is-on-3", "is-on-4");
    if (i < score) bar.classList.add(`is-on-${score}`);
  });

  const rules = {
    length: v.length >= 8,
    case: /[a-z]/.test(v) && /[A-Z]/.test(v),
    digit: /\d/.test(v),
    special: /[^A-Za-z0-9]/.test(v),
  };
  ruleEls.forEach((el) => {
    el.classList.toggle("is-met", !!rules[el.dataset.rule]);
  });
});

formSignup.addEventListener("submit", async (e) => {
  e.preventDefault();
  const fd = new FormData(formSignup);
  const userError = validateUsername(fd.get("username"));
  if (userError) {
    toast.error(userError);
    qs("#signup-username").focus();
    return;
  }
  const emError = validateEmail(fd.get("email"));
  if (emError) {
    toast.error(emError);
    qs("#signup-email").focus();
    return;
  }
  const phError = validatePhone(fd.get("phone_number"));
  if (phError) {
    toast.error(phError);
    qs("#signup-phone").focus();
    return;
  }
  const password = fd.get("password");
  const pwError = validatePassword(password);
  if (pwError) {
    toast.error(pwError);
    qs("#signup-password").focus();
    return;
  }
  if (password !== fd.get("confirmPassword")) {
    toast.error(t("login.signup.mismatch"));
    qs("#signup-confirm").focus();
    return;
  }
  try {
    const res = await fetch("/create-user", {
      method: "POST",
      body: fd,
      credentials: "same-origin",
    });
    const data = await res.json().catch(() => ({}));
    if (res.ok && data.success) {
      toast.success(data.message || t("login.signup.ok"));
      formSignup.reset();
      bars.forEach((b) => b.className = "password-strength__bar");
      ruleEls.forEach((el) => el.classList.remove("is-met"));
      setTimeout(() => showTab("login"), 800);
    } else {
      toast.error(data.message || t("login.signup.failed"));
    }
  } catch (err) {
    toast.error(t("login.signup.networkErr") + (err.message || err));
  }
});
