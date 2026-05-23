// Shared user-account actions: change password, edit notes, view profile.
// Used by both the user management page (per-row actions) and the topbar
// avatar (current user's profile).

import { api } from "./api.js";
import { h, mount } from "./dom.js";
import { openFormModal, openModal } from "./components/modal.js";
import { toast } from "./components/toast.js";
import { userStatusBadge, validatePassword, validateEmail, validatePhone } from "./format.js";
import { buildPasswordMeter } from "./components/password-meter.js";
import { openRoleDetailModal } from "./template-viewer.js";
import { t, tHeader } from "./i18n.js";

export function openPasswordModal(targetUser, onSaved) {
  const oldInput = h("input", {
    type: "password", class: "input", maxlength: 128, autocomplete: "current-password",
  });
  const newInput = h("input", {
    type: "password", class: "input", maxlength: 128, autocomplete: "new-password",
  });
  const confirmInput = h("input", {
    type: "password", class: "input", maxlength: 128, autocomplete: "new-password",
  });
  const meter = buildPasswordMeter(newInput);
  const errorEl = h("p", {
    class: "field__hint field__hint--error",
    style: { display: "none" },
  });

  let submitting = false;

  function field(label, control, extra) {
    return h("div", { class: "field" }, [
      h("label", { class: "field__label" }, label),
      control,
      extra,
    ]);
  }

  const handle = openModal({
    title: t("ua.password.title", { user: targetUser }),
    body: h("form", {
      class: "stack",
      novalidate: true,
      onsubmit: (e) => { e.preventDefault(); submit(); },
    }, [
      field(t("ua.password.old"), oldInput),
      field(t("ua.password.new"), newInput, meter),
      field(t("ua.password.confirm"), confirmInput),
      errorEl,
    ]),
    actions: [
      { label: t("common.cancel"), kind: "ghost", onClick: (close) => close() },
      { label: t("common.confirm"), kind: "primary", onClick: () => submit() },
    ],
  });

  async function submit() {
    if (submitting) return;
    errorEl.style.display = "none";

    const oldPw = oldInput.value;
    const newPw = newInput.value;
    const confirmPw = confirmInput.value;

    if (!oldPw) return showError(t("ua.password.err.oldEmpty"), oldInput);
    if (!newPw) return showError(t("ua.password.err.newEmpty"), newInput);

    const pwError = validatePassword(newPw);
    if (pwError) return showError(pwError, newInput);
    if (newPw !== confirmPw) return showError(t("ua.password.err.confirmMismatch"), confirmInput);
    if (newPw === oldPw)     return showError(t("ua.password.err.sameAsOld"), newInput);

    submitting = true;
    try {
      await api.post("/tacacs/user/update/password", {
        user: targetUser,
        oldPassword: oldPw,
        newPassword: newPw,
      });
      handle.close();
      toast.success(t("ua.password.toast.ok"));
      if (onSaved) await onSaved();
    } catch (err) {
      showError(err.message || t("ua.password.toast.fail"));
    } finally {
      submitting = false;
    }
  }

  function showError(msg, focusEl) {
    errorEl.textContent = msg;
    errorEl.style.display = "";
    if (focusEl) focusEl.focus();
  }
}

export function openNotesModal(targetUser, currentNotes = "", onSaved) {
  openFormModal({
    title: t("ua.notes.title", { user: targetUser }),
    fields: [
      {
        name: "notes", label: t("ua.notes.label"), type: "textarea", required: true, maxlength: 200,
        value: currentNotes,
      },
    ],
    onSubmit: async (values) => {
      try {
        await api.post("/tacacs/user/update/notes", {
          user: targetUser, notes: values.notes,
        });
        toast.success(t("ua.notes.toast.ok"));
        if (onSaved) await onSaved();
      } catch (err) {
        throw new Error(err.message || t("ua.notes.toast.fail"));
      }
    },
  });
}

