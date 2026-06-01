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
  // 三种日志类型各自是否对普通用户开放(来自后端 /tacacs/system/log-redirect-config
  // 的 visibleAuthen / visibleAuthor / visibleAccount 字段)。bootstrap 阶段异步拉取,
  // 失败时全部默认 false(隐藏入口)。侧栏「操作日志」只要有一项 visible+url 都齐
  // 就出现,管理员永远可见。
  logVisibility: { authen: false, author: false, account: false },
  // 配套的 URL 表(只用于侧栏判断"该类型是否真的可点"——visible=true 但 URL 还
  // 没填的中间态不应让用户看到入口然后点进去面对空页)。
  logUrls: { authen: "", author: "", account: "" },
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
      // log 不再无条件 adminOnly:任意一个类型同时打开了 visibleX 开关并配上了 URL,
      // 普通用户都能看到入口。具体是哪几个按钮可点交给 log.js 二次过滤。
      { id: "log",      labelKey: "nav.log",      icon: "≡", render: renderLogPage },
      { id: "system",   labelKey: "nav.system",   icon: "⚙", render: renderSystemPage, adminOnly: true },
    ],
  },
];

// hasAnyAccessibleLog 判断侧栏是否应该给普通用户展示「操作日志」入口。
// 任意一种日志类型 visibleX=true 且 URL 非空即返回 true;全部空或全部隐藏则 false。
function hasAnyAccessibleLog() {
  return ["authen", "author", "account"].some(
    (k) => ctx.logVisibility[k] && !!ctx.logUrls[k]
  );
}

// isNavItemVisible 统一封装侧栏 / 路由注册 / fallback 三处的可见性判定,
// 让 log 入口的"动态可见性"规则只活在一处。
function isNavItemVisible(it) {
  if (it.adminOnly && !ctx.isAdmin) return false;
  if (it.id === "log" && !ctx.isAdmin && !hasAnyAccessibleLog()) return false;
  return true;
}

function buildSidebar() {
  const nav = qs("#sidebar-nav");
  if (!nav) return;

  const sidebar = qs("#sidebar-aside");
  if (sidebar) sidebar.setAttribute("aria-label", t("app.aria.mainNav"));

  const brand = qs("#sidebar-brand-text");
  if (brand) brand.textContent = t("app.title");

  const visibleItems = NAV_ITEMS.map((g) => ({
    ...g,
    items: g.items.filter(isNavItemVisible),
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
      if (!isNavItemVisible(it)) continue;
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

async function bootstrap() {
  // Replace the static "正在加载…" placeholder so it doesn't flash in the
  // wrong language between bootstrap and the first render.
  const initial = qs("#initial-loading");
  if (initial) initial.textContent = t("app.loading");

  // 先拉一次外部日志跳转配置,拿到三个 visibleX 开关 + 三个 URL 决定普通用户侧栏
  // 是否展示「操作日志」入口。后端 GET 对所有已登录用户开放;失败时保持默认全 false
  // (隐藏)。必须 await:buildSidebar/registerRoutes/fallback 都依赖这些值。
  try {
    const res = await api.get("/tacacs/system/log-redirect-config");
    const cfg = (res && res.data) || {};
    ctx.logVisibility.authen  = !!cfg.visibleAuthen;
    ctx.logVisibility.author  = !!cfg.visibleAuthor;
    ctx.logVisibility.account = !!cfg.visibleAccount;
    ctx.logUrls.authen  = cfg.authen  || "";
    ctx.logUrls.author  = cfg.author  || "";
    ctx.logUrls.account = cfg.account || "";
  } catch {
    ctx.logVisibility = { authen: false, author: false, account: false };
    ctx.logUrls = { authen: "", author: "", account: "" };
  }

  buildSidebar();
  registerRoutes();
  wireLogout();
  wireProfile();
  wireLanguageToggle();
  hydrateUserPanel();
  wireGlobalErrorHandler();
  startSessionWatch();

  const fallback = NAV_ITEMS[0].items.find(isNavItemVisible).id;
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
