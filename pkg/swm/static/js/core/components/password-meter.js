// Reusable password strength meter + rule checklist.
//
// Pass a password <input>; returns a Node to drop into the form below it.
// As the user types, the strength bars and rule items update live.

import { h } from "../dom.js";
import { passwordStrength } from "../format.js";
import { t } from "../i18n.js";

const RULES = [
  { id: "length",  labelKey: "login.rule.length",  check: (v) => v.length >= 8 },
  { id: "case",    labelKey: "login.rule.case",    check: (v) => /[a-z]/.test(v) && /[A-Z]/.test(v) },
  { id: "digit",   labelKey: "login.rule.digit",   check: (v) => /\d/.test(v) },
  { id: "special", labelKey: "login.rule.special", check: (v) => /[^A-Za-z0-9]/.test(v) },
];

const STATIC_RULE_KEYS = [
  "login.rule.expire",
  "login.rule.reuse",
];

export function buildPasswordMeter(input) {
  const bars = Array.from({ length: 4 }, () => h("div", { class: "password-strength__bar" }));
  const strengthEl = h("div", { class: "password-strength", "aria-hidden": "true" }, bars);

  const checkEls = RULES.map((r) => h("li", { dataset: { rule: r.id } }, t(r.labelKey)));
  const staticEls = STATIC_RULE_KEYS.map((k) => h("li", null, t(k)));
  const ruleList = h("ul", { class: "password-rules" }, [...checkEls, ...staticEls]);

  function update() {
    const v = input.value;
    const score = passwordStrength(v);
    bars.forEach((bar, i) => {
      bar.className = "password-strength__bar";
      if (i < score) bar.classList.add(`is-on-${score}`);
    });
    checkEls.forEach((el, i) => {
      el.classList.toggle("is-met", RULES[i].check(v));
    });
  }

  input.addEventListener("input", update);

  return h("div", { class: "stack", style: { gap: "var(--space-2)" } }, [
    strengthEl,
    ruleList,
  ]);
}
