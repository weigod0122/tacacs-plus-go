// External-log redirect page — up to three buttons (authen / author / account)
// rendered per TACACS+ log type. Each opens the admin-configured URL in a new
// tab via window.open(... "noopener,noreferrer").
//
// Visibility rules:
//   - admin: always sees all 3 buttons; ones whose URL is empty are disabled
//     so admin can spot un-configured types at a glance.
//   - non-admin: only sees buttons where the per-type visibleX flag is true
//     AND the URL is non-empty — both gates must pass, otherwise the type is
//     hidden entirely (don't leak the existence of types they can't reach).
//
// If a non-admin has zero accessible types (either all visibility flags are
// off, or those that are on still have empty URLs), the page shows a "no
// access" message instead of an empty action bar.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { t } from "../core/i18n.js";

const TYPES = [
  { key: "authen",  labelKey: "log.redirect.authen",  visibleField: "visibleAuthen"  },
  { key: "author",  labelKey: "log.redirect.author",  visibleField: "visibleAuthor"  },
  { key: "account", labelKey: "log.redirect.account", visibleField: "visibleAccount" },
];

export default async function renderLogPage(container, ctx) {
  const statusHost = h("div", { class: "card__body stack" });
  const buttonsHost = h("div", { class: "page__actions" });

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("log.title")),
        h("p", { class: "page__subtitle" }, t("log.subtitle")),
      ]),
    ]),
    h("section", { class: "card" }, [
      statusHost,
      h("div", { class: "card__body" }, [buttonsHost]),
    ]),
  ]));

  let cfg;
  try {
    const res = await api.get("/tacacs/system/log-redirect-config");
    cfg = (res && res.data) || {};
  } catch (err) {
    mount(statusHost, h("p", { class: "text-danger" },
      t("log.redirect.loadFailed") + (err.message || err)));
    return;
  }

  const isAdmin = !!(ctx && ctx.isAdmin);

  // 决定本次要渲染的 TYPES 子集:admin 全展示;非 admin 只看 visible && url 都满足的项
  const visibleTypes = isAdmin
    ? TYPES
    : TYPES.filter((typ) => !!cfg[typ.visibleField] && !!cfg[typ.key]);

  if (visibleTypes.length === 0) {
    // admin 走到这里:三个 URL 全为空,提示去系统设置配置
    // 非 admin 走到这里:可见的 0 个,提示无访问权限
    const msg = isAdmin ? t("log.redirect.notConfigured") : t("log.redirect.forbidden");
    mount(statusHost, h("p", { class: "page__subtitle" }, msg));
    return;
  }

  mount(statusHost, h("p", { class: "page__subtitle" },
    t("log.redirect.pickType")));

  mount(buttonsHost, ...visibleTypes.map((typ) => {
    const url = cfg[typ.key] || "";
    return h("button", {
      class: "btn btn--primary",
      type: "button",
      disabled: !url,
      title: url || t("log.redirect.typeNotConfigured"),
      onclick: () => {
        if (url) window.open(url, "_blank", "noopener,noreferrer");
      },
    }, t(typ.labelKey));
  }));
}
