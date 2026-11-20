const REFRESH_INTERVAL_MS = 5000;

const API = {
  graph: "/api/graph",
  namespaces: "/api/namespaces",
};

const namespaceSelect = document.getElementById("namespace");
const searchForm = document.getElementById("node-search");
const searchInput = document.getElementById("node-search-input");
const searchClear = document.getElementById("node-search-clear");
const searchStatus = document.getElementById("node-search-status");
const suggestionList = document.getElementById("node-suggestions");

let searchedNode = "";

function graphURL(node) {
  const params = new URLSearchParams({ namespace: namespaceSelect.value });
  if (node !== "") {
    params.set("node", node);
  }
  return `${API.graph}?${params}`;
}

function setSearchStatus(message) {
  searchStatus.textContent = message;
  searchInput.setAttribute("aria-invalid", message === "" ? "false" : "true");
}

async function refreshNamespaces() {
  const response = await window.fetch(API.namespaces);
  if (!response.ok) {
    return;
  }
  const namespaces = await response.json();
  const previous = namespaceSelect.value;

  namespaceSelect.replaceChildren(...namespaces.map((namespace) => new Option(namespace, namespace)));
  namespaceSelect.value = namespaces.includes(previous) ? previous : namespaces[0] ?? "";
}


const cssValues = window.getComputedStyle(document.documentElement);
const cssToken = (name) => cssValues.getPropertyValue(name).trim();
const cssNumber = (name) => parseFloat(cssToken(name));

const STYLE = {
  node: {
    color: cssToken("--graph-node-color"),
    labelColor: cssToken("--graph-node-label-color"),
    size: cssNumber("--graph-node-size"),
    labelMargin: cssNumber("--graph-node-label-margin"),
    labelFontSize: cssNumber("--graph-label-font-size"),
  },
  hovered: { color: cssToken("--graph-node-hovered-color") },
  selected: { color: cssToken("--graph-node-selected-color") },
  faulty: { color: cssToken("--graph-node-faulty-color") },
  pending: { color: cssToken("--graph-node-pending-color") },
  ok: { color: cssToken("--graph-node-ok-color") },
  edge: {
    width: cssNumber("--graph-edge-width"),
    color: cssToken("--graph-edge-color"),
  },
};

const LAYOUT = {
  initial: { name: "random" },
  tree: { name: "breadthfirst", directed: true, spacingFactor: 1.2, padding: 20 },
};

const INTERACTION = {
  wheelSensitivity: 0.2,
  minPaneFraction: 0.1,
  maxPaneFraction: 0.9,
};


const Containers = {
  divider: document.getElementById("divider"),
  graphPane: document.getElementById("graph"),
  layout: document.querySelector(".layout"),
}



const cy = window.cytoscape({
  container: Containers.graphPane,
  style: [
    {
      selector: "node",
      style: {
        "label": "",
        "background-color": STYLE.node.color,
        "color": STYLE.node.labelColor,
        "text-valign": "bottom",
        "text-margin-y": STYLE.node.labelMargin,
        "width": STYLE.node.size,
        "height": STYLE.node.size,
      },
    },
    {
      selector: "node[type = 'pdu']",
      style: { "shape": "rectangle" },
    },
    {
      selector: "node[type = 'baremetal']",
      style: { "shape": "star" },
    },
    {
      selector: "node[type = 'vm']",
      style: { "shape": "hexagon" },
    },
    {
      selector: "node[type = 'kubernetes']",
      style: { "shape": "diamond" },
    },
    {
      selector: "node[type = 'heat']",
      style: { "shape": "triangle" },
    },
    {
      selector: "node[status = 'ok']",
      style: { "background-color": STYLE.ok.color },
    },
    {
      selector: "node[status = 'faulty']",
      style: { "background-color": STYLE.faulty.color },
    },
    {
      selector: "node[status = 'pending']",
      style: { "background-color": STYLE.pending.color },
    },
    {
      selector: "node.hovered",
      style: {
        "label": "data(label)",
        "background-color": STYLE.hovered.color,
      },
    },
    {
      selector: "node.selected",
      style: {
        "label": "data(label)",
        "background-color": STYLE.selected.color,
      },
    },
    {
      selector: "edge",
      style: {
        "width": STYLE.edge.width,
        "line-color": STYLE.edge.color,
        "target-arrow-color": STYLE.edge.color,
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
      },
    },
  ],
  layout: LAYOUT.initial,
  wheelSensitivity: INTERACTION.wheelSensitivity,
});

// Node interaction logic
cy.on("mouseover", "node", (event) => event.target.addClass("hovered"));
cy.on("mouseout", "node", (event) => event.target.removeClass("hovered"));

function selectNode(node) {
  cy.nodes(".selected").removeClass("selected");
  node.addClass("selected");
  document.body.dispatchEvent(
    new window.CustomEvent("node-selected", { bubbles: true, detail: { id: node.id() } }),
  );
}

cy.on("tap", "node", (event) => selectNode(event.target));
cy.on("dblclick", "node", (event) => {
  searchInput.value = event.target.data("label");
  runSearch(event.target.data("label"));
});
cy.on("dblclick", (event) => {
  if (event.target === cy) {
    searchInput.value = "";
    runSearch("");
  }
});


