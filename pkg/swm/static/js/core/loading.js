// Tiny top-of-page loading bar. Stacks concurrent requests; only goes away when all settle.

let active = 0;
let bar;

function ensureBar() {
  if (bar) return bar;
  bar = document.createElement("div");
  bar.className = "loading-bar";
  bar.setAttribute("aria-hidden", "true");
  document.body.appendChild(bar);
  return bar;
}

export function startLoading() {
  active += 1;
  const el = ensureBar();
  el.style.opacity = "1";
  el.style.width = active === 1 ? "20%" : "70%";
}

export function stopLoading() {
  active = Math.max(0, active - 1);
  if (active > 0) return;
  const el = ensureBar();
  el.style.width = "100%";
  setTimeout(() => {
    if (active === 0) {
      el.style.opacity = "0";
      el.style.width = "0";
    }
  }, 200);
}
