// Mission Control UI. No build step, no dependencies: the cockpit must keep
// working even when nobody has run npm in this repository for months.

const KIND_COLOR = {
  pod: "var(--pod)", service: "var(--service)", volume: "var(--volume)",
  config: "var(--config)", secret: "var(--secret)", outside: "var(--outside)",
  workload: "var(--workload)",
};
const KIND_LABEL = {
  pod: "Pod", service: "Service", volume: "Volume", config: "ConfigMap",
  secret: "Secret", outside: "außerhalb", workload: "Workload",
};
const EDGE_LABEL = {
  routes: "leitet zu", mounts: "hängt ein", reads: "liest",
  "may-reach": "darf erreichen", accepts: "akzeptiert von", "owned-by": "gehört zu",
};

const $ = (selector) => document.querySelector(selector);
const el = (tag, attrs = {}, ...children) => {
  const node = document.createElementNS(attrs.ns || "http://www.w3.org/1999/xhtml", tag);
  for (const [key, value] of Object.entries(attrs)) {
    if (key !== "ns") node.setAttribute(key, value);
  }
  for (const child of children.flat()) {
    node.append(child instanceof Node ? child : document.createTextNode(String(child)));
  }
  return node;
};
const svgEl = (tag, attrs = {}, ...children) => el(tag, { ...attrs, ns: "http://www.w3.org/2000/svg" }, ...children);

async function api(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) {
    const problem = await response.json().catch(() => ({ fehler: "Unbekannter Fehler" }));
    throw new Error(problem.fehler ?? `HTTP ${response.status}`);
  }
  return response.status === 204 ? null : response.json();
}

function banner(message) {
  const node = $("#banner");
  node.hidden = !message;
  node.textContent = message ?? "";
}

// ---------------------------------------------------------------- cluster ---

// Column layout: dependencies read left to right. What routes traffic to a pod
// sits on its left, what the pod itself needs sits on its right.
const COLUMN = { service: 0, workload: 0, pod: 1, volume: 2, config: 2, secret: 2, outside: 2 };
const COLUMN_TITLE = ["erreichbar über / gehört zu", "Pods", "wovon der Pod abhängt"];

let activeNamespace = null;
let focused = null;

function layout(nodes) {
  const columns = [[], [], []];
  for (const node of nodes) columns[COLUMN[node.kind] ?? 2].push(node);
  const columnX = [40, 400, 780];
  const boxWidth = 250, boxHeight = 46, gap = 14;
  const positions = new Map();
  columns.forEach((column, index) => {
    column.forEach((node, row) => {
      positions.set(node.id, { x: columnX[index], y: 56 + row * (boxHeight + gap), w: boxWidth, h: boxHeight });
    });
  });
  const rows = Math.max(...columns.map((column) => column.length), 1);
  return { positions, columnX, width: 1080, height: 56 + rows * (boxHeight + gap) + 16, boxWidth, boxHeight };
}

// Only edges that carry a number or a rule get a label. Volume and env names
// are visible in the tooltip; printing them all turned the middle of the graph
// into a smear.
const LABELLED = new Set(["routes", "may-reach", "accepts"]);

function drawGraph(nodes, edges) {
  const { positions, columnX, width, height, boxWidth } = layout(nodes);
  const svg = svgEl("svg", { viewBox: `0 0 ${width} ${height}`, role: "img", "aria-label": "Abhängigkeiten" });
  svg.append(svgEl("defs", {}, svgEl("marker", {
    id: "arrow", viewBox: "0 0 10 10", refX: "9", refY: "5",
    markerWidth: "6", markerHeight: "6", orient: "auto-start-reverse",
  }, svgEl("path", { d: "M 0 0 L 10 5 L 0 10 z", fill: "#8b9bab" }))));

  columnX.forEach((x, index) => {
    svg.append(svgEl("text", { class: "column-title", x: x, y: 28 }, COLUMN_TITLE[index]));
  });

  const related = new Set();
  if (focused) {
    related.add(focused);
    for (const edge of edges) {
      if (edge.from === focused) related.add(edge.to);
      if (edge.to === focused) related.add(edge.from);
    }
  }
  const dimmed = (id) => focused && !related.has(id);

  edges.forEach((edge, index) => {
    const from = positions.get(edge.from), to = positions.get(edge.to);
    if (!from || !to) return;
    const forward = to.x > from.x;
    const x1 = from.x + (forward ? from.w : 0), y1 = from.y + from.h / 2;
    const x2 = to.x + (forward ? 0 : to.w), y2 = to.y + to.h / 2;
    const mid = (x1 + x2) / 2;
    const off = focused && (edge.from === focused || edge.to === focused);
    const classes = ["edge", edge.kind];
    if (dimmed(edge.from) || dimmed(edge.to)) classes.push("dim");
    if (off) classes.push("focus");
    svg.append(svgEl("path", {
      class: classes.join(" "), "marker-end": "url(#arrow)",
      d: `M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`,
    }, svgEl("title", {}, `${EDGE_LABEL[edge.kind] ?? edge.kind}${edge.label ? " · " + edge.label : ""} (aus ${edge.source})`)));
    if (edge.label && LABELLED.has(edge.kind) && !dimmed(edge.from) && !dimmed(edge.to)) {
      // Stagger labels so two rules on the same track stay readable.
      const shift = (index % 3) * 11 - 11;
      svg.append(svgEl("text", { class: "edge-label", x: mid, y: (y1 + y2) / 2 - 5 + shift, "text-anchor": "middle" }, edge.label));
    }
  });

  for (const node of nodes) {
    const box = positions.get(node.id);
    if (!box) continue;
    const classes = ["node", node.healthy ? "" : "unhealthy", dimmed(node.id) ? "dim" : "", node.id === focused ? "focus" : ""];
    const group = svgEl("g", { class: classes.filter(Boolean).join(" "), tabindex: "0", role: "button" });
    group.append(svgEl("rect", {
      x: box.x, y: box.y, width: box.w, height: box.h, rx: 9,
      fill: KIND_COLOR[node.kind] ?? "var(--outside)", stroke: "#0d1218",
    }));
    group.append(svgEl("text", { class: "name", x: box.x + 12, y: box.y + 20 }, truncate(node.name, 30)));
    group.append(svgEl("text", { class: "sub", x: box.x + 12, y: box.y + 35 },
      `${KIND_LABEL[node.kind] ?? node.kind}${node.status ? " · " + node.status : ""}`));
    const detail = Object.entries(node.detail ?? {}).map(([key, value]) => `${key}: ${value}`).join("\n");
    group.append(svgEl("title", {}, [node.name, detail].filter(Boolean).join("\n")));
    const select = () => { focused = focused === node.id ? null : node.id; renderCluster(lastSnapshot); };
    group.addEventListener("click", select);
    group.addEventListener("keydown", (event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); select(); } });
    svg.append(group);
  }
  return svg;
}

const truncate = (value, max) => (value.length > max ? value.slice(0, max - 1) + "…" : value);

