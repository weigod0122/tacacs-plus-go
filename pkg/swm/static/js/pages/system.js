// System / settings page — admin-only operations that don't fit other pages.
// Currently exposes one action: force-bump tacacs_meta versions so all
// clients rebuild their local caches on the next 2s poll.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { t } from "../core/i18n.js";

export default async function renderSystemPage(container) {
  const refreshBtn = h("button", {
    class: "btn btn--primary",
    type: "button",
    onclick: refreshMeta,
  }, t("system.meta.btn"));

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
  ]));

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
}
