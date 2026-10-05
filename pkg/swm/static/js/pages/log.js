// TACACS log page. The backend selects either the legacy external redirect
// mode or a safe, parameterized ClickHouse query mode.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { toast } from "../core/components/toast.js";
import { t } from "../core/i18n.js";

const TYPES = [
  { key: "authen", labelKey: "log.redirect.authen", visibleField: "visibleAuthen" },
  { key: "author", labelKey: "log.redirect.author", visibleField: "visibleAuthor" },
  { key: "account", labelKey: "log.redirect.account", visibleField: "visibleAccount" },
];

export default async function renderLogPage(container, ctx) {
  const root = h("div", { class: "page" });
  const statusHost = h("div", { class: "card__body stack" });
  mount(root, h("header", { class: "page__header" }, [
    h("div", { class: "page__heading" }, [
      h("h1", { class: "page__title" }, t("log.title")),
      h("p", { class: "page__subtitle" }, t("log.subtitle")),
    ]),
  ]), h("section", { class: "card" }, [statusHost]));
  mount(container, root);

  let meta;
  try {
    const res = await api.get("/tacacs/log/meta");
    meta = (res && res.data) || {};
  } catch (err) {
    mount(statusHost, h("p", { class: "text-danger" }, t("log.loadFailed") + (err.message || err)));
    return;
  }

  if (meta.mode !== "clickhouse") {
    renderExternal(meta.external || {}, !!(ctx && ctx.isAdmin));
    return;
  }
  renderClickHouse(meta, ctx);

  function renderExternal(cfg, isAdmin) {
    const visibleTypes = isAdmin
      ? TYPES
      : TYPES.filter((typ) => !!cfg[typ.visibleField] && !!cfg[typ.key]);
    const buttons = h("div", { class: "page__actions" });
    if (!visibleTypes.length) {
      mount(statusHost, h("p", { class: "page__subtitle" },
        isAdmin ? t("log.redirect.notConfigured") : t("log.redirect.forbidden")));
      return;
    }
    mount(statusHost,
      h("p", { class: "page__subtitle" }, t("log.redirect.pickType")),
      buttons,
    );
    mount(buttons, ...visibleTypes.map((typ) => {
      const url = cfg[typ.key] || "";
      return h("button", {
        class: "btn btn--primary", type: "button", disabled: !url,
        title: url || t("log.redirect.typeNotConfigured"),
        onclick: () => { if (url) window.open(url, "_blank", "noopener,noreferrer"); },
      }, t(typ.labelKey));
    }));
  }

  function renderClickHouse(data, context) {
    const isAdmin = !!(context && context.isAdmin);
    const types = Array.isArray(data.types) ? data.types : [];
    if (!types.length) {
      mount(statusHost, h("p", { class: "page__subtitle" },
        data.clickhouseReady === false ? t("log.ck.notConfigured") : t("log.redirect.forbidden")));
      return;
    }

    const tabsHost = h("div", { class: "page__actions" });
    const filtersHost = h("div", { class: "stack" });
    const columnsHost = h("div", { class: "stack" });
    const tableHost = h("div");
    const pagerHost = h("div", { class: "page__actions", style: { justifyContent: "space-between" } });
    const fromInput = h("input", { class: "input", type: "datetime-local", step: "1" });
    const toInput = h("input", { class: "input", type: "datetime-local", step: "1" });
    const queryBtn = h("button", { class: "btn btn--primary", type: "button" }, t("log.btn.query"));
    const addFilterBtn = h("button", { class: "btn", type: "button" }, t("log.btn.add"));
    const clearFilterBtn = h("button", { class: "btn btn--ghost", type: "button" }, t("log.btn.clear"));
    const columnsDetails = h("details", { style: { width: "100%" } });
    const columnsSummary = h("summary", {
      style: {
        cursor: "pointer", display: "flex", alignItems: "center", justifyContent: "space-between",
        gap: "var(--space-3)", padding: "var(--space-2) 0", fontWeight: "500",
      },
    }, [
      h("span", null, t("log.columns")),
      h("span", { class: "text-subtle" }, t("log.columns.collapsed")),
    ]);
    mount(columnsDetails, columnsSummary, h("div", { style: { paddingTop: "var(--space-3)" } }, [columnsHost]));

    const now = new Date();
    fromInput.value = toDateTimeLocal(new Date(now.getTime() - 24 * 60 * 60 * 1000));
    toInput.value = toDateTimeLocal(now);

    const perType = new Map(types.map((item) => [item.type, {
      filters: [], page: 1, pageSize: 50,
      ...loadColumnPrefs(item.type, item.fields),
      result: null,
      fetchedResult: null,
    }]));
    let active = types[0].type;
    let draggingIndex = -1;
    let requestSerial = 0;
    const availabilityHint = h("p", {
      class: "field__hint field__hint--error",
      style: { display: types.some((item) => !item.available) ? "" : "none" },
    }, t("log.ck.notConfigured"));

    const eventRangeField = h("div", { class: "field", style: { minWidth: "420px", flex: "1" } }, [
      h("span", { class: "field__label" }, t("log.eventRange")),
      h("div", {
        class: "toolbar",
        style: {
          padding: "0", border: "0", background: "transparent", borderRadius: "0",
          alignItems: "center", gap: "var(--space-2)", flexWrap: "wrap",
        },
      }, [
        fromInput,
        h("span", { class: "text-muted", style: { lineHeight: "36px" } }, "→"),
        toInput,
      ]),
    ]);
    const eventRangeToolbar = h("div", {
      class: "toolbar", style: { alignItems: "flex-end" },
    }, [eventRangeField, queryBtn, addFilterBtn, clearFilterBtn]);
    const filterSection = h("div", { class: "stack" }, [
      h("div", {
        class: "field__label",
        style: { display: "flex", alignItems: "baseline", gap: "var(--space-2)", flexWrap: "wrap" },
      }, [
        h("span", null, t("log.filters")),
        !isAdmin ? h("span", { class: "text-subtle", style: { fontWeight: "400" } },
          t("log.filters.nonAdminHint")) : null,
      ]),
      filtersHost,
    ]);

    mount(statusHost, [
      h("p", { class: "page__subtitle" }, t("log.ck.subtitle")),
      availabilityHint,
      tabsHost,
      eventRangeToolbar,
      filterSection,
    ]);
    root.appendChild(h("section", { class: "card" }, [
      h("div", { class: "card__body stack" }, [columnsDetails, tableHost, pagerHost]),
    ]));

    queryBtn.addEventListener("click", () => load());
    addFilterBtn.addEventListener("click", () => {
      const state = perType.get(active);
      state.filters.push({ field: "", operator: "eq", value: "" });
      renderFilters();
    });
    clearFilterBtn.addEventListener("click", () => {
      perType.get(active).filters = [];
      renderFilters();
    });

    function activeMeta() { return types.find((item) => item.type === active) || types[0]; }
    function activeState() { return perType.get(active); }

    function renderTabs() {
      mount(tabsHost, ...types.map((item) => h("button", {
        class: ["btn", item.type === active ? "btn--primary" : "btn--ghost"],
        type: "button", onclick: () => {
          active = item.type;
          renderAll();
          load();
        },
      }, t(`log.redirect.${item.type}`))));
    }

    function renderFilters() {
      const state = activeState();
      const fields = (activeMeta().fields || []).filter((field) => isAdmin || field.key !== "user");
      if (!isAdmin) {
        // Remove stale state from an older page version or a forged response;
        // the server still applies the authoritative user scope.
        state.filters = state.filters.filter((filter) =>
          String(filter.field || "").trim().toLowerCase() !== "user");
      }
      mount(filtersHost, ...state.filters.map((filter, index) => {
        const fieldSelect = h("select", { class: "select" });
        const operatorSelect = h("select", { class: "select" });
        const valueInput = h("input", { class: "input", value: filter.value || "", placeholder: t("log.value.ph") });
        const remove = h("button", {
          class: "btn btn--ghost btn--sm", type: "button", "aria-label": t("log.value.aria"),
          onclick: () => { state.filters.splice(index, 1); renderFilters(); },
        }, "×");

        mount(fieldSelect, [
          h("option", { value: "" }, t("log.dim.empty")),
          ...fields.map((field) => h("option", {
            value: field.key, selected: field.key === filter.field,
          }, `${fieldDisplayLabel(field)} (${field.key})`)),
        ]);
        fieldSelect.value = filter.field || "";
        function refreshOperators() {
          const field = fields.find((x) => x.key === filter.field);
          const operators = operatorsFor(field && field.kind);
          mount(operatorSelect, operators.map((op) => h("option", {
            value: op.value, selected: op.value === filter.operator,
          }, op.label)));
          if (!operators.some((op) => op.value === filter.operator)) {
            filter.operator = operators[0] ? operators[0].value : "eq";
          }
          operatorSelect.value = filter.operator;
        }
        fieldSelect.addEventListener("change", () => {
          filter.field = fieldSelect.value;
          filter.operator = "eq";
          refreshOperators();
        });
        operatorSelect.addEventListener("change", () => { filter.operator = operatorSelect.value; });
        valueInput.addEventListener("input", () => { filter.value = valueInput.value; });
        refreshOperators();
        return h("div", { class: "toolbar" }, [
          h("span", { class: "field__label" }, t("log.cond", { n: index + 1 })),
          fieldSelect, operatorSelect, valueInput, remove,
        ]);
      }));
    }

    function renderColumns() {
      const state = activeState();
      const fields = activeMeta().fields || [];
      const fieldByKey = new Map(fields.map((x) => [x.key, x]));
      mount(columnsHost, h("div", { class: "stack" }, state.columns.map((key, index) => {
        const field = fieldByKey.get(key) || { key, label: key };
        return h("label", {
          class: "toolbar", draggable: "true", style: { cursor: "grab", gap: "8px", width: "100%" },
          ondragstart: () => { draggingIndex = index; },
          ondragover: (event) => event.preventDefault(),
          ondrop: () => {
            if (draggingIndex < 0 || draggingIndex === index) return;
            const moved = state.columns.splice(draggingIndex, 1)[0];
            state.columns.splice(index, 0, moved);
            draggingIndex = -1;
            saveColumns(active, state);
            renderColumns();
            renderDisplayedResult(state);
          },
        }, [
          h("input", {
            type: "checkbox", checked: state.selected.includes(key),
            onchange: (event) => {
              if (event.target.checked && !state.selected.includes(key)) state.selected.push(key);
              if (!event.target.checked) state.selected = state.selected.filter((x) => x !== key);
              saveColumns(active, state);
              renderColumns();
              handleColumnSelectionChange(state);
            },
          }),
          h("span", null, fieldDisplayLabel(field)),
          h("span", { class: "text-subtle" }, "↕"),
        ]);
      })));
    }

    function selectedColumnKeys(state) {
      return state.columns.filter((key) => state.selected.includes(key));
    }

    function handleColumnSelectionChange(state) {
      if (!state.fetchedResult) return;
      const fetchedKeys = new Set((state.fetchedResult.columns || []).map((column) => column.key));
      const needsQuery = selectedColumnKeys(state).some((key) => !fetchedKeys.has(key));
      if (needsQuery) {
        load();
        return;
      }
      renderDisplayedResult(state);
    }

    function renderDisplayedResult(state) {
      const source = state.fetchedResult;
      if (!source) {
        state.result = null;
        mount(tableHost);
        renderPager(null);
        return;
      }
      const columnsByKey = new Map((source.columns || []).map((column) => [column.key, column]));
      const columns = selectedColumnKeys(state)
        .map((key) => columnsByKey.get(key))
        .filter(Boolean);
      const rows = (source.rows || []).map((row) => {
        const projected = {};
        for (const column of columns) projected[column.key] = row[column.key];
        return projected;
      });
      state.result = { ...source, columns, rows };
      renderTable(tableHost, {
        columns: columns.map((column) => ({
          key: column.key,
          label: column.label || column.key,
          render: (value) => formatCell(value),
        })),
        rows,
        emptyText: t("log.empty"),
      });
      renderPager(state.result);
    }

    function renderPager(result) {
      if (!result) {
        mount(pagerHost);
        return;
      }
      const state = activeState();
      const totalKnown = result.totalKnown === true;
      const totalPages = totalKnown ? (result.totalPages || 0) : 0;
      const pageLabel = totalKnown && totalPages
        ? `${result.page} / ${totalPages}`
        : t("log.ck.page", { n: result.page || state.page });
      const pageSize = h("select", { class: "select" }, [10, 20, 50, 100, 200].map((n) =>
        h("option", { value: String(n), selected: n === state.pageSize }, String(n))));
      pageSize.value = String(state.pageSize);
      pageSize.addEventListener("change", () => { state.pageSize = Number(pageSize.value) || 50; state.page = 1; load(); });
      const prev = h("button", { class: "btn", type: "button", disabled: state.page <= 1, onclick: () => { state.page -= 1; load(); } }, t("log.ck.prev"));
      const hasMore = totalKnown ? (totalPages > 0 && state.page < totalPages) : result.hasMore === true;
      const next = h("button", { class: "btn", type: "button", disabled: !hasMore, onclick: () => { state.page += 1; load(); } }, t("log.ck.next"));
      const totalLabel = totalKnown ? h("span", { class: "page__subtitle" }, t("log.ck.total", { n: result.total || 0 })) : null;
      mount(pagerHost, [
        totalLabel,
        h("span", { class: "page__subtitle" }, pageLabel), prev,
        h("label", { class: "toolbar", style: { margin: 0 } }, [h("span", { class: "field__label" }, t("log.ck.pageSize")), pageSize]),
        next,
      ]);
    }

    async function load() {
      const state = activeState();
      const requestType = active;
      if (!activeMeta().available) {
        return;
      }
      if (!state.selected.length) {
        toast.error(t("log.ck.selectColumn"));
        return;
      }
      const from = dateTimeToISO(fromInput.value);
      const to = dateTimeToISO(toInput.value);
      if (!from || !to) {
        toast.error(t("log.ck.rangeRequired"));
        return;
      }
      const requestID = ++requestSerial;
      queryBtn.disabled = true;
      renderTable(tableHost, { columns: [], rows: [], loading: true });
      try {
        const selectedColumns = selectedColumnKeys(state);
        const res = await api.post("/tacacs/log/query", {
          type: active,
          eventRange: { from, to },
          filters: state.filters.filter((x) => x.field && x.value !== ""),
          columns: selectedColumns,
          page: state.page,
          pageSize: state.pageSize,
          includeTotal: false,
        });
        const result = (res && res.data) || {};
        if (requestID !== requestSerial || requestType !== active) return;
        state.fetchedResult = result;
        renderDisplayedResult(state);
      } catch (err) {
        if (requestID !== requestSerial || requestType !== active) return;
        toast.error(t("common.loadFailed") + (err.message || err));
        mount(tableHost, h("div", { class: "card" }, [
          h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
        ]));
      } finally {
        if (requestID === requestSerial && requestType === active) queryBtn.disabled = false;
      }
    }

    function renderAll() {
      renderTabs();
      renderFilters();
      renderColumns();
      renderDisplayedResult(activeState());
      queryBtn.disabled = !activeMeta().available;
    }

    renderAll();
    load();
  }
}