// describe turns the edges of one node into sentences. An arrow diagram is fine
// for an overview; when something is broken people want to read it.
function describe(node, nodes, edges) {
  const byID = new Map(nodes.map((entry) => [entry.id, entry]));
  const lines = [];
  const push = (label, list) => {
    if (list.length) lines.push(el("p", {}, el("b", {}, label + ": "), list.join(", ")));
  };
  const out = (kind) => edges.filter((edge) => edge.from === node.id && edge.kind === kind)
    .map((edge) => (byID.get(edge.to)?.name ?? "?") + (edge.label ? ` (${edge.label})` : ""));
  const into = (kind) => edges.filter((edge) => edge.to === node.id && edge.kind === kind)
    .map((edge) => (byID.get(edge.from)?.name ?? "?") + (edge.label ? ` (${edge.label})` : ""));

  push("Erreichbar über", into("routes"));
  push("Gehört zu", out("owned-by"));
  push("Hängt Speicher ein", out("mounts"));
  push("Liest Konfiguration", out("reads"));
  push("Darf erreichen", out("may-reach"));
  push("Akzeptiert Verbindungen von", into("accepts"));
  push("Leitet weiter zu", out("routes"));
  if (!lines.length) lines.push(el("p", {}, "Keine Abhängigkeiten gefunden."));

  const panel = el("aside", { class: "detail" },
    el("h3", {}, node.name),
    el("p", { class: "sub" }, `${KIND_LABEL[node.kind] ?? node.kind}${node.status ? " · " + node.status : ""}`),
    ...Object.entries(node.detail ?? {}).map(([key, value]) => el("p", {}, el("b", {}, key + ": "), value)),
    ...lines,
    el("button", { class: "quiet" }, "Auswahl aufheben"),
  );
  panel.querySelector("button").addEventListener("click", () => { focused = null; renderCluster(lastSnapshot); });
  return panel;
}

let lastSnapshot = { nodes: [], edges: [], namespaces: [] };

function renderCluster(snapshot) {
  lastSnapshot = snapshot;
  $("#stamp").textContent = snapshot.takenAt
    ? "Stand " + new Date(snapshot.takenAt).toLocaleTimeString("de-DE")
    : "noch kein Snapshot";
  banner(snapshot.warnings?.length ? "Snapshot unvollständig:\n" + snapshot.warnings.join("\n") : null);

  const pods = snapshot.nodes.filter((node) => node.kind === "pod");
  const unhealthy = snapshot.nodes.filter((node) => !node.healthy);
  $("#clusterStats").replaceChildren(
    el("article", {}, el("span", {}, "Pods"), el("strong", {}, String(pods.length))),
    el("article", {}, el("span", {}, "Objekte"), el("strong", {}, String(snapshot.nodes.length))),
    el("article", {}, el("span", {}, "Abhängigkeiten"), el("strong", {}, String(snapshot.edges.length))),
    el("article", { class: unhealthy.length ? "bad" : "" }, el("span", {}, "Auffällig"), el("strong", {}, String(unhealthy.length))),
  );

  $("#legend").replaceChildren(...Object.entries(KIND_LABEL).map(([kind, label]) =>
    el("span", {}, el("i", { style: `background:${KIND_COLOR[kind]}` }), label)));

  const namespaces = snapshot.namespaces ?? [];
  if (!namespaces.includes(activeNamespace)) activeNamespace = namespaces[0] ?? null;

  const tabs = el("div", { class: "nstabs" });
  for (const namespace of namespaces) {
    const count = snapshot.nodes.filter((node) => node.namespace === namespace && node.kind === "pod").length;
    const broken = snapshot.nodes.some((node) => node.namespace === namespace && !node.healthy);
    const button = el("button", { class: namespace === activeNamespace ? "active" : "" },
      namespace, el("span", { class: "count" }, String(count)));
    if (broken) button.classList.add("warn");
    button.addEventListener("click", () => { activeNamespace = namespace; focused = null; renderCluster(lastSnapshot); });
    tabs.append(button);
  }

  const nodes = snapshot.nodes.filter((node) => node.namespace === activeNamespace);
  const ids = new Set(nodes.map((node) => node.id));
  const edges = snapshot.edges.filter((edge) => ids.has(edge.from) && ids.has(edge.to));
  const focusedNode = nodes.find((node) => node.id === focused);

  const stage = el("div", { class: "stage" });
  stage.append(nodes.length ? drawGraph(nodes, edges) : el("div", { class: "empty" }, "Keine Objekte gelesen"));
  if (focusedNode) stage.append(describe(focusedNode, nodes, edges));

  $("#namespaces").replaceChildren(tabs,
    el("p", { class: "hint" }, focusedNode ? "Erneut klicken hebt die Auswahl auf." : "Auf einen Kasten klicken, um seine Abhängigkeiten zu isolieren."),
    stage);

  renderLogPicker(snapshot);
}

// --------------------------------------------------------------------- log ---

// The log view follows Kibana's Discover: counters that stay honest under a
// filter, a histogram that shows when something happened, a field sidebar that
// answers "what values are even in here", and rows you can open.
let logPod = null;
let logLines = 200;
let logTimer = null;
let logState = { levels: new Set(), text: "", fields: [], from: null, to: null };
let logData = null;

const LEVEL_ORDER = ["ERROR", "WARN", "INFO", "DEBUG", "UNBEKANNT"];
const LEVEL_CLASS = { ERROR: "err", WARN: "warn", INFO: "info", DEBUG: "debug", UNBEKANNT: "unknown" };

function renderLogPicker(snapshot) {
  const pods = snapshot.nodes.filter((node) => node.kind === "pod" && node.namespace === activeNamespace);
  if (!pods.length) {
    $("#logs").replaceChildren(el("p", { class: "empty" }, "Keine Pods in diesem Namespace."));
    return;
  }
  const focusedPod = pods.find((pod) => pod.id === focused);
  if (focusedPod) logPod = focusedPod.name;
  if (!pods.some((pod) => pod.name === logPod)) logPod = pods[0].name;

  const select = el("select", {}, ...pods.map((pod) =>
    el("option", pod.name === logPod ? { selected: "selected" } : {}, pod.name)));
  select.addEventListener("change", () => { logPod = select.value; void loadLog(); });

  const lines = el("select", {}, ...[50, 200, 500, 2000].map((count) =>
    el("option", count === logLines ? { selected: "selected", value: count } : { value: count }, count + " Zeilen")));
  lines.addEventListener("change", () => { logLines = Number(lines.value); void loadLog(); });

  const follow = el("label", { class: "inline" },
    el("input", { type: "checkbox", ...(logTimer ? { checked: "checked" } : {}) }), " Mitlaufen");
  follow.querySelector("input").addEventListener("change", (event) => {
    clearInterval(logTimer);
    logTimer = event.currentTarget.checked ? setInterval(loadLog, 5000) : null;
  });

  const reload = el("button", { class: "quiet" }, "Neu laden");
  reload.addEventListener("click", () => void loadLog());

  $("#logControls").replaceChildren(el("span", { class: "label" }, "Pod"), select, lines, follow, reload);
  void loadLog();
}

function logQuery() {
  const params = new URLSearchParams({ namespace: activeNamespace, pod: logPod, lines: String(logLines) });
  if (logState.levels.size) params.set("level", [...logState.levels].join(","));
  if (logState.text) params.set("q", logState.text);
  for (const filter of logState.fields) params.append("feld", `${filter.name}:${filter.value}`);
  if (logState.from) params.set("von", String(logState.from));
  if (logState.to) params.set("bis", String(logState.to));
  return "/api/logs?" + params.toString();
}

async function loadLog() {
  if (!logPod || !activeNamespace) return;
  try {
    logData = await api(logQuery());
    renderLog();
  } catch (error) {
    banner(error.message);
  }
}

