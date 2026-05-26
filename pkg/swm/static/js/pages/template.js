// Generic template page (server / command). Both share the same shape,
// only the API base path and labels differ.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { openFormModal } from "../core/components/modal.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { t } from "../core/i18n.js";

export function renderTemplatePage({
  base,            // e.utils. "/tacacs/template/server"
  titleKey,
  subtitleKey,
  detailLabelKey,
  detailHintKey,
  detailPlaceholder,
  kindKey,
  detailRows = 3,
  separator = /[,\n]/,   // how to split the textarea into entries
  validateEntry,         // optional (entry: string) => string | null
  extraToolbar,          // optional (rows: object[]) => Node — rendered between filter and table
}) {
  return async function render(container) {
    const tableHost = h("div");
    const extraHost = extraToolbar ? h("div") : null;
    let rows = [];

    let activeFilter = "all";
    mount(container, h("div", { class: "page" }, [
      h("header", { class: "page__header" }, [
        h("div", { class: "page__heading" }, [
          h("h1", { class: "page__title" }, t(titleKey)),
          h("p", { class: "page__subtitle" }, t(subtitleKey)),
        ]),
        h("div", { class: "page__actions" }, [
          h("button", { class: "btn", type: "button", onclick: load }, t("app.refresh")),
          h("button", { class: "btn btn--danger", type: "button", onclick: openBulkDeleteModal }, t("tpl.btn.bulkDelete")),
          h("button", { class: "btn btn--primary", type: "button", onclick: openAddModal }, t("tpl.btn.add")),
        ]),
      ]),
      filterBar(),
      extraHost,
      tableHost,
    ]));

    function rebuildExtra() {
      if (!extraHost) return;
      mount(extraHost, extraToolbar(rows));
    }

    function filterBar() {
      const select = h("select", {
        class: "select",
        onchange: (e) => { activeFilter = e.target.value; renderRows(); },
      });
      select.dataset.role = "filter";
      return h("div", { class: "toolbar" }, [
        h("label", { class: "field__label" }, t("tpl.toolbar.filter")),
        select,
      ]);
    }

    function refreshFilterOptions() {
      const select = container.querySelector('select[data-role="filter"]');
      if (!select) return;
      const names = Array.from(new Set(rows.map((r) => r.Template))).sort();
      select.innerHTML = "";
      select.appendChild(h("option", { value: "all" }, t("common.all")));
      for (const n of names) select.appendChild(h("option", { value: n }, n));
      select.value = names.includes(activeFilter) ? activeFilter : "all";
    }

    function visibleRows() {
      if (activeFilter === "all") return rows;
      return rows.filter((r) => r.Template === activeFilter);
    }

    // Render entries grouped by template — each template gets its own card,
    // entries inside sorted by ID. This way the same template never gets
    // split across non-contiguous rows.
    function renderRows() {
      const visible = visibleRows();
      if (visible.length === 0) {
        mount(tableHost, h("div", { class: "card" }, [
          h("div", {
            class: "card__body text-muted",
            style: { textAlign: "center", padding: "var(--space-6)" },
          }, t("tpl.empty")),
        ]));
        return;
      }

      // Detect the entry/detail field name (the one that isn't ID or Template).
      const detailKey = Object.keys(visible[0]).find((k) => k !== "ID" && k !== "Template");

      // Group by template, sort templates alphabetically, entries by ID.
      const groups = new Map();
      for (const r of visible) {
        const tpl = r.Template;
        if (!groups.has(tpl)) groups.set(tpl, []);
        groups.get(tpl).push(r);
      }
      const ordered = Array.from(groups.entries())
        .sort((a, b) => String(a[0]).localeCompare(String(b[0])))
        .map(([tpl, entries]) => [
          tpl,
          entries.slice().sort((a, b) => Number(a.ID) - Number(b.ID)),
        ]);

      mount(tableHost, h("div", { class: "tpl-groups" },
        ordered.map(([tpl, entries]) => renderGroup(tpl, entries, detailKey))
      ));
    }

    function renderGroup(tplName, entries, detailKey) {
      return h("div", { class: "tpl-group" }, [
        h("div", { class: "tpl-group__header" }, [
          h("h3", { class: "tpl-group__name" }, tplName),
          h("span", { class: "tpl-group__count" },
            t("tpl.group.count", { n: entries.length })),
          h("div", { class: "tpl-group__actions" }, [
            h("button", {
              class: "btn btn--sm btn--danger",
              type: "button",
              onclick: () => deleteTemplateGroup(tplName, entries.length),
            }, t("tpl.group.deleteAll")),
          ]),
        ]),
        h("div", { class: "tpl-group__entries" },
          entries.map((entry) =>
            h("div", { class: "tpl-group__entry" }, [
              h("span", { class: "tpl-group__id" }, `#${entry.ID}`),
              h("span", { class: "tpl-group__detail" },
                String(entry[detailKey] ?? "—")),
              h("button", {
                class: "btn btn--sm btn--ghost",
                type: "button",
                onclick: () => deleteRow(entry),
              }, t("common.delete")),
            ])
          )
        ),
      ]);
    }

    function openAddModal() {
      openFormModal({
        title: t("tpl.add.title", { kind: t(kindKey) }),
        size: "lg",
        fields: [
          { name: "template", label: t("tpl.add.name"), required: true, maxlength: 64,
            placeholder: t("tpl.add.namePh"), hint: t("fmt.name.noCommaHint") },
          { name: "templateDetail", label: t(detailLabelKey), required: true,
            placeholder: detailPlaceholder, hint: t(detailHintKey), type: "textarea",
            rows: detailRows },
        ],
        onSubmit: async (values) => {
          // 模板名禁止英文逗号:角色创建时 server_template_list / command_template_list
          // 都用 "," 拼接,模板名中混入逗号会破坏后端 Split 解析。
          if (values.template && values.template.includes(",")) {
            throw new Error(t("fmt.name.noComma"));
          }
          const entries = values.templateDetail
            .split(separator)
            .map((s) => s.trim())
            .filter(Boolean);
          if (entries.length === 0) {
            throw new Error(t("tpl.add.entriesEmpty"));
          }
          if (validateEntry) {
            for (const e of entries) {
              const err = validateEntry(e);
              if (err) throw new Error(err);
            }
          }
          try {
            await api.post(`${base}/add`, {
              template: values.template,
              templateDetail: entries,
            });
            toast.success(t("tpl.add.toast.ok"));
            await load();
          } catch (err) { throw new Error(err.message || t("tpl.add.toast.failed")); }
        },
      });
    }

    async function deleteRow(row) {
      const ok = await confirm({
        title: t("tpl.delete.title"),
        message: t("tpl.delete.msg", { id: row.ID, tpl: row.Template }),
        confirmLabel: t("common.delete"),
        danger: true,
      });
      if (!ok) return;
      try {
        await api.del(`${base}/delete`, { id: Number(row.ID) });
        toast.success(t("tpl.delete.ok"));
        await load();
      } catch (err) { toast.error(err.message || t("tpl.delete.failed")); }
    }

    async function deleteTemplateGroup(tplName, count) {
      const ok = await confirm({
        title: t("tpl.bulk.confirmTitle"),
        message: t("tpl.bulk.confirmMsg", { tpl: tplName, n: count }),
        confirmLabel: t("tpl.bulk.confirmBtn"),
        danger: true,
      });
      if (!ok) return;
      try {
        await api.del(`${base}/delete`, { template: tplName });
        toast.success(t("tpl.bulk.toast.ok", { tpl: tplName }));
        await load();
      } catch (err) {
        toast.error(err.message || t("tpl.delete.failed"));
      }
    }

    function openBulkDeleteModal() {
      const templates = Array.from(new Set(rows.map((r) => r.Template))).sort();
      if (!templates.length) {
        toast.info(t("tpl.bulk.empty"));
        return;
      }
      const counts = Object.fromEntries(
        templates.map((tn) => [tn, rows.filter((r) => r.Template === tn).length])
      );

      openFormModal({
        title: t("tpl.bulk.title"),
        submitLabel: t("tpl.bulk.submit"),
        submitKind: "danger",
        fields: [
          {
            name: "template",
            label: t("tpl.bulk.label"),
            type: "select",
            required: true,
            placeholder: t("tpl.bulk.placeholder"),
            hint: t("tpl.bulk.hint"),
            options: templates.map((tn) => ({
              value: tn,
              label: t("tpl.bulk.option", { name: tn, n: counts[tn] }),
            })),
          },
        ],
        onSubmit: async (values, close) => {
          const ok = await confirm({
            title: t("tpl.bulk.confirmTitle"),
            message: t("tpl.bulk.confirmMsg", { tpl: values.template, n: counts[values.template] }),
            confirmLabel: t("tpl.bulk.confirmBtn"),
            danger: true,
          });
          if (!ok) {
            close({ confirmed: false });
            return;
          }
          try {
            await api.del(`${base}/delete`, { template: values.template });
            toast.success(t("tpl.bulk.toast.ok", { tpl: values.template }));
            await load();
          } catch (err) {
            throw new Error(err.message || t("tpl.delete.failed"));
          }
        },
      });
    }

    async function load() {
      renderTable(tableHost, { columns: [], rows: [], loading: true });
      try {
        const res = await api.get(`${base}/get`);
        rows = (res && res.data) || [];
        refreshFilterOptions();
        rebuildExtra();
        renderRows();
      } catch (err) {
        mount(tableHost, h("div", { class: "card" }, [
          h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
        ]));
      }
    }

    await load();
  };
}
