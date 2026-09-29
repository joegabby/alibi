// ---------- API URLs ----------
const baseURL = window.location.origin;

const PROJECTS_API = `${baseURL}/api/projects`;
const PROJECT_API = `${baseURL}/api/project`;
const COMMIT_API = `${baseURL}/api/commit`;
const AI_SUMMARY_API = `${baseURL}/api/ai-summary`;
const SAVE_SUMMARY_API = `${baseURL}/api/save`;
const GENERATE_PDF = `${baseURL}/api/generate-pdf`;
const DOWNLOAD_PDF = `${baseURL}/api/download-pdf`;
const MODELS_API = `${baseURL}/api/models`;

let providers = [
  { name: "OpenAI", logo: "./assets/imgs/openai.png" },
  { name: "DeepSeek", logo: "./assets/imgs/deepseek.png" },
  { name: "OpenRouter", logo: "./assets/imgs/openrouter.png" },
  { name: "HuggingFace", logo: "./assets/imgs/huggingface.png" },
];

// ---------- state ----------
let projects = [];
let currentProjectId = null;
let currentProjectData = null;
let selectedProvider = providers[0];
let currentProjectProviders = [];
// null means "let the server use the provider's default model"
let selectedModel = null;
// providerName -> ModelInfo[], cleared when the project changes because each
// project can hold different API keys
let modelsByProvider = {};
let filteredActivities = [];
let summaryError = "";
let currentPage = 1;
let pageSize = parseInt(document.getElementById("pageSize").value, 10);
let isCommtitView = false;

// ---------- helper functions ----------
const fmtDate = (iso) => {
  try {
    const d = new Date(iso);
    return new Intl.DateTimeFormat(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    }).format(d);
  } catch (e) {
    return iso;
  }
};

function makeEl(tag, attrs = {}, ...children) {
  const el = document.createElement(tag);
  for (const k in attrs) {
    if (k === "class") el.className = attrs[k];
    else if (k.startsWith("data-")) el.setAttribute(k, attrs[k]);
    else if (k === "html") el.innerHTML = attrs[k];
    else el[k] = attrs[k];
  }
  for (const c of children) {
    if (c == null) continue;
    el.append(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return el;
}

// ---------- model helpers ----------

// Model catalogues come from the server, which holds the API keys. The browser
// never sees a key.
async function fetchModels(providerName) {
  if (!providerName || !currentProjectId) return [];
  if (modelsByProvider[providerName]) return modelsByProvider[providerName];

  const url =
    `${MODELS_API}?provider=${encodeURIComponent(providerName)}` +
    `&project=${encodeURIComponent(currentProjectId)}`;

  const response = await fetch(url);
  if (!response.ok) {
    const detail = (await response.text()).trim();
    throw new Error(detail || `Could not load models (${response.status})`);
  }

  const models = (await response.json()) || [];
  modelsByProvider[providerName] = models;
  return models;
}

// OpenAI and DeepSeek publish no pricing, so "not published" is a real state
// and must not be shown as free.
function fmtPrice(model) {
  if (!model.pricingKnown) return "price not published";
  if (model.free) return "free";

  const money = (n) => (n >= 1 ? n.toFixed(2) : n.toFixed(3));
  return `$${money(model.inputPer1M)} in / $${money(model.outputPer1M)} out per 1M`;
}

function modelBadge(model) {
  if (!model.pricingKnown) return { text: "?", kind: "unknown" };
  return model.free ? { text: "Free", kind: "free" } : { text: "Paid", kind: "paid" };
}

// ---------- favourite model ----------

// Remembered per project, because each project can be configured with a
// different provider. Stored in the browser rather than alibi.yml so the CLI
// config format stays untouched.
const FAVOURITES_KEY = "alibi.favouriteModel";

function loadFavourites() {
  try {
    return JSON.parse(localStorage.getItem(FAVOURITES_KEY)) || {};
  } catch (err) {
    // Private windows and cleared site data both land here.
    return {};
  }
}

function favouriteFor(projectId) {
  return loadFavourites()[projectId] || null;
}

function writeFavourites(all) {
  try {
    localStorage.setItem(FAVOURITES_KEY, JSON.stringify(all));
  } catch (err) {
    console.warn("Could not save the default model:", err);
  }
}

function setFavourite(projectId, providerName, model) {
  const all = loadFavourites();
  all[projectId] = { provider: providerName, model };
  writeFavourites(all);
}

function clearFavourite(projectId) {
  const all = loadFavourites();
  delete all[projectId];
  writeFavourites(all);
}

function isFavourite(projectId, providerName, modelId) {
  const fav = favouriteFor(projectId);
  return !!fav && fav.provider === providerName && fav.model?.id === modelId;
}

// ---------- API functions ----------
async function fetchProjects() {
  try {
    const response = await fetch(PROJECTS_API);
    if (!response.ok) throw new Error("Failed to fetch projects");
    projects = await response.json();

    currentProjectId = projects[0].id;
    fetchProjectData({ projectId: currentProjectId });

    renderSidebar();
  } catch (error) {
    console.error("Error fetching projects:", error);
    document.getElementById("projectsList").innerHTML =
      '<div class="loading">Error loading projects</div>';
  }
}

async function fetchProjectData({
  projectId = null,
  pageNum = 1,
  cardIndex = 0,
} = {}) {
  try {
    isCommtitView = false;
    document.getElementById("activitiesGrid").innerHTML =
      '<div class="loading">Loading project data...</div>';

    const response = await fetch(
      `${PROJECT_API}?id=${encodeURIComponent(projectId)}`,
    );
    if (!response.ok) throw new Error("Failed to fetch project data");

    const projectData = await response.json();
    currentProjectData = projectData;

    // Filter only providers with non-empty values
    const availableProviders = projectData?.AIProviders?.map((name) => {
      // Normalize both names properly
      const provider = providers.find(
        (p) =>
          p.name.replace(/\s+/g, "").toLowerCase() ===
          name.replace(/\s+/g, "").toLowerCase(),
      );

      return provider ? { ...provider } : null;
    })
      .filter(Boolean);

    currentProjectProviders = availableProviders;

    // A different project can carry different API keys, so the cached
    // catalogues do not survive the switch.
    modelsByProvider = {};
    selectedModel = null;
    selectedProvider = availableProviders.length ? availableProviders[0] : null;

    // Restore the remembered default so a returning user does not have to hunt
    // for the same model again. Only honoured if that provider is still
    // configured for this project.
    const favourite = favouriteFor(projectId);
    if (favourite) {
      const provider = availableProviders.find(
        (p) => p.name === favourite.provider,
      );
      if (provider) {
        selectedProvider = provider;
        selectedModel = favourite.model;
      }
    }

    filteredActivities = [...(projectData.data?.activities || [])];
    currentPage = pageNum;
    render({ cardIndex });
  } catch (error) {
    console.error("Error fetching project data:", error);
    document.getElementById("activitiesGrid").innerHTML =
      '<div class="empty">Error loading project data</div>';
  }
}

async function fetchCommitData(commitHash, projectId, pageNum, cardIndex) {
  try {
    isCommtitView = true;
    document.getElementById("activitiesGrid").innerHTML =
      '<div class="loading">Loading project data...</div>';

    const response = await fetch(
      `${COMMIT_API}?id=${encodeURIComponent(
        commitHash,
      )}&project=${encodeURIComponent(projectId)}`,
    );
    if (!response.ok) throw new Error("Failed to fetch commit data");

    const projectData = await response.json();

    const dataArray = Array.isArray(projectData) ? projectData : [projectData];

    filteredActivities = [...(dataArray || [])];
    currentPage = 1;
    render({ projectId, pageNum, cardIndex });
  } catch (error) {
    console.error("Error fetching commit data:", error);
    const backButton = makeEl(
      "button",
      {
        class: "btn",
        style: "width: fit-content",
      },
      "Go back",
    );

    const container = document.getElementById("activitiesGrid");
    container.innerHTML = "";

    const content = makeEl("div", { class: "empty" });
    const message = makeEl("p", {}, "Error fetching project commit data");

    content.append(message, backButton);
    container.appendChild(content);

    backButton.addEventListener("click", () => {
      fetchProjectData({ projectId, pageNum, cardIndex });
    });
  }
}

async function fetchAISummary(info, commit, files, onSummary) {
  const controller = new AbortController();
  const response = await fetch(AI_SUMMARY_API, {
    signal: controller.signal,
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      projectId: info.name,
      hash: commit.id,
      message: commit.message,
      paths: files,
      source: info.source,
      dir: info.path,
      provider: selectedProvider.name,
      model: selectedModel?.id ?? "",
    }),
  });

  if (!response.ok) {
    console.error("Bad response", response.status);
    summaryError = response.statusText || "Unknown error";
    onSummary("", "error");
    return;
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";

  try {
    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const parts = buffer.split("\n\n");
      buffer = parts.pop();

      for (const sse of parts) {
        const eventMatch = sse.match(/^event:\s*(.+)$/m);
        const dataMatch = sse.match(/^data:\s*(.+)$/m);
        if (!eventMatch || !dataMatch) continue;

        const event = eventMatch[1];
        const payload = JSON.parse(dataMatch[1]);

        if (event === "error") {
          console.log(payload);
          summaryError = payload.err || "Unknown error";

          // A per-file error is not fatal to the run. Aborting here cancelled
          // the server's request context, which killed every other in-flight
          // worker and reported each one as a second failure — so one slow
          // file looked like a total collapse. Report it and keep reading;
          // the stream closes itself with "done".
          onSummary("", event);
          continue;
        }

        // The raw payload goes through too: the "saved" event carries the
        // stored record, including the model that actually ran.
        if (event === "summary") {
          onSummary(payload.html, event, payload);
        } else {
          onSummary("", event, payload);
        }
      }
    }
  } catch (err) {
    if (err.name !== "AbortError") {
      console.error("Stream error:", err);
    }
  }
}