function renderLog() {
  const data = logData;
  if (!data) return;

  // Level chips keep their counts even while a filter is active: hiding the
  // errors must not also hide that there are errors.
  $("#logLevels").replaceChildren(...LEVEL_ORDER.filter((level) => data.zaehler?.[level]).map((level) => {
    const active = logState.levels.has(level);
    const chip = el("button", { class: `level ${LEVEL_CLASS[level]}${active ? " on" : ""}` },
      level, el("span", { class: "n" }, String(data.zaehler[level])));
    chip.addEventListener("click", () => {
      if (active) logState.levels.delete(level); else logState.levels.add(level);
      void loadLog();
    });
    return chip;
  }));

  const chips = logState.fields.map((filter, index) => {
    const chip = el("button", { class: "chip" }, `${filter.name}: ${filter.value} ✕`);
    chip.addEventListener("click", () => { logState.fields.splice(index, 1); void loadLog(); });
    return chip;
  });
  if (logState.text) {
    const chip = el("button", { class: "chip" }, `Text: ${logState.text} ✕`);
    chip.addEventListener("click", () => { logState.text = ""; $("#logSearch").value = ""; void loadLog(); });
    chips.push(chip);
  }
  if (logState.from || logState.to) {
    const clock = (millis) => millis ? new Date(millis).toLocaleTimeString("de-DE") : "…";
    const chip = el("button", { class: "chip range" },
      `Zeitraum: ${clock(logState.from)} – ${clock(logState.to)} ✕`);
    chip.addEventListener("click", () => { logState.from = null; logState.to = null; void loadLog(); });
    chips.push(chip);
  }
  chips.push(el("span", { class: "hint" },
    `${data.gefiltert ?? 0} von ${data.gesamt ?? 0} Zeilen · ${Object.entries(data.formate ?? {}).map(([f, n]) => f + " " + n).join(" · ")}`));
  $("#logChips").replaceChildren(...chips);

  renderHistogram(data);
  renderFields(data);
  renderLines(data);
}

// The histogram is the fastest way to see "it broke at 07:52": bars stacked by
// level, errors on top and in red, so a spike is visible before reading a line.
function renderHistogram(data) {
  const buckets = data.histogramm ?? [];
  const target = $("#logHistogram");
  if (!buckets.length) {
    target.replaceChildren();
    return;
  }
  const width = 1000, height = 90, pad = 22;
  const max = Math.max(...buckets.map((b) => b.gesamt), 1);
  const step = (width - 2 * pad) / buckets.length;
  const svg = svgEl("svg", { viewBox: `0 0 ${width} ${height}`, class: "hist", preserveAspectRatio: "none" });
  buckets.forEach((bucket, index) => {
    let offset = 0;
    const x = pad + index * step;
    for (const level of [...LEVEL_ORDER].reverse()) {
      const count = bucket.zaehler?.[level] ?? 0;
      if (!count) continue;
      const barHeight = (count / max) * (height - 26);
      offset += barHeight;
      svg.append(svgEl("rect", {
        class: "bar " + LEVEL_CLASS[level], x: x + 1, width: Math.max(step - 2, 1),
        y: height - 18 - offset, height: barHeight,
      }, svgEl("title", {}, `${new Date(bucket.zeit).toLocaleTimeString("de-DE")} · ${level}: ${count}`)));
    }
  });
  const first = new Date(buckets[0].zeit).toLocaleTimeString("de-DE");
  const last = new Date(buckets[buckets.length - 1].zeit).toLocaleTimeString("de-DE");
  svg.append(svgEl("text", { class: "axis", x: pad, y: height - 4 }, first));
  svg.append(svgEl("text", { class: "axis", x: width - pad, y: height - 4, "text-anchor": "end" }, last));
  const seconds = Math.round((data.bucketMs ?? 0) / 1000);
  svg.append(svgEl("text", { class: "axis", x: width / 2, y: height - 4, "text-anchor": "middle" },
    `ein Balken = ${seconds || 1} s · ziehen wählt einen Zeitraum`));

  attachRangeSelection(svg, buckets, data.bucketMs ?? 1000, { width, height, pad, step });
  target.replaceChildren(svg);
}

// Dragging across the histogram is the fastest way to say "show me what
// happened around then". The mapping goes through the bucket timestamps the
// server sent, so the selection means the same thing the bars mean.
function attachRangeSelection(svg, buckets, bucketMs, box) {
  const selection = svgEl("rect", { class: "selection", y: 6, height: box.height - 24, width: 0, x: 0 });
  svg.append(selection);

  // A transparent surface over the whole plot: without it the pointer only
  // reacts on the bars themselves, and the gaps between them would swallow
  // half the drags.
  const surface = svgEl("rect", { class: "surface", x: box.pad, y: 0,
    width: box.width - 2 * box.pad, height: box.height - 18 });
  svg.append(surface);

  const timeAt = (x) => {
    const index = Math.max(0, Math.min(buckets.length - 1, Math.floor((x - box.pad) / box.step)));
    return new Date(buckets[index].zeit).getTime();
  };
  const svgX = (event) => {
    const rect = svg.getBoundingClientRect();
    return ((event.clientX - rect.left) / rect.width) * box.width;
  };

  let start = null;
  surface.addEventListener("pointerdown", (event) => {
    start = svgX(event);
    surface.setPointerCapture(event.pointerId);
    selection.setAttribute("x", String(start));
    selection.setAttribute("width", "0");
  });
  surface.addEventListener("pointermove", (event) => {
    if (start === null) return;
    const current = svgX(event);
    selection.setAttribute("x", String(Math.min(start, current)));
    selection.setAttribute("width", String(Math.abs(current - start)));
  });
  const finish = (event) => {
    if (start === null) return;
    const end = svgX(event);
    const [left, right] = [Math.min(start, end), Math.max(start, end)];
    start = null;
    selection.setAttribute("width", "0");
    // Ein Klick ist keine Auswahl: unter ein paar Pixeln bleibt alles, wie es
    // war, sonst löscht ein Fehlklick den Blick auf alles.
    if (right - left < 4) return;
    logState.from = timeAt(left);
    // Das rechte Ende schließt den getroffenen Balken mit ein.
    logState.to = timeAt(right) + bucketMs;
    void loadLog();
  };
  surface.addEventListener("pointerup", finish);
  surface.addEventListener("pointercancel", () => { start = null; selection.setAttribute("width", "0"); });
}

// The sidebar answers questions before they are typed: which status codes
// occur, which routes, which scrape pools.
function renderFields(data) {
  const fields = data.felder ?? [];
  const target = $("#logFields");
  if (!fields.length) {
    target.replaceChildren(el("p", { class: "hint" }, "Keine strukturierten Felder in diesen Zeilen."));
    return;
  }
  target.replaceChildren(el("h4", {}, "Felder"), ...fields.map((field) => {
    const values = field.werte.map((entry) => {
      const share = Math.round((entry.anzahl / field.gesamt) * 100);
      const row = el("button", { class: "fieldvalue" },
        el("span", { class: "v" }, entry.wert),
        el("span", { class: "n" }, share + "%"));
      row.addEventListener("click", () => {
        if (!logState.fields.some((f) => f.name === field.name && f.value === entry.wert)) {
          logState.fields.push({ name: field.name, value: entry.wert });
        }
        void loadLog();
      });
      return row;
    });
    return el("details", { class: "field" }, el("summary", {}, field.name,
      el("span", { class: "n" }, String(field.gesamt))), ...values);
  }));
}

