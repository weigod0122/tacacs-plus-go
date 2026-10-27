// Client-side pager for lists that are already fully loaded in memory.
// The caller owns page / pageSize state and re-renders its rows on change;
// this component only draws the controls.
//   opts: { page, pageSize, total, pageSizes?, onChange({ page, pageSize }) }

import { h, mount } from "../dom.js";
import { t } from "../i18n.js";
import { toast } from "./toast.js";

export const DEFAULT_PAGE_SIZES = [10, 20, 50, 100];

export function totalPagesOf(total, pageSize) {
  return Math.max(1, Math.ceil(Math.max(0, total) / Math.max(1, pageSize)));
}

export function clampPage(page, pageSize, total) {
  const n = Number.isInteger(page) ? page : 1;
  return Math.min(Math.max(1, n), totalPagesOf(total, pageSize));
}

export function sliceRows(rows, page, pageSize) {
  const start = (page - 1) * pageSize;
  return rows.slice(start, start + pageSize);
}

export function renderPager(container, opts) {
  const { page, pageSize, total, pageSizes = DEFAULT_PAGE_SIZES, onChange } = opts;
  if (!total) {
    mount(container);
    return;
  }
  const totalPages = totalPagesOf(total, pageSize);
  const current = clampPage(page, pageSize, total);

  function goTo(next) {
    if (!Number.isInteger(next) || next < 1 || next > totalPages) {
      toast.error(t("pager.pageInvalid", { total: totalPages }));
      return;
    }
    if (next !== current) onChange({ page: next, pageSize });
  }

  const sizeSelect = h("select", { class: "select", style: { width: "auto", minWidth: "0" } },
    pageSizes.map((n) => h("option", { value: String(n), selected: n === pageSize }, String(n))));
  sizeSelect.value = String(pageSize);
  sizeSelect.addEventListener("change", () => {
    onChange({ page: 1, pageSize: Number(sizeSelect.value) || pageSize });
  });

  const jumpInput = h("input", {
    class: "input", type: "number", min: "1", max: String(totalPages), step: "1",
    value: String(current), style: { width: "72px", minWidth: "0" },
    "aria-label": t("pager.jumpTo"), disabled: totalPages <= 1,
  });
  const jump = () => goTo(Number(jumpInput.value));
  jumpInput.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      jump();
    }
  });

  mount(container, [
    h("span", { class: "page__subtitle", style: { whiteSpace: "nowrap" } }, t("pager.total", { n: total })),
    h("label", { class: "row", style: { gap: "var(--space-2)", whiteSpace: "nowrap" } }, [
      h("span", { class: "field__label", style: { margin: 0 } }, t("pager.pageSize")),
      sizeSelect,
    ]),
    h("button", { class: "btn btn--sm", type: "button", disabled: current <= 1, onclick: () => goTo(1) }, t("pager.first")),
    h("button", { class: "btn btn--sm", type: "button", disabled: current <= 1, onclick: () => goTo(current - 1) }, t("pager.prev")),
    h("span", { class: "page__subtitle", style: { whiteSpace: "nowrap" } },
      t("pager.pageOf", { n: current, total: totalPages })),
    h("button", { class: "btn btn--sm", type: "button", disabled: current >= totalPages, onclick: () => goTo(current + 1) }, t("pager.next")),
    h("button", { class: "btn btn--sm", type: "button", disabled: current >= totalPages, onclick: () => goTo(totalPages) }, t("pager.last")),
    h("label", { class: "row", style: { gap: "var(--space-2)", whiteSpace: "nowrap" } }, [
      h("span", { class: "field__label", style: { margin: 0 } }, t("pager.jumpTo")),
      jumpInput,
      h("button", { class: "btn btn--sm", type: "button", disabled: totalPages <= 1, onclick: jump }, t("pager.jump")),
    ]),
  ]);
}