function fieldDisplayLabel(field) {
  const label = (field && (field.fieldName || field.label || field.key)) || "";
  const name = field && typeof field.name === "string" ? field.name.trim() : "";
  return name ? `${label} (${name})` : label;
}

function operatorsFor(kind) {
  if (kind === "int") {
    return ["eq", "neq", "gt", "gte", "lt", "lte"].map((value) => ({ value, label: t(`log.op.${value}`) }));
  }
  if (kind === "bool") {
    return ["eq", "neq"].map((value) => ({ value, label: t(`log.op.${value}`) }));
  }
  if (kind === "string_array") {
    return ["contains", "not_contains"].map((value) => ({ value, label: t(`log.op.${value}`) }));
  }
  return ["eq", "neq", "contains", "not_contains", "starts_with", "ends_with"]
    .map((value) => ({ value, label: t(`log.op.${value}`) }));
}

function formatCell(value) {
  if (value == null) return "";
  if (Array.isArray(value)) return value.join(", ");
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

function toDateTimeLocal(date) {
  const pad = (x) => String(x).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

function dateTimeToISO(value) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : date.toISOString();
}

function loadColumnPrefs(type, fields) {
  const fallback = (fields || []).map((x) => x.key);
  try {
    const raw = localStorage.getItem(`tacacs-log-columns-${type}`);
    const saved = raw ? JSON.parse(raw) : null;
    if (Array.isArray(saved)) {
      const allowed = new Set(fallback);
      const selected = saved.filter((x) => allowed.has(x));
      return { columns: fallback, selected: selected.length ? selected : fallback.slice() };
    }
    if (!saved || !Array.isArray(saved.order)) return { columns: fallback, selected: fallback.slice() };
    const allowed = new Set(fallback);
    const order = saved.order.filter((x) => allowed.has(x));
    for (const key of fallback) if (!order.includes(key)) order.push(key);
    const selected = Array.isArray(saved.selected)
      ? saved.selected.filter((x) => allowed.has(x))
      : fallback.slice();
    return { columns: order, selected: selected.length ? selected : fallback.slice() };
  } catch {
    return { columns: fallback, selected: fallback.slice() };
  }
}

function saveColumns(type, state) {
  try {
    localStorage.setItem(`tacacs-log-columns-${type}`, JSON.stringify({
      order: state.columns, selected: state.selected,
    }));
  }
  catch { /* private browsing or storage disabled */ }
}