function renderLines(data) {
  const output = $("#logs");
  if (data.hinweis) {
    output.replaceChildren(el("p", { class: "empty" }, "Kein Log: " + data.hinweis));
    return;
  }
  const entries = data.eintraege ?? [];
  if (!entries.length) {
    output.replaceChildren(el("p", { class: "empty" },
      data.gesamt ? "Kein Treffer für diesen Filter." : "Der Pod hat noch nichts geschrieben."));
    return;
  }
  output.replaceChildren(...entries.map((entry) => {
    const row = el("details", { class: "logline " + LEVEL_CLASS[entry.level] });
    const time = entry.zeit ? new Date(entry.zeit).toLocaleTimeString("de-DE") : "—";
    row.append(el("summary", {},
      el("time", {}, time),
      el("span", { class: "lvl" }, entry.level),
      el("span", { class: "msg" }, entry.nachricht)));
    const fields = Object.entries(entry.felder ?? {});
    if (fields.length) {
      row.append(el("table", { class: "doc" }, el("tbody", {}, ...fields.map(([name, value]) => {
        const add = el("button", { class: "quiet tiny" }, "filtern");
        add.addEventListener("click", () => {
          if (!logState.fields.some((f) => f.name === name && f.value === value)) {
            logState.fields.push({ name, value });
          }
          void loadLog();
        });
        return el("tr", {}, el("td", { class: "k" }, name), el("td", { class: "v" }, value), el("td", {}, add));
      }))));
    }
    row.append(el("pre", { class: "raw" }, entry.roh));
    return row;
  }));
  output.scrollTop = 0;
}

// ---------------------------------------------------------------- tickets ---

// The board follows Jira's vocabulary because that is the one we all already
// know: project, issue type, priority, epic, component, fix version.
let META = { spalten: [], projekte: [], typen: [], prioritaeten: [], version: "" };
let TICKETS = [];
const AUTHORS = ["Marco", "Claude Code", "Codex"];
const COLUMN_CLASS = { "Backlog": "backlog", "Ready": "ready", "In Bearbeitung": "doing", "Test": "test", "Fertig": "done" };
const TYPE_MARK = { Epic: "◆", Task: "▸", Bug: "●", Beobachtung: "◇", Protokoll: "▪" };
const PRIORITY_MARK = { Hoch: "↑", Mittel: "→", Niedrig: "↓" };

// Records are the imported Austausch.md protocol: 28 finished entries that
// would otherwise bury the Fertig column.
const filters = { projekt: "", typ: "", zustaendig: "", version: "", suche: "", archiv: false };

const author = () => localStorage.getItem("mc.author") || "Marco";

function matchesFilters(ticket) {
  if (ticket.type === "Protokoll" && !filters.archiv) return false;
  if (filters.projekt && ticket.project !== filters.projekt) return false;
  if (filters.typ && ticket.type !== filters.typ) return false;
  if (filters.zustaendig && ticket.owner !== filters.zustaendig) return false;
  if (filters.version && ticket.version !== filters.version) return false;
  if (filters.suche) {
    const haystack = `${ticket.key} ${ticket.title} ${ticket.body} ${(ticket.tags || []).join(" ")}`.toLowerCase();
    if (!haystack.includes(filters.suche.toLowerCase())) return false;
  }
  return true;
}

function filterBar(target, onChange) {
  const bar = el("div", { class: "filterbar" });
  const select = (label, key, options) => {
    const node = el("select", {}, el("option", { value: "" }, label),
      ...options.map((value) => el("option", value === filters[key] ? { selected: "selected" } : {}, value)));
    node.addEventListener("change", () => { filters[key] = node.value; onChange(); });
    return node;
  };
  const owners = [...new Set(TICKETS.map((t) => t.owner).filter(Boolean))].sort();
  const versions = [...new Set(TICKETS.map((t) => t.version).filter(Boolean))].sort();

  const search = el("input", { type: "search", placeholder: "Suchen …", value: filters.suche });
  search.addEventListener("input", () => { filters.suche = search.value; onChange(); });

  bar.append(search,
    select("Alle Projekte", "projekt", META.projekte.map((p) => p.key)),
    select("Alle Typen", "typ", META.typen),
    select("Alle Zuständigen", "zustaendig", owners),
    select("Alle Versionen", "version", versions));

  const archive = el("label", { class: "inline" },
    el("input", { type: "checkbox", ...(filters.archiv ? { checked: "checked" } : {}) }), " Protokoll zeigen");
  archive.querySelector("input").addEventListener("change", (event) => {
    filters.archiv = event.currentTarget.checked; onChange();
  });
  bar.append(archive);

  if (Object.entries(filters).some(([key, value]) => key !== "archiv" && value)) {
    const reset = el("button", { class: "quiet" }, "Filter zurücksetzen");
    reset.addEventListener("click", () => {
      Object.keys(filters).forEach((key) => { if (key !== "archiv") filters[key] = ""; });
      onChange();
    });
    bar.append(reset);
  }
  target.replaceChildren(bar, el("p", { class: "hint" },
    `${TICKETS.filter(matchesFilters).length} von ${TICKETS.length} Tickets sichtbar`));
}

// Zwei Boards statt eines gemischten. HF und MC sind zwei Produkte mit eigenen
// Releases, eigenen Abläufen und eigener Versionierung; in einer Spalte
// gemischt begraben 60 HF-Tickets die 20 MC-Tickets (MC-0021).
let BOARD_PROJEKT = localStorage.getItem("mc.board.projekt") || "HF";

// Die Reiter sind der einzige Ort, an dem das Projekt gewaehlt wird. Sie
// schreiben in filters.projekt, damit jede Ansicht, die matchesFilters
// benutzt, ohne Zutun mitzieht.
function projektReiter(container, onChange) {
  const projekte = META.projekte.map((p) => p.key);
  if (!projekte.includes(BOARD_PROJEKT)) BOARD_PROJEKT = projekte[0] ?? "HF";
  filters.projekt = BOARD_PROJEKT;
  container.replaceChildren(...projekte.map((key) => {
    const offen = TICKETS.filter((t) => t.project === key && t.status !== "Fertig").length;
    const meta = META.projekte.find((p) => p.key === key) ?? {};
    const tab = el("button", {
      class: "board-tab" + (key === BOARD_PROJEKT ? " active" : ""),
      title: meta.description ?? "",
    }, meta.name ? `${key} — ${meta.name}` : key, el("span", { class: "count" }, String(offen)));
    tab.addEventListener("click", () => {
      BOARD_PROJEKT = key;
      localStorage.setItem("mc.board.projekt", key);
      onChange();
    });
    return tab;
  }));
}

function renderBoard() {
  projektReiter($("#boardTabs"), renderBoard);
  filterBar($("#boardFilters"), renderBoard);
  const visible = TICKETS.filter(matchesFilters);
  const veraltet = visible.filter((t) => t.lebenslauf?.claimVeraltet);
  $("#boardWarn").replaceChildren(...(veraltet.length
    ? [el("p", { class: "board-warn" },
        veraltet.length + " Claim(s) stehen seit über vier Stunden still: ",
        ...veraltet.map((t) => el("code", {}, t.key + " ")),
        " — ein Claim, an dem niemand arbeitet, sperrt Pfade ohne Zweck.")]
    : []));

  $("#board").replaceChildren(...META.spalten.map((column) => {
    const inColumn = visible.filter((ticket) => ticket.status === column);
    const body = el("div", { class: "column-body" });
    if (!inColumn.length) body.append(el("p", { class: "column-empty" }, "—"));
    else body.append(...inColumn.map((ticket) => ticketCard(ticket, true)));
    return el("section", { class: `column ${COLUMN_CLASS[column] ?? ""}` },
      el("header", {}, el("h3", {}, column), el("span", { class: "count" }, String(inColumn.length))),
      body);
  }));
}