// async function fetchAISummary(info, commit, files, onSummary) {
//   const controller = new AbortController();
//   const response = await fetch(AI_SUMMARY_API, {
//     signal: controller.signal,
//     method: "POST",
//     headers: { "Content-Type": "application/json" },
//     body: JSON.stringify({
//       projectId: info.name,
//       hash: commit.id,
//       message: commit.message,
//       paths: files,
//       source: info.source,
//       dir: info.path,
//       provider: selectedProvider.name,
//     }),
//   });

//   if (!response.ok) {
//     console.error("Bad response", response.status);
//     return;
//   }

//   const reader = response.body.getReader();
//   const decoder = new TextDecoder();

//   let buffer = "";
//   while (true) {
//     const { value, done } = await reader.read();
//     if (done) break;

//     buffer += decoder.decode(value, { stream: true });
//     let parts = buffer.split("\n\n");
//     buffer = parts.pop();

//     for (let sse of parts) {
//       let summaryChunk = "";
//       const eventMatch = sse.match(/^event: (.+)$/m);
//       const dataMatch = sse.match(/^data: (.+)$/m);
//       if (!eventMatch || !dataMatch) continue;

//       const event = eventMatch[1];
//       const payload = JSON.parse(dataMatch[1]);

//       if (event === "summary") {
//         summaryChunk = payload.html;
//       }
//       onSummary(summaryChunk, event);

//       if (event === "error") {
//         errorList.push(payload);
//         onSummary("", "error");
//         return; // ← VERY IMPORTANT
//       }
//     }
//   }
// }

// ---------- render sidebar ----------
function renderSidebar() {
  const projectsList = document.getElementById("projectsList");
  projectsList.innerHTML = "";

  if (projects.length === 0) {
    projectsList.appendChild(
      makeEl("div", { class: "empty" }, "No projects found"),
    );
    return;
  }

  projects.forEach((project) => {
    const projectEl = makeEl("div", {
      class: `project-item ${project.id === currentProjectId ? "active" : ""}`,
      "data-project-id": project.id,
    });

    const icon = makeEl(
      "div",
      { class: "project-icon" },
      makeEl(
        "span",
        { class: "small" },
        project.name.substring(0, 1).toUpperCase(),
      ),
    );

    const name = makeEl("div", { class: "project-name" }, project.name);

    projectEl.appendChild(icon);
    projectEl.appendChild(name);

    projectEl.addEventListener("click", () => {
      currentProjectId = project.id;
      fetchProjectData({ projectId: project.id });
      document.getElementById("fromDate").value = "";
      document.getElementById("toDate").value = "";
      document.getElementById("actionType").value = "all";
      renderSidebar();
    });

    projectsList.appendChild(projectEl);
  });
}

// ---------- render ----------
const grid = document.getElementById("activitiesGrid");
const dateHeader = document.getElementById("dateHeader");
const dateSub = document.getElementById("dateSub");
const pageIndicator = document.getElementById("pageIndicator");

function updateHeader() {
  if (!currentProjectData) {
    dateHeader.textContent = "Activity Viewer";
    dateSub.textContent = "Select a project to view activities";
    return;
  }

  dateHeader.textContent = currentProjectData.name;
  const activityCount = currentProjectData.data?.activities?.length || 0;
  const filteredActivitiesCount = filteredActivities?.length || 0;
  dateSub.textContent = `${filteredActivitiesCount}/${activityCount} activities`;
}

function paginate(arr, page = 1, size = 5) {
  const start = (page - 1) * size;
  return arr.slice(start, start + size);
}

const CHEVRON_SVG =
  '<svg width="12" height="8" viewBox="0 0 12 8" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M1 1L6 6L11 1" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

