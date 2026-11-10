const REFRESH_INTERVAL_MS = 5000;

const API = {
  graph: "/api/graph",
};

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
      selector: "node.hovered",
      style: {
        "label": "data(id)",
        "background-color": STYLE.hovered.color,
      },
    },
    {
      selector: "node.selected",
      style: {
        "label": "data(id)",
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

cy.on("tap", "node", (event) => {
  cy.nodes(".selected").removeClass("selected");
  event.target.addClass("selected");
  document.body.dispatchEvent(
    new window.CustomEvent("node-selected", { bubbles: true, detail: { id: event.target.id() } }),
  );
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
async function refreshGraph() {
  const response = await window.fetch(API.graph);
  if (!response.ok) {
    return;
  }
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
    if (cy.getElementById(element.data.id).empty()) {
      cy.add({ group: "nodes", data: element.data });
      changed = true;
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
}

refreshGraph();
window.setInterval(refreshGraph, REFRESH_INTERVAL_MS);
