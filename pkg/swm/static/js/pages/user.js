// User management — admin sees everyone + close-account; non-admins typically
// won't reach this page (the sidebar entry is admin-only) but if forced here
// they only see their own row. Per-row password / notes use the shared
// user-actions module so the topbar avatar can reuse the same widgets.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { openFormModal, openModal } from "../core/components/modal.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { userStatusBadge, PASSWORD_RULES_KEYS, validatePassword, validateEmail, validatePhone, generatePassword } from "../core/format.js";
import { openPasswordModal, openNotesModal } from "../core/user-actions.js";
import { t, tHeader } from "../core/i18n.js";

export default async function renderUserPage(container, ctx) {
  const { username, isAdmin } = ctx;
  const tableHost = h("div");
  let allRows = [];

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("user.title")),
        h("p", { class: "page__subtitle" },
          isAdmin ? t("user.subtitle.admin") : t("user.subtitle.user")),
      ]),
      h("div", { class: "page__actions" }, [
        h("button", { class: "btn", type: "button", onclick: load }, t("app.refresh")),
        isAdmin
          ? h("button", { class: "btn", type: "button", onclick: clearPasswordLockouts }, t("user.btn.unlock"))
          : null,
        isAdmin
          ? h("button", { class: "btn btn--primary", type: "button", onclick: openCreateModal }, t("user.btn.create"))
          : null,
      ]),
    ]),
    tableHost,
  ]));

  function visibleRows() {
    return isAdmin ? allRows : allRows.filter((r) => r.User === username);
  }

  function render() {
    const rows = visibleRows();
    const cols = inferColumns(rows);
    renderTable(tableHost, {
      columns: cols,
      rows,
      emptyText: isAdmin ? t("user.empty.admin") : t("user.empty.user"),
      rowActions: (row) => actionsFor(row),
    });
  }

  function actionsFor(row) {
    // Status values are canonical Chinese strings supplied by the backend;
    // we compare against them directly regardless of UI locale.
    const active = row.Status === "使用中" || row.Status === "暂停使用";
    if (!active) return [];
    const items = [
      { label: t("user.btn.password"), kind: "ghost", onClick: () => openPasswordModal(row.User) },
      { label: t("user.btn.notes"),    kind: "ghost", onClick: () => openNotesModal(row.User, row.Notes || "", load) },
    ];
    if (isAdmin && row.User !== username) {
      items.push({ label: t("user.btn.reset"),   kind: "ghost",  onClick: () => openResetPasswordModal(row.User) });
      items.push({ label: t("user.btn.disable"), kind: "danger", onClick: () => deleteUser(row.User) });
    }
    return items;
  }

  function openResetPasswordModal(targetUser) {
    openFormModal({
      title: t("user.reset.title", { user: targetUser }),
      submitLabel: t("user.reset.submit"),
      submitKind: "danger",
      fields: [
        {
          name: "operator_password",
          label: t("user.reset.opPwLabel"),
          type: "password",
          required: true,
          maxlength: 128,
          hint: t("user.reset.opPwHint"),
        },
      ],
      onSubmit: async (values) => {
        try {
          await api.post("/tacacs/user/check", {
            user: username,
            password: values.operator_password,
          });
        } catch (err) {
          throw new Error(t("user.reset.opPwFail"));
        }

        const newPassword = generatePassword(14);
        try {
          await api.post("/tacacs/user/reset/password", {
            operator: username,
            operator_password: values.operator_password,
            user: targetUser,
            password: newPassword,
          });
        } catch (err) {
          throw new Error(err.message || t("user.reset.failed"));
        }
        showResetResult(targetUser, newPassword);
        await load();
      },
    });
  }

  function showResetResult(targetUser, password) {
    const passwordEl = h("div", {
      style: {
        padding: "var(--space-4)",
        background: "var(--color-surface-muted)",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--radius-md)",
        fontFamily: "var(--font-mono)",
        fontSize: "var(--font-size-md)",
        textAlign: "center",
        letterSpacing: "1px",
        wordBreak: "break-all",
        userSelect: "all",
      },
    }, password);

    const copyBtn = h("button", {
      class: "btn btn--primary",
      type: "button",
      onclick: async () => {
        try {
          await navigator.clipboard.writeText(password);
          toast.success(t("user.reset.done.copyOk"));
        } catch {
          const range = document.createRange();
          range.selectNodeContents(passwordEl);
          const sel = window.getSelection();
          sel.removeAllRanges();
          sel.addRange(range);
          toast.warning(t("user.reset.done.copyFallback"));
        }
      },
    }, t("user.reset.done.copy"));

    openModal({
      title: t("user.reset.done.title"),
      body: h("div", { class: "stack" }, [
        h("p", { style: { margin: 0 } }, t("user.reset.done.intro", { user: targetUser })),
        passwordEl,
        h("div", { class: "row row--end" }, [copyBtn]),
        h("p", { class: "field__hint" }, t("user.reset.done.warn")),
      ]),
      actions: [
        { label: t("user.reset.done.ok"), kind: "primary", onClick: (close) => close() },
      ],
    });
  }

  function openCreateModal() {
    openFormModal({
      title: t("user.create.title"),
      fields: [
        { name: "user",  label: t("user.create.user"),  required: true, placeholder: t("user.create.userPh"), maxlength: 64 },
        { name: "email", label: t("user.create.email"), type: "email", required: true,
          placeholder: "you@example.com", maxlength: 128 },
        { name: "phone_number", label: t("user.create.phone"), type: "tel", required: true,
          placeholder: "13800138000", maxlength: 32 },
        {
          name: "password", label: t("user.create.password"), type: "password", required: true,
          placeholder: t("user.create.passwordPh"), maxlength: 128,
          hint: PASSWORD_RULES_KEYS.map((k) => t(k)).join("；"),
        },
        { name: "notes", label: t("user.create.notes"), placeholder: t("user.create.notesPh"), maxlength: 200 },
      ],
      onSubmit: async (values) => {
        const emError = validateEmail(values.email);
        if (emError) throw new Error(emError);
        const phError = validatePhone(values.phone_number);
        if (phError) throw new Error(phError);
        const pwError = validatePassword(values.password);
        if (pwError) throw new Error(pwError);
        try {
          await api.post("/tacacs/user/create", values);
          toast.success(t("user.create.toast.ok"));
          await load();
        } catch (err) {
          throw new Error(err.message || t("user.create.toast.failed"));
        }
      },
    });
  }

  async function deleteUser(targetUser) {
    const ok = await confirm({
      title: t("user.disable.title"),
      message: t("user.disable.msg", { user: targetUser }),
      confirmLabel: t("user.btn.disable"),
      danger: true,
    });
    if (!ok) return;
    try {
      await api.del("/tacacs/user/delete", { user: targetUser });
      toast.success(t("user.disable.ok"));
      await load();
    } catch (err) {
      toast.error(err.message || t("user.disable.failed"));
    }
  }

  async function clearPasswordLockouts() {
    const ok = await confirm({
      title: t("user.unlock.title"),
      message: t("user.unlock.msg"),
      confirmLabel: t("user.unlock.btn"),
      danger: true,
    });
    if (!ok) return;
    try {
      await Promise.all([
        api.get("/tacacs/user/clear/checkPasswordErrMap"),
        api.get("/tacacs/user/clear/updatePasswordErrMap"),
      ]);
      toast.success(t("user.unlock.ok"));
    } catch (err) {
      toast.error(t("user.unlock.failed") + (err.message || err));
    }
  }

  async function load() {
    renderTable(tableHost, { columns: [], rows: [], loading: true });
    try {
      const res = await api.get("/tacacs/user/get");
      allRows = (res && res.data) || [];
      render();
    } catch (err) {
      mount(tableHost, h("div", { class: "card" }, [
        h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
      ]));
    }
  }

  await load();
}

function inferColumns(rows) {
  if (rows.length === 0) {
    return [
      { key: "User",   label: tHeader("User") },
      { key: "Status", label: tHeader("Status"), render: userStatusBadge },
      { key: "Notes",  label: tHeader("Notes") },
    ];
  }
  return Object.keys(rows[0]).map((k) => {
    if (k === "Status") return { key: k, label: tHeader("Status"), render: userStatusBadge };
    return { key: k, label: tHeader(k) };
  });
}