// The backlog groups by epic, which is the only structure 53 imported tickets
// have; everything else is grouped by type.
function renderBacklog() {
  projektReiter($("#backlogTabs"), renderBacklog);
  filterBar($("#backlogFilters"), renderBacklog);
  const visible = TICKETS.filter(matchesFilters);
  const epics = visible.filter((t) => t.type === "Epic");
  const loose = visible.filter((t) => t.type !== "Epic" && !t.epic);
  const byType = {};
  for (const ticket of loose) (byType[ticket.type] ||= []).push(ticket);

  const container = $("#backlogList");
  container.replaceChildren();
  for (const epic of epics) {
    const children = visible.filter((t) => t.epic === epic.key);
    container.append(el("section", { class: "epic-group" },
      el("h3", {}, el("span", { class: "type epic" }, TYPE_MARK.Epic), epic.key + " · " + epic.title),
      ticketCard(epic, false),
      ...(children.length
        ? children.map((t) => ticketCard(t, false))
        : [el("p", { class: "column-empty" }, "noch keine untergeordneten Tickets")])));
  }
  for (const type of META.typen) {
    const list = byType[type];
    if (!list?.length) continue;
    container.append(el("section", { class: "epic-group" },
      el("h3", {}, el("span", { class: "type" }, TYPE_MARK[type] ?? "▸"), type + " (" + list.length + ")"),
      ...list.map((t) => ticketCard(t, false))));
  }
  if (!container.children.length) container.append(el("p", { class: "empty" }, "Nichts passt zu diesem Filter."));
}

function ticketCard(ticket, compact) {
  const details = el("details", { class: "ticket" + (compact ? " compact" : "") });
  details.append(el("summary", {},
    el("span", { class: "type " + ticket.type.toLowerCase(), title: ticket.type }, TYPE_MARK[ticket.type] ?? "▸"),
    el("span", { class: "id" }, ticket.key),
    el("span", { class: "prio " + (ticket.priority || "").toLowerCase(), title: "Priorität " + ticket.priority }, PRIORITY_MARK[ticket.priority] ?? ""),
    el("span", { class: "title" }, ticket.title),
    // Zwei verschiedene Aussagen, deshalb zwei verschiedene Marken: worin es
    // gefunden wurde, und worin es ausgeliefert ist. Eine Zahl für beides hat
    // jedes offene Ticket behaupten lassen, zu einer fertigen Release zu
    // gehören.
    ...(ticket.delivered
      ? [el("span", { class: "version shipped", title: "ausgeliefert in" }, "✓ " + ticket.delivered)]
      : []),
    ...(ticket.version
      ? [el("span", { class: "version found", title: "gefunden in Version " + ticket.version }, "gef. " + ticket.version)]
      : []),
    ...(compact ? [] : [el("span", { class: "tag" }, ticket.owner || "offen")]),
    ...(ticket.component ? [el("span", { class: "tag" }, ticket.component)] : []),
    ...((ticket.scope ?? []).length ? [el("span", { class: "tag scope", title: ticket.scope.join(", ") },
      "⛁ " + ticket.scope.length)] : []),
  ));
  details.append(lifecycleRow(ticket));
  if (ticket.body) {
    details.append(el("section", { class: "block" },
      el("h4", {}, "Beschreibung"),
      el("div", { class: "body" }, ticket.body)));
  }
  const comments = ticket.comments ?? [];
  details.append(el("section", { class: "block" },
    el("h4", {}, comments.length ? `Kommentare (${comments.length})` : "Kommentare"),
    ...(comments.length
      ? comments.map((comment) => el("div", { class: "comment" },
          el("div", { class: "comment-head" },
            el("b", {}, comment.author),
            el("time", {}, new Date(comment.at).toLocaleString("de-DE"))),
          el("div", { class: "comment-body" }, comment.text)))
      : [el("p", { class: "hint" }, "Noch nichts festgehalten.")]),
    commentRow(ticket)));
  details.append(el("section", { class: "block" }, el("h4", {}, "Felder"), fieldRow(ticket)));
  return details;
}

// Der Lebenslauf beantwortet, was bisher nirgends stand: seit wann es das
// Ticket gibt, wie lange es schon liegt, und ob es je ausgeliefert wurde -
// mit Datum, nicht nur mit Versionsnummer (MC-0021).
function lifecycleRow(ticket) {
  const life = ticket.lebenslauf;
  if (!life) return el("div", { class: "life" });
  const datum = (value) => new Date(value).toLocaleDateString("de-DE");
  const teile = [
    el("span", {}, "angelegt " + datum(life.erstellt)),
    ...(ticket.version ? [el("span", { class: "dim" }, "gefunden in " + ticket.version)] : []),
  ];
  if (life.geliefertAm) {
    teile.push(el("span", { class: "shipped" },
      "ausgeliefert " + datum(life.geliefertAm) + " in " + life.geliefert));
    teile.push(el("span", { class: "dim" }, life.tageOffen + " Tage offen"));
  } else if (life.geliefert) {
    // Vor MC-0021 gesetzte Auslieferungen haben keine Datumsangabe. Das ist
    // ehrlicher als ein erfundenes Datum.
    teile.push(el("span", { class: "shipped" }, "ausgeliefert in " + life.geliefert));
  } else {
    teile.push(el("span", { class: "dim" }, "seit " + life.tageOffen + " Tagen offen"));
  }
  if (life.claimVeraltet) {
    teile.push(el("span", { class: "stale" }, "Claim steht seit über vier Stunden still"));
  }
  return el("div", { class: "life" }, ...teile);
}

function fieldRow(ticket) {
  const change = (field, value) => act(() =>
    api(`/api/tickets/${ticket.key}/${field}`, jsonBody({ wert: value, author: author() })));
  // Ein Ticket, das nicht auf "In Bearbeitung" steht, ist nicht beansprucht.
  // Daran zu arbeiten ist erlaubt und war heute trotzdem ein Fehler, weil es
  // niemand sah. Jetzt steht es da - als Hinweis, nicht als Sperre: die Regel
  // gehoert zu den Menschen, die Sichtbarkeit zum Werkzeug (MC-0021).
  const unbeansprucht = ticket.status !== "In Bearbeitung"
    ? el("span", { class: "unclaimed", title: "Status ist " + ticket.status },
        "nicht beansprucht")
    : null;
  const picker = (field, current, options) => {
    const node = el("select", {}, ...options.map((value) =>
      el("option", value === current ? { selected: "selected" } : {}, value)));
    node.addEventListener("change", () => change(field, node.value));
    return node;
  };
  const version = el("input", { value: ticket.version ?? "", size: "8", title: "gefunden in Version" });
  version.addEventListener("change", () => change("version", version.value));
  // Der Scope ist die Verbindung zum Repo: er macht aus einem Ticket in
  // Bearbeitung einen Claim auf Dateien.
  const scope = el("input", {
    value: (ticket.scope ?? []).join(", "), placeholder: "Scope: backend/internal/pets/, Makefile",
    class: "scope",
  });
  scope.addEventListener("change", () => change("scope", scope.value));
  return el("div", { class: "row" },
    picker("status", ticket.status, META.spalten),
    picker("priority", ticket.priority, META.prioritaeten),
    picker("type", ticket.type, META.typen),
    picker("owner", ticket.owner, ["offen", ...AUTHORS]),
    version, scope, ...(unbeansprucht ? [unbeansprucht] : []));
}

