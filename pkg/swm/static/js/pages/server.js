// Server template page — IP and CIDR are both accepted; entries are
// validated client-side before submission. One entry per line, matching
// the command template UX. The shared template.js still posts the parsed
// list as an array under `templateDetail`, so no backend change is needed.

import { renderTemplatePage } from "./template.js";
import { validateIpOrCidr } from "../core/net.js";

export default renderTemplatePage({
  base: "/tacacs/template/server",
  titleKey: "server.title",
  subtitleKey: "server.subtitle",
  detailLabelKey: "server.detailLabel",
  detailHintKey: "server.detailHint",
  kindKey: "server.kind",
  detailPlaceholder:
`1.1.1.1
10.0.0.0/24
192.168.1.0/16`,
  detailRows: 8,
  separator: /\n+/,
  validateEntry: validateIpOrCidr,
});
