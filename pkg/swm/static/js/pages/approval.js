// 权限管理 page — single screen with two stacked sections:
//   上 (top):    权限展示 — role cards for current user's active permissions
//   下 (bottom): 权限申请 — approval workflow (list + create + admin actions)
// Both sections share a single fetch of /tacacs/approval/get.

import { api } from "../core/api.js";
import { h, mount, qs, qsa } from "../core/dom.js";
import { renderTable } from "../core/components/table.js";
import { openModal } from "../core/components/modal.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { approvalStatusBadge, fmtDateTime } from "../core/format.js";
import { openRoleDetailModal } from "../core/template-viewer.js";
import { t, tHeader, getLocale } from "../core/i18n.js";

export default async function renderPermissionPage(container, ctx) {
  const { username, isAdmin } = ctx;

  let allRows = [];
  let manageView = isAdmin ? "all" : "mine";
  let viewVariant = "card"; // card | calendar

  const viewHost = h("div");
  const manageActionsHost = h("div");
  const manageHost = h("div");

  const viewTabs = h("div", { class: "tabs", role: "tablist", "aria-label": t("approval.viewTabs.aria") }, [
    viewTabBtn("card", t("approval.tab.card")),
    viewTabBtn("calendar", t("approval.tab.calendar")),
  ]);

  function viewTabBtn(id, label) {
    return h("button", {
      class: ["tabs__tab", id === viewVariant && "is-active"],
      type: "button",
      role: "tab",
      "aria-selected": String(id === viewVariant),
      dataset: { variant: id },
      onclick: () => setViewVariant(id),
    }, label);
  }

  function setViewVariant(v) {
    viewVariant = v;
    qsa(".tabs__tab", viewTabs).forEach((el) => {
      const on = el.dataset.variant === v;
      el.classList.toggle("is-active", on);
      el.setAttribute("aria-selected", String(on));
    });
    renderViewSection();
  }

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("approval.title")),
        h("p", { class: "page__subtitle" },
          isAdmin ? t("approval.subtitle.admin") : t("approval.subtitle.user")),
      ]),
      h("div", { class: "page__actions" }, [
        h("button", { class: "btn", type: "button", onclick: load }, t("app.refresh")),
      ]),
    ]),
    // 权限展示
    h("section", { class: "stack", "aria-labelledby": "sec-view" }, [
      h("div", { class: "row row--between row--wrap" }, [
        h("h2", { id: "sec-view", class: "section-title" }, t("approval.section.view")),
        viewTabs,
      ]),
      viewHost,
    ]),
    // 权限申请
    h("section", { class: "stack", "aria-labelledby": "sec-manage" }, [
      h("div", { class: "row row--between row--wrap" }, [
        h("h2", { id: "sec-manage", class: "section-title" }, t("approval.section.manage")),
        manageActionsHost,
      ]),
      manageHost,
    ]),
  ]));

  function render() {
    renderViewSection();
    renderManageSection();
  }

  // ----- 权限展示 -----

  function renderViewSection() {
    const myActive = allRows.filter((r) => r.User === username && r.Status === 4);
    if (myActive.length === 0) {
      mount(viewHost, h("div", { class: "card" }, [
        h("div", {
          class: "card__body text-muted",
          style: { textAlign: "center", padding: "var(--space-6)" },
        }, t("approval.empty.noPerm")),
      ]));
      return;
    }
    const aggregated = aggregateActiveRoles(myActive);
    if (viewVariant === "calendar") {
      mount(viewHost, h("div", { class: "card" }, [
        h("div", { class: "card__body" }, buildCalendarBody(aggregated)),
      ]));
    } else {
      mount(viewHost, h("div", { class: "role-grid" }, aggregated.map(roleCard)));
    }
  }

  function roleCard({ role, intervals, totalCount }) {
    const lastEnd = intervals[intervals.length - 1]?.end;
    const overall = daysFrom(lastEnd);
    const isEnding = overall != null && overall <= 7 && overall > 0;
    const mergedFromMany = totalCount > intervals.length;
    return h("div", {
      class: ["card", "role-card", isEnding && "role-card--ending"],
    }, [
      h("div", { class: "role-card__header" }, [
        h("button", {
          class: "role-card__name",
          type: "button",
          title: t("approval.role.viewDetail", { role }),
          onclick: () => openRoleDetailModal(role),
        }, role),
        h("span", { class: "badge badge--approved" }, t("approval.role.active")),
      ]),
      intervalsContainer(intervals),
      totalCount > 1
        ? h("p", { class: "field__hint" },
            mergedFromMany
              ? t("approval.role.merged.both", { total: totalCount, count: intervals.length })
              : t("approval.role.merged.simple", { total: totalCount }))
        : null,
    ]);
  }

  // Render the intervals list. If more than 2 intervals, wrap with a
  // scroll container that shows a small ↑ / ↓ floating arrow at top/bottom
  // depending on whether more content exists in that direction.
  function intervalsContainer(intervals) {
    const scrollable = intervals.length > 2;
    const scrollEl = h("div", {
      class: ["role-card__intervals", scrollable && "role-card__intervals--scroll"],
    }, intervals.map(intervalBlock));
    if (!scrollable) return scrollEl;

    const wrap = h("div", { class: "role-card__intervals-wrap" }, [scrollEl]);

    function update() {
      const top = scrollEl.scrollTop > 2;
      const bot = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight > 2;
      wrap.classList.toggle("has-top", top);
      wrap.classList.toggle("has-bot", bot);
    }
    scrollEl.addEventListener("scroll", update);
    // Initial check after the element is laid out
    requestAnimationFrame(update);
    return wrap;
  }

  function intervalBlock(iv) {
    const remaining = daysFrom(iv.end);
    return h("div", { class: "role-card__interval kv" }, [
      h("span", { class: "kv__label" }, t("approval.role.startEnd.start")),
      h("span", { class: "kv__value" }, formatTime(iv.start)),
      h("span", { class: "kv__label" }, t("approval.role.startEnd.end")),
      h("span", { class: "kv__value" }, formatTime(iv.end)),
      h("span", { class: "kv__label" }, t("approval.role.remain")),
      h("span", { class: ["kv__value", "role-card__remain"] },
        remaining == null ? "—" : t("approval.role.remain.unit", { n: remaining })),
      h("span", { class: "kv__label" }, t("approval.role.ticket")),
      h("span", { class: "kv__value" }, iv.ids.map((id) => `#${id}`).join("，")),
    ]);
  }

  /**
   * Group rows by role, then within each role merge overlapping/touching
   * (start, end) intervals.  E.utils. [t→t+1, t+1→t+2]  collapses to [t→t+2].
   */
  function aggregateActiveRoles(rows) {
    const groups = new Map();
    for (const r of rows) {
      const role = r.ApprovalPermissions || "—";
      if (!groups.has(role)) groups.set(role, []);
      groups.get(role).push(r);
    }
    const out = [];
    for (const [role, items] of groups.entries()) {
      const intervals = items
        .map((it) => ({
          start: parseTimeString(it.StartTime),
          end:   parseTimeString(it.EndTime),
          id:    it.ID,
        }))
        .filter((iv) => iv.start && iv.end)
        .sort((a, b) => a.start - b.start);

      const merged = [];
      for (const iv of intervals) {
        const last = merged[merged.length - 1];
        if (last && iv.start.getTime() <= last.end.getTime()) {
          if (iv.end > last.end) last.end = iv.end;
          last.ids.push(iv.id);
        } else {
          merged.push({ start: iv.start, end: iv.end, ids: [iv.id] });
        }
      }
      out.push({ role, intervals: merged, totalCount: items.length });
    }
    return out;
  }

  function parseTimeString(s) {
    if (!s) return null;
    const t = new Date(String(s).replace(" ", "T"));
    return isNaN(t) ? null : t;
  }

  function formatTime(d) {
    if (!d) return "—";
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
  }

  function daysFrom(d) {
    if (!d) return null;
    const ms = d.getTime() - Date.now();
    if (ms < 0) return 0;
    return Math.ceil(ms / 86400000);
  }

  // ----- Calendar view (inline) -----

  const CAL_PALETTE = [
    "#2563eb", "#10b981", "#f59e0b", "#ef4444",
    "#8b5cf6", "#ec4899", "#14b8a6", "#f97316",
    "#6366f1", "#84cc16", "#06b6d4", "#a855f7",
  ];

  // Localized weekday + month labels — recomputed each render so locale
  // changes propagate.
  function weekdayLabels() {
    return [
      t("approval.cal.weekday.sun"),
      t("approval.cal.weekday.mon"),
      t("approval.cal.weekday.tue"),
      t("approval.cal.weekday.wed"),
      t("approval.cal.weekday.thu"),
      t("approval.cal.weekday.fri"),
      t("approval.cal.weekday.sat"),
    ];
  }
  function calTitle(year, monthIdx) {
    if (getLocale() === "en") {
      const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
      return t("approval.cal.title", { year, month: months[monthIdx] });
    }
    return t("approval.cal.title", { year, month: monthIdx + 1 });
  }

  function buildCalendarBody(aggregated) {
    const colors = new Map(aggregated.map((g, i) => [g.role, CAL_PALETTE[i % CAL_PALETTE.length]]));

    const today = new Date();
    let viewYear = today.getFullYear();
    let viewMonth = today.getMonth();

    const titleEl = h("span", { class: "cal__title" });
    const gridEl = h("div", { class: "cal__grid" });
    const legendEl = h("div", { class: "cal__legend" });

    function refresh() {
      titleEl.textContent = calTitle(viewYear, viewMonth);
      buildGrid();
      buildLegend();
    }

    function buildGrid() {
      while (gridEl.firstChild) gridEl.removeChild(gridEl.firstChild);
      weekdayLabels().forEach((w) =>
        gridEl.appendChild(h("div", { class: "cal__weekday" }, w))
      );

      const firstDay = new Date(viewYear, viewMonth, 1);
      const startWeekday = firstDay.getDay();           // 0 Sun
      const daysInMonth = new Date(viewYear, viewMonth + 1, 0).getDate();
      const today0 = startOfDay(new Date());

      for (let i = startWeekday - 1; i >= 0; i--) {
        const date = new Date(viewYear, viewMonth, -i);
        gridEl.appendChild(buildDayCell(date, true, today0));
      }
      for (let d = 1; d <= daysInMonth; d++) {
        gridEl.appendChild(buildDayCell(new Date(viewYear, viewMonth, d), false, today0));
      }
      const filled = startWeekday + daysInMonth;
      const trailing = (7 - (filled % 7)) % 7;
      for (let i = 1; i <= trailing; i++) {
        gridEl.appendChild(buildDayCell(new Date(viewYear, viewMonth + 1, i), true, today0));
      }
    }

    function buildDayCell(date, isOther, today0) {
      const isToday = sameDay(date, today0);
      const dayStart = startOfDay(date);
      const dayEnd   = endOfDay(date);
      const activeRoles = [];
      for (const g of aggregated) {
        const hit = g.intervals.some((iv) => iv.start <= dayEnd && iv.end >= dayStart);
        if (hit) activeRoles.push(g.role);
      }
      return h("div", {
        class: [
          "cal__day",
          isOther && "cal__day--other",
          isToday && "cal__day--today",
        ],
        title: activeRoles.length
          ? t("approval.cal.dayLabel", { date: formatDateOnly(date), roles: activeRoles.join("、") })
          : formatDateOnly(date),
      }, [
        h("span", { class: "cal__day-num" }, String(date.getDate())),
        activeRoles.length
          ? h("div", { class: "cal__day-strips" },
              activeRoles.map((r) => h("span", {
                class: "cal__strip",
                style: { background: colors.get(r) },
                title: r,
              })))
          : null,
      ]);
    }

    function buildLegend() {
      while (legendEl.firstChild) legendEl.removeChild(legendEl.firstChild);
      if (!aggregated.length) {
        legendEl.appendChild(h("span", { class: "text-subtle" }, t("approval.cal.empty")));
        return;
      }
      aggregated.forEach((g) => {
        legendEl.appendChild(h("div", { class: "cal__legend-item" }, [
          h("span", { class: "cal__legend-swatch", style: { background: colors.get(g.role) } }),
          h("span", null, g.role),
        ]));
      });
    }

    function nav(delta) {
      viewMonth += delta;
      if (viewMonth > 11) { viewMonth = 0; viewYear++; }
      else if (viewMonth < 0) { viewMonth = 11; viewYear--; }
      refresh();
    }
    function goToday() {
      const t2 = new Date();
      viewYear = t2.getFullYear();
      viewMonth = t2.getMonth();
      refresh();
    }

    refresh();

    return h("div", { class: "cal" }, [
      h("div", { class: "cal__nav" }, [
        h("button", { class: "btn btn--ghost btn--sm", type: "button", onclick: () => nav(-1) }, "‹"),
        titleEl,
        h("button", { class: "btn btn--ghost btn--sm", type: "button", onclick: goToday }, t("approval.cal.today")),
        h("button", { class: "btn btn--ghost btn--sm", type: "button", onclick: () => nav(1) }, "›"),
      ]),
      gridEl,
      legendEl,
    ]);
  }

  function startOfDay(d) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate());
  }
  function endOfDay(d) {
    return new Date(d.getFullYear(), d.getMonth(), d.getDate(), 23, 59, 59, 999);
  }
  function sameDay(a, b) {
    return a.getFullYear() === b.getFullYear()
        && a.getMonth() === b.getMonth()
        && a.getDate() === b.getDate();
  }
  function formatDateOnly(d) {
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
  }

  // ----- 权限申请 -----

  function renderManageSection() {
    const subTabConfigs = isAdmin
      ? [
          { id: "all",     label: t("approval.tab.all") },
          { id: "mine",    label: t("approval.tab.mine") },
          { id: "pending", label: t("approval.tab.pending") },
        ]
      : [{ id: "mine", label: t("approval.tab.mine") }];

    const subTabsBar = h("div", { class: "tabs", role: "tablist" },
      subTabConfigs.map((tc) =>
        h("button", {
          class: ["tabs__tab", tc.id === manageView && "is-active"],
          type: "button",
          role: "tab",
          "aria-selected": String(tc.id === manageView),
          dataset: { view: tc.id },
          onclick: () => setManageView(tc.id),
        }, tc.label)
      )
    );

    mount(manageActionsHost, h("div", { class: "row", style: { gap: "var(--space-2)" } }, [
      subTabsBar,
      h("button", {
        class: "btn btn--primary", type: "button",
        onclick: () => openCreateModal(),
      }, t("approval.btn.create")),
    ]));

    renderManageTable();
  }

  function setManageView(v) {
    manageView = v;
    renderManageSection();
  }

  function visibleManageRows() {
    if (manageView === "mine")    return allRows.filter((r) => r.User === username);
    if (manageView === "pending") return allRows.filter((r) => r.Status === 3);
    return allRows;
  }

  function renderManageTable() {
    const rows = visibleManageRows();
    renderTable(manageHost, {
      columns: inferColumns(rows),
      rows,
      emptyText: t("approval.empty.records"),
      rowActions: (row) => actionsFor(row),
    });
  }

  function actionsFor(row) {
    if (row.Status === 3) {
      const items = [];
      if (isAdmin) {
        items.push({ label: t("approval.action.approve"), kind: "success", onClick: () => updateStatus(row, 4, t("approval.toast.approved")) });
        items.push({ label: t("approval.action.reject"),  kind: "danger",  onClick: () => updateStatus(row, 2, t("approval.toast.rejected")) });
      }
      items.push({ label: t("approval.action.close"), kind: "ghost", onClick: () => updateStatus(row, 0, t("approval.toast.closed")) });
      return items;
    }
    return [
      {
        label: t("approval.action.recreate"), kind: "ghost",
        onClick: () => openCreateModal(row.ApprovalPermissions || "", row.User || ""),
      },
    ];
  }

  async function updateStatus(row, status, successText) {
    const verb = status === 4
      ? t("approval.verb.approve")
      : status === 2
        ? t("approval.verb.reject")
        : t("approval.verb.close");
    const ok = await confirm({
      title: t("approval.confirm.title", { verb }),
      size: "",
      danger: status !== 4,
      message: h("div", { class: "stack" }, [
        h("p", { style: { margin: 0 } }, t("approval.confirm.intro", { verb })),
        h("div", {
          class: "kv",
          style: {
            padding: "var(--space-3) var(--space-4)",
            background: "var(--color-surface-muted)",
            borderRadius: "var(--radius-md)",
          },
        }, [
          h("span", { class: "kv__label" }, t("approval.confirm.applicant")),
          h("span", { class: "kv__value" }, row.User || "—"),
          h("span", { class: "kv__label" }, t("approval.confirm.role")),
          h("span", { class: "kv__value" }, row.ApprovalPermissions || "—"),
          h("span", { class: "kv__label" }, t("approval.confirm.validity")),
          h("span", { class: "kv__value" }, `${row.StartTime || "—"}  →  ${row.EndTime || "—"}`),
          h("span", { class: "kv__label" }, t("approval.confirm.created")),
          h("span", { class: "kv__value" }, row.CreateTime || "—"),
          h("span", { class: "kv__label" }, t("approval.confirm.ticket")),
          h("span", { class: "kv__value" }, `#${row.ID}`),
        ]),
      ]),
    });
    if (!ok) return;
    try {
      await api.post("/tacacs/approval/update", { id: row.ID, status });
      toast.success(successText);
      await load();
    } catch (err) {
      toast.error(err.message || t("approval.toast.opFailed"));
    }
  }

  const MAX_ACTIVE_ROLES_PER_USER = 200;

  // 计算指定用户当前生效角色数,与服务端的 per-user 上限对齐;申请前会在
  // submit() 里再校验一次,避免管理员代申请时把上限算到管理员自己头上。
  function activeRolesOf(user) {
    return allRows.filter((r) => r.User === user && r.Status === 4).length;
  }

  async function openCreateModal(defaultRole = "", defaultTargetUser = "") {
    // 非管理员只能给自己申请,所以可以提前做容量预检,体验上少弹一次 modal;
    // 管理员目标用户在 modal 里选,容量检查推迟到 submit()。
    if (!isAdmin) {
      const activeCount = activeRolesOf(username);
      if (activeCount >= MAX_ACTIVE_ROLES_PER_USER) {
        toast.error(
          t("approval.create.error.activeCap", { n: activeCount, cap: MAX_ACTIVE_ROLES_PER_USER })
        );
        return;
      }
    }

    let roles = [];
    try {
      const res = await api.get("/tacacs/template/role/get");
      roles = (res && res.data) || [];
    } catch (err) {
      toast.error(t("approval.create.toast.loadRolesFailed") + err.message);
      return;
    }

    // 仅管理员需要 user 列表;普通用户直接锁定自己,避免多发一个无用请求。
    let userOptions = [username];
    if (isAdmin) {
      try {
        const res = await api.get("/tacacs/user/get");
        const rows = (res && res.data) || [];
        const names = rows.map((r) => r.User).filter(Boolean);
        // 用 Set 去重并保证当前用户始终在列表里(管理员账号在 user 表中也存在,
        // 但保险起见兜个底)。
        userOptions = Array.from(new Set([username, ...names]));
      } catch (err) {
        toast.error(t("approval.create.toast.loadUsersFailed") + (err.message || err));
        return;
      }
    }

    const initialTarget = defaultTargetUser && userOptions.includes(defaultTargetUser)
      ? defaultTargetUser
      : username;

    const today = new Date().toISOString().slice(0, 10);
    const future = new Date(); future.setDate(future.getDate() + 1);
    const defaultEnd = future.toISOString().slice(0, 10);

    let mode = "days";
    let submitting = false;

    // 管理员才渲染申请人选择器;普通用户字段直接省略,提交时用 username 填。
    const userCombo = isAdmin
      ? buildCombobox(userOptions, initialTarget, {
          placeholder: t("approval.create.targetUserPlaceholder"),
        })
      : null;

    const roleCombo = buildCombobox(
      roles.map((r) => r.Template),
      defaultRole,
    );

    const daysInput = h("input", {
      type: "number", class: "input",
      min: 1, max: 365, value: 1,
      placeholder: t("approval.create.daysPlaceholder"),
    });
    const daysBox = h("div", { class: "stack" }, [
      daysInput,
      h("p", { class: "field__hint" }, t("approval.create.daysHint")),
    ]);

    const startInput = h("input", {
      type: "date", class: "input", value: today, min: today,
      style: { width: "auto", flex: "1" },
      onchange: refreshRangeHint,
    });
    const endInput = h("input", {
      type: "date", class: "input", value: defaultEnd, min: today,
      style: { width: "auto", flex: "1" },
      onchange: refreshRangeHint,
    });
    const rangeHint = h("p", { class: "field__hint" });
    const rangeBox = h("div", { class: "stack", style: { display: "none" } }, [
      h("div", { class: "row" }, [
        startInput,
        h("span", { class: "text-muted" }, "→"),
        endInput,
      ]),
      rangeHint,
    ]);
    refreshRangeHint();

    function rangeDays() {
      if (!startInput.value || !endInput.value) return null;
      const s = new Date(startInput.value);
      const e = new Date(endInput.value);
      if (isNaN(s) || isNaN(e)) return null;
      const ms = e.getTime() - s.getTime();
      if (ms <= 0) return 0;
      return Math.ceil(ms / 86400000);
    }

    function refreshRangeHint() {
      const d = rangeDays();
      if (d == null) {
        rangeHint.textContent = t("approval.create.range.empty");
        rangeHint.classList.remove("field__hint--error");
        return;
      }
      if (d <= 0) {
        rangeHint.textContent = t("approval.create.range.invalid");
        rangeHint.classList.add("field__hint--error");
        return;
      }
      if (d > 365) {
        rangeHint.textContent = t("approval.create.range.tooLong", { n: d });
        rangeHint.classList.add("field__hint--error");
        return;
      }
      rangeHint.classList.remove("field__hint--error");
      rangeHint.textContent = t("approval.create.range.ok", { n: d });
    }

    const tabsBar = h("div", { class: "tabs" }, [
      h("button", {
        class: "tabs__tab is-active", type: "button",
        dataset: { mode: "days" },
        onclick: () => switchMode("days"),
      }, t("approval.create.tabDays")),
      h("button", {
        class: "tabs__tab", type: "button",
        dataset: { mode: "range" },
        onclick: () => switchMode("range"),
      }, t("approval.create.tabRange")),
    ]);

    function switchMode(m) {
      mode = m;
      daysBox.style.display = m === "days" ? "" : "none";
      rangeBox.style.display = m === "range" ? "" : "none";
      qs(".is-active", tabsBar)?.classList.remove("is-active");
      qs(`[data-mode="${m}"]`, tabsBar)?.classList.add("is-active");
      errorEl.style.display = "none";
    }

    const errorEl = h("p", {
      class: "field__hint field__hint--error",
      style: { display: "none" },
    });

    const formBody = h("form", {
      class: "stack",
      novalidate: true,
      onsubmit: (e) => { e.preventDefault(); submit(); },
    }, [
      userCombo ? h("div", { class: "field" }, [
        h("label", { class: "field__label" }, t("approval.create.targetUser")),
        userCombo.el,
        h("p", { class: "field__hint" }, t("approval.create.targetUserHint")),
      ]) : null,
      h("div", { class: "field" }, [
        h("label", { class: "field__label" }, t("approval.create.role")),
        roleCombo.el,
      ]),
      h("div", { class: "field" }, [
        h("label", { class: "field__label" }, t("approval.create.validity")),
        tabsBar,
        daysBox,
        rangeBox,
      ]),
      errorEl,
    ]);

    const handle = openModal({
      title: t("approval.create.title"),
      body: formBody,
      actions: [
        { label: t("common.cancel"), kind: "ghost", onClick: (close) => close() },
        { label: t("approval.create.btnSubmit"), kind: "primary", onClick: () => submit() },
      ],
    });

    async function submit() {
      if (submitting) return;
      errorEl.style.display = "none";

      // 管理员从下拉里选,普通用户没有选择器、直接用自己。
      const targetUser = userCombo ? userCombo.value : username;
      if (!targetUser) {
        return showError(t("approval.create.error.targetUser"), userCombo?.input);
      }

      if (!roleCombo.value) return showError(t("approval.create.error.role"), roleCombo.input);

      // 容量上限按"目标用户"算,管理员代申请不能把别人的额度算到自己头上。
      const activeCount = activeRolesOf(targetUser);
      if (activeCount >= MAX_ACTIVE_ROLES_PER_USER) {
        return showError(
          t("approval.create.error.activeCap", { n: activeCount, cap: MAX_ACTIVE_ROLES_PER_USER })
        );
      }

      const payload = { user: targetUser, permission: roleCombo.value };
      if (mode === "days") {
        const days = Number(daysInput.value);
        if (!Number.isInteger(days) || days < 1 || days > 365) {
          return showError(t("approval.create.error.daysRange"), daysInput);
        }
        payload.validity = String(days);
      } else {
        const days = rangeDays();
        if (days == null) return showError(t("approval.create.range.empty"), startInput);
        if (days <= 0) return showError(t("approval.create.range.invalid"), endInput);
        if (days > 365) return showError(t("approval.create.error.rangeOver"), endInput);
        payload.startTime = `${startInput.value} 00:00:00`;
        payload.endTime   = `${endInput.value} 23:59:59`;
      }

      submitting = true;
      try {
        await api.post("/tacacs/approval/create", payload);
        handle.close();
        toast.success(t("approval.create.toast.submitted"));
        await load();
      } catch (err) {
        showError(err.message || t("approval.create.toast.failed"));
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

  // ----- shared load -----

  async function load() {
    mount(viewHost, h("div", { class: "card" }, [
      h("div", { class: "card__body text-muted", style: { textAlign: "center", padding: "var(--space-6)" } }, t("common.loading")),
    ]));
    renderTable(manageHost, { columns: [], rows: [], loading: true });
    try {
      const res = await api.get("/tacacs/approval/get");
      allRows = ((res && res.data) || []).map((r) => {
        // 统一格式化所有以 Time 结尾的字段（CreateTime / StartTime / EndTime /
        // ApproveTime 等），fmtDateTime 内部会把 Go 的零值时间转成空串，避免
        // 未审批的工单展示一串 0。
        const out = { ...r };
        for (const k of Object.keys(out)) {
          if (k.endsWith("Time")) out[k] = fmtDateTime(out[k]);
        }
        return out;
      });
      render();
    } catch (err) {
      mount(viewHost, h("div", { class: "card" }, [
        h("div", { class: "card__body text-danger" }, t("common.loadFailed") + (err.message || err)),
      ]));
      mount(manageHost);
    }
  }

  await load();
}

function inferColumns(rows) {
  if (rows.length === 0) {
    return [
      { key: "ID",                  label: tHeader("ID") },
      { key: "User",                label: tHeader("User") },
      { key: "ApprovalPermissions", label: tHeader("ApprovalPermissions") },
      { key: "Status",              label: tHeader("Status"),     render: approvalStatusBadge },
      { key: "CreateTime",          label: tHeader("CreateTime") },
      { key: "StartTime",           label: tHeader("StartTime") },
      { key: "EndTime",             label: tHeader("EndTime") },
    ];
  }
  return Object.keys(rows[0]).map((k) => {
    if (k === "Status") return { key: k, label: tHeader("Status"), render: approvalStatusBadge };
    return { key: k, label: tHeader(k) };
  });
}

/**
 * buildCombobox(options, defaultValue, opts) — searchable single-select.
 * 一个普通 input 加下拉面板：点击展开全部、输入实时模糊过滤（不区分大小写
 * 子串匹配），上下键移动高亮，回车 / 点击提交。`value` 仅在用户从列表中
 * 真正选中后才更新——只输入但未确认会在失焦时还原显示,避免界面与提交值
 * 不一致。返回 { el, input, get value() }。
 *
 * opts.placeholder — 输入框占位符,默认沿用 approval.create.rolePlaceholder。
 */
function buildCombobox(options, defaultValue, opts = {}) {
  const placeholder = opts.placeholder || t("approval.create.rolePlaceholder");
  let selected = options.includes(defaultValue) ? defaultValue : "";
  let activeIdx = -1;
  let items = [];

  const input = h("input", {
    type: "text", class: "input", autocomplete: "off",
    spellcheck: "false",
    placeholder,
    value: selected,
    role: "combobox",
    "aria-autocomplete": "list",
    "aria-expanded": "false",
  });

  const list = h("div", { class: "combobox__list", role: "listbox" });
  const empty = h("div", { class: "combobox__empty" }, t("common.empty"));
  empty.style.display = "none";
  const panel = h("div", { class: "combobox__panel" }, [list, empty]);
  const wrap = h("div", { class: "combobox" }, [input, panel]);

  function render(filter) {
    const needle = (filter || "").trim().toLowerCase();
    const matches = needle
      ? options.filter((o) => o.toLowerCase().includes(needle))
      : options.slice();

    while (list.firstChild) list.removeChild(list.firstChild);
    items = matches.map((opt) => {
      const node = h("div", {
        class: ["combobox__item", opt === selected && "is-selected"],
        role: "option",
        "aria-selected": opt === selected ? "true" : "false",
        onmousedown: (e) => { e.preventDefault(); commit(opt); },
      }, opt);
      list.appendChild(node);
      return { value: opt, node };
    });

    activeIdx = items.findIndex((i) => i.value === selected);
    if (activeIdx < 0 && items.length) activeIdx = 0;
    paintActive();
    empty.style.display = items.length ? "none" : "";
  }

  function paintActive() {
    items.forEach((it, i) => it.node.classList.toggle("is-active", i === activeIdx));
    if (activeIdx >= 0) {
      const node = items[activeIdx].node;
      const top = node.offsetTop;
      const bottom = top + node.offsetHeight;
      if (top < list.scrollTop) list.scrollTop = top;
      else if (bottom > list.scrollTop + list.clientHeight) {
        list.scrollTop = bottom - list.clientHeight;
      }
    }
  }

  function open() {
    panel.classList.add("is-open");
    input.setAttribute("aria-expanded", "true");
    render(input.value === selected ? "" : input.value);
  }

  function close() {
    panel.classList.remove("is-open");
    input.setAttribute("aria-expanded", "false");
    if (input.value !== selected) input.value = selected;
  }

  function commit(value) {
    selected = value;
    input.value = value;
    close();
    input.dispatchEvent(new Event("change", { bubbles: true }));
  }

  input.addEventListener("focus", open);
  input.addEventListener("click", open);
  input.addEventListener("input", () => {
    if (!panel.classList.contains("is-open")) open();
    else render(input.value);
  });
  input.addEventListener("keydown", (e) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      if (!panel.classList.contains("is-open")) return open();
      if (!items.length) return;
      activeIdx = (activeIdx + 1) % items.length;
      paintActive();
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (!panel.classList.contains("is-open")) return open();
      if (!items.length) return;
      activeIdx = (activeIdx - 1 + items.length) % items.length;
      paintActive();
    } else if (e.key === "Enter") {
      if (panel.classList.contains("is-open") && activeIdx >= 0 && items[activeIdx]) {
        e.preventDefault();
        commit(items[activeIdx].value);
      }
    } else if (e.key === "Escape") {
      if (panel.classList.contains("is-open")) {
        e.preventDefault();
        close();
      }
    }
  });

  // 点击外部关闭。用 mousedown 而不是 click，避免与 onmousedown 选项冲突。
  // modal 关闭后 wrap 会从 DOM 摘掉，监听器自行注销，避免反复开关 modal 后
  // 在 document 上堆积一堆死引用。
  const onDocDown = (e) => {
    if (!wrap.isConnected) {
      document.removeEventListener("mousedown", onDocDown);
      return;
    }
    if (!panel.classList.contains("is-open")) return;
    if (!wrap.contains(e.target)) close();
  };
  document.addEventListener("mousedown", onDocDown);

  return {
    el: wrap,
    input,
    get value() { return selected; },
  };
}
