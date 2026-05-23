// Tiny DOM helpers — replace jQuery for the limited set of things we need.

export const qs = (selector, root = document) => root.querySelector(selector);
export const qsa = (selector, root = document) => Array.from(root.querySelectorAll(selector));

export function on(target, event, handler, options) {
  target.addEventListener(event, handler, options);
  return () => target.removeEventListener(event, handler, options);
}

const SVG_NS = "http://www.w3.org/2000/svg";
const SVG_TAGS = new Set(["svg", "path", "circle", "rect", "line", "utils"]);

/**
 * h(tag, attrs, children) — safe element creation.
 * - attrs: object; values are set via setAttribute except for "style" (object), "dataset" (object),
 *   event handlers (key starts with "on", camelCase), and "ref" (callback).
 * - children: string | Node | array of these. Strings become text nodes (no innerHTML).
 */
export function h(tag, attrs = {}, children = []) {
  const el = SVG_TAGS.has(tag)
    ? document.createElementNS(SVG_NS, tag)
    : document.createElement(tag);

  for (const [key, value] of Object.entries(attrs || {})) {
    if (value == null || value === false) continue;

    if (key === "class" || key === "className") {
      el.className = Array.isArray(value) ? value.filter(Boolean).join(" ") : String(value);
    } else if (key === "style" && typeof value === "object") {
      Object.assign(el.style, value);
    } else if (key === "dataset" && typeof value === "object") {
      for (const [dk, dv] of Object.entries(value)) {
        if (dv != null) el.dataset[dk] = String(dv);
      }
    } else if (key === "ref" && typeof value === "function") {
      value(el);
    } else if (key.startsWith("on") && typeof value === "function") {
      const evt = key.slice(2).toLowerCase();
      el.addEventListener(evt, value);
    } else if (key === "html") {
      // explicit opt-in; only use with trusted/static content
      el.innerHTML = String(value);
    } else if (typeof value === "boolean") {
      if (value) el.setAttribute(key, "");
    } else {
      el.setAttribute(key, String(value));
    }
  }

  appendChildren(el, children);
  return el;
}

function appendChildren(el, children) {
  if (children == null || children === false) return;
  if (Array.isArray(children)) {
    children.forEach((c) => appendChildren(el, c));
    return;
  }
  if (children instanceof Node) {
    el.appendChild(children);
    return;
  }
  el.appendChild(document.createTextNode(String(children)));
}

export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

export function mount(parent, ...children) {
  clear(parent);
  appendChildren(parent, children);
  return parent;
}

/** A 16x16 unicode/inline icon (kept text-only to avoid extra assets). */
export function icon(symbol, label) {
  return h("span", {
    class: "sidebar__link-icon",
    "aria-hidden": label ? "false" : "true",
    "aria-label": label || null,
  }, symbol);
}

/** Format an ISO timestamp like "2024-09-01T08:30:00+08:00" → "2024-09-01 08:30:00". */
export function formatTime(value) {
  if (typeof value !== "string") return String(value ?? "");
  const t = value.indexOf("T");
  if (t === -1) return value;
  return (value.slice(0, t) + " " + value.slice(t + 1)).replace(/[+-]\d{2}:?\d{2}$/, "").slice(0, 19);
}
