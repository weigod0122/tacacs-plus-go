// IPv4 validators. Used by the server template page.

import { t } from "./i18n.js";

const IPV4_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})$/;
const IPV4_CIDR_RE = /^(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(?:\/(\d{1,2}))?$/;

/** validateIpv4(s) — null if valid IPv4, else a human-readable error. */
export function validateIpv4(s) {
  const m = s.match(IPV4_RE);
  if (!m) return t("net.ipv4.format", { src: s });
  for (let i = 1; i <= 4; i++) {
    const oct = Number(m[i]);
    if (oct < 0 || oct > 255) return t("net.ipv4.octet", { src: s, val: m[i] });
  }
  return null;
}

/** validateIpOrCidr(s) — null if valid IPv4 or IPv4 CIDR, else a human-readable error. */
export function validateIpOrCidr(s) {
  const m = s.match(IPV4_CIDR_RE);
  if (!m) {
    return t("net.cidr.format", { src: s });
  }
  for (let i = 1; i <= 4; i++) {
    const oct = Number(m[i]);
    if (oct < 0 || oct > 255) return t("net.ipv4.octet", { src: s, val: m[i] });
  }
  if (m[5] !== undefined) {
    const prefix = Number(m[5]);
    if (prefix < 0 || prefix > 32) return t("net.cidr.prefix", { src: s });
  }
  return null;
}