const STAR_SVG =
  '<svg width="14" height="14" viewBox="0 0 16 16" aria-hidden="true"><path d="M8 1.6l1.9 3.9 4.3.6-3.1 3 .7 4.3L8 11.4l-3.8 2 .7-4.3-3.1-3 4.3-.6L8 1.6z" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linejoin="round"/></svg>';

// Rendering every model at once is slow for providers with hundreds of them
// (OpenRouter currently lists 413), so the list is capped and the search box
// narrows it.
const MODEL_RENDER_LIMIT = 200;

const PANEL_MARGIN = 8;

// The model panel lives on document.body rather than next to the pill that
// opens it. Inside the AI dialog its natural parent is .ai-dialog-content,
// which sets overflow:auto and so clips the panel to the dialog box — most of
// the list simply disappears. z-index cannot fix that, and neither can
// position:fixed, because that same element sets a transform and therefore
// becomes the containing block for fixed descendants. Moving the panel out of
// the subtree is the only reliable escape.
function positionPanel(panel, anchor) {
  const box = anchor.getBoundingClientRect();
  const width = panel.offsetWidth;
  const height = panel.offsetHeight;

  // Right-align to the pill, then clamp inside the viewport.
  let left = box.right - width;
  left = Math.min(
    Math.max(PANEL_MARGIN, left),
    Math.max(PANEL_MARGIN, window.innerWidth - width - PANEL_MARGIN),
  );

  // Below the pill by default; flip above when there is no room below.
  let top = box.bottom + 4;
  if (top + height > window.innerHeight - PANEL_MARGIN) {
    const above = box.top - height - 4;
    top =
      above >= PANEL_MARGIN
        ? above
        : Math.max(PANEL_MARGIN, window.innerHeight - height - PANEL_MARGIN);
  }

  panel.style.left = `${Math.round(left)}px`;
  panel.style.top = `${Math.round(top)}px`;
}


// ModelSelector returns { el, reload }. reload() refetches for whatever
// provider is currently selected and resets the choice back to the default.
function ModelSelector() {
  const wrap = makeEl("div", { class: "provider-selector model-selector" });

  const button = makeEl("div", { class: "provider-btn" });
  const label = makeEl("strong", { class: "provider-name small" }, "Default model");
  button.append(label);

  const chevron = makeEl("button", {
    class: "provider-chevron",
    "aria-expanded": "false",
    title: "Select a model",
  });
  chevron.innerHTML = CHEVRON_SVG;

  const dropdown = makeEl("div", { class: "provider-dropdown model-dropdown hidden" });

  const search = makeEl("input", {
    class: "model-search",
    type: "search",
    placeholder: "Search models",
  });

  const freeOnly = makeEl("input", { type: "checkbox", class: "model-free-check" });
  const freeLabel = makeEl("label", { class: "model-free-toggle small" });
  freeLabel.append(freeOnly, document.createTextNode("Free only"));

  const status = makeEl("div", { class: "model-status small" });
  const list = makeEl("div", { class: "model-list" });

  dropdown.append(
    makeEl("div", { class: "model-tools" }, search, freeLabel),
    status,
    list,
  );
  wrap.append(button, chevron);
  // The panel is not attached here — it is portalled onto <body> only while
  // open (see openPanel/closePanel), so at most one exists at a time no matter
  // how many selectors have been built.

  let all = [];

  function setLabel() {
    label.textContent = selectedModel ? selectedModel.id : "Default model";
    button.title = selectedModel
      ? `${selectedModel.id} — ${fmtPrice(selectedModel)}`
      : "Using this provider's default model";
  }

  function renderList() {
    const query = search.value.trim().toLowerCase();
    const onlyFree = freeOnly.checked;

    const shown = all.filter((m) => {
      if (onlyFree && !m.free) return false;
      if (!query) return true;
      return (
        m.id.toLowerCase().includes(query) ||
        (m.name || "").toLowerCase().includes(query)
      );
    });

    list.innerHTML = "";

    if (!shown.length) {
      list.append(
        makeEl(
          "div",
          { class: "model-empty small" },
          onlyFree
            ? `${selectedProvider?.name ?? "This provider"} publishes no free models.`
            : "No models match that search.",
        ),
      );
      return;
    }

    shown.slice(0, MODEL_RENDER_LIMIT).forEach((m) => {
      const item = makeEl("div", { class: "dropdown-item model-item" });
      const badge = modelBadge(m);

      const main = makeEl(
        "div",
        { class: "model-item-main" },
        makeEl("span", { class: "small model-id" }, m.id),
        makeEl("span", { class: "model-price" }, fmtPrice(m)),
      );

      const starred = isFavourite(currentProjectId, selectedProvider?.name, m.id);
      const star = makeEl("button", {
        class: `model-star${starred ? " on" : ""}`,
        title: starred
          ? "This is the default for this project — click to clear it"
          : "Make this the default model for this project",
        "aria-pressed": String(starred),
      });
      star.innerHTML = STAR_SVG;

      star.addEventListener("click", (e) => {
        // Starring is a separate action from choosing; without this the row
        // click would also fire and close the panel.
        e.stopPropagation();

        if (isFavourite(currentProjectId, selectedProvider?.name, m.id)) {
          clearFavourite(currentProjectId);
        } else {
          setFavourite(currentProjectId, selectedProvider?.name, m);
          selectedModel = m;
          setLabel();
        }
        renderList();
      });

      item.append(
        main,
        makeEl("span", { class: `model-badge ${badge.kind}` }, badge.text),
        star,
      );

      item.addEventListener("click", () => {
        selectedModel = m;
        setLabel();
        closePanel();
      });

      list.appendChild(item);
    });

    if (shown.length > MODEL_RENDER_LIMIT) {
      list.append(
        makeEl(
          "div",
          { class: "model-empty small" },
          `Showing ${MODEL_RENDER_LIMIT} of ${shown.length}. Search to narrow it down.`,
        ),
      );
    }

    // Filtering changes the panel height, which can change whether it still
    // fits below the pill.
    reposition();
  }

  // keepSelection is used when opening the panel for the first time, so a model
  // restored from the saved default survives. Switching provider passes false,
  // because a model from one provider means nothing to another.
  async function reload(keepSelection = false) {
    all = [];
    if (!keepSelection) selectedModel = null;
    setLabel();
    list.innerHTML = "";
    search.value = "";

    if (!selectedProvider) {
      status.textContent = "No AI provider is configured for this project.";
      return;
    }

    status.textContent = `Loading models for ${selectedProvider.name}…`;

    try {
      all = await fetchModels(selectedProvider.name);
      const freeCount = all.filter((m) => m.free).length;

      const favourite = favouriteFor(currentProjectId);
      const defaultNote =
        favourite && favourite.provider === selectedProvider.name
          ? ` · default ${favourite.model?.id}`
          : "";

      status.textContent = all.length
        ? `${all.length} models · ${freeCount} free${defaultNote}`
        : `${selectedProvider.name} returned no models.`;

      renderList();
    } catch (err) {
      status.textContent = err.message;
      reposition();
    }
  }

  search.addEventListener("input", renderList);
  freeOnly.addEventListener("change", renderList);

  function isOpen() {
    return dropdown.isConnected && !dropdown.classList.contains("hidden");
  }

  function openPanel() {
    if (!dropdown.isConnected) document.body.appendChild(dropdown);
    dropdown.classList.remove("hidden");
    chevron.setAttribute("aria-expanded", "true");
    positionPanel(dropdown, wrap);
    // Keep whatever is selected: on first open that is the restored default.
    if (!all.length) reload(true);
  }

  function closePanel() {
    dropdown.classList.add("hidden");
    chevron.setAttribute("aria-expanded", "false");
    dropdown.remove();
  }

  function reposition() {
    if (isOpen()) positionPanel(dropdown, wrap);
  }

  chevron.addEventListener("click", (e) => {
    e.stopPropagation();
    if (isOpen()) closePanel();
    else openPanel();
  });

  // Keep clicks inside the panel from reaching the document handler that
  // closes it — otherwise typing in the search box would dismiss the list.
  dropdown.addEventListener("click", (e) => e.stopPropagation());
  document.addEventListener("click", closePanel);

  // The pill moves when the dialog behind it scrolls or the window resizes,
  // and a portalled panel does not follow on its own.
  window.addEventListener("resize", reposition);
  window.addEventListener("scroll", reposition, true);

  setLabel();
  return { el: wrap, reload };
}

