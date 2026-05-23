// Command template page — entries are full regular expressions, one per line.
// Backend accepts Go RE2 syntax; we lightly pre-check via JS RegExp which
// catches the common typos (unbalanced brackets, bad quantifiers).

import { renderTemplatePage } from "./template.js";
import { h, mount } from "../core/dom.js";
import { t } from "../core/i18n.js";

function validateRegex(s) {
  try {
    new RegExp(s);
    return null;
  } catch (e) {
    return t("command.regexBad", { src: s, msg: e.message });
  }
}

// Pick the first column that isn't ID/Template — that's where the regex string lives.
function detectDetailKey(rows) {
  if (!rows.length) return null;
  return Object.keys(rows[0]).find((k) => k !== "ID" && k !== "Template") || null;
}

function renderTester(rows) {
  if (!rows.length) {
    return h("div", { class: "card" }, [
      h("div", { class: "card__body text-muted" }, t("command.tester.empty")),
    ]);
  }

  const detailKey = detectDetailKey(rows);
  const templates = Array.from(new Set(rows.map((r) => r.Template))).sort();
  let selectedTemplate = templates[0] || "";

  const tplSelect = h("select", {
    class: "select",
    onchange: (e) => { selectedTemplate = e.target.value; runTest(); },
  }, templates.map((tn) => h("option", { value: tn, selected: tn === selectedTemplate }, tn)));

  const cmdInput = h("input", {
    class: "input",
    type: "text",
    placeholder: t("command.tester.cmdPh"),
    onkeydown: (e) => { if (e.key === "Enter") { e.preventDefault(); runTest(); } },
    oninput: () => runTest(),
    style: { flex: "1", minWidth: "240px" },
  });

  const verdict = h("div");
  const breakdown = h("div", { class: "stack", style: { marginTop: "8px" } });

  function runTest() {
    const cmd = cmdInput.value;
    if (!cmd) {
      mount(verdict);
      mount(breakdown);
      return;
    }
    if (!detailKey) {
      mount(verdict, h("span", { class: "badge" }, t("command.tester.dataErr")));
      return;
    }

    const patterns = rows
      .filter((r) => r.Template === selectedTemplate)
      .map((r) => ({ id: r.ID, src: String(r[detailKey] ?? "") }));

    const results = patterns.map((p) => {
      let re;
      try { re = new RegExp(p.src); }
      catch (err) { return { ...p, ok: false, error: err.message }; }
      return { ...p, ok: re.test(cmd) };
    });

    const hit = results.find((r) => r.ok);
    mount(verdict,
      hit
        ? h("span", { class: "badge badge--approved" },
            t("command.tester.hit", { id: hit.id, src: hit.src }))
        : h("span", { class: "badge badge--rejected" },
            t("command.tester.miss", { tpl: selectedTemplate }))
    );

    mount(breakdown, ...results.map((r) =>
      h("div", {
        class: "row",
        style: {
          padding: "6px 10px",
          background: r.error ? "var(--color-warning-soft)"
            : r.ok ? "var(--color-success-soft)" : "var(--color-surface-muted)",
          borderRadius: "var(--radius-sm)",
          fontSize: "var(--font-size-xs)",
          fontFamily: "var(--font-mono)",
        },
      }, [
        h("span", {
          style: { width: "16px", color: r.error ? "var(--color-warning)"
            : r.ok ? "var(--color-success)" : "var(--color-text-subtle)" },
        }, r.error ? "!" : r.ok ? "✓" : "·"),
        h("span", { style: { color: "var(--color-text-subtle)", width: "40px" } }, `#${r.id}`),
        h("span", { style: { flex: "1", wordBreak: "break-all" } }, r.src),
        r.error ? h("span", { class: "text-danger" }, r.error) : null,
      ])
    ));
  }

  return h("div", { class: "card" }, [
    h("div", { class: "card__header" }, [
      h("span", { class: "card__title" }, t("command.tester.title")),
      h("span", { class: "text-subtle", style: { fontSize: "var(--font-size-xs)" } },
        t("command.tester.tip")),
    ]),
    h("div", { class: "card__body stack" }, [
      h("div", { class: "row row--wrap" }, [
        h("label", { class: "field__label" }, t("command.tester.label")),
        tplSelect,
        cmdInput,
        h("button", { class: "btn btn--primary", type: "button", onclick: runTest }, t("command.tester.btn")),
      ]),
      verdict,
      breakdown,
    ]),
  ]);
}

export default renderTemplatePage({
  base: "/tacacs/template/command",
  titleKey: "command.title",
  subtitleKey: "command.subtitle",
  detailLabelKey: "command.detailLabel",
  detailHintKey: "command.detailHint",
  kindKey: "command.kind",
  detailPlaceholder:
`^display .*$
^show (version|interfaces).*$
^ping \\d+\\.\\d+\\.\\d+\\.\\d+$`,
  detailRows: 8,
  separator: /\n+/,
  validateEntry: validateRegex,
  extraToolbar: renderTester,
});