function commentRow(ticket) {
  const text = el("textarea", { rows: "2", placeholder: "Kommentar …" });
  const send = el("button", { class: "quiet" }, "Kommentieren");
  send.addEventListener("click", () => {
    if (!text.value.trim()) return;
    act(() => api(`/api/tickets/${ticket.key}/comments`, jsonBody({ author: author(), text: text.value })));
  });
  return el("div", { class: "row" }, text, send);
}

const jsonBody = (value) => ({ method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) });

async function act(operation) {
  try {
    await operation();
    banner(null);
    await loadTickets();
  } catch (error) {
    banner(error.message);
  }
}

async function loadTickets() {
  TICKETS = (await api("/api/tickets")).tickets ?? [];
  renderBoard();
  renderBacklog();
}

// ------------------------------------------------------------------- repo ---

// The repository view is the join: which ticket owns which files, and which
// commit belongs to which ticket. Both directions matter - one catches
// collisions before they happen, the other says what was actually done.
// Ein Spiegel hat drei Zustände, die man nicht verwechseln darf: nicht
// erreichbar (Datenträger nicht eingehängt), erreichbar aber im Rückstand, und
// aktuell. Nur der dritte ist eine Sicherung.
function mirrorRow(m) {
  const alter = m.stand ? Math.floor((Date.now() - new Date(m.stand)) / 86400000) : null;
  let zustand, klasse;
  if (!m.erreichbar) {
    zustand = "nicht erreichbar";
    klasse = "mirror-bad";
  } else if (m.rueckstand > 0) {
    zustand = m.rueckstand + " Commit(s) zurück";
    klasse = "mirror-warn";
  } else {
    zustand = "aktuell";
    klasse = "mirror-ok";
  }
  return el("div", { class: "mirror " + klasse },
    el("span", { class: "mirror-name", title: m.url || "" }, m.name),
    el("span", { class: "mirror-state" }, zustand),
    el("span", { class: "mirror-age" },
      alter === null ? "" : alter === 0 ? "heute" : "vor " + alter + " Tag(en)"));
}

async function loadRepo() {
  try {
    const data = await api("/api/repo");
    const target = $("#repoState");
    const head = el("section", { class: "card" },
      el("h2", {}, "Repository"),
      el("div", { class: "repo-facts" },
        fact("Branch", data.branch || "—"),
        fact("Remote", data.remote || "keins"),
        fact("Commits", data.hatCommits ? String(data.commits.length) : "0"),
        fact("Unversionierte Änderungen", String((data.aenderungen ?? []).length))),
      ...(data.hinweise ?? []).map((note) => el("p", { class: "repo-note" }, note)));

      // Spiegeln ist nicht das Problem. Zu merken, dass es seit Wochen nicht
      // passiert ist, schon - deshalb steht der Rückstand hier und nicht in
      // einer Skriptausgabe, die niemand aufruft (ADR 0008, HF-0106).
      const spiegel = data.spiegel ?? [];
      const spiegelCard = el("section", { class: "card" },
        el("h2", {}, "Spiegel"),
        spiegel.length
          ? el("div", { class: "mirror-list" }, ...spiegel.map(mirrorRow))
          : el("p", { class: "hint" },
              "Keiner eingerichtet. Die Historie liegt genau einmal, auf dieser Platte. "
              + "„make mirror-setup“ legt den lokalen Spiegel an."));

    const conflicts = (data.tickets ?? []).filter((t) => (t.kollisionen ?? []).length);
    const conflictCard = conflicts.length
      ? el("section", { class: "card conflict-card" },
          el("h2", {}, "Kollidierende Claims"),
          el("p", { class: "hint" }, "Zwei Tickets in Bearbeitung auf denselben Pfaden."),
          ...conflicts.map((ticket) => el("div", { class: "conflict" },
            el("b", {}, ticket.key), " (", ticket.zustaendig, ") ",
            ...ticket.kollisionen.map((c) => el("span", {}, "↔ ", el("b", {}, c.mit), " (", c.zustaendig, "): ",
              el("code", {}, c.grund.join(" · ")))))))
      : null;

    const linked = (data.tickets ?? []).filter((t) => (t.scope ?? []).length || (t.commits ?? []).length);
    const ticketCard = el("section", { class: "card" },
      el("h2", {}, "Tickets mit Repo-Bezug"),
      el("p", { class: "hint" }, linked.length
        ? "Scope verbindet ein Ticket mit Dateien, der Ticketschlüssel im Commit-Betreff mit Commits."
        : "Noch kein Ticket hat einen Scope. Im Backlog beim Ticket unter „Scope“ eintragen, z. B. backend/internal/pets/"),
      ...linked.map((ticket) => el("div", { class: "repo-ticket" },
        el("div", { class: "line" }, el("b", {}, ticket.key), " ", ticket.titel,
          el("span", { class: "tag" }, ticket.status), el("span", { class: "tag" }, ticket.zustaendig)),
        ...(ticket.scope ?? []).map((pattern) => el("code", { class: "scope-pattern" }, pattern)),
        ...(ticket.aenderungen ?? []).map((change) => el("div", { class: "change" },
          el("span", { class: "st" }, change.status), " ", change.pfad)),
        ...(ticket.commits ?? []).map((commit) => el("div", { class: "commit" },
          el("code", {}, commit.kurz), " ", commit.betreff,
          el("span", { class: "who" }, commit.autor))))));

    const orphans = (data.aenderungen ?? []).filter((change) => !change.ticket);
    const orphanCard = el("section", { class: "card" },
      el("h2", {}, "Änderungen ohne Ticket"),
      el("p", { class: "hint" }, orphans.length
        ? orphans.length + " von " + data.aenderungen.length + " geänderten Pfaden gehören zu keinem Ticket-Scope."
        : "Jede Änderung ist einem Ticket zugeordnet."),
      ...orphans.slice(0, 40).map((change) => el("div", { class: "change" },
        el("span", { class: "st" }, change.status), " ", change.pfad)),
      ...(orphans.length > 40 ? [el("p", { class: "hint" }, "… und " + (orphans.length - 40) + " weitere")] : []));

    target.replaceChildren(head, spiegelCard, ...(conflictCard ? [conflictCard] : []),
      ticketCard, orphanCard);
  } catch (error) {
    banner(error.message);
  }
}

const fact = (label, value) => el("div", { class: "fact" },
  el("span", {}, label), el("strong", {}, value));

// ----------------------------------------------------------------- deploy ---

// The only writing view. It asks twice on purpose: once by making the button
// say what it will do, once by confirming - a deployment replaces the running
// application.
let deployTimer = null;

