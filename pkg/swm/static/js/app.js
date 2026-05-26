// 网络设备权限系统 frontend entry — wires sidebar navigation, router, session watch.

import { h, mount, qs } from "./core/dom.js";
import { register, start, navigate, currentRoute, rerender } from "./core/router.js";
import { startSessionWatch } from "./core/session.js";
import { toast } from "./core/components/toast.js";
import { api } from "./core/api.js";
import { confirm } from "./core/components/confirm.js";
import { openProfileModal, fetchUserInfo } from "./core/user-actions.js";
import { userStatusBadge } from "./core/format.js";
import { t, getLocale, toggleLocale, onLocaleChange } from "./core/i18n.js";

import renderApprovalPage from "./pages/approval.js";
import renderUserPage from "./pages/user.js";
import renderRolePage from "./pages/role.js";
import renderServerPage from "./pages/server.js";
import renderCommandPage from "./pages/command.js";
import renderLogPage from "./pages/log.js";
import renderSystemPage from "./pages/system.js";

const ctx = {
  username: document.body.dataset.username || "",
  isAdmin: document.body.dataset.isAdmin === "1",
};

const NAV_ITEMS = [
  {
    labelKey: "nav.group.tacacs",
    items: [
      { id: "approval", labelKey: "nav.approval", icon: "✓", render: renderApprovalPage },
      { id: "user",     labelKey: "nav.user",     icon: "◎", render: renderUserPage, adminOnly: true },
      { id: "role",     labelKey: "nav.role",     icon: "◇", render: renderRolePage },
      { id: "server",   labelKey: "nav.server",   icon: "▤", render: renderServerPage },
      { id: "command",  labelKey: "nav.command",  icon: "›_", render: renderCommandPage },
      { id: "log",      labelKey: "nav.log",      icon: "≡", render: renderLogPage, adminOnly: true },
      { id: "system",   labelKey: "nav.system",   icon: "⚙", render: renderSystemPage, adminOnly: true },
    ],
  },
];

function buildSidebar() {
  const nav = qs("#sidebar-nav");
  if (!nav) return;

  const sidebar = qs("#sidebar-aside");
  if (sidebar) sidebar.setAttribute("aria-label", t("app.aria.mainNav"));

  const brand = qs("#sidebar-brand-text");
  if (brand) brand.textContent = t("app.title");

  const visibleItems = NAV_ITEMS.map((g) => ({
    ...g,
    items: g.items.filter((it) => !it.adminOnly || ctx.isAdmin),
  }));

  mount(nav, ...visibleItems.map((group) =>
    h("div", { class: "sidebar__group" }, [
      h("div", { class: "sidebar__group-label" }, t(group.labelKey)),
      h("div", { class: "sidebar__items" },
        group.items.map((item) =>
          h("button", {
            class: "sidebar__link",
            type: "button",
            dataset: { route: item.id },
            onclick: () => navigate(item.id),
          }, [
            h("span", { class: "sidebar__link-icon", "aria-hidden": "true" }, item.icon),
            h("span", null, t(item.labelKey)),
          ])
        )
      ),
    ])
  ));
}

function highlightSidebar(routeId) {
  document.querySelectorAll(".sidebar__link").forEach((el) => {
    el.classList.toggle("is-active", el.dataset.route === routeId);
  });
  const item = NAV_ITEMS.flatMap((g) => g.items).find((i) => i.id === routeId);
  const titleEl = qs("#topbar-title");
  const label = item ? t(item.labelKey) : t("app.title");
  if (titleEl) titleEl.textContent = label;
  document.title = item ? `${label} · ${t("app.title")}` : t("app.title");
}

function registerRoutes() {
  for (const group of NAV_ITEMS) {
    for (const it of group.items) {
      if (it.adminOnly && !ctx.isAdmin) continue;
      register(it.id, (container) => it.render(container, ctx));
    }
  }
}

function wireLogout() {
  const btn = qs("#logoutBtn");
  if (!btn) return;
  btn.addEventListener("click", async () => {
    const ok = await confirm({
      title: t("app.logout.confirmTitle"),
      message: t("app.logout.confirmMsg"),
      confirmLabel: t("app.logout.confirmBtn"),
    });
    if (!ok) return;
    window.location.href = "/logout";
  });
}

function wireProfile() {
  const btn = qs("#userBtn");
  if (!btn) return;
  btn.addEventListener("click", () => openProfileModal(ctx.username));
}

function wireLanguageToggle() {
  const btn = qs("#langBtn");
  if (!btn) return;
  applyLangBtn(btn);
  btn.addEventListener("click", () => toggleLocale());
}

function applyLangBtn(btn) {
  btn.textContent = t("app.langToggle");
  btn.title = t("app.langToggle.title");
}

let cachedUserInfo = null;

async function hydrateUserPanel() {
  const statusHost = qs("#userStatus");
  const userBtn = qs("#userBtn");
  if (!statusHost) return;

  try {
    cachedUserInfo = await fetchUserInfo(ctx.username);
  } catch {
    cachedUserInfo = null;
  }
  refreshUserPanel();
}

function refreshUserPanel() {
  const statusHost = qs("#userStatus");
  const userBtn = qs("#userBtn");
  const adminBadge = qs("#adminBadge");
  if (statusHost) {
    while (statusHost.firstChild) statusHost.removeChild(statusHost.firstChild);
    if (cachedUserInfo && cachedUserInfo.Status) {
      statusHost.appendChild(userStatusBadge(cachedUserInfo.Status));
    }
  }
  if (userBtn) {
    const parts = [ctx.username];
    const info = cachedUserInfo;
    if (info && info.Email)  parts.push(`${t("ua.basic.email")}: ${info.Email}`);
    const phone = info && (info.phone_number || info.PhoneNumber);
    if (phone)               parts.push(`${t("ua.basic.phone")}: ${phone}`);
    if (info && info.Notes)  parts.push(`${t("ua.notes.label")}: ${info.Notes}`);
    userBtn.title = info ? parts.join("\n") : t("app.userBtn.title");
  }
  if (adminBadge) {
    adminBadge.textContent = t("app.admin");
    adminBadge.title = t("app.admin");
  }
  const logoutBtn = qs("#logoutBtn");
  if (logoutBtn) logoutBtn.textContent = t("app.logout");
}

function wireGlobalErrorHandler() {
  window.addEventListener("unhandledrejection", (e) => {
    if (e.reason && e.reason.status === 401) return;
    toast.error(e.reason && e.reason.message ? e.reason.message : t("app.error.uncaught"));
  });
}

function rerenderRoute() {
  rerender();
}

function bootstrap() {
  // Replace the static "正在加载…" placeholder so it doesn't flash in the
  // wrong language between bootstrap and the first render.
  const initial = qs("#initial-loading");
  if (initial) initial.textContent = t("app.loading");

  buildSidebar();
  registerRoutes();
  wireLogout();
  wireProfile();
  wireLanguageToggle();
  hydrateUserPanel();
  wireGlobalErrorHandler();
  startSessionWatch();

  const fallback = NAV_ITEMS[0].items.find((i) => !i.adminOnly || ctx.isAdmin).id;
  start({
    container: qs("#app-main"),
    fallback,
    ctx,
    onNavigate: highlightSidebar,
  });

  onLocaleChange(() => {
    buildSidebar();
    const langBtn = qs("#langBtn");
    if (langBtn) applyLangBtn(langBtn);
    refreshUserPanel();
    highlightSidebar(currentRoute());
    rerenderRoute();
  });
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", bootstrap);
} else {
  bootstrap();
}