function AIProviderSelector(providers, currentProvider) {
  const container = makeEl("div", { class: "ai-provider-model-selector" });
  const providerSelector = makeEl("div", { class: "provider-selector" });
  const modelSelector = ModelSelector();

  // --- main button ---
  const button = makeEl("div", {
    class: "provider-btn",
    title: "Summarize with AI",
  });
  const iconImg = makeEl("img", {
    src: currentProvider.logo,
    class: "provider-icon",
  });
  const name = makeEl(
    "strong",
    { class: "provider-name small" },
    currentProvider.name,
  );
  const chevron = makeEl("button", {
    class: "provider-chevron",
    "aria-expanded": "false",
    title: "Select AI Provider",
  });
  chevron.innerHTML =
    '<svg width="12" height="8" viewBox="0 0 12 8" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M1 1L6 6L11 1" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  button.append(iconImg, name);
  providerSelector.append(button, chevron);

  // --- dropdown ---
  const dropdown = makeEl("div", { class: "provider-dropdown hidden" });

  providers.forEach((p) => {
    const item = makeEl("div", { class: "dropdown-item" });
    const itemIcon = makeEl("img", { src: p.logo, class: "provider-icon" });
    const itemName = makeEl("span", { class: "small" }, p.name);
    item.append(itemIcon, itemName);

    item.addEventListener("click", () => {
      // update selected provider
      iconImg.src = p.logo;
      name.textContent = p.name;
      selectedProvider = p;
      dropdown.classList.add("hidden");
      // Models are provider-specific, so the previous choice cannot carry over.
      modelSelector.reload();
    });

    dropdown.appendChild(item);
  });

  providerSelector.appendChild(dropdown);

  // Toggle dropdown visibility
  chevron.addEventListener("click", (e) => {
    e.stopPropagation();
    dropdown.classList.toggle("hidden");
  });

  // Close dropdown on outside click
  document.addEventListener("click", () => dropdown.classList.add("hidden"));

  container.append(providerSelector, modelSelector.el);
  return container;
}
// function createErrorDialog(err) {
//   const errorDialog = makeEl("div", { class: "error-dialog" });
//   const errorNotice = makeEl("div", { class: "error-notice" }, "");
//   errorNotice.append(
//     makeEl("p", { class: "small", style: "color:white; margin:0px" }, err)
//   );
//   const errorBtn = makeEl("button", {
//     class: "error-btn",
//   });
//   errorBtn.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-exclamation-triangle" viewBox="0 0 16 16">
//   <path d="M7.938 2.016A.13.13 0 0 1 8.002 2a.13.13 0 0 1 .063.016.15.15 0 0 1 .054.057l6.857 11.667c.036.06.035.124.002.183a.2.2 0 0 1-.054.06.1.1 0 0 1-.066.017H1.146a.1.1 0 0 1-.066-.017.2.2 0 0 1-.054-.06.18.18 0 0 1 .002-.183L7.884 2.073a.15.15 0 0 1 .054-.057m1.044-.45a1.13 1.13 0 0 0-1.96 0L.165 13.233c-.457.778.091 1.767.98 1.767h13.713c.889 0 1.438-.99.98-1.767z"/>
//   <path d="M7.002 12a1 1 0 1 1 2 0 1 1 0 0 1-2 0M7.1 5.995a.905.905 0 1 1 1.8 0l-.35 3.507a.552.552 0 0 1-1.1 0z"/>
// </svg>`;
//   errorDialog.append(errorBtn, errorNotice);

//   errorBtn.addEventListener("click", (e) => {
//     e.stopPropagation();
//     errorNotice.classList.remove("hidden");
//   });

//   document.addEventListener("click", () => errorNotice.classList.add("hidden"));

//   return errorDialog;
// }

