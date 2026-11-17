const REFRESH_INTERVAL_MS = 5000;
const LABEL_FONT_SIZE = 18;

const container = document.getElementById("graph");

const cy = cytoscape({
  container,
  style: [
    {
      selector: "node",
      style: {
        "label": "",
        "background-color": "#a6e22e",
        "color": "#f8f8f2",
        "font-size": 12,
        "text-valign": "bottom",
        "text-margin-y": 6,
        "width": 18,
        "height": 18,
      },
    },
    {
      selector: "node.hovered",
      style: {
        "label": "data(id)",
        "background-color": "#e6db74",
      },
    },
    {
      selector: "node.selected",
      style: {
        "label": "data(id)",
        "background-color": "#66d9ef",
      },
    },
    {
      selector: "edge",
      style: {
        "width": 1.5,
        "line-color": "#75715e",
        "target-arrow-color": "#75715e",
        "target-arrow-shape": "triangle",
        "curve-style": "bezier",
      },
    },
  ],
  layout: { name: "random" },
  wheelSensitivity: 0.2,
});

cy.on("mouseover", "node", (event) => event.target.addClass("hovered"));
cy.on("mouseout", "node", (event) => event.target.removeClass("hovered"));

const panelContent = document.getElementById("panel-content");

cy.on("tap", "node", (event) => {
  cy.nodes(".selected").removeClass("selected");
  event.target.addClass("selected");
  htmx.ajax("GET", `/api/nodes/${encodeURIComponent(event.target.id())}`, {
    target: panelContent,
    swap: "innerHTML",
  });
});

const divider = document.getElementById("divider");
const graphPane = document.getElementById("graph");
const layout = document.querySelector(".layout");

divider.addEventListener("pointerdown", (event) => {
  divider.setPointerCapture(event.pointerId);
  divider.addEventListener("pointermove", resizePanes);
});

divider.addEventListener("pointerup", (event) => {
  divider.releasePointerCapture(event.pointerId);
  divider.removeEventListener("pointermove", resizePanes);
  cy.resize();
});

function resizePanes(event) {
  const rect = layout.getBoundingClientRect();
  const fraction = (event.clientX - rect.left) / rect.width;
  const clamped = Math.min(0.9, Math.max(0.1, fraction));
  graphPane.style.flexBasis = `${clamped * 100}%`;
}

function updateLabelSize() {
  cy.nodes().style("font-size", LABEL_FONT_SIZE / cy.zoom());
}

cy.on("zoom", updateLabelSize);

async function refreshGraph() {
  const response = await fetch("/api/graph");
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
    cy.layout({ name: "breadthfirst", directed: true, spacingFactor: 1.2, padding: 20 }).run();
    updateLabelSize();
  }
}

refreshGraph();
setInterval(refreshGraph, REFRESH_INTERVAL_MS);
