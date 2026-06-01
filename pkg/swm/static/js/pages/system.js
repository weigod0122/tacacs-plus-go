// System / settings page — admin-only operations that don't fit other pages.
// Two sections:
//   1) Force-bump tacacs_meta versions so all clients rebuild their local
//      caches on the next 2-second poll.
//   2) Configure the external log-redirect URLs (one per TACACS+ log type:
//      authen / author / account) and a per-type "visible to non-admin"
//      checkbox — each log type's visibility is controlled independently so
//      admins can e.g. open accounting to all users while keeping authen
//      admin-only.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { t } from "../core/i18n.js";

const TYPES = [
  { key: "authen",  labelKey: "system.logRedirect.authen",  visibleField: "visibleAuthen"  },
  { key: "author",  labelKey: "system.logRedirect.author",  visibleField: "visibleAuthor"  },
  { key: "account", labelKey: "system.logRedirect.account", visibleField: "visibleAccount" },
];

export default async function renderSystemPage(container) {
  const refreshBtn = h("button", {
    class: "btn btn--primary",
    type: "button",
    onclick: refreshMeta,
  }, t("system.meta.btn"));

  const urlInputs = {};
  const visibleInputs = {};
  for (const typ of TYPES) {
    urlInputs[typ.key] = h("input", {
      type: "url",
      class: "input",
      placeholder: t("system.logRedirect.placeholder"),
      style: { minWidth: "320px", maxWidth: "560px", flex: "1" },
    });
    visibleInputs[typ.key] = h("input", {
      type: "checkbox",
      class: "checkbox",
    });
  }
  const saveBtn = h("button", {
    class: "btn btn--primary",
    type: "button",
    onclick: saveLogRedirectConfig,
  }, t("system.logRedirect.btn"));

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("system.title")),
        h("p", { class: "page__subtitle" }, t("system.subtitle")),
      ]),
    ]),
    h("section", { class: "card" }, [
      h("header", { class: "card__header" }, [
        h("h2", { class: "card__title" }, t("system.meta.title")),
      ]),
      h("div", { class: "card__body stack" }, [
        h("p", { class: "page__subtitle" }, t("system.meta.desc")),
        h("div", { class: "page__actions" }, [refreshBtn]),
      ]),
    ]),
    h("section", { class: "card" }, [
      h("header", { class: "card__header" }, [
        h("h2", { class: "card__title" }, t("system.logRedirect.title")),
      ]),
      h("div", { class: "card__body stack" }, [
        h("p", { class: "page__subtitle" }, t("system.logRedirect.desc")),
        ...TYPES.map((typ) => h("div", { class: "toolbar", style: { gap: "12px", flexWrap: "wrap" } }, [
          h("label", { class: "field__label", style: { minWidth: "80px" } }, t(typ.labelKey)),
          urlInputs[typ.key],
          h("label", { class: "toolbar", style: { gap: "6px", margin: 0 } }, [
            visibleInputs[typ.key],
            h("span", { class: "page__subtitle", style: { margin: 0 } }, t("system.logRedirect.visiblePerType")),
          ]),
        ])),
        h("div", { class: "page__actions" }, [saveBtn]),
      ]),
    ]),
  ]));

  // Best-effort prefill — failure surfaces a toast but does not block the page.
  try {
    const res = await api.get("/tacacs/system/log-redirect-config");
    const cfg = (res && res.data) || {};
    for (const typ of TYPES) {
      urlInputs[typ.key].value = cfg[typ.key] || "";
      visibleInputs[typ.key].checked = !!cfg[typ.visibleField];
    }
  } catch (err) {
    toast.error(t("system.logRedirect.loadFail") + (err.message || err));
  }

  async function refreshMeta() {
    const ok = await confirm({
      title: t("system.meta.confirmTitle"),
      message: t("system.meta.confirmMsg"),
      confirmLabel: t("system.meta.btn"),
    });
    if (!ok) return;

    refreshBtn.disabled = true;
    try {
      await api.post("/tacacs/meta/refresh");
      toast.success(t("system.meta.toast.ok"));
    } catch (err) {
      toast.error(t("system.meta.toast.fail") + (err.message || err));
    } finally {
      refreshBtn.disabled = false;
    }
  }

  async function saveLogRedirectConfig() {
    const body = {};
    for (const typ of TYPES) {
      const v = (urlInputs[typ.key].value || "").trim();
      if (v && !/^https?:\/\//i.test(v)) {
        toast.error(t(typ.labelKey) + " " + t("system.logRedirect.invalid"));
        return;
      }
      body[typ.key] = v;
      body[typ.visibleField] = !!visibleInputs[typ.key].checked;
    }
    saveBtn.disabled = true;
    try {
      await api.post("/tacacs/system/log-redirect-config", body);
      toast.success(t("system.logRedirect.toast.ok"));
    } catch (err) {
      toast.error(t("system.logRedirect.toast.fail") + (err.message || err));
    } finally {
      saveBtn.disabled = false;
    }
  }
}
