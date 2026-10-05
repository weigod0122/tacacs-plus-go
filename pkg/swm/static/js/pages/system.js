// System / settings page. The log section has two mutually exclusive modes:
// external redirect (the legacy behaviour) and direct ClickHouse querying.

import { api } from "../core/api.js";
import { h, mount } from "../core/dom.js";
import { confirm } from "../core/components/confirm.js";
import { toast } from "../core/components/toast.js";
import { t } from "../core/i18n.js";

const TYPES = [
  { key: "authen", labelKey: "system.logRedirect.authen", visibleField: "visibleAuthen" },
  { key: "author", labelKey: "system.logRedirect.author", visibleField: "visibleAuthor" },
  { key: "account", labelKey: "system.logRedirect.account", visibleField: "visibleAccount" },
];

const FIELDS = {
  authen: [
    ["time", "AuthenInfo.Time"], ["timeStamp", "AuthenInfo.TimeStamp"],
    ["timeRange", "AuthenInfo.TimeRange"], ["user", "AuthenInfo.User"],
    ["switchAddr", "AuthenInfo.SwitchAddr"], ["serverAddr", "AuthenInfo.ServerAddr"],
    ["authenStatus", "AuthenInfo.AuthenStatus"], ["details", "AuthenInfo.Details"],
    ["isSingleConnect", "AuthenInfo.IsSingleConnect"], ["tacacsClient", "AuthenInfo.TacacsClient"],
  ],
  author: [
    ["time", "AuthorInfo.Time"], ["timeStamp", "AuthorInfo.TimeStamp"],
    ["timeRange", "AuthorInfo.TimeRange"], ["user", "AuthorInfo.User"],
    ["switchAddr", "AuthorInfo.SwitchAddr"], ["serverAddr", "AuthorInfo.ServerAddr"],
    ["authorStatus", "AuthorInfo.AuthorStatus"], ["details", "AuthorInfo.Details"],
    ["cmd", "AuthorInfo.Cmd"], ["isSingleConnect", "AuthorInfo.IsSingleConnect"],
    ["tacacsClient", "AuthorInfo.TacacsClient"],
  ],
  account: [
    ["time", "AccountInfo.Time"], ["timeStamp", "AccountInfo.TimeStamp"],
    ["timeRange", "AccountInfo.TimeRange"], ["user", "AccountInfo.User"],
    ["switchAddr", "AccountInfo.SwitchAddr"], ["serverAddr", "AccountInfo.ServerAddr"],
    ["cmd", "AccountInfo.Cmd"], ["port", "AccountInfo.Port"], ["flags", "AccountInfo.Flags"],
    ["authenMethod", "AccountInfo.AuthenMethod"], ["privLvl", "AccountInfo.PrivLvl"],
    ["authenType", "AccountInfo.AuthenType"], ["authenService", "AccountInfo.AuthenService"],
    ["arg", "AccountInfo.Arg"], ["isSingleConnect", "AccountInfo.IsSingleConnect"],
    ["tacacsClient", "AccountInfo.TacacsClient"],
  ],
};

