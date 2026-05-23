// Shared modal viewers for templates and roles.
//
//   openTemplateDetailModal(type, name) — pops a modal listing the entries
//     (IPs/CIDR for server, regexes for command) under that template.
//   openRoleDetailModal(name)            — pops a modal showing the server +
//     command templates bound to a role; each chip is clickable and forwards
//     into openTemplateDetailModal for quick drill-down.

import { api } from "./api.js";
import { h } from "./dom.js";
import { openModal } from "./components/modal.js";
import { toast } from "./components/toast.js";
import { t } from "./i18n.js";

const TYPE_META = {
  server:  { path: "/tacacs/template/server/get",  titleKey: "server.title",  entryKey: "server.entryLabel" },
  command: { path: "/tacacs/template/command/get", titleKey: "command.title", entryKey: "command.entryLabel" },
};

export function parseList(value) {
  if (Array.isArray(value)) return value.filter(Boolean).map((s) => String(s).trim()).filter(Boolean);
  if (typeof value === "string") {
    return value.split(",").map((s) => s.trim()).filter(Boolean);
  }
  return [];
}

// Pick the first column that isn't ID/Template — that's the entry/regex value.
function detectEntryKey(rows) {
  if (!rows.length) return null;
  return Object.keys(rows[0]).find((k) => k !== "ID" && k !== "Template") || null;
}

export async function openTemplateDetailModal(type, name) {
  const meta = TYPE_META[type];
  if (!meta) return;
  let rows;
  try {
    const res = await api.get(meta.path);
    rows = ((res && res.data) || []).filter((r) => r.Template === name);
  } catch (err) {
    toast.error(t("viewer.loadFailed") + (err.message || err));
    return;
  }

  const detailKey = detectEntryKey(rows);
  const entryLabel = t(meta.entryKey);

  openModal({
    title: t("viewer.tpl.title", { title: t(meta.titleKey), name }),
    body: h("div", { class: "stack" }, [
      h("p", { class: "field__hint" },
        rows.length ? t("viewer.summary", { n: rows.length, entryLabel }) : t("viewer.empty")),
      rows.length
        ? h("div", { class: "stack", style: { gap: "4px" } },
            rows.map((r) =>
              h("div", {
                class: "row",
                style: {
                  alignItems: "center",
                  padding: "6px 10px",
                  background: "var(--color-surface-muted)",
                  borderRadius: "var(--radius-sm)",
                  fontFamily: "var(--font-mono)",
                  fontSize: "var(--font-size-xs)",
                  gap: "var(--space-3)",
                },
              }, [
                h("span", { style: { color: "var(--color-text-subtle)", flexShrink: "0" } }, `#${r.ID}`),
                h("span", { style: { flex: "1", wordBreak: "break-all" } },
                  String(r[detailKey] ?? "—")),
              ])
            ))
        : null,
    ]),
    actions: [{ label: t("common.close"), kind: "ghost", onClick: (close) => close() }],
  });
}

export async function openRoleDetailModal(roleName) {
  let row;
  try {
    const res = await api.get("/tacacs/template/role/get");
    row = ((res && res.data) || []).find((r) => r.Template === roleName);
  } catch (err) {
    toast.error(t("viewer.role.loadFailed") + (err.message || err));
    return;
  }
  if (!row) {
    toast.warning(t("viewer.role.notFound", { name: roleName }));
    return;
  }

  const serverKey  = Object.keys(row).find((k) => /server/i.test(k));
  const commandKey = Object.keys(row).find((k) => /command/i.test(k));
  const servers  = parseList(serverKey  ? row[serverKey]  : null);
  const commands = parseList(commandKey ? row[commandKey] : null);

  openModal({
    title: t("viewer.role.title", { name: roleName }),
    body: h("div", { class: "stack", style: { gap: "var(--space-5)" } }, [
      sectionWithChips(t("viewer.role.servers"),  servers,  "server"),
      sectionWithChips(t("viewer.role.commands"), commands, "command"),
      h("p", { class: "field__hint" }, t("viewer.role.tip")),
    ]),
    actions: [{ label: t("common.close"), kind: "ghost", onClick: (close) => close() }],
  });
}

function sectionWithChips(title, items, type) {
  return h("div", { class: "stack", style: { gap: "var(--space-2)" } }, [
    h("h3", { class: "section-title" }, title),
    items.length === 0
      ? h("p", { class: "text-muted" }, t("common.none"))
      : h("div", { style: { display: "flex", flexWrap: "wrap", gap: "var(--space-2)" } },
          items.map((name) =>
            h("button", {
              class: "chip-btn",
              type: "button",
              onclick: () => openTemplateDetailModal(type, name),
            }, name)
          )),
  ]);
}