async function loadDeploy() {
  try {
    const data = await api("/api/deploy");
    const target = $("#deployPanel");
    if (!data.aktiv) {
      target.replaceChildren(el("section", { class: "card" },
        el("h2", {}, "Deploy"), el("p", { class: "hint" }, data.hinweis)));
      return;
    }
    const running = data.laufend && data.laufend.status === "läuft" ? data.laufend : null;

    // Was würde sich überhaupt ändern? Ohne Antwort darauf ist jeder Knopf
    // eine Einladung, die laufende Anwendung ohne Grund zu ersetzen.
    let checks = {};
    try {
      const result = await api("/api/deploy/changes");
      for (const assessment of result.pipelines) checks[assessment.pipeline] = assessment;
    } catch (error) {
      banner("Änderungsprüfung fehlgeschlagen: " + error.message);
    }

    const pipelines = el("section", { class: "card" },
      el("h2", {}, "Abläufe"),
      el("p", { class: "hint" },
        `Nur gegen den lokalen Cluster „${data.cluster}“. Mission Control führt ausschließlich diese make-Ziele aus - keine freien Kommandos. Ein Ablauf startet nur, wenn sich dadurch wirklich etwas ändert.`),
      ...data.pipelines.map((pipeline) => {
        const check = checks[pipeline.name] ?? {};
          const stand = (data.stand ?? []).find((s) => s.pipeline === pipeline.name) ?? {};
        const changes = check.aenderungen ?? [];
        // Gesperrt wird nur, was ein laufendes System verändert. Ein Ablauf,
        // der bloß prüft und baut, kostet zwanzig Sekunden und richtet keinen
        // Schaden an - ihn zu sperren hieße, jemanden auszusperren, der genau
        // das will.
        const blocked = pipeline.brauchtCluster && !changes.length && !check.fehler;
        const button = el("button", { class: pipeline.brauchtCluster ? "primary" : "quiet" },
          running ? "läuft …" : blocked ? "nichts zu tun" : pipeline.titel);
        if (running || blocked) button.setAttribute("disabled", "disabled");
          // Was laeuft gerade, und worauf ginge ein Rueckfall? Beides weiss
          // seit MC-0018 das Deploy-Journal und ueberlebt einen Neustart des
          // Cockpits. Ohne zweiten Eintrag steht hier nichts - eine
          // Rueckfall-Anzeige ohne Ziel waere ein Versprechen (HF-0104).
          const standZeile = stand.aktuell
            ? el("div", { class: "deploy-stand" },
                el("span", {}, "ausgeliefert: "),
                el("b", {}, stand.aktuell.version || "ohne Version"),
                el("span", { class: "dim" },
                  " am " + new Date(stand.aktuell.zeit).toLocaleString("de-DE")),
                ...(stand.rueckfall
                  ? [el("span", { class: "dim" }, " · zurueck auf "),
                     el("code", {}, stand.rueckfall.version || stand.rueckfall.commit.slice(0, 8))]
                  : [el("span", { class: "dim" }, " · kein Rueckfallziel")]))
            : el("div", { class: "deploy-stand dim" }, "noch nie ueber die Pipeline ausgeliefert");

        // Die Tickets, die dieser Ablauf abnehmen kann, stehen als Kästchen da -
        // vorausgewählt, aber abwählbar. Wer sie abtippen muss, startet den Lauf
        // dreimal; wer sie gar nicht sieht, schließt versehentlich fremde Arbeit.
        const covered = (data.abnahme ?? []).filter((entry) => entry.pipeline === pipeline.name);
        const boxes = covered.map((entry) => {
          const input = el("input", { type: "checkbox", checked: "checked" });
          return { key: entry.key, input,
            node: el("label", { class: "ticketbox" }, input,
              el("span", {}, `${entry.key} — ${entry.titel}`)) };
        });
        const selected = () => boxes.filter((box) => box.input.checked).map((box) => box.key);
        const refreshLabel = () => {
          if (running || blocked) return;
          const count = selected().length;
          button.textContent = count
            ? `${pipeline.titel} (${count} Ticket${count === 1 ? "" : "s"})`
            : pipeline.titel;
        };
        for (const box of boxes) box.input.addEventListener("change", refreshLabel);
        refreshLabel();
        button.addEventListener("click", () => startDeploy(pipeline, false, selected()));

        const status = changes.length
          ? el("ul", { class: "changes" }, ...changes.map((change) =>
              el("li", {}, el("b", {}, change.art + ": "), change.detail)))
          : el("p", { class: "hint nothing" }, check.fehler || check.hinweis || "Wird geprüft …");

        const side = el("div", { class: "pipeline-actions" }, button);
        if (blocked) {
          // Quelltext, der sich ohne Versionssprung geändert hat, sieht die
          // Prüfung nicht. Deshalb gibt es einen Ausweg - benannt als das,
          // was er ist.
          const force = el("button", { class: "quiet tiny" }, "trotzdem deployen");
          force.addEventListener("click", () => startDeploy(pipeline, true, selected()));
          side.append(force);
        }
        return el("div", { class: "pipeline" + (blocked ? " blocked" : "") },
          el("div", {},
            el("b", {}, pipeline.titel),
            ...(pipeline.brauchtCluster ? [el("span", { class: "tag warnrot" }, "verändert den Cluster")] : [el("span", { class: "tag" }, "ändert nichts")]),
            el("p", { class: "hint" }, pipeline.beschreibung),
              standZeile,
            status,
            ...(boxes.length
              ? [el("div", { class: "ticketboxes" },
                  el("p", { class: "hint" }, "Nimmt bei Erfolg ab:"), ...boxes.map((box) => box.node))]
              : []),
            ...(check.geprueft?.length ? [el("p", { class: "hint checked" }, "Geprüft: " + check.geprueft.join(" · "))] : []),
            el("code", { class: "steps" }, pipeline.schritte.map((s) => "make " + s.args.join(" ")).join("  →  "))),
          side);
      }));

    // Ein gebautes, aber nicht übernommenes Binary ist der Zustand, den man
    // sonst vergisst - deshalb steht der Neustart neben den Abläufen und nicht
    // in einem Menü.
    const restart = el("section", { class: "card" },
      el("h2", {}, "Dienst neu starten"),
      el("p", { class: "hint" },
        "Übernimmt ein neu gebautes Mission-Control-Binary. Die Seite verliert dabei kurz die Verbindung."));
    const restartButton = el("button", { class: "quiet" }, "Neu starten");
    restartButton.addEventListener("click", async () => {
      if (!confirm("Mission Control neu starten? Die Verbindung bricht für ein paar Sekunden ab.")) return;
      try {
        await api("/api/deploy/restart", { method: "POST" });
        banner("Neustart läuft - die Seite lädt gleich neu.");
        setTimeout(() => location.reload(), 4000);
      } catch (error) {
        banner(error.message);
      }
    });
    restart.append(restartButton);

    // Die Frage, die diese Ansicht bisher offen ließ: was wartet auf Abnahme,
    // und kann ein Deploy sie überhaupt geben?
      const waiting = data.abnahme ?? [];
      const acceptance = el("section", { class: "card" },
        el("h2", {}, "Wartet auf Abnahme"),
        el("p", { class: "hint" }, waiting.length
          ? "Tickets in der Spalte Test. Ein erfolgreicher Deploy zieht nur die auf Fertig, deren Pfade er auch ausrollt \u2014 alle anderen nimmst du hier von Hand ab."
          : "Nichts steht auf Test."),
        ...waiting.map((entry) => {
          // Eine Liste, die einen offenen Vorgang zeigt und keinen Weg anbietet,
          // ihn zu schlie\u00dfen, ist eine Liste zum Anstarren. Acht Tickets, die
          // kein Ablauf abdeckt, blieben genau deshalb stehen (MC-0020).
          const abnehmen = el("button", { class: "quiet tiny" }, "abnehmen");
          abnehmen.addEventListener("click", async () => {
            abnehmen.setAttribute("disabled", "disabled");
            await act2(async () => {
              // Erst begr\u00fcnden, dann verschieben: ein Status, der sich unerkl\u00e4rt
              // \u00e4ndert, ist schlimmer als einer von Hand.
              await api(`/api/tickets/${entry.key}/comments`, jsonBody({
                author: author(),
                text: entry.pipeline
                  ? "Von Hand abgenommen, obwohl der Ablauf \u201e" + entry.pipeline
                    + "\u201c die Pfade abdeckt. Das Feld geliefert bleibt leer: "
                    + "ausgeliefert wurde dabei nichts."
                  : "Von Hand abgenommen. Kein Ablauf deckt diese Pfade ab \u2014 "
                    + "die Abnahme war Lesen, nicht Ausliefern. Das Feld geliefert "
                    + "bleibt deshalb leer.",
              }));
              await api(`/api/tickets/${entry.key}/status`, jsonBody({
                wert: "Fertig", author: author(),
              }));
            });
          });
          return el("div", { class: "waiting" + (entry.pipeline ? "" : " unreachable") },
            el("b", {}, entry.key),
            el("span", { class: "titel" }, entry.titel),
            el("span", { class: "tag" }, entry.zustaendig),
            entry.pipeline
              ? el("span", { class: "tag ok" }, "\u2192 " + entry.pipeline)
              : el("span", { class: "hint" }, entry.grund),
            abnehmen);
        }));

    const cards = [pipelines, acceptance, restart];
    if (data.laufend) cards.push(runCard(data.laufend, !!running));
    if (data.verlauf?.length) {
      cards.push(el("section", { class: "card" }, el("h2", {}, "Verlauf"),
        ...data.verlauf.map((run) => el("div", { class: "run-row" },
          el("span", { class: "state " + statusClass(run.status) }, run.status),
          el("b", {}, "#" + run.id + " " + run.titel),
          el("span", { class: "hint" }, new Date(run.start).toLocaleString("de-DE") + " · " + run.gestartetVon
            + (run.ticket ? " · " + run.ticket : ""))))));
    }
    target.replaceChildren(...cards);

    // Solange etwas läuft, wird nachgeladen; danach hört das von selbst auf.
    clearInterval(deployTimer);
    deployTimer = running ? setInterval(loadDeploy, 2000) : null;
  } catch (error) {
    banner(error.message);
  }
}