function createAIDialog(act) {
  const dialog = makeEl("div", { class: "ai-dialog hidden" });

  const overlay = makeEl("div", { class: "ai-dialog-overlay" });
  const content = makeEl("div", { class: "ai-dialog-content" });
  const dialogHeader = makeEl("div", { class: "ai-dialog-content-header" });
  const left = makeEl("div", { class: "activity-meta" });

  // Example content
  const commitEl = makeEl("div", {
    class: "commit",
    style: "padding:10px 0px 0px 0px",
  });

  const info = makeEl("div", { class: "msg" });
  info.appendChild(
    makeEl("div", null, makeEl("strong", {}, act.commits[0].message)),
  );
  info.appendChild(makeEl("div", { class: "id" }, act.commits[0].id));
  const commits = act.commits[0];

  // A stored summary names the model that actually wrote it, not whichever one
  // happens to be selected now. Kept updatable so a summary generated in this
  // dialog can fill it in without waiting for a reload.
  const notice = makeEl("p", { class: "small" });

  function setNotice(provider, model) {
    const usedProvider = provider || selectedProvider?.name || "AI";
    notice.textContent = model
      ? `Summarized by ${usedProvider} · ${model}`
      : `Summarized by ${usedProvider}`;
  }

  setNotice(commits.summary?.provider, commits.summary?.model);
  const copySummary = makeEl("div", {
    class: "icon-box",
    title: "Copy summary",
  });
  const clipboardIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-clipboard" viewBox="0 0 16 16">
  <path d="M4 1.5H3a2 2 0 0 0-2 2V14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V3.5a2 2 0 0 0-2-2h-1v1h1a1 1 0 0 1 1 1V14a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3.5a1 1 0 0 1 1-1h1v-1z"/>
  <path d="M9.5 1a.5.5 0 0 1 .5.5v1a.5.5 0 0 1-.5.5h-3a.5.5 0 0 1-.5-.5v-1a.5.5 0 0 1 .5-.5h3zm-3-1A1.5 1.5 0 0 0 5 1.5v1A1.5 1.5 0 0 0 6.5 4h3A1.5 1.5 0 0 0 11 2.5v-1A1.5 1.5 0 0 0 9.5 0h-3z"/>
</svg>`;
  const checkIcon = `<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-check-lg" viewBox="0 0 16 16">
  <path d="M12.736 3.97a.733.733 0 0 1 1.047 0c.286.289.29.756.01 1.05L7.88 12.01a.733.733 0 0 1-1.065.02L3.217 8.384a.757.757 0 0 1 0-1.06.733.733 0 0 1 1.047 0l3.052 3.093 5.4-6.425z"/>
</svg>`;
  copySummary.innerHTML = clipboardIcon;

  const closeBtn = makeEl("button", {
    class: "ai-dialog-close",
    title: "Close",
  });
  const { data, ...rest } = currentProjectData;
  const files = act?.changes?.map((c) => c.path);

  const summaryText = makeEl("div", { class: "small ai-summary-text" });
  const placeholder = makeEl(
    "em",
    { style: "text-align:center;display:block" },
    "Your summary will appear here once generated.",
  );
  const listSummaryText = makeEl("ul", { class: "list-summary" }, "");
  summaryText.appendChild(listSummaryText);
  const summaryTextTop = makeEl("div", { class: "ai-dialog-content-header" });

  if (commits.summary.content) {
    listSummaryText.innerHTML = commits.summary.content;
    summaryTextTop.append(notice, copySummary);

    const observer = new MutationObserver(() => {
      if (!dialog.classList.contains("hidden")) {
        observer.disconnect(); // stop watching

        requestAnimationFrame(() => {
          if (summaryText.scrollHeight > summaryText.clientHeight) {
            summaryText.style.paddingBottom = "0px";

            if (!summaryText.querySelector(".bottom-box")) {
              summaryText.appendChild(makeEl("div", { class: "bottom-box" }));
            }
          }
        });
      }
    });

    observer.observe(dialog, { attributes: true });
  } else {
    summaryTextTop.style.marginBottom = "13px";
    summaryText.appendChild(placeholder);
  }

  closeBtn.innerHTML = `
<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-x" viewBox="0 0 16 16">
  <path d="M4.646 4.646a.5.5 0 0 1 .708 0L8 7.293l2.646-2.647a.5.5 0 0 1 .708.708L8.707 8l2.647 2.646a.5.5 0 0 1-.708.708L8 8.707l-2.646 2.647a.5.5 0 0 1-.708-.708L7.293 8 4.646 5.354a.5.5 0 0 1 0-.708"/>
</svg>
`;
  const generateBtn = makeEl("button", {
    class: "generate-btn",
    title: "Generate summary",
  });

  generateBtn.innerHTML = "<span>Summarize</span>";

  closeBtn.addEventListener("click", () => dialog.classList.add("hidden"));

  left.append(
    AIProviderSelector(currentProjectProviders, selectedProvider),
    generateBtn,
  );
  dialogHeader.append(left, closeBtn);
  content.appendChild(dialogHeader);

  commitEl.appendChild(info);

  content.append(commitEl, summaryTextTop, summaryText);
  dialog.append(overlay, content);

  generateBtn.addEventListener("click", async () => {
    if (!files || files?.length < 1) {
      alert("No files captured in this commit");
      return;
    }
    const loadingPlaceholder = makeEl(
      "div",
      { class: "small ai-summary-text", style: "display:block" },
      "Loading...",
    );
    placeholder.style.display = "none";
    summaryTextTop.innerHTML = "";
    listSummaryText.innerHTML = "";
    listSummaryText.appendChild(loadingPlaceholder);

    if (document.querySelector(".error-dialog")) {
      document.querySelector(".error-dialog").remove();
    }
    if (document.querySelector(".bottom-box")) {
      document.querySelector(".bottom-box").style.display = "none";
    }
    try {
      fetchAISummary(rest, commits, files, (partialHTML, event, payload) => {
        if (event !== "done") {
          generateBtn.disabled = true;
        }
        switch (event) {
          case "summary":
            generateBtn.textContent = "Generating...";
            placeholder.style.display = "none";
            loadingPlaceholder.style.display = "none";
            summaryTextTop.innerHTML = "";
            summaryTextTop.style.marginBottom = "13px";
            break;
          case "saving":
            generateBtn.textContent = "Saving...";
            summaryTextTop.innerHTML = "";
            break;
          case "saved":
            generateBtn.textContent = "Saved";
            summaryTextTop.innerHTML = "";
            // Prefer the stored record, which names the resolved model even
            // when the request asked for the default. Fall back to the local
            // selection so the model still appears if the record does not
            // arrive, rather than waiting for a reload to reveal it.
            setNotice(
              payload?.provider || selectedProvider?.name,
              payload?.model || selectedModel?.id || "",
            );
            break;
          case "error":
            generateBtn.disabled = false;
            generateBtn.textContent = "Retry";
            // left.appendChild(createErrorDialog(summaryError));
            listSummaryText.innerHTML = "";
            summaryTextTop.innerHTML = "";
            summaryTextTop.style.marginBottom = "13px";
            listSummaryText.appendChild(
              makeEl(
                "p",
                { class: "small", style: "color:#f05959" },
                `An error occurred while generating the summary: ${summaryError}`,
              ),
            );
            return;
          case "done":
            generateBtn.disabled = false;
            generateBtn.textContent = "Summarize";
            summaryTextTop.style.marginBottom = "0px";
            summaryTextTop.append(notice, copySummary);
            break;
          default:
            generateBtn.disabled = true;
            generateBtn.textContent = "Please wait...";
            summaryTextTop.innerHTML = "";
            summaryTextTop.style.marginBottom = "13px";
            break;
        }

        listSummaryText.insertAdjacentHTML("beforeend", partialHTML);

        if (summaryText.offsetHeight >= 400) {
          summaryText.style.paddingBottom = "0px";

          if (!summaryText.querySelector(".bottom-box")) {
            summaryText.appendChild(makeEl("div", { class: "bottom-box" }, ""));
          }
        }
      });
    } catch (err) {
      console.error("Error generating summary:", err);
    }
  });

  copySummary.addEventListener("click", () => {
    navigator.clipboard
      ?.writeText(summaryText.innerText)
      .then(() => {
        copySummary.innerHTML = checkIcon;
        setTimeout(() => (copySummary.innerHTML = clipboardIcon), 1500);
      })
      .catch((b) => {
        const ta = document.createElement("textarea");
        ta.value = summaryText.innerHTML;
        document.body.appendChild(ta);
        ta.select();
        try {
          document.execCommand("copy");
          copySummary.textContent = "Copied";
        } catch (e) {
          alert("Copy failed");
        }
        ta.remove();
        setTimeout(() => (copy.textContent = "Copy Summary"), 1500);
      });
  });

  // Clicking overlay also closes
  overlay.addEventListener("click", () => dialog.classList.add("hidden"));

  document.body.appendChild(dialog);
  return dialog;
}

function render({ projectId = null, pageNum = 1, cardIndex = 0 } = {}) {
  updateHeader();
  grid.innerHTML = "";

  if (!currentProjectData || !filteredActivities.length) {
    grid.appendChild(
      makeEl(
        "div",
        { class: "empty" },
        currentProjectData
          ? "No activities for this filter range."
          : "Select a project from the sidebar to view activities",
      ),
    );
    return;
  }

  // compute pagination
  pageSize = parseInt(document.getElementById("pageSize").value, 10) || 5;
  const total = filteredActivities.length;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  if (currentPage > totalPages) currentPage = totalPages;
  pageIndicator.textContent = `Page ${currentPage} / ${totalPages}`;

  const toRender = paginate(filteredActivities, currentPage, pageSize);

  toRender.forEach((act, idx) => {
    const aiDialog = createAIDialog(act);
    if (isCommtitView) {
      const backButton = makeEl(
        "button",
        {
          class: "btn",
          style: "width:fit-content",
        },
        "Go back",
      );
      grid.appendChild(backButton);

      backButton.addEventListener("click", () => {
        console.log(projectId, pageNum, idx);
        fetchProjectData({ projectId, pageNum, cardIndex });
      });
    }

    const card = makeEl("article", {
      class: "activity-card",
      "data-activity-index": idx,
    });

    // header
    const head = makeEl("div", { class: "activity-head" });
    const meta = makeEl("div", { class: "activity-meta" });

    meta.appendChild(makeEl("div", { class: "pill" }, act.type.toUpperCase()));
    meta.appendChild(
      makeEl("div", { class: "timestamp" }, fmtDate(act.timestamp)),
    );

    head.appendChild(meta);

    const right = makeEl("div", { class: "activity-meta" });

    right.appendChild(makeEl("div", { class: "small muted" }, `ID: ${act.id}`));

    if (act.type === "commit") {
      const AIBtn = makeEl(
        "button",
        { class: "bordered-ai small muted count-badge" },
        "Summarize with AI",
      );
      right.appendChild(AIBtn);

      AIBtn.addEventListener("click", () => {
        aiDialog.classList.remove("hidden");
      });
    }

    head.appendChild(right);

    card.appendChild(head);

    // commits
    const commitsWrap = makeEl("div", { class: "commits" });
    (act.commits || []).forEach((c) => {
      const commitEl = makeEl("div", { class: "commit" });
      const left = makeEl("div", { class: "msg" });
      left.appendChild(makeEl("div", null, makeEl("strong", {}, c.message)));
      left.appendChild(makeEl("div", { class: "id" }, c.id));

      // Alibi link only for push activities
      if (act.type === "push") {
        const getCommit = makeEl(
          "button",
          { class: "commit-link", title: "Open commit in Alibi" },
          "Alibi",
        );
        left.appendChild(getCommit);
        getCommit.addEventListener("click", () => {
          fetchCommitData(c.id, currentProjectData?.name, currentPage, idx);
        });
      }

      // GitHub link for all commit types
      if (c.githubUrl) {
        const githubLink = makeEl(
          "a",
          {
            class: "commit-link",
            href: c.githubUrl,
            target: "_blank",
          },
          "GitHub",
        );
        left.appendChild(githubLink);
      }

      const copy = makeEl(
        "button",
        { class: "copy-btn", title: "Copy commit id" },
        "Copy ID",
      );
      left.appendChild(copy);

      commitEl.appendChild(left);

      const rightBtns = makeEl("div", {
        style: "display:flex;gap:8px;align-items:center;",
      });
      const time = makeEl(
        "div",
        { class: "small muted" },
        fmtDate(c.timestamp),
      );

      const links = makeEl("div", { class: "commit-links" });

      copy.addEventListener("click", () => {
        navigator.clipboard
          ?.writeText(c.id)
          .then(() => {
            copy.textContent = "Copied";
            setTimeout(() => (copy.textContent = "Copy ID"), 1500);
          })
          .catch(() => {
            const ta = document.createElement("textarea");
            ta.value = c.id;
            document.body.appendChild(ta);
            ta.select();
            try {
              document.execCommand("copy");
              copy.textContent = "Copied";
            } catch (e) {
              alert("Copy failed — commit id: " + c.id);
            }
            ta.remove();
            setTimeout(() => (copy.textContent = "Copy ID"), 1500);
          });
      });

      rightBtns.appendChild(time);
      rightBtns.appendChild(links);
      commitEl.appendChild(rightBtns);

      commitsWrap.appendChild(commitEl);
    });

    card.appendChild(commitsWrap);

    // changes
    const changesWrap = makeEl("div", { class: "changes" });
    (act.changes || []).forEach((ch, ci) => {
      const changeEl = makeEl("div", {
        class: "change",
        "data-change-index": ci,
      });
      const row = makeEl("div", { class: "change-row" });

      const fileInfo = makeEl("div", { class: "file-info" });
      fileInfo.appendChild(makeEl("div", { class: "file-path" }, ch.path));
      fileInfo.appendChild(makeEl("div", { class: "change-type" }, ch.type));
      row.appendChild(fileInfo);

      const toggles = makeEl("div", { class: "toggle" });
      const fnCount = Array.isArray(ch.functions) ? ch.functions.length : 0;
      const fnBadge = makeEl(
        "div",
        { class: "small muted count-badge" },
        fnCount
          ? `${fnCount} function${fnCount > 1 ? "s" : ""}`
          : "no functions",
      );

      const chevron = makeEl("button", {
        class: "icon-box",
        "aria-expanded": "false",
        title: "Toggle functions",
      });
      chevron.innerHTML =
        '<svg width="12" height="8" viewBox="0 0 12 8" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M1 1L6 6L11 1" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/></svg>';

      toggles.appendChild(fnBadge);
      toggles.appendChild(chevron);
      row.appendChild(toggles);

      changeEl.appendChild(row);

      // functions area (collapsible)
      const fns = makeEl("div", {
        class: "functions",
        "aria-hidden": "true",
        style: "display:none",
      });
      if (fnCount) {
        ch.functions.forEach((fn, fi) => {
          const fnEl = makeEl("div", { class: "fn" });
          const left = makeEl(
            "div",
            null,
            makeEl("div", { class: "name" }, fn.name),
          );
          const rightMeta = makeEl("div", {
            style: "display:flex;gap:8px;align-items:center;",
          });
          rightMeta.appendChild(
            makeEl("div", { class: "meta small muted" }, fn.action),
          );
          // lines count badge
          const linesCount = Array.isArray(fn.lineCounts)
            ? fn.lineCounts.length
            : 0;

          if (linesCount === 2) {
            // Always convert both numbers to absolute integers
            const [added, removed] = fn.lineCounts.map((count) =>
              Math.abs(parseInt(count, 10)),
            );

            const lineCountBadge = makeEl(
              "div",
              { class: "count-badge small muted" },
              `+${added} line${added > 1 ? "s" : ""}, -${removed} line${removed > 1 ? "s" : ""
              }`,
            );

            rightMeta.appendChild(lineCountBadge);
          }

          fnEl.append(left, rightMeta);

          fns.appendChild(fnEl);
        });
      } else {
        fns.appendChild(
          makeEl("div", { class: "muted small" }, "No functions"),
        );
      }

      // clicking chevron toggles functions
      chevron.addEventListener("click", () => {
        const expanded = chevron.getAttribute("aria-expanded") === "true";
        chevron.setAttribute("aria-expanded", String(!expanded));
        fns.style.display = expanded ? "none" : "flex";
        fns.setAttribute("aria-hidden", String(expanded));
        chevron.style.transform = expanded ? "rotate(0deg)" : "rotate(180deg)";
      });

      changeEl.appendChild(fns);
      changesWrap.appendChild(changeEl);
    });

    card.appendChild(changesWrap);
    grid.appendChild(card);
  });

  const el = document.querySelector(`[data-activity-index="${cardIndex}"]`);
  const container = document.querySelector(".main-content");

  if (el && container) {
    const offset = 150;
    const target = el.offsetTop - offset;

    container.scrollTo({
      top: target,
      behavior: "smooth",
    });
  }
}

// initial load
fetchProjects();

// ---------- sidebar toggle ----------
document.querySelectorAll(".toggleSidebar").forEach((button) => {
  button.addEventListener("click", () => {
    const sidebar = document.getElementById("sidebar");
    sidebar.classList.toggle("collapsed");

    // Find icon inside the *clicked button*
    const icon = button.querySelector("svg");
    if (sidebar.classList.contains("collapsed")) {
      icon.style.transform = "rotate(180deg)";
    } else {
      icon.style.transform = "rotate(0deg)";
    }
  });
});

// ---------- filter logic ----------
function applyDateFilter(fromVal, toVal) {
  const from = fromVal ? new Date(fromVal + "T00:00:00") : null;
  const to = toVal ? new Date(toVal + "T23:59:59") : null;
  const actionType = document.getElementById("actionType").value;

  if (!currentProjectData) return;

  filteredActivities = (currentProjectData.data?.activities || []).filter(
    (act) => {
      if (actionType !== "all" && act.type !== actionType) {
        return false;
      }
      const t = new Date(act.timestamp);
      if (from && t < from) return false;
      if (to && t > to) return false;
      return true;
    },
  );

  currentPage = 1;
  render();
}

document.querySelectorAll(".dateRange").forEach((input) => {
  input.addEventListener("change", () => {
    const fromVal = document.getElementById("fromDate").value;
    const toVal = document.getElementById("toDate").value;
    applyDateFilter(fromVal, toVal);
  });
});

document.getElementById("clearFilter").addEventListener("click", () => {
  document.getElementById("fromDate").value = "";
  document.getElementById("toDate").value = "";
  document.getElementById("actionType").value = "all";
  if (currentProjectData) {
    filteredActivities = [...(currentProjectData.data?.activities || [])];
  }
  currentPage = 1;
  render();
});

// ---------- expand/collapse all ----------
document.getElementById("expandAll").addEventListener("click", () => {
  document.querySelectorAll(".change .icon-box").forEach((btn) => {
    if (
      btn.getAttribute("aria-expanded") === "false" ||
      !btn.getAttribute("aria-expanded")
    ) {
      btn.click();
    }
  });
  document
    .querySelectorAll(".comments")
    .forEach((c) => (c.style.display = "flex"));
});
document.getElementById("collapseAll").addEventListener("click", () => {
  document.querySelectorAll(".change .icon-box").forEach((btn) => {
    if (btn.getAttribute("aria-expanded") === "true") {
      btn.click();
    }
  });
  document
    .querySelectorAll(".comments")
    .forEach((c) => (c.style.display = "none"));
});

// ---------- pagination ----------
document.getElementById("prevPage").addEventListener("click", () => {
  if (currentPage > 1) {
    currentPage--;
    render();
  }
});
document.getElementById("nextPage").addEventListener("click", () => {
  const totalPages = Math.max(
    1,
    Math.ceil(filteredActivities.length / pageSize),
  );
  if (currentPage < totalPages) {
    currentPage++;
    render();
  }
});
document.getElementById("pageSize").addEventListener("change", (e) => {
  pageSize = parseInt(e.target.value, 10) || 5;
  currentPage = 1;
  render();
});

document.getElementById("actionType").addEventListener("change", (e) => {
  const val = e.target.value;
  if (!currentProjectData) return;

  const fromVal = document.getElementById("fromDate").value;
  const toVal = document.getElementById("toDate").value;

  const from = fromVal ? new Date(fromVal + "T00:00:00") : null;
  const to = toVal ? new Date(toVal + "T23:59:59") : null;

  filteredActivities = (currentProjectData.data?.activities || []).filter(
    (act) => {
      // ✅ Action type filter
      if (val !== "all" && act.type !== val) return false;

      // ✅ Date filter
      const t = new Date(act.timestamp);
      if (from && t < from) return false;
      if (to && t > to) return false;

      return true;
    },
  );

  currentPage = 1;
  render();
});

// ---------- download PDF ----------

document.getElementById("downloadPdf").addEventListener("click", async () => {
  const aiDialog = createDownloadDialog();
  aiDialog.classList.remove("hidden");
});

function createDownloadDialog() {
  const dialog = makeEl("div", { class: "ai-dialog hidden" });
  const overlay = makeEl("div", { class: "ai-dialog-overlay" });
  const content = makeEl("div", { class: "ai-dialog-content" });
  const closeBtn = makeEl("button", {
    class: "ai-dialog-close",
    title: "Close",
  });
  closeBtn.innerHTML = `
<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" class="bi bi-x" viewBox="0 0 16 16">
  <path d="M4.646 4.646a.5.5 0 0 1 .708 0L8 7.293l2.646-2.647a.5.5 0 0 1 .708.708L8.707 8l2.647 2.646a.5.5 0 0 1-.708.708L8 8.707l-2.646 2.647a.5.5 0 0 1-.708-.708L7.293 8 4.646 5.354a.5.5 0 0 1 0-.708"/>
</svg>
`;
  const dialogHeader = makeEl("div", { class: "ai-dialog-content-header" });

  dialogHeader.append(makeEl("div"), closeBtn);

  content.append(dialogHeader, renderDownloadView());

  dialog.append(overlay, content);
  overlay.addEventListener("click", () => dialog.classList.add("hidden"));
  closeBtn.addEventListener("click", () => dialog.classList.add("hidden"));

  document.body.appendChild(dialog);
  return dialog;
}

function renderDownloadView() {
  let useAI = false;

  const fromVal = document.getElementById("fromDate").value;
  const toVal = document.getElementById("toDate").value;
  const selectedType = document.getElementById("actionType").value;
  const projectName = currentProjectId;

  // ---- Action type selector
  const actionType = makeEl("select", { class: "dialog-meta-dropdown" });
  ["all", "commit", "push"].forEach((type) => {
    actionType.appendChild(
      makeEl("option", { value: type }, type[0].toUpperCase() + type.slice(1)),
    );
  });
  actionType.value = selectedType;

  // ---- Date inputs
  const startDateInput = makeEl("input", {
    id: "fromDateDialog",
    class: "dialogDateRange dialog-meta-dropdown",
    type: "date",
    value: fromVal,
  });

  const endDateInput = makeEl("input", {
    id: "toDateDialog",
    class: "dialogDateRange dialog-meta-dropdown",
    type: "date",
    value: toVal,
  });

  // ---- Containers
  const preparation = makeEl("div", { class: "center" });
  const decisionBox = makeEl("div", { class: "decisionBox" });
  const selectProvider = makeEl("div", { class: "selectProvider" });

  // ---- AI checkbox
  const checkBox = makeEl("input", {
    type: "checkbox",
    class: "checkBox",
  });

  const noticeBox = makeEl("div", { class: "noticeBox" });
  noticeBox.append(
    makeEl("b", { class: "text" }, "Include AI summary"),
    makeEl(
      "span",
      { class: "text" },
      "This will generate (if necessary) and include AI summaries. Falls back to raw summaries if unavailable.",
    ),
  );

  decisionBox.append(checkBox, noticeBox);

  const generateBtn = makeEl("button", { class: "decisionBtn" }, "Generate");

  // ---- Layout (SAFE composition)
  const header = makeEl("div", { class: "text dialog-meta" });

  header.append(
    makeEl(
      "div",
      { class: "" },
      "Download ",
      actionType,
      " report for ",
      makeEl("b", {}, projectName),
    ),
  );

  header.append(
    makeEl(
      "div",
      { class: "" },
      " from ",
      startDateInput,
      " to ",
      endDateInput,
    ),
  );

  // Shown only when the server reports it fell back to an HTML report.
  const fallbackNotice = makeEl("p", {
    class: "small fallback-notice",
    style: "display:none",
  });

  preparation.append(
    header,
    decisionBox,
    selectProvider,
    fallbackNotice,
    generateBtn,
  );

  // ---- Date change sync
  [startDateInput, endDateInput].forEach((input) => {
    input.addEventListener("change", () => {
      document.getElementById("fromDate").value = startDateInput.value;
      document.getElementById("toDate").value = endDateInput.value;
      applyDateFilter(startDateInput.value, endDateInput.value);
    });
  });

  // ---- Action type change sync
  actionType.addEventListener("change", (e) => {
    const val = e.target.value;
    document.getElementById("actionType").value = val;
    if (!currentProjectData) return;
    const fromVal = document.getElementById("fromDate").value;
    const toVal = document.getElementById("toDate").value;
    applyDateFilter(fromVal, toVal);
  });
  // ---- AI provider UI
  function renderProviderSelector() {
    selectProvider.innerHTML = "";
    if (!useAI) return;

    selectProvider.append(
      makeEl("span", { class: "text" }, "Select an AI provider:"),
      AIProviderSelector(currentProjectProviders, selectedProvider),
    );
  }

  checkBox.addEventListener("change", () => {
    useAI = checkBox.checked;
    renderProviderSelector();
  });

  // ---- Generate

  generateBtn.addEventListener("click", async () => {
    const response = await fetch(`${GENERATE_PDF}?t=${Date.now()}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      cache: "no-store",
      body: JSON.stringify({
        projectID: currentProjectId,
        activities: filteredActivities,
        useAI,
        provider: selectedProvider?.name ?? null,
        model: selectedModel?.id ?? "",
        startDate: startDateInput.value,
        endDate: endDateInput.value,
        source: currentProjectData.source,
        dir: currentProjectData.path,
      }),
    });

    if (!response.ok) {
      console.error("Bad response", response.status);
      return;
    }
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { value, done } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const chunks = buffer.split("\n\n");
      buffer = chunks.pop();

      for (const chunk of chunks) {
        const event = chunk.match(/^event:\s*(.+)$/m)?.[1];
        const data = chunk.match(/^data:\s*(.+)$/m)?.[1];

        if (!event || !data) continue;
        console.log("PDF event:", event, data);

        // The server sends this when it could not find a browser to print with
        // and saved an HTML report instead. It is information, not progress.
        if (event === "notice") {
          fallbackNotice.textContent = JSON.parse(data);
          fallbackNotice.style.display = "block";
          continue;
        }

        if (event !== "pdf-generated") {
          generateBtn.disabled = true;
          generateBtn.textContent = `Please wait... (${data})`;
        }

        if (event === "summary") {
          console.log("PDF generation summary:", JSON.parse(data));
          // ⛔ DO NOT try to read PDF here
          // return;
        }
        if (event === "saved") {
          console.log("PDF generation saved:", JSON.parse(data));
          // ⛔ DO NOT try to read PDF here
          // window.location.href = `/api/download-pdf/${JSON.parse(data)}`;
          // return;
        }
        if (event === "pdf-generated") {
          generateBtn.disabled = false;
          generateBtn.textContent = "Generate PDF";

          const docId = JSON.parse(data);
          console.log("PDF generation completed:", docId);

          const url =
            `${DOWNLOAD_PDF}?docId=${encodeURIComponent(docId)}` +
            `&projectId=${encodeURIComponent(currentProjectId)}`;

          const link = document.createElement("a");
          link.href = url;
          // Use the real name: the report is .html when no browser was
          // available, and forcing "report.pdf" would mislabel it.
          link.download = docId;
          link.click();
          return;
        }
      }
    }
  });

  return preparation;
}

// // keyboard shortcuts
// window.addEventListener("keydown", (e) => {
//   if (e.key === "E" || e.key === "e") {
//     document.getElementById("expandAll").click();
//   }
//   if (e.key === "C" || e.key === "c") {
//     document.getElementById("collapseAll").click();
//   }
// });
