// Generic table renderer.
//   columns: [{ key, label, render?(value, row), align?, hidden?, className? }]
//   rowActions: (row) => [{ label, kind, onClick }]
//   filters: [{ key, label, options: [{value, label}] }] OR derived from data
//   loading, emptyText
//
// All cell text uses textContent — render() may return a Node for richer cells.

import { h, clear } from "../dom.js";
import { t } from "../i18n.js";

export function renderTable(container, opts) {
  const {
    columns,
    rows = [],
    rowActions,
    emptyText,
    loading = false,
  } = opts;

  clear(container);
  container.classList.add("fade-in");
  void container.offsetWidth;

  if (loading) {
    container.appendChild(skeleton());
    return;
  }

  if (!Array.isArray(rows) || rows.length === 0) {
    container.appendChild(h("div", { class: "table-wrap" }, [
      h("div", { class: "empty" }, emptyText != null ? emptyText : t("common.empty")),
    ]));
    return;
  }

  const cols = columns.filter((c) => !c.hidden);

  const head = h("thead", null, [
    h("tr", null, [
      ...cols.map((c) =>
        h("th", { style: c.align ? { textAlign: c.align } : null }, c.label || c.key)
      ),
      rowActions ? h("th", { style: { textAlign: "right" } }, t("common.actions")) : null,
    ]),
  ]);

  const body = h("tbody", null,
    rows.map((row) =>
      h("tr", null, [
        ...cols.map((c) => {
          const raw = row[c.key];
          let cell;
          if (typeof c.render === "function") {
            const rendered = c.render(raw, row);
            cell = rendered instanceof Node
              ? rendered
              : document.createTextNode(rendered == null ? "" : String(rendered));
          } else {
            cell = document.createTextNode(raw == null ? "" : String(raw));
          }
          return h("td", {
            class: c.className,
            style: c.align ? { textAlign: c.align } : null,
          }, cell);
        }),
        rowActions
          ? h("td", { class: "is-actions" },
              rowActions(row).filter(Boolean).map((a) =>
                h("button", {
                  class: ["btn", "btn--sm", a.kind ? `btn--${a.kind}` : "btn--ghost"],
                  type: "button",
                  onclick: () => a.onClick(row),
                }, a.label)
              )
            )
          : null,
      ])
    )
  );

  container.appendChild(h("div", { class: "table-wrap" }, [
    h("table", { class: "table" }, [head, body]),
  ]));
}

function skeleton(rows = 4) {
  return h("div", { class: "table-wrap" }, [
    h("div", { style: { padding: "16px" } },
      Array.from({ length: rows }).map(() =>
        h("div", { class: "skeleton", style: { marginBottom: "12px", height: "16px" } })
      )
    ),
  ]);
}
