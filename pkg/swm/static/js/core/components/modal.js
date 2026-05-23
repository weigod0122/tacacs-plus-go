// Reusable modal — focus trap, ESC to close, click-outside to dismiss,
// declarative form spec or arbitrary body.
//
// Usage:
//   openModal({ title, body: Node | render(close), actions: [{label, kind, onClick}] })
//   openFormModal({ title, fields: [{name, label, type, ...}], onSubmit: async values => ... })

import { h, qs, qsa } from "../dom.js";
import { t } from "../i18n.js";

let openCount = 0;

const FOCUSABLE =
  'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function openModal({ title, body, actions, size = "" }) {
  const overlay = h("div", { class: "modal-overlay", role: "presentation" });
  const titleId = `modal-title-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;

  const close = (result) => destroy(overlay, prevFocus, result);

  const dialog = h("div", {
    class: ["modal", size && `modal--${size}`],
    role: "dialog",
    "aria-modal": "true",
    "aria-labelledby": titleId,
    tabindex: "-1",
    onclick: (e) => e.stopPropagation(),
  }, [
    h("div", { class: "modal__header" }, [
      h("h2", { class: "modal__title", id: titleId }, title),
      h("button", {
        class: "modal__close", type: "button",
        "aria-label": t("app.aria.close"), onclick: () => close({ confirmed: false }),
      }, "×"),
    ]),
    h("div", { class: "modal__body" }, [
      typeof body === "function" ? body(close) : body,
    ]),
    actions && actions.length
      ? h("div", { class: "modal__footer" }, actions.map((a) =>
          h("button", {
            class: ["btn", a.kind ? `btn--${a.kind}` : ""],
            type: "button",
            onclick: a.onClick ? () => a.onClick(close) : () => close({ confirmed: false }),
          }, a.label)
        ))
      : null,
  ]);

  overlay.appendChild(dialog);
  overlay.addEventListener("mousedown", (e) => {
    if (e.target === overlay) close({ confirmed: false });
  });

  const prevFocus = document.activeElement;
  document.body.appendChild(overlay);
  openCount += 1;
  if (openCount === 1) document.body.style.overflow = "hidden";

  // ESC + focus trap
  const onKey = (e) => {
    if (e.key === "Escape") {
      e.preventDefault();
      close({ confirmed: false });
      return;
    }
    if (e.key !== "Tab") return;
    const focusables = qsa(FOCUSABLE, dialog).filter((el) => el.offsetParent !== null);
    if (focusables.length === 0) return;
    const first = focusables[0];
    const last = focusables[focusables.length - 1];
    if (e.shiftKey && document.activeElement === first) {
      e.preventDefault(); last.focus();
    } else if (!e.shiftKey && document.activeElement === last) {
      e.preventDefault(); first.focus();
    }
  };
  document.addEventListener("keydown", onKey);
  overlay.dataset.keyHandler = "1";
  overlay._cleanupKey = () => document.removeEventListener("keydown", onKey);

  // Initial focus: first focusable other than the close button, fallback to dialog.
  const focusables = qsa(FOCUSABLE, dialog);
  const target = focusables.find((el) => !el.classList.contains("modal__close")) || focusables[0] || dialog;
  setTimeout(() => target.focus(), 30);

  return { close, dialog, overlay };
}

function destroy(overlay, prevFocus) {
  if (!overlay.isConnected) return;
  if (overlay._cleanupKey) overlay._cleanupKey();
  overlay.style.opacity = "0";
  overlay.style.transition = "opacity 120ms";
  setTimeout(() => {
    overlay.remove();
    openCount = Math.max(0, openCount - 1);
    if (openCount === 0) document.body.style.overflow = "";
    if (prevFocus && typeof prevFocus.focus === "function") {
      try { prevFocus.focus(); } catch {}
    }
  }, 120);
}

/**
 * openFormModal({ title, fields, onSubmit, submitLabel })
 * fields: [{ name, label, type='text', placeholder, required, hint, options, value }]
 *   - type: 'text' | 'password' | 'number' | 'date' | 'textarea' | 'select'
 *   - options (for select): [{value, label}]
 * onSubmit: async (values) => string | void.  Throw to keep modal open and show error.
 */
export function openFormModal({ title, fields, onSubmit, submitLabel, cancelLabel, submitKind = "primary", size = "" }) {
  const inputs = {};
  const submitText = submitLabel != null ? submitLabel : t("common.confirm");
  const cancelText = cancelLabel != null ? cancelLabel : t("common.cancel");
  const errorEl = h("p", { class: "field__hint field__hint--error", style: { display: "none" } });

  const formBody = h("form", {
    class: "stack",
    novalidate: true,
    onsubmit: (e) => { e.preventDefault(); submit(); },
  }, fields.map((f) => renderField(f, inputs)));

  formBody.appendChild(errorEl);

  let submitting = false;

  const handle = openModal({
    title,
    size,
    body: formBody,
    actions: [
      { label: cancelText, kind: "ghost", onClick: (close) => close({ confirmed: false }) },
      { label: submitText, kind: submitKind, onClick: () => submit() },
    ],
  });

  async function submit() {
    if (submitting) return;
    errorEl.style.display = "none";
    const values = {};
    for (const f of fields) {
      const el = inputs[f.name];
      let v = el.value;
      if (typeof v === "string") v = v.trim();
      const empty = v == null || v === "" || (Array.isArray(v) && v.length === 0);
      if (f.required && empty) {
        errorEl.textContent = f.type === "multiselect"
          ? t("form.err.requiredMulti", { label: f.label || f.name })
          : t("form.err.required",      { label: f.label || f.name });
        errorEl.style.display = "";
        el.focus();
        return;
      }
      values[f.name] = v;
    }
    submitting = true;
    try {
      await onSubmit(values, handle.close);
      handle.close({ confirmed: true, values });
    } catch (err) {
      errorEl.textContent = err && err.message ? err.message : String(err);
      errorEl.style.display = "";
    } finally {
      submitting = false;
    }
  }

  return handle;
}

function renderField(f, inputs) {
  const id = `f-${f.name}-${Math.random().toString(36).slice(2, 6)}`;
  let control;

  if (f.type === "textarea") {
    control = h("textarea", {
      id, class: "textarea", name: f.name, placeholder: f.placeholder || "",
      required: f.required, rows: f.rows || 3,
    }, f.value || "");
  } else if (f.type === "select") {
    control = h("select", { id, class: "select", name: f.name, required: f.required },
      [
        f.placeholder ? h("option", { value: "", disabled: true, selected: !f.value }, f.placeholder) : null,
        ...(f.options || []).map((o) =>
          h("option", { value: o.value, selected: f.value === o.value }, o.label)
        ),
      ]);
  } else if (f.type === "multiselect") {
    control = renderMultiSelect(f, id);
  } else {
    control = h("input", {
      id, class: "input", name: f.name,
      type: f.type || "text",
      placeholder: f.placeholder || "",
      required: f.required,
      value: f.value || "",
      autocomplete: f.autocomplete || "off",
      min: f.min, max: f.max, maxlength: f.maxlength,
    });
  }
  inputs[f.name] = control;

  return h("div", { class: "field" }, [
    h("label", { class: "field__label", for: id }, f.label || f.name),
    control,
    f.hint ? h("p", { class: "field__hint" }, f.hint) : null,
  ]);
}

function renderMultiSelect(f, id) {
  const options = f.options || [];
  const selected = new Set(Array.isArray(f.value) ? f.value : []);
  let isOpen = false;
  let outsideHandler = null;

  const trigger = h("button", {
    id, type: "button", class: "select multiselect__trigger",
    "aria-haspopup": "listbox", "aria-expanded": "false",
    onclick: (e) => { e.stopPropagation(); isOpen ? closePanel() : openPanel(); },
  });

  const search = options.length >= 8
    ? h("input", {
        type: "search", class: "input multiselect__search",
        placeholder: t("common.search"),
        oninput: (e) => filterPanel(e.target.value.toLowerCase()),
      })
    : null;

  const list = h("div", { class: "multiselect__list", role: "listbox", "aria-multiselectable": "true" });

  const allBtn = h("button", { type: "button", class: "btn btn--ghost btn--sm",
    onclick: (e) => { e.preventDefault(); options.forEach(o => selected.add(o.value)); refresh(); }
  }, t("common.selectAll"));
  const noneBtn = h("button", { type: "button", class: "btn btn--ghost btn--sm",
    onclick: (e) => { e.preventDefault(); selected.clear(); refresh(); }
  }, t("common.clear"));

  const panel = h("div", {
    class: "multiselect__panel",
    onclick: (e) => e.stopPropagation(),
  }, [
    search,
    h("div", { class: "multiselect__actions" }, [allBtn, noneBtn]),
    list,
  ]);

  const wrap = h("div", { class: "multiselect" }, [trigger, panel]);

  function refresh() {
    const arr = Array.from(selected);
    if (arr.length === 0) {
      trigger.textContent = f.placeholder || t("form.multiselect.placeholder");
      trigger.classList.add("is-empty");
    } else if (arr.length <= 2) {
      trigger.textContent = arr.join("、");
      trigger.classList.remove("is-empty");
    } else {
      trigger.textContent = t("form.multiselect.summary", {
        prefix: arr.slice(0, 2).join("、"),
        n: arr.length,
      });
      trigger.classList.remove("is-empty");
    }
    while (list.firstChild) list.removeChild(list.firstChild);
    options.forEach((opt) => {
      const checked = selected.has(opt.value);
      const item = h("label", {
        class: ["multiselect__item", checked && "is-checked"],
        dataset: { label: String(opt.label).toLowerCase() },
      }, [
        h("input", {
          type: "checkbox", value: opt.value, checked,
          onchange: (e) => {
            if (e.target.checked) selected.add(opt.value);
            else selected.delete(opt.value);
            refresh();
          },
        }),
        h("span", null, opt.label),
      ]);
      list.appendChild(item);
    });
  }

  function filterPanel(needle) {
    Array.from(list.children).forEach((item) => {
      const match = !needle || item.dataset.label.includes(needle);
      item.style.display = match ? "" : "none";
    });
  }

  function openPanel() {
    isOpen = true;
    panel.classList.add("is-open");
    trigger.setAttribute("aria-expanded", "true");
    if (search) setTimeout(() => search.focus(), 30);
    outsideHandler = (e) => { if (!wrap.contains(e.target)) closePanel(); };
    setTimeout(() => document.addEventListener("click", outsideHandler), 0);
    document.addEventListener("keydown", onEsc);
  }

  function closePanel() {
    isOpen = false;
    panel.classList.remove("is-open");
    trigger.setAttribute("aria-expanded", "false");
    if (outsideHandler) document.removeEventListener("click", outsideHandler);
    document.removeEventListener("keydown", onEsc);
  }

  function onEsc(e) {
    if (e.key === "Escape" && isOpen) {
      e.stopPropagation();
      closePanel();
      trigger.focus();
    }
  }

  Object.defineProperty(wrap, "value", { get: () => Array.from(selected) });
  wrap.focus = () => trigger.focus();

  refresh();
  return wrap;
}