export default async function renderSystemPage(container) {
  const state = {
    mode: "external",
    schemas: [],
    mappings: { authen: {}, author: {}, account: {} },
    clickhouseTested: false,
  };

  const refreshBtn = h("button", {
    class: "btn btn--primary", type: "button", onclick: refreshMeta,
  }, t("system.meta.btn"));

  const externalInputs = {};
  const visibleInputs = {};
  for (const typ of TYPES) {
    externalInputs[typ.key] = h("input", {
      type: "url", class: "input", placeholder: t("system.logRedirect.placeholder"),
      style: { minWidth: "300px", maxWidth: "560px", flex: "1" },
    });
    visibleInputs[typ.key] = h("input", { type: "checkbox", class: "checkbox" });
  }

  const modeExternal = h("input", { type: "radio", name: "log-display-mode", value: "external" });
  const modeClickHouse = h("input", { type: "radio", name: "log-display-mode", value: "clickhouse" });
  const externalSection = h("div", { class: "stack" });
  const clickhouseSection = h("div", { class: "stack" });
  const schemaStatus = h("p", { class: "field__hint" });
  const mappingHost = h("div", { class: "stack" });
  const saveBtn = h("button", {
    class: "btn btn--primary", type: "button", onclick: saveLogConfig,
  }, t("system.logConfig.save"));

  const ckAddress = h("input", { class: "input", placeholder: "clickhouse.internal:9000" });
  const ckUsername = h("input", { class: "input", autocomplete: "username" });
  const ckPassword = h("input", {
    class: "input", type: "password", autocomplete: "new-password",
    placeholder: t("system.logConfig.passwordPlaceholder"),
  });
  const ckDatabase = h("input", { class: "input", placeholder: "default" });
  const testBtn = h("button", {
    class: "btn", type: "button", onclick: testClickHouse,
  }, t("system.logConfig.test"));

  for (const input of [ckAddress, ckUsername, ckPassword, ckDatabase]) {
    input.addEventListener("input", () => {
      state.clickhouseTested = false;
      if (schemaStatus.classList.contains("text-success")) {
        schemaStatus.textContent = "";
        schemaStatus.className = "field__hint";
      }
    });
  }

  mount(externalSection, [
    h("p", { class: "page__subtitle" }, t("system.logRedirect.desc")),
    ...TYPES.map((typ) => h("div", { class: "toolbar", style: { gap: "12px", flexWrap: "wrap" } }, [
      h("label", { class: "field__label", style: { minWidth: "80px" } }, t(typ.labelKey)),
      externalInputs[typ.key],
      h("label", { class: "toolbar", style: { gap: "6px", margin: 0 } }, [
        visibleInputs[typ.key],
        h("span", { class: "page__subtitle", style: { margin: 0 } }, t("system.logRedirect.visiblePerType")),
      ]),
    ])),
  ]);

  mount(clickhouseSection, [
    h("p", { class: "page__subtitle" }, t("system.logConfig.desc")),
    h("div", { class: "toolbar", style: { alignItems: "stretch" } }, [
      field(t("system.logConfig.address"), ckAddress),
      field(t("system.logConfig.username"), ckUsername),
      field(t("system.logConfig.password"), ckPassword),
      field(t("system.logConfig.database"), ckDatabase),
    ]),
    h("div", { class: "page__actions" }, [testBtn, schemaStatus]),
    h("p", { class: "field__hint" }, t("system.logConfig.mappingHint")),
    mappingHost,
  ]);

  const modeSection = h("section", { class: "card" }, [
    h("header", { class: "card__header" }, [
      h("h2", { class: "card__title" }, t("system.logConfig.title")),
    ]),
    h("div", { class: "card__body stack" }, [
      h("div", { class: "toolbar" }, [
        h("label", { class: "toolbar", style: { gap: "8px", margin: 0 } }, [
          modeExternal, h("span", null, t("system.logConfig.externalMode")),
        ]),
        h("label", { class: "toolbar", style: { gap: "8px", margin: 0 } }, [
          modeClickHouse, h("span", null, t("system.logConfig.clickhouseMode")),
        ]),
      ]),
      externalSection,
      clickhouseSection,
      h("div", { class: "page__actions" }, [saveBtn]),
    ]),
  ]);

  modeExternal.addEventListener("change", () => {
    if (modeExternal.checked) {
      state.mode = "external";
      renderMode();
    }
  });
  modeClickHouse.addEventListener("change", () => {
    if (modeClickHouse.checked) {
      state.mode = "clickhouse";
      renderMode();
    }
  });

  mount(container, h("div", { class: "page" }, [
    h("header", { class: "page__header" }, [
      h("div", { class: "page__heading" }, [
        h("h1", { class: "page__title" }, t("system.title")),
        h("p", { class: "page__subtitle" }, t("system.subtitle")),
      ]),
    ]),
    h("section", { class: "card" }, [
      h("header", { class: "card__header" }, [h("h2", { class: "card__title" }, t("system.meta.title"))]),
      h("div", { class: "card__body stack" }, [
        h("p", { class: "page__subtitle" }, t("system.meta.desc")),
        h("div", { class: "page__actions" }, [refreshBtn]),
      ]),
    ]),
    modeSection,
  ]));

  renderMode();
  await loadConfig();

  function field(label, control) {
    return h("label", { class: "field", style: { minWidth: "180px", flex: "1" } }, [
      h("span", { class: "field__label" }, label), control,
    ]);
  }

  function renderMode() {
    externalSection.style.display = state.mode === "external" ? "" : "none";
    clickhouseSection.style.display = state.mode === "clickhouse" ? "" : "none";
    modeExternal.checked = state.mode === "external";
    modeClickHouse.checked = state.mode === "clickhouse";
    if (state.mode === "clickhouse") renderMappings();
  }

  async function loadConfig() {
    try {
      const res = await api.get("/tacacs/system/log-config");
      const data = (res && res.data) || {};
      state.mode = data.mode === "clickhouse" ? "clickhouse" : "external";
      const external = data.external || {};
      for (const typ of TYPES) {
        externalInputs[typ.key].value = external[typ.key] || "";
        visibleInputs[typ.key].checked = !!external[typ.visibleField];
      }
      const ck = data.clickhouse || {};
      ckAddress.value = ck.address || "";
      ckUsername.value = ck.username || "";
      ckDatabase.value = ck.database || "";
      state.mappings = clone(ck.mappings || state.mappings);
      renderMode();
      if (state.mode === "clickhouse" && ck.address && ck.database) {
        // Stored credentials are reused server-side when password is blank.
        await loadSchema(false);
      }
    } catch (err) {
      toast.error(t("system.logConfig.loadFail") + (err.message || err));
    }
  }

  function clickhouseBody() {
    return {
      address: ckAddress.value.trim(),
      username: ckUsername.value.trim(),
      password: ckPassword.value,
      database: ckDatabase.value.trim(),
      mappings: state.mappings,
    };
  }

  async function testClickHouse() {
    testBtn.disabled = true;
    state.clickhouseTested = false;
    schemaStatus.textContent = t("system.logConfig.testing");
    schemaStatus.className = "field__hint";
    try {
      await api.post("/tacacs/system/clickhouse/test", clickhouseBody());
      await loadSchema(true);
      state.clickhouseTested = true;
      schemaStatus.textContent = t("system.logConfig.testOk");
      schemaStatus.className = "field__hint text-success";
    } catch (err) {
      schemaStatus.textContent = t("system.logConfig.testFail") + (err.message || err);
      schemaStatus.className = "field__hint field__hint--error";
    } finally {
      testBtn.disabled = false;
    }
  }

  async function loadSchema(markTested) {
    const res = await api.post("/tacacs/system/clickhouse/schema", clickhouseBody());
    state.schemas = ((res && res.data && res.data.tables) || []).map((x) => ({
      name: x.name, columns: Array.isArray(x.columns) ? x.columns : [],
    }));
    if (markTested) state.clickhouseTested = true;
    renderMappings();
  }

  function renderMappings() {
    if (!state.schemas.length) {
      mount(mappingHost, h("p", { class: "field__hint" }, t("system.logConfig.testFirst")));
      return;
    }
    mount(mappingHost, ...TYPES.map((typ) => mappingCard(typ)));
  }

  function mappingCard(typ) {
    const mapping = state.mappings[typ.key] || { table: "", eventTimeColumn: "", fields: {}, names: {} };
    if (!mapping.fields) mapping.fields = {};
    if (!mapping.names) mapping.names = {};
    state.mappings[typ.key] = mapping;
    const tableSelect = h("select", { class: "select" });
    const eventTimeSelect = h("select", { class: "select" });
    const rowsHost = h("div", { class: "stack" });

    function setOptions(select, options, current) {
      mount(select, [
        h("option", { value: "" }, t("common.placeholder.select")),
        ...options.map((x) => h("option", {
          value: x.value, selected: x.value === current,
        }, x.label)),
      ]);
      select.value = current || "";
    }

    function currentSchema() {
      return state.schemas.find((x) => x.name === mapping.table) || { columns: [] };
    }

    function renderFields() {
      const columns = currentSchema().columns || [];
      const options = columns.map((x) => ({ value: x.name, label: `${x.name} (${x.type})` }));
      setOptions(eventTimeSelect, options, mapping.eventTimeColumn);
      mount(rowsHost, ...FIELDS[typ.key].map(([key, label]) => {
        const select = h("select", { class: "select" });
        const nameInput = h("input", {
          class: "input",
          value: mapping.names[key] || "",
          placeholder: t("system.logConfig.namePlaceholder"),
        });
        setOptions(select, options, mapping.fields[key]);
        select.addEventListener("change", () => {
          mapping.fields[key] = select.value;
        });
        nameInput.addEventListener("input", () => {
          mapping.names[key] = nameInput.value;
        });
        return h("div", { class: "toolbar", style: { gap: "12px", alignItems: "flex-end" } }, [
          h("div", { class: "field", style: { minWidth: "220px", flex: "1" } }, [
            h("span", { class: "field__label" }, label),
            h("span", { class: "text-subtle" }, key),
          ]),
          h("label", { class: "field", style: { minWidth: "200px", flex: "1" } }, [
            h("span", { class: "field__label" }, t("system.logConfig.column")), select,
          ]),
          h("label", { class: "field", style: { minWidth: "200px", flex: "1" } }, [
            h("span", { class: "field__label" }, t("system.logConfig.name")), nameInput,
          ]),
        ]);
      }));
    }

    setOptions(tableSelect, state.schemas.map((x) => ({ value: x.name, label: x.name })), mapping.table);
    tableSelect.addEventListener("change", () => {
      mapping.table = tableSelect.value;
      mapping.eventTimeColumn = "";
      mapping.fields = {};
      mapping.names = {};
      renderFields();
    });
    eventTimeSelect.addEventListener("change", () => { mapping.eventTimeColumn = eventTimeSelect.value; });
    renderFields();

    return h("section", { class: "card" }, [
      h("header", { class: "card__header" }, [
        h("h3", { class: "card__title" }, t(typ.labelKey)),
      ]),
      h("div", { class: "card__body stack" }, [
        field(t("system.logConfig.table"), tableSelect),
        field(t("system.logConfig.eventTime"), eventTimeSelect),
        rowsHost,
      ]),
    ]);
  }

  async function saveLogConfig() {
    const external = {};
    for (const typ of TYPES) {
      const value = (externalInputs[typ.key].value || "").trim();
      if (state.mode === "external" && value && !/^https?:\/\//i.test(value)) {
        toast.error(t(typ.labelKey) + " " + t("system.logRedirect.invalid"));
        return;
      }
      external[typ.key] = value;
      external[typ.visibleField] = !!visibleInputs[typ.key].checked;
    }
    if (state.mode === "clickhouse" && !state.clickhouseTested) {
      toast.error(t("system.logConfig.testRequired"));
      return;
    }
    saveBtn.disabled = true;
    try {
      await api.post("/tacacs/system/log-config", {
        mode: state.mode,
        external,
        clickhouse: clickhouseBody(),
      });
      toast.success(t("system.logConfig.saved"));
    } catch (err) {
      toast.error(t("system.logConfig.saveFail") + (err.message || err));
    } finally {
      saveBtn.disabled = false;
    }
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
}

function clone(value) {
  try { return JSON.parse(JSON.stringify(value)); }
  catch { return { authen: {}, author: {}, account: {} }; }
}
