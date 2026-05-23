// TACACS log viewer — date picker + multi-dimension filter.

import { api } from "../core/api.js";
import { h, mount, qsa } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { toast } from "../core/components/toast.js";
import { t, tHeader } from "../core/i18n.js";

export default async function renderLogPage(container) {
  const today = new Date().toISOString().slice(0, 10);
  let queryDate = today;
  let allRows = [];
  let filters = []; // [{ key, value }]

  const tableHost = h("div");
  const filterRows = h("div", { class: "stack" });

  const dateInput = h("input", {
    type: "date",
    class: "input",
    value: queryDate,
    onchange: (e) => { queryDate = e.target.value; },
    style: { width: "180px" },
  });

  const queryBtn = h("button", { class: "btn btn--primary", type: "button", onclick: load }, t("log.btn.query"));

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("log.title")),
        h("p", { class: "page__subtitle" }, t("log.subtitle")),
      ]),
    ]),
    h("div", { class: "toolbar" }, [
      h("label", { class: "field__label" }, t("log.date")),
      dateInput,
      queryBtn,
      h("span", { class: "grow" }),
      h("button", {
        class: "btn", type: "button",
        onclick: addFilter,
      }, t("log.btn.add")),
      h("button", {
        class: "btn btn--ghost", type: "button",
        onclick: () => { filters = []; rebuildFilterRows(); applyFilters(); },
      }, t("log.btn.clear")),
    ]),
    filterRows,
    tableHost,
  ]));

  function addFilter() {
    filters.push({ key: "", value: "" });
    rebuildFilterRows();
  }

  function rebuildFilterRows() {
    mount(filterRows, ...filters.map((f, idx) => filterRow(f, idx)));
  }

  function filterRow(filter, idx) {
    const keys = allRows.length ? Object.keys(allRows[0]) : [];
    const valueOptions = filter.key
      ? Array.from(new Set(allRows.map((r) => String(r[filter.key]))))
          .filter(Boolean)
          .sort()
      : [];

    const valueListId = `flv-${idx}`;
    return h("div", { class: "toolbar" }, [
      h("span", { class: "field__label" }, t("log.cond", { n: idx + 1 })),
      h("select", {
        class: "select",
        onchange: (e) => { filter.key = e.target.value; rebuildFilterRows(); applyFilters(); },
      }, [
        h("option", { value: "" }, t("log.dim.empty")),
        ...keys.map((k) => h("option", { value: k, selected: k === filter.key }, tHeader(k))),
      ]),
      h("input", {
        class: "input",
        list: filter.key && filter.key !== "Arg" ? valueListId : null,
        value: filter.value,
        placeholder: filter.key === "Arg" ? t("log.value.argPh") : t("log.value.ph"),
        oninput: (e) => { filter.value = e.target.value; applyFilters(); },
      }),
      filter.key && filter.key !== "Arg"
        ? h("datalist", { id: valueListId },
            valueOptions.map((v) => h("option", { value: v })))
        : null,
      h("button", {
        class: "btn btn--ghost btn--sm",
        type: "button",
        "aria-label": t("log.value.aria"),
        onclick: () => { filters.splice(idx, 1); rebuildFilterRows(); applyFilters(); },
      }, "×"),
    ]);
  }

  function applyFilters() {
    let data = allRows;
    for (const f of filters) {
      if (!f.key) continue;
      if (f.key === "Arg") {
        const v = f.value;
        if (v) data = data.filter((r) => String(r[f.key] ?? "").includes(v));
      } else {
        if (f.value) data = data.filter((r) => String(r[f.key] ?? "") === f.value);
      }
    }
    renderTable(tableHost, {
      columns: inferColumns(allRows),
      rows: data,
      emptyText: t("log.empty"),
    });
  }

  async function load() {
    renderTable(tableHost, { columns: [], rows: [], loading: true });
    try {
      const res = await api.get("/tacacs/log/get/simple", { query: { date: queryDate } });
      allRows = (res && res.data) || [];
      rebuildFilterRows();
      applyFilters();
    } catch (err) {
      toast.error(t("common.loadFailed") + (err.message || err));
      mount(tableHost, h("div", { class: "card" }, [
        h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
      ]));
    }
  }

  await load();
}

function inferColumns(rows) {
  if (rows.length === 0) return [];
  return Object.keys(rows[0]).map((k) => ({ key: k, label: tHeader(k) }));
}