// Resizing logic
function resizePanes(event) {
  const rect = Containers.layout.getBoundingClientRect();
  const fraction = (event.clientX - rect.left) / rect.width;
  const clamped = Math.min(INTERACTION.maxPaneFraction, Math.max(INTERACTION.minPaneFraction, fraction));
  Containers.graphPane.style.flexBasis = `${clamped * 100}%`;
}

Containers.divider.addEventListener("pointerdown", (event) => {
  Containers.divider.setPointerCapture(event.pointerId);
  Containers.divider.addEventListener("pointermove", resizePanes);
});

Containers.divider.addEventListener("pointerup", (event) => {
  Containers.divider.releasePointerCapture(event.pointerId);
  Containers.divider.removeEventListener("pointermove", resizePanes);
  cy.resize();
});

// Label size logic
function updateLabelSize() {
  cy.nodes().style("font-size", STYLE.node.labelFontSize / cy.zoom());
}
cy.on("zoom", updateLabelSize);

// Graph logic
async function refreshGraph(node = searchedNode, isSearch = false) {
  const response = await window.fetch(graphURL(node));
  if (response.status === 404 && node !== "") {
    if (isSearch) {
      setSearchStatus(`No node named "${node}" in ${namespaceSelect.value}`);
    } else if (node === searchedNode) {
      setSearchStatus(`Node "${node}" no longer exists in ${namespaceSelect.value}`);
      searchedNode = "";
      searchInput.value = "";
    }
    return false;
  }
  if (!response.ok) {
    return false;
  }
  if (isSearch) {
    searchedNode = node;
  }
  setSearchStatus("");
  const { nodes = [], edges = [] } = await response.json();

  const incomingIds = new Set([
    ...nodes.map((element) => element.data.id),
    ...edges.map((element) => element.data.id),
  ]);

  let changed = false;

  cy.elements().forEach((element) => {
    if (!incomingIds.has(element.id())) {
      element.remove();
      changed = true;
    }
  });

  nodes.forEach((element) => {
    const existing = cy.getElementById(element.data.id);
    if (existing.empty()) {
      cy.add({ group: "nodes", data: element.data });
      changed = true;
      return;
    }
    for (const [key, value] of Object.entries(element.data)) {
      if (existing.data(key) !== value) {
        existing.data(key, value);
      }
    }
  });

  edges.forEach((element) => {
    if (cy.getElementById(element.data.id).empty()) {
      cy.add({ group: "edges", data: element.data });
      changed = true;
    }
  });

  if (changed) {
    cy.layout(LAYOUT.tree).run();
    updateLabelSize();
  }
  return true;
}

async function runSearch(name) {
  const ok = await refreshGraph(name, true);
  if (ok) {
    cy.fit(undefined, 20);
  }
}

searchForm.addEventListener("submit", (event) => {
  event.preventDefault();
  clearSuggestions();
  runSearch(searchInput.value.trim());
});

searchClear.addEventListener("click", () => {
  searchInput.value = "";
  clearSuggestions();
  runSearch("");
});

function clearSuggestions() {
  suggestionList.replaceChildren();
  searchInput.setAttribute("aria-expanded", "false");
}

function activeSuggestion() {
  return suggestionList.querySelector(".suggestion.active");
}

function setActiveSuggestion(item) {
  activeSuggestion()?.classList.remove("active");
  if (item) {
    item.classList.add("active");
    searchInput.setAttribute("aria-activedescendant", item.id);
  } else {
    searchInput.removeAttribute("aria-activedescendant");
  }
}

function chooseSuggestion(item) {
  searchInput.value = item.dataset.label;
  clearSuggestions();
  runSearch(item.dataset.label).then(() => {
    const node = cy.getElementById(item.dataset.id);
    if (node.nonempty()) {
      selectNode(node);
    }
  });
}

suggestionList.addEventListener("click", (event) => {
  const item = event.target.closest(".suggestion");
  if (item) {
    chooseSuggestion(item);
  }
});

searchInput.addEventListener("keydown", (event) => {
  const items = [...suggestionList.querySelectorAll(".suggestion")];
  if (items.length === 0) {
    return;
  }
  const index = items.indexOf(activeSuggestion());
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    const step = event.key === "ArrowDown" ? 1 : -1;
    const next = (index + step + items.length) % items.length;
    setActiveSuggestion(items[next]);
  } else if (event.key === "Enter" && index !== -1) {
    event.preventDefault();
    chooseSuggestion(items[index]);
  } else if (event.key === "Escape") {
    clearSuggestions();
  }
});

searchInput.addEventListener("htmx:afterSettle", () => {
  searchInput.setAttribute("aria-expanded", suggestionList.children.length > 0 ? "true" : "false");
});

namespaceSelect.addEventListener("change", () => {
  searchedNode = "";
  searchInput.value = "";
  setSearchStatus("");
  clearSuggestions();
  refreshGraph();
});
refreshNamespaces().then(() => refreshGraph());
window.setInterval(refreshNamespaces, REFRESH_INTERVAL_MS);
window.setInterval(() => refreshGraph(), REFRESH_INTERVAL_MS);