const statusClass = (status) => ({ "läuft": "running", "erfolgreich": "ok", "fehlgeschlagen": "bad", "abgebrochen": "warn" }[status] ?? "");

function runCard(run, isRunning) {
  const card = el("section", { class: "card run-card " + statusClass(run.status) },
    el("h2", {}, "Durchlauf #" + run.id + " · " + run.titel),
    el("p", { class: "hint" },
      `${run.status}${run.schritt ? " · " + run.schritt : ""} · gestartet ${new Date(run.start).toLocaleTimeString("de-DE")} von ${run.gestartetVon}${run.ticket ? " · " + run.ticket : ""}`));
  if (isRunning) {
    const stop = el("button", { class: "quiet" }, "Abbrechen");
    stop.addEventListener("click", () => act2(() => api("/api/deploy/cancel", { method: "POST" })));
    card.append(stop);
  }
  const output = el("div", { class: "logs deploy-log" },
    ...(run.zeilen ?? []).map((line) => el("div", { class: "logline " + (line.fehler ? "err" : "info") },
      el("time", {}, new Date(line.zeit).toLocaleTimeString("de-DE")),
      el("span", { class: "lvl" }, line.schritt),
      el("span", { class: "msg" }, line.text))));
  card.append(output);
  // Beim Nachladen ans Ende springen, sonst muss man mitscrollen.
  requestAnimationFrame(() => { output.scrollTop = output.scrollHeight; });
  return card;
}

async function startDeploy(pipeline, force, ticketKeys) {
  const keys = ticketKeys ?? [];
  const lines = [pipeline.titel, "", pipeline.beschreibung];
  if (force) {
    lines.push("", "ACHTUNG: Die Prüfung hat keine Änderung gefunden. Dieser Lauf wird erzwungen und als erzwungen protokolliert.");
  }
  lines.push("", keys.length
    ? "Nimmt bei Erfolg ab: " + keys.join(", ")
    : "Es wird kein Ticket abgenommen.");
  if (!confirm(lines.join("\n"))) return;
  await act2(() => api("/api/deploy", jsonBody({
    pipeline: pipeline.name, author: author(), tickets: keys, trotzdem: !!force,
  })));
}

async function act2(operation) {
  try {
    await operation();
    banner(null);
  } catch (error) {
    banner(error.message);
  }
  await loadDeploy();
}

// -------------------------------------------------------------- versionen ---

async function loadVersions() {
  try {
    const data = await api("/api/versions");
    $("#versionGroups").replaceChildren(...data.gruppen.map((group) => el("section", { class: "card version-card" },
      el("h2", {}, group.title),
      ...(group.hint ? [el("p", { class: "hint" }, group.hint)] : []),
      el("table", { class: "versions" },
        el("tbody", {}, ...group.entries.map((entry) => el("tr", { class: entry.drift ? "drift" : "" },
          el("td", { class: "vname" }, entry.name),
          el("td", { class: "vvalue" }, entry.value),
          el("td", { class: "vsource" }, entry.source + (entry.note ? " · " + entry.note : "")))))),
    )));
  } catch (error) {
    banner(error.message);
  }
}

async function loadCluster() {
  try {
    renderCluster(await api("/api/cluster"));
  } catch (error) {
    banner(error.message);
  }
}

// ------------------------------------------------------------------- init ---

document.querySelectorAll("nav button").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelectorAll("nav button").forEach((other) => other.classList.toggle("active", other === button));
    for (const view of ["cluster", "board", "backlog", "repo", "deploy", "versions"]) {
      $("#view-" + view).hidden = button.dataset.view !== view;
    }
    if (button.dataset.view === "versions") void loadVersions();
    if (button.dataset.view === "repo") void loadRepo();
    if (button.dataset.view === "deploy") void loadDeploy();
  });
});

$("#logSearch").addEventListener("keydown", (event) => {
  if (event.key !== "Enter") return;
  logState.text = event.currentTarget.value.trim();
  void loadLog();
});

$("#refresh").addEventListener("click", () => act(async () => {
  renderCluster(await api("/api/cluster/refresh", { method: "POST" }));
}));

$("#newTicket").addEventListener("submit", (event) => {
  event.preventDefault();
  const form = event.currentTarget;
  const data = new FormData(form);
  act(async () => {
    await api("/api/tickets", jsonBody({
      project: String(data.get("project")),
      type: String(data.get("type")),
      title: String(data.get("title")),
      body: String(data.get("body") ?? ""),
      owner: String(data.get("owner") ?? "offen"),
      priority: String(data.get("priority")),
      version: String(data.get("version") ?? "").trim(),
      component: String(data.get("component") ?? "").trim(),
      scope: String(data.get("scope") ?? "").split(",").map((entry) => entry.trim()).filter(Boolean),
      tags: String(data.get("tags") ?? "").split(",").map((tag) => tag.trim()).filter(Boolean),
      author: author(),
    }));
    form.reset();
    fillFormOptions();
  });
});

function fillFormOptions() {
  const form = $("#newTicket");
  form.elements.project.replaceChildren(...META.projekte.map((p) =>
    el("option", { value: p.key, title: p.description }, p.key + " — " + p.name)));
  // Task ist der Normalfall; Epic legt man selten an, Protokoll gar nicht mehr
  // von Hand - das entsteht nur beim Import.
  form.elements.type.replaceChildren(...META.typen.filter((t) => t !== "Protokoll").map((t) =>
    el("option", t === "Task" ? { selected: "selected" } : {}, t)));
  form.elements.priority.replaceChildren(...META.prioritaeten.map((p) =>
    el("option", p === "Mittel" ? { selected: "selected" } : {}, p)));
  form.elements.version.placeholder = META.version;
}

META = await api("/api/board");
$("#mcVersion").textContent = "Mission Control v" + META.version;
fillFormOptions();

await loadCluster();
await loadTickets().catch((error) => banner(error.message));
setInterval(loadCluster, 15000);
