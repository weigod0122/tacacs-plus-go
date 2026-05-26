// Role template page — list / add / delete.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { openModal } from "../core/components/modal.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { openTemplateDetailModal, parseList } from "../core/template-viewer.js";
import { t, tHeader } from "../core/i18n.js";

export default async function renderRolePage(container) {
  const tableHost = h("div");
  let rows = [];

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("role.title")),
        h("p", { class: "page__subtitle" }, t("role.subtitle")),
      ]),
      h("div", { class: "page__actions" }, [
        h("button", { class: "btn", type: "button", onclick: load }, t("app.refresh")),
        h("button", { class: "btn btn--primary", type: "button", onclick: openAddModal }, t("role.btn.create")),
      ]),
    ]),
    tableHost,
  ]));

  async function openAddModal() {
    let serverNames = [];
    let commandNames = [];
    try {
      const [s, c] = await Promise.all([
        api.get("/tacacs/template/server/get"),
        api.get("/tacacs/template/command/get"),
      ]);
      serverNames = uniqueTemplateNames(s.data || []);
      commandNames = uniqueTemplateNames(c.data || []);
    } catch (err) {
      toast.error(t("role.create.loadFailed") + (err.message || err));
      return;
    }
    if (serverNames.length === 0 || commandNames.length === 0) {
      toast.warning(t("role.create.warnEmpty"));
      return;
    }

    // Custom layout: name input on top, then two side-by-side pickers so
    // long template lists no longer require scrolling inside a narrow
    // dropdown nested inside a narrow modal.
    const nameInput = h("input", {
      class: "input", maxlength: 64, required: true,
      placeholder: t("role.create.namePh"),
    });

    // 用户一旦自己改过名字,就不再被两个 picker 的联动覆盖;只有 input 事件
    // 来自真实键入,程序设置 .value 不会触发,所以这个标志可信。
    let nameTouched = false;
    nameInput.addEventListener("input", () => { nameTouched = true; });

    function refreshAutoName() {
      if (nameTouched) return;
      const srv = serverPicker.values();
      const cmd = commandPicker.values();
      if (srv.length === 0 || cmd.length === 0) {
        nameInput.value = "";
        return;
      }
      const raw = `server:${srv.join("+")}-cmd:${cmd.join("+")}`;
      nameInput.value = raw.length > 64 ? raw.slice(0, 64) : raw;
    }

    const serverPicker = buildPicker(serverNames, { onChange: refreshAutoName });
    const commandPicker = buildPicker(commandNames, { onChange: refreshAutoName });

    const errorEl = h("p", {
      class: "field__hint field__hint--error",
      style: { display: "none" },
    });

    let submitting = false;

    const formBody = h("form", {
      class: "stack",
      novalidate: true,
      onsubmit: (e) => { e.preventDefault(); submit(); },
    }, [
      h("div", { class: "field" }, [
        h("label", { class: "field__label" }, t("role.create.name")),
        nameInput,
      ]),
      h("div", { class: "form-cols" }, [
        h("div", { class: "field" }, [
          h("label", { class: "field__label" }, t("role.create.servers")),
          serverPicker.el,
        ]),
        h("div", { class: "field" }, [
          h("label", { class: "field__label" }, t("role.create.commands")),
          commandPicker.el,
        ]),
      ]),
      errorEl,
    ]);

    const handle = openModal({
      title: t("role.create.title"),
      size: "xl",
      body: formBody,
      actions: [
        { label: t("common.cancel"),  kind: "ghost",   onClick: (close) => close() },
        { label: t("common.confirm"), kind: "primary", onClick: () => submit() },
      ],
    });

    async function submit() {
      if (submitting) return;
      errorEl.style.display = "none";

      const name = nameInput.value.trim();
      if (!name) return showError(t("form.err.required", { label: t("role.create.name") }), nameInput);
      const servers = serverPicker.values();
      if (servers.length === 0) {
        return showError(t("form.err.requiredMulti", { label: t("role.create.servers") }));
      }
      const commands = commandPicker.values();
      if (commands.length === 0) {
        return showError(t("form.err.requiredMulti", { label: t("role.create.commands") }));
      }

      submitting = true;
      try {
        await api.post("/tacacs/template/role/create", {
          template: name,
          server_template_list: servers.join(","),
          command_template_list: commands.join(","),
        });
        handle.close();
        toast.success(t("role.create.toast.ok"));
        await load();
      } catch (err) {
        showError(err.message || t("role.create.toast.failed"));
      } finally {
        submitting = false;
      }
    }

    function showError(msg, focusEl) {
      errorEl.textContent = msg;
      errorEl.style.display = "";
      if (focusEl) focusEl.focus();
    }
  }

  async function deleteRole(template) {
    const ok = await confirm({
      title: t("role.delete.title"),
      message: t("role.delete.msg", { name: template }),
      confirmLabel: t("common.delete"),
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del("/tacacs/template/role/delete", { template });
      toast.success(t("role.delete.ok"));
      await load();
    } catch (err) { toast.error(err.message || t("role.delete.failed")); }
  }

  function render() {
    const cols = inferColumns(rows);
    renderTable(tableHost, {
      columns: cols,
      rows,
      emptyText: t("role.empty"),
      rowActions: (row) => [
        { label: t("common.delete"), kind: "danger", onClick: () => deleteRole(row.Template) },
      ],
    });
  }

  async function load() {
    renderTable(tableHost, { columns: [], rows: [], loading: true });
    try {
      const res = await api.get("/tacacs/template/role/get");
      rows = (res && res.data) || [];
      render();
    } catch (err) {
      mount(tableHost, h("div", { class: "card" }, [
        h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
      ]));
    }
  }

  await load();
}