export function openBasicInfoModal(targetUser, currentEmail = "", currentPhone = "", onSaved) {
  openFormModal({
    title: t("ua.basic.title", { user: targetUser }),
    fields: [
      {
        name: "email", label: t("ua.basic.email"), type: "email", required: true,
        maxlength: 128, value: currentEmail, placeholder: "you@example.com",
      },
      {
        name: "phone_number", label: t("ua.basic.phone"), type: "tel", required: true,
        maxlength: 32, value: currentPhone, placeholder: "13800138000",
      },
    ],
    onSubmit: async (values) => {
      const emError = validateEmail(values.email);
      if (emError) throw new Error(emError);
      const phError = validatePhone(values.phone_number);
      if (phError) throw new Error(phError);
      try {
        await api.post("/tacacs/user/update/basicInfo", {
          user: targetUser,
          email: values.email,
          phone_number: values.phone_number,
        });
        toast.success(t("ua.basic.toast.ok"));
        if (onSaved) await onSaved();
      } catch (err) {
        throw new Error(err.message || t("ua.basic.toast.fail"));
      }
    },
  });
}

async function fetchUserInfo(user) {
  const res = await api.get("/tacacs/user/get");
  return ((res && res.data) || []).find((r) => r.User === user) || null;
}

/** Approved roles currently held by this user. Status === 4 means approved. */
export async function getUserRoles(user) {
  try {
    const res = await api.get("/tacacs/approval/get");
    const rows = (res && res.data) || [];
    const roles = rows
      .filter((r) => r.User === user && r.Status === 4)
      .map((r) => r.ApprovalPermissions);
    return Array.from(new Set(roles.filter(Boolean)));
  } catch {
    return [];
  }
}

export { fetchUserInfo };

export async function openProfileModal(user) {
  let info;
  let roles = [];
  try {
    [info, roles] = await Promise.all([
      fetchUserInfo(user),
      getUserRoles(user),
    ]);
  } catch (err) {
    toast.error(t("ua.profile.loadFail") + (err.message || err));
    return;
  }
  if (!info) {
    toast.warning(t("ua.profile.notFound", { user }));
    return;
  }

  const bodyHost = h("div");

  function renderBody() {
    const items = [];
    for (const [k, v] of Object.entries(info)) {
      const valueNode = k === "Status"
        ? userStatusBadge(v)
        : document.createTextNode(String(v == null || v === "" ? "—" : v));
      items.push(
        h("span", { class: "kv__label" }, tHeader(k)),
        h("span", { class: "kv__value" }, valueNode),
      );
    }
    items.push(
      h("span", { class: "kv__label" }, t("ua.profile.label.roles")),
      h("span", { class: "kv__value" },
        roles.length
          ? h("div", {
              style: { display: "flex", flexWrap: "wrap", gap: "var(--space-1)" },
            }, roles.map((r) => h("button", {
              class: "chip-btn chip-btn--role",
              type: "button",
              title: t("approval.role.viewDetail", { role: r }),
              onclick: () => openRoleDetailModal(r),
            }, r)))
          : document.createTextNode("—")
      ),
    );
    mount(bodyHost,
      h("div", {
        class: "kv",
        style: {
          padding: "var(--space-4)",
          background: "var(--color-surface-muted)",
          borderRadius: "var(--radius-md)",
        },
      }, items)
    );
  }

  async function refresh() {
    try {
      [info, roles] = await Promise.all([
        fetchUserInfo(user),
        getUserRoles(user),
      ]);
      if (info) renderBody();
    } catch {/* swallow — toast shown elsewhere */}
  }

  renderBody();

  openModal({
    title: t("ua.profile.title"),
    body: h("div", { class: "stack" }, [
      bodyHost,
      h("div", { class: "row row--end", style: { gap: "var(--space-2)", flexWrap: "wrap" } }, [
        h("button", {
          class: "btn", type: "button",
          onclick: () => openBasicInfoModal(
            user,
            info.Email || "",
            info.phone_number || info.PhoneNumber || "",
            refresh,
          ),
        }, t("ua.profile.btn.basic")),
        h("button", {
          class: "btn", type: "button",
          onclick: () => openNotesModal(user, info.Notes || "", refresh),
        }, t("ua.profile.btn.notes")),
        h("button", {
          class: "btn btn--primary", type: "button",
          onclick: () => openPasswordModal(user),
        }, t("ua.profile.btn.password")),
      ]),
    ]),
    actions: [
      { label: t("common.close"), kind: "ghost", onClick: (close) => close() },
    ],
  });
}
