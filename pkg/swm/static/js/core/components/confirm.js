// Confirm dialog wrapping openModal — Promise-based.

import { openModal } from "./modal.js";
import { t } from "../i18n.js";

export function confirm({
  title,
  message,
  confirmLabel,
  cancelLabel,
  danger = false,
  size = "sm",
} = {}) {
  const titleText = title != null ? title : t("common.confirmTitle");
  const confirmText = confirmLabel != null ? confirmLabel : t("common.confirm");
  const cancelText = cancelLabel != null ? cancelLabel : t("common.cancel");
  return new Promise((resolve) => {
    const handle = openModal({
      title: titleText,
      size,
      body: typeof message === "string"
        ? document.createTextNode(message)
        : message,
      actions: [
        {
          label: cancelText, kind: "ghost",
          onClick: (close) => { close(); resolve(false); },
        },
        {
          label: confirmText, kind: danger ? "danger" : "primary",
          onClick: (close) => { close(); resolve(true); },
        },
      ],
    });

    // If user dismisses via overlay/Escape/X without picking, treat as cancel.
    const obs = new MutationObserver(() => {
      if (!handle.overlay.isConnected) {
        obs.disconnect();
        resolve(false);
      }
    });
    obs.observe(document.body, { childList: true });
  });
}
