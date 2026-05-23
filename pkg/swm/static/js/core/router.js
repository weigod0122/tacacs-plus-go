// Tiny hash router — registers (route -> render(container, ctx)) and re-renders on hashchange.
// Each render call is given a fresh, empty container.

const routes = new Map();
let defaultRoute = "";
let main;
let onChange;
let currentCtx;

export function register(name, render) {
  routes.set(name, render);
}

export function start({ container, fallback, ctx, onNavigate }) {
  main = container;
  defaultRoute = fallback;
  onChange = onNavigate;
  currentCtx = ctx;
  window.addEventListener("hashchange", () => renderCurrent(currentCtx));
  renderCurrent(currentCtx);
}

function renderCurrent(ctx) {
  const hash = (location.hash || `#${defaultRoute}`).slice(1);
  const r = routes.get(hash) || routes.get(defaultRoute);
  if (!r) return;
  while (main.firstChild) main.removeChild(main.firstChild);
  main.classList.remove("fade-in");
  void main.offsetWidth;
  main.classList.add("fade-in");
  if (onChange) onChange(hash);
  r(main, ctx);
}

export function navigate(name) {
  if (location.hash === `#${name}`) return;
  location.hash = `#${name}`;
}

export function currentRoute() {
  return (location.hash || `#${defaultRoute}`).slice(1);
}

/** Re-render the current route in place (used by locale switching). */
export function rerender() {
  if (main) renderCurrent(currentCtx);
}