function uniqueTemplateNames(rows) {
  return Array.from(new Set(rows.map((r) => r.Template))).sort();
}

function inferColumns(rows) {
  if (rows.length === 0) {
    return [
      { key: "Template",        label: t("role.create.name") },
      { key: "ServerTemplate",  label: t("role.create.servers"),  render: (v) => chipsFor(v, "server") },
      { key: "CommandTemplate", label: t("role.create.commands"), render: (v) => chipsFor(v, "command") },
    ];
  }
  // Backend field names vary (e.utils. server_template_list vs ServerTemplate vs
  // serverTemplateList). Detect role-page columns by content rather than
  // exact key match so the headers stay localized regardless of casing.
  return Object.keys(rows[0])
    .filter((k) => k !== "ID")
    .map((k) => {
      if (/server/i.test(k))  return { key: k, label: t("role.create.servers"),  render: (v) => chipsFor(v, "server") };
      if (/command/i.test(k)) return { key: k, label: t("role.create.commands"), render: (v) => chipsFor(v, "command") };
      if (/^template$/i.test(k)) return { key: k, label: t("role.create.name") };
      return { key: k, label: tHeader(k) };
    });
}

function chipsFor(value, type) {
  const items = parseList(value);
  if (items.length === 0) return document.createTextNode("—");
  return h("div", {
    style: { display: "flex", flexWrap: "wrap", gap: "4px" },
  }, items.map((name) =>
    h("button", {
      class: "chip-btn",
      type: "button",
      title: t("approval.role.viewDetail", { role: name }),
      onclick: () => openTemplateDetailModal(type, name),
    }, name)
  ));
}

/**
 * buildPicker(options, opts) — always-visible search + checkbox list. Used inside
 * the create-role modal where the previous dropdown multiselect required
 * scrolling within a small panel.
 *
 * opts.onChange — optional callback fired after each selection change (个项勾选、
 * 全选、清空都会触发),用于联动外部状态(例如自动生成角色名)。
 *
 * Returns { el, values() }.
 */
function buildPicker(options, opts = {}) {
  const { onChange } = opts;
  const selected = new Set();
  const itemMap = new Map();

  const list = h("div", { class: "picker__list", role: "listbox", "aria-multiselectable": "true" });
  const search = h("input", {
    type: "search", class: "input picker__search",
    placeholder: t("common.search"),
    oninput: (e) => filterItems(e.target.value.toLowerCase()),
  });
  const countEl = h("span", { class: "picker__count" });

  function renderItem(value) {
    const checkbox = h("input", {
      type: "checkbox", value,
      onchange: (e) => {
        if (e.target.checked) selected.add(value);
        else selected.delete(value);
        item.classList.toggle("is-checked", e.target.checked);
        updateCount();
        onChange?.();
      },
    });
    const item = h("label", {
      class: "picker__item",
      dataset: { label: String(value).toLowerCase() },
    }, [checkbox, h("span", null, value)]);
    itemMap.set(value, { item, checkbox });
    return item;
  }

  options.forEach((opt) => list.appendChild(renderItem(opt)));

  function filterItems(needle) {
    Array.from(list.children).forEach((item) => {
      item.style.display = !needle || item.dataset.label.includes(needle) ? "" : "none";
    });
  }

  function updateCount() {
    countEl.textContent = t("common.selectedCount", { n: selected.size });
  }
  updateCount();

  const allBtn = h("button", {
    type: "button", class: "btn btn--ghost btn--sm",
    onclick: () => {
      options.forEach((o) => selected.add(o));
      itemMap.forEach((m) => {
        m.checkbox.checked = true;
        m.item.classList.add("is-checked");
      });
      updateCount();
      onChange?.();
    },
  }, t("common.selectAll"));

  const noneBtn = h("button", {
    type: "button", class: "btn btn--ghost btn--sm",
    onclick: () => {
      selected.clear();
      itemMap.forEach((m) => {
        m.checkbox.checked = false;
        m.item.classList.remove("is-checked");
      });
      updateCount();
      onChange?.();
    },
  }, t("common.clear"));

  const el = h("div", { class: "picker" }, [
    h("div", { class: "picker__head" }, [
      search,
      h("div", { class: "picker__actions" }, [allBtn, noneBtn, countEl]),
    ]),
    list,
  ]);

  return {
    el,
    values: () => Array.from(selected),
  };
}
