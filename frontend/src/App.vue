<script setup>
import { computed, nextTick, onMounted, ref, watch } from "vue";
import * as AppBindings from "../wailsjs/go/main/App";
import FolderTree from "./FolderTree.vue";

const backend = () => AppBindings;
const projects = ref([]);
const sidebarWidth = ref(268);
const sidebarResizing = ref(false);
const projectId = ref("");
const collectionId = ref("");
const folderId = ref("");
const requestId = ref("");
const requestPath = ref("");
const response = ref(null);
const busy = ref(false);
const activeEditorTab = ref("params");
const queryParams = ref([]);
const notice = ref("");
const errorMessage = ref("");
const isDark = ref(getPreference("theme") !== "light");
const projectDialog = ref(false);
const editingProject = ref(null);
const projectForm = ref(emptyProject());
const authTarget = ref("primary");
const authForm = ref(emptyProjectAuth());
const authSaving = ref(false);
const authLoadedProjectId = ref("");
const authLoadingProjectId = ref("");
const authCredentialsMissing = ref(false);
const ignoredJSONPathsText = ref("");
const destinationVariableDrafts = ref(emptyDestinationVariableDrafts());
const primaryProjectInput = ref(null);
const candidateProjectInput = ref(null);
const secondaryProjectInput = ref(null);
const projectFileInput = ref(null);
const postmanFileInput = ref(null);
const expandedCollections = ref(new Set());
const activeMenu = ref("");
const collectionSearchOpen = ref(false);
const collectionSearch = ref("");
const collectionSearchInput = ref(null);

function getPreference(name) {
  const key = `difftwin-${name}`;
  const current = localStorage.getItem(key);
  if (current !== null) return current;
  const legacy = localStorage.getItem(`diffbench-${name}`);
  if (legacy !== null) {
    localStorage.setItem(key, legacy);
    return legacy;
  }
  return null;
}

function setPreference(name, value) {
  localStorage.setItem(`difftwin-${name}`, value);
}

function emptyProject() {
  return { id: "", name: "", primary: "", candidate: "", secondary: "", auth: {}, ignored_json_paths: [], destination_variables: {}, variables: "{}", collections: [] };
}

function emptyDestinationVariableDrafts(destinationVariables = {}) {
  return {
    primary: JSON.stringify(destinationVariables.primary || {}, null, 2),
    candidate: JSON.stringify(destinationVariables.candidate || {}, null, 2),
    secondary: JSON.stringify(destinationVariables.secondary || {}, null, 2)
  };
}

function emptyProjectAuth() {
  return {
    auth: {
      primary: { type: "none" },
      candidate: { type: "none" },
      secondary: { type: "none" }
    },
    credentials: {
      primary: {},
      candidate: {},
      secondary: {}
    }
  };
}

const minSidebarWidth = 210;
const maxSidebarWidth = () => Math.min(480, Math.floor(window.innerWidth * 0.45));
let sidebarDragStartX = 0;
let sidebarDragStartWidth = sidebarWidth.value;

function clampSidebarWidth(width) {
  return Math.min(maxSidebarWidth(), Math.max(minSidebarWidth, width));
}

function startSidebarResize(event) {
  if (event.button !== 0) return;
  event.preventDefault();
  sidebarDragStartX = event.clientX;
  sidebarDragStartWidth = sidebarWidth.value;
  sidebarResizing.value = true;
  event.currentTarget.setPointerCapture(event.pointerId);
}

function moveSidebarResize(event) {
  if (!sidebarResizing.value) return;
  sidebarWidth.value = clampSidebarWidth(sidebarDragStartWidth + event.clientX - sidebarDragStartX);
}

function finishSidebarResize(event) {
  if (!sidebarResizing.value) return;
  sidebarResizing.value = false;
  setPreference("sidebar-width", String(sidebarWidth.value));
  if (event.currentTarget.hasPointerCapture(event.pointerId)) {
    event.currentTarget.releasePointerCapture(event.pointerId);
  }
}

function resizeSidebarWithKeyboard(event) {
  if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
  event.preventDefault();
  const direction = event.key === "ArrowRight" ? 1 : -1;
  sidebarWidth.value = clampSidebarWidth(sidebarWidth.value + direction * (event.shiftKey ? 32 : 12));
  setPreference("sidebar-width", String(sidebarWidth.value));
}

const project = computed(() => projects.value.find((item) => item.id === projectId.value) || null);
const collection = computed(() => project.value?.collections?.find((item) => item.id === collectionId.value) || null);
const selectedFolder = computed(() => findFolder(collection.value?.folders || [], folderId.value));
const selectedRequest = computed(() => findRequest(collection.value, requestId.value));
const visibleCollections = computed(() => (project.value?.collections || []).filter((item) => collectionMatches(item, collectionSearch.value)));
const projectLabel = computed(() => project.value?.name || "Selecciona un proyecto");
const latencySummaries = computed(() => {
  if (!response.value) return [];
  const primaryDuration = response.value.primary.duration_ms;
  const destinations = [["Candidata", response.value.candidate]];
  if (response.value.secondary_enabled) destinations.push(["Estable secundaria", response.value.secondary]);
  return destinations.map(([name, result]) => ({
    name,
    duration: result.duration_ms,
    delta: result.duration_ms - primaryDuration
  }));
});
watch(() => project.value?.secondary, (secondary) => {
  if (!secondary && authTarget.value === "secondary") {
    authTarget.value = "primary";
    authForm.value.auth.secondary = { type: "none" };
    authForm.value.credentials.secondary = {};
  }
});
const requestCount = computed(() => (project.value?.collections || []).reduce((sum, item) =>
  sum + countRequests(item.requests || [], item.folders || []), 0));
const queryParamCount = computed(() => queryParams.value.filter((param) => param.key !== "" || param.value !== "").length);
const comparisonSummary = computed(() => {
  if (!response.value) return null;
  const results = [response.value.primary, response.value.candidate];
  if (response.value.secondary_enabled) results.push(response.value.secondary);
  const failed = results
    .some((item) => item.error);
  if (failed) return { tone: "error", icon: "!", title: "Error al consultar", detail: "Una o más respuestas no pudieron completarse." };
  if (response.value.candidate_differs) {
    return { tone: "different", icon: "≠", title: "Hay diferencias", detail: "La candidata no coincide con la estable principal." };
  }
  return { tone: "same", icon: "✓", title: "Las respuestas coinciden", detail: "La candidata coincide con la estable principal." };
});
const responseCards = computed(() => {
  if (!response.value) return [];
  const cards = [
    ["Estable principal", response.value.primary],
    ["Candidata", response.value.candidate]
  ];
  if (response.value.secondary_enabled) cards.push(["Estable secundaria", response.value.secondary]);
  return cards;
});
function getResponseDiffs(primary, compared) {
  const diffs = [];
  const addDiff = (path, before, after, kind = "changed") => {
    if (diffs.length < 101) diffs.push({ path, before, after, kind });
  };

  if (primary.status_code !== compared.status_code) {
    addDiff("HTTP status", primary.status_code || "(sin respuesta)", compared.status_code || "(sin respuesta)");
  }
  if (primary.error !== compared.error) {
    addDiff("Error de transporte", primary.error || "(sin error)", compared.error || "(sin error)");
  }

  const ignoredHeaders = new Set([
    "connection", "content-length", "date", "keep-alive", "proxy-authenticate", "proxy-authorization",
    "te", "trailer", "transfer-encoding", "upgrade", "cf-ray"
  ]);
  const normalizeHeaders = (headers) => Object.fromEntries(
    Object.entries(headers || {})
      .map(([key, value]) => [key.toLowerCase(), String(value).trim().replace(/\s+/g, " ")])
      .filter(([key]) => !ignoredHeaders.has(key))
  );
  const leftHeaders = normalizeHeaders(primary.headers);
  const rightHeaders = normalizeHeaders(compared.headers);
  for (const key of [...new Set([...Object.keys(leftHeaders), ...Object.keys(rightHeaders)])].sort()) {
    if (leftHeaders[key] !== rightHeaders[key]) {
      addDiff(`Header: ${key}`, leftHeaders[key] ?? "(no existe)", rightHeaders[key] ?? "(no existe)",
        leftHeaders[key] === undefined ? "added" : rightHeaders[key] === undefined ? "removed" : "changed");
    }
  }

  if (primary.body !== compared.body) {
    const parseBody = (body) => {
      try { return { valid: true, value: JSON.parse(body) }; }
      catch { return { valid: false }; }
    };
    const before = parseBody(primary.body);
    const after = parseBody(compared.body);
    if (before.valid && after.valid) {
      const formatPath = (path, key) => /^[A-Za-z_$][\w$]*$/.test(key)
        ? `${path}.${key}`
        : `${path}[${JSON.stringify(key)}]`;
      const ignoredPaths = (project.value?.ignored_json_paths || [])
        .map(parseJSONPath).filter((path) => path !== null);
      const isIgnored = (segments) => ignoredPaths.some((pattern) =>
        pattern.length === segments.length && pattern.every((token, index) =>
          token.wildcard || (token.index !== undefined
            ? token.index === segments[index].index
            : token.key === segments[index].key)
        )
      );
      const removed = Symbol("ignored-json-path");
      const prune = (value, segments = []) => {
        if (isIgnored(segments)) return removed;
        if (Array.isArray(value)) {
        return value.map((item, index) => {
          const result = prune(item, [...segments, { index }]);
          return result === removed ? null : result;
        });
        }
        if (value !== null && typeof value === "object") {
        const result = {};
        for (const [key, item] of Object.entries(value)) {
          const child = prune(item, [...segments, { key }]);
          if (child !== removed) result[key] = child;
        }
        return result;
        }
        return value;
      };
      const leftBody = prune(before.value);
      const rightBody = prune(after.value);
      const formatValue = (value) => JSON.stringify(value);
      const compare = (left, right, path = "$", depth = 0, segments = []) => {
        if (diffs.length >= 101) return;
        if (isIgnored(segments)) return;
        if (Object.is(left, right)) return;
        if (depth >= 100) {
          addDiff(path, formatValue(left), formatValue(right));
          return;
        }
        if (Array.isArray(left) && Array.isArray(right)) {
          for (let index = 0; index < Math.max(left.length, right.length) && diffs.length < 101; index += 1) {
            if (index >= left.length) addDiff(`${path}[${index}]`, "(no existe)", formatValue(right[index]), "added");
            else if (index >= right.length) addDiff(`${path}[${index}]`, formatValue(left[index]), "(no existe)", "removed");
            else compare(left[index], right[index], `${path}[${index}]`, depth + 1, [...segments, { index }]);
          }
          return;
        }
        const leftObject = left !== null && typeof left === "object";
        const rightObject = right !== null && typeof right === "object";
        if (leftObject && rightObject && !Array.isArray(left) && !Array.isArray(right)) {
          const keys = new Set([...Object.keys(left), ...Object.keys(right)]);
          for (const key of [...keys].sort()) {
            const childPath = formatPath(path, key);
            if (!Object.hasOwn(left, key)) addDiff(childPath, "(no existe)", formatValue(right[key]), "added");
            else if (!Object.hasOwn(right, key)) addDiff(childPath, formatValue(left[key]), "(no existe)", "removed");
            else compare(left[key], right[key], childPath, depth + 1, [...segments, { key }]);
            if (diffs.length >= 101) break;
          }
          return;
        }
        addDiff(path, formatValue(left), formatValue(right));
      };
      if (leftBody !== removed || rightBody !== removed) compare(leftBody, rightBody);
    } else {
      addDiff("Cuerpo (texto)", primary.body || "(vacío)", compared.body || "(vacío)");
    }
  }

  return diffs;
}

function parseJSONPath(path) {
  if (!path.startsWith("$")) return null;
  const segments = [];
  for (let index = 1; index < path.length;) {
    if (path[index] === ".") {
      index += 1;
      if (index >= path.length) return null;
      if (path[index] === "*") {
        segments.push({ wildcard: true });
        index += 1;
        continue;
      }
      const start = index;
      while (index < path.length && /[A-Za-z0-9_$]/.test(path[index])) index += 1;
      if (start === index) return null;
      segments.push({ key: path.slice(start, index) });
      continue;
    }
    if (path[index] === "[") {
      index += 1;
      if (index >= path.length) return null;
      if (path[index] === "*") {
        if (path[index + 1] !== "]") return null;
        segments.push({ wildcard: true });
        index += 2;
        continue;
      }
      if (path[index] === '"') {
        const start = index;
        index += 1;
        let escaped = false;
        while (index < path.length) {
          if (escaped) escaped = false;
          else if (path[index] === "\\") escaped = true;
          else if (path[index] === '"') {
            index += 1;
            break;
          }
          index += 1;
        }
        if (path[index - 1] !== '"' || path[index] !== "]") return null;
        try {
          segments.push({ key: JSON.parse(path.slice(start, index)) });
        } catch {
          return null;
        }
        index += 1;
        continue;
      }
      const end = path.indexOf("]", index);
      if (end < index || !/^\d+$/.test(path.slice(index, end))) return null;
      segments.push({ index: Number(path.slice(index, end)) });
      index = end + 1;
      continue;
    }
    return null;
  }
  return segments;
}
const comparisonPanels = computed(() => {
  if (!response.value) return [];
  const panels = [];
  if (response.value.candidate_differs) {
    panels.push({
      id: "candidate",
      title: "Candidata frente a estable principal",
      leftLabel: "Estable principal",
      rightLabel: "Candidata",
      diffs: getResponseDiffs(response.value.primary, response.value.candidate)
    });
  }
  if (response.value.secondary_enabled && response.value.noise_detected) {
    panels.push({
      id: "stable",
      title: "Variación entre instancias estables",
      leftLabel: "Estable principal",
      rightLabel: "Estable secundaria",
      diffs: getResponseDiffs(response.value.primary, response.value.secondary)
    });
  }
  return panels;
});

function decodeQueryPart(value) {
  try {
    return decodeURIComponent(value.replaceAll("+", " "));
  } catch {
    return value.replaceAll("+", " ");
  }
}

function parseQueryParams(path) {
  const hashIndex = path.indexOf("#");
  const queryIndex = path.indexOf("?");
  if (queryIndex < 0 || (hashIndex >= 0 && queryIndex > hashIndex)) return [];
  const queryEnd = hashIndex >= 0 ? hashIndex : path.length;
  const rawQuery = path.slice(queryIndex + 1, queryEnd);
  if (!rawQuery) return [];
  return rawQuery.split("&").filter(Boolean).map((part) => {
    const equalsIndex = part.indexOf("=");
    return {
      key: decodeQueryPart(equalsIndex < 0 ? part : part.slice(0, equalsIndex)),
      value: decodeQueryPart(equalsIndex < 0 ? "" : part.slice(equalsIndex + 1)),
      hasEquals: equalsIndex >= 0
    };
  });
}

function encodeQueryPart(value) {
  return String(value).split(/({{.*?}})/g).map((part) =>
    part.startsWith("{{") && part.endsWith("}}")
      ? part
      : encodeURIComponent(part).replaceAll("%20", "+")
  ).join("");
}

function syncQueryParams() {
  queryParams.value = parseQueryParams(selectedRequest.value?.path || "");
}

function updateQueryPath() {
  if (!selectedRequest.value) return;
  const path = selectedRequest.value.path || "";
  const hashIndex = path.indexOf("#");
  const queryIndex = path.indexOf("?");
  const hasQuery = queryIndex >= 0 && (hashIndex < 0 || queryIndex < hashIndex);
  const prefix = hasQuery ? path.slice(0, queryIndex) : hashIndex < 0 ? path : path.slice(0, hashIndex);
  const fragment = hashIndex >= 0 ? path.slice(hashIndex) : "";
  const values = queryParams.value
    .filter((param) => param.key !== "" || param.value !== "")
    .map((param) => `${encodeQueryPart(param.key)}${param.hasEquals || param.value !== "" ? `=${encodeQueryPart(param.value)}` : ""}`);
  selectedRequest.value.path = `${prefix}${values.length ? `?${values.join("&")}` : hasQuery && !queryParams.value.length ? "?" : ""}${fragment}`;
}

function addQueryParam() {
  queryParams.value.push({ key: "", value: "", hasEquals: true });
}

function removeQueryParam(index) {
  queryParams.value.splice(index, 1);
  updateQueryPath();
}

watch(
  [() => selectedRequest.value?.id, () => selectedRequest.value?.path],
  syncQueryParams
);

function findFolder(folders, id) {
  for (const folder of folders) {
    if (folder.id === id) return folder;
    const nested = findFolder(folder.folders || [], id);
    if (nested) return nested;
  }
  return null;
}

function findRequest(item, id) {
  if (!item || !id) return null;
  const top = (item.requests || []).find((entry) => entry.id === id);
  if (top) return top;
  const search = (folders) => {
    for (const folder of folders || []) {
      const request = (folder.requests || []).find((entry) => entry.id === id);
      if (request) return request;
      const nested = search(folder.folders || []);
      if (nested) return nested;
    }
    return null;
  };
  return search(item.folders || []);
}

function countRequests(requests, folders) {
  return requests.length + folders.reduce((sum, folder) =>
    sum + countRequests(folder.requests || [], folder.folders || []), 0);
}

function matchesSearch(value, query) {
  return String(value || "").toLocaleLowerCase().includes(query);
}

function requestMatches(request, query) {
  return !query || matchesSearch(`${request.name} ${request.method} ${request.path}`, query);
}

function folderMatches(folder, query) {
  return !query || matchesSearch(folder.name, query) ||
    (folder.requests || []).some((request) => requestMatches(request, query)) ||
    (folder.folders || []).some((child) => folderMatches(child, query));
}

function collectionMatches(item, rawQuery = collectionSearch.value) {
  const query = rawQuery.trim().toLocaleLowerCase();
  return !query || matchesSearch(item.name, query) ||
    (item.requests || []).some((request) => requestMatches(request, query)) ||
    (item.folders || []).some((folder) => folderMatches(folder, query));
}

function collectionItemSearch(item) {
  const query = collectionSearch.value.trim();
  return matchesSearch(item.name, query) ? "" : query;
}

function toggleCollectionSearch() {
  closeMenus();
  collectionSearchOpen.value = !collectionSearchOpen.value;
  collectionSearch.value = "";
  if (collectionSearchOpen.value) {
    requestAnimationFrame(() => collectionSearchInput.value?.focus());
  }
}

function menuKey(type, id = "") {
  return id ? `${type}:${id}` : type;
}

function closeMenus() {
  activeMenu.value = "";
}

function toggleMenu(type, id = "") {
  const key = menuKey(type, id);
  activeMenu.value = activeMenu.value === key ? "" : key;
}

function flash(message, bad = false) {
  notice.value = bad ? "" : message;
  errorMessage.value = bad ? message : "";
  window.setTimeout(() => {
    notice.value = "";
    errorMessage.value = "";
  }, 4500);
}

async function refreshProjects() {
  projects.value = await backend().GetProjects();
  if (projectId.value && !projects.value.some((item) => item.id === projectId.value)) {
    projectId.value = "";
    collectionId.value = "";
    folderId.value = "";
    requestId.value = "";
  }
}

function setTheme(dark) {
  isDark.value = dark;
  document.documentElement.dataset.theme = dark ? "dark" : "light";
  setPreference("theme", dark ? "dark" : "light");
}

function toggleTheme() {
  setTheme(!isDark.value);
}

function openProject(id) {
  projectId.value = id;
  collectionId.value = "";
  folderId.value = "";
  requestId.value = "";
  response.value = null;
  ignoredJSONPathsText.value = (projects.value.find((item) => item.id === id)?.ignored_json_paths || []).join("\n");
  destinationVariableDrafts.value = emptyDestinationVariableDrafts(projects.value.find((item) => item.id === id)?.destination_variables);
  authTarget.value = "primary";
  closeMenus();
  loadProjectAuth(id);
}

async function loadProjectAuth(id) {
  authLoadedProjectId.value = "";
  authLoadingProjectId.value = id;
  authCredentialsMissing.value = false;
  authForm.value = emptyProjectAuth();
  destinationVariableDrafts.value = emptyDestinationVariableDrafts(project.value?.destination_variables);
  try {
    const settings = await backend().GetProjectAuth(id);
    if (projectId.value !== id) return;
    authForm.value = emptyProjectAuth();
    Object.assign(authForm.value.auth, settings.auth || {});
    Object.assign(authForm.value.credentials, settings.credentials || {});
    authCredentialsMissing.value = settings.credentials_missing || false;
    authLoadedProjectId.value = id;
  } catch (error) {
    if (projectId.value === id) {
      authForm.value.auth = { ...authForm.value.auth, ...(project.value?.auth || {}) };
      flash(error.message || String(error), true);
    }
  } finally {
    if (authLoadingProjectId.value === id) authLoadingProjectId.value = "";
  }
}

function goHome() {
  projectId.value = "";
  collectionId.value = "";
  folderId.value = "";
  requestId.value = "";
  response.value = null;
  closeMenus();
}

function selectCollection(item) {
  collectionId.value = item.id;
  folderId.value = "";
  requestId.value = "";
  response.value = null;
  expandedCollections.value.add(item.id);
  closeMenus();
}

function toggleCollection(item) {
  if (expandedCollections.value.has(item.id)) {
    expandedCollections.value.delete(item.id);
    return;
  }
  selectCollection(item);
}

function selectFolder(id) {
  folderId.value = id;
  requestId.value = "";
  response.value = null;
}

function selectRequest(id, parentFolderId = "") {
  requestId.value = id;
  folderId.value = parentFolderId;
  response.value = null;
  closeMenus();
  activeEditorTab.value = "params";
  syncQueryParams();
}

function newProject() {
  editingProject.value = false;
  projectForm.value = emptyProject();
  ignoredJSONPathsText.value = "";
  destinationVariableDrafts.value = emptyDestinationVariableDrafts();
  projectDialog.value = true;
}

async function editProject(target = "") {
  editingProject.value = true;
  projectForm.value = JSON.parse(JSON.stringify(project.value));
  projectForm.value.variables = JSON.stringify(project.value.variables || {}, null, 2);
  ignoredJSONPathsText.value = (project.value.ignored_json_paths || []).join("\n");
  destinationVariableDrafts.value = emptyDestinationVariableDrafts(project.value.destination_variables);
  projectDialog.value = true;
  const targetInput = {
    primary: primaryProjectInput,
    candidate: candidateProjectInput,
    secondary: secondaryProjectInput
  }[target];
  if (targetInput) nextTick(() => targetInput.value?.focus());
}

async function saveProject() {
  try {
    const saved = await backend().SaveProject({
      ...projectForm.value,
      ignored_json_paths: ignoredJSONPathsText.value.split(/\r?\n/).map((path) => path.trim()).filter(Boolean),
      variables: JSON.parse(projectForm.value.variables || "{}")
    });
    projectId.value = saved.id;
    await refreshProjects();
    await loadProjectAuth(saved.id);
    projectDialog.value = false;
    flash(editingProject.value ? "Proyecto actualizado." : "Proyecto creado.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function saveProjectAuth() {
  if (!project.value || authSaving.value || authLoadedProjectId.value !== project.value.id) return;
  authSaving.value = true;
  try {
    const destinationVariables = {};
    for (const target of ["primary", "candidate", "secondary"]) {
      const parsed = JSON.parse(destinationVariableDrafts.value[target] || "{}");
      if (parsed === null || Array.isArray(parsed) || typeof parsed !== "object" ||
          Object.values(parsed).some((value) => typeof value !== "string")) {
        throw new Error(`Las variables de ${destinationLabel(target)} deben ser un objeto JSON con valores de texto.`);
      }
      destinationVariables[target] = parsed;
    }
    const saved = await backend().SaveProjectWithAuth({
      ...JSON.parse(JSON.stringify(project.value)),
      auth: authForm.value.auth,
      ignored_json_paths: ignoredJSONPathsText.value.split(/\r?\n/).map((path) => path.trim()).filter(Boolean),
      destination_variables: destinationVariables
    }, authForm.value.credentials);
    await refreshProjects();
    if (projectId.value !== saved.id) return;
    authLoadedProjectId.value = saved.id;
    authCredentialsMissing.value = false;
    response.value = null;
    flash("Opciones guardadas.");
  } catch (error) {
    flash(error.message || String(error), true);
  } finally {
    authSaving.value = false;
  }
}

function setAuthTarget(target) {
  authTarget.value = target;
}

function destinationLabel(target) {
  return {
    primary: "la estable principal",
    candidate: "la candidata",
    secondary: "la estable secundaria"
  }[target];
}

function setAuthType(type) {
  const current = authForm.value.auth[authTarget.value];
  authForm.value.auth[authTarget.value] = {
    ...current,
    ...(type === "api_key" && !current.api_key_location ? { api_key_location: "header" } : {}),
    type
  };
  authForm.value.credentials[authTarget.value] = {};
}

async function deleteProject() {
  if (!project.value || !window.confirm(`¿Eliminar el proyecto «${project.value.name}»?`)) return;
  try {
    await backend().DeleteProject(project.value.id);
    projectDialog.value = false;
    goHome();
    await refreshProjects();
    flash("Proyecto eliminado.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function persistCollection() {
  if (!collection.value) return;
  const saved = await backend().SaveCollection(project.value.id, collection.value);
  collectionId.value = saved.id;
  await refreshProjects();
}

async function createCollection() {
  const name = window.prompt("Nombre de la colección");
  if (!name?.trim()) return;
  try {
    const saved = await backend().SaveCollection(project.value.id, {
      id: "", name: name.trim(), requests: [], folders: []
    });
    await refreshProjects();
    selectCollection(saved);
    flash("Colección creada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function renameCollection() {
  const name = window.prompt("Nombre de la colección", collection.value.name);
  if (!name?.trim()) return;
  collection.value.name = name.trim();
  try {
    await persistCollection();
    flash("Colección renombrada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteCollection() {
  if (!window.confirm(`¿Eliminar «${collection.value.name}» y todo su contenido?`)) return;
  try {
    await backend().DeleteCollection(project.value.id, collection.value.id);
    collectionId.value = "";
    folderId.value = "";
    requestId.value = "";
    await refreshProjects();
    flash("Colección eliminada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function createFolder(parentId = "") {
  if (!collection.value) {
    flash("Selecciona una colección antes de crear una carpeta.", true);
    return;
  }
  const name = window.prompt(parentId ? "Nombre de la subcarpeta" : "Nombre de la carpeta");
  if (!name?.trim()) return;
  const folder = { id: crypto.randomUUID().replaceAll("-", ""), name: name.trim(), requests: [], folders: [] };
  collection.value.folders ||= [];
  let siblings = collection.value.folders;
  if (parentId) {
    const parent = findFolder(collection.value.folders, parentId);
    if (!parent) {
      flash("No se encontró la carpeta donde crear la subcarpeta.", true);
      return;
    }
    parent.folders ||= [];
    siblings = parent.folders;
  }
  siblings.push(folder);
  try {
    await persistCollection();
    expandedCollections.value.add(collectionId.value);
    folderId.value = folder.id;
    requestId.value = "";
    flash("Carpeta creada.");
  } catch (error) {
    const folderIndex = siblings.indexOf(folder);
    if (folderIndex >= 0) siblings.splice(folderIndex, 1);
    flash(error.message || String(error), true);
  }
}

function updateFolder(folders, id, action) {
  const index = folders.findIndex((folder) => folder.id === id);
  if (index >= 0) {
    action(folders, index);
    return true;
  }
  return folders.some((folder) => updateFolder(folder.folders || [], id, action));
}

function renameFolder(id) {
  const target = findFolder(collection.value.folders || [], id);
  const name = window.prompt("Nombre de la carpeta", target?.name || "");
  if (!name?.trim()) return;
  target.name = name.trim();
  persistCollection().then(() => flash("Carpeta renombrada."))
    .catch((error) => flash(error.message || String(error), true));
}

function deleteFolder(id) {
  const target = findFolder(collection.value.folders || [], id);
  if (!window.confirm(`¿Eliminar «${target?.name}» y todo su contenido?`)) return;
  updateFolder(collection.value.folders, id, (siblings, index) => siblings.splice(index, 1));
  if (folderId.value === id || !findFolder(collection.value.folders, folderId.value)) {
    folderId.value = "";
    requestId.value = "";
  }
  persistCollection().then(() => flash("Carpeta eliminada."))
    .catch((error) => flash(error.message || String(error), true));
}

async function createRequest(parentId = "") {
  if (!collection.value) {
    flash("Selecciona una colección antes de crear una petición.", true);
    return;
  }
  const request = {
    id: crypto.randomUUID().replaceAll("-", ""),
    name: "Nueva petición", method: "GET", path: "/", headers: {}, body: ""
  };
  collection.value.requests ||= [];
  let siblings = collection.value.requests;
  if (parentId) {
    const parent = findFolder(collection.value.folders || [], parentId);
    if (!parent) {
      flash("No se encontró la carpeta donde crear la petición.", true);
      return;
    }
    parent.requests ||= [];
    siblings = parent.requests;
  }
  siblings.push(request);
  folderId.value = parentId;
  requestId.value = request.id;
  response.value = null;
  closeMenus();
  try {
    await persistCollection();
    flash("Petición creada.");
  } catch (error) {
    const requestIndex = siblings.indexOf(request);
    if (requestIndex >= 0) siblings.splice(requestIndex, 1);
    requestId.value = "";
    flash(error.message || String(error), true);
  }
}

async function duplicateRequest(request, parentId = "") {
  const duplicate = {
    ...JSON.parse(JSON.stringify(request)),
    id: crypto.randomUUID().replaceAll("-", ""),
    name: `${request.name} (copia)`
  };
  if (parentId) {
    const parent = findFolder(collection.value?.folders || [], parentId);
    if (!parent) {
      flash("No se encontró la carpeta donde duplicar la petición.", true);
      return;
    }
    parent.requests.push(duplicate);
  } else {
    collection.value.requests.push(duplicate);
  }
  folderId.value = parentId;
  requestId.value = duplicate.id;
  response.value = null;
  closeMenus();
  try {
    await persistCollection();
    flash("Petición duplicada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteRequestByID(id, parentId = "") {
  const request = findRequest(collection.value, id);
  if (!request || !window.confirm(`¿Eliminar la petición «${request.name}»?`)) return;
  if (parentId) {
    const parent = findFolder(collection.value?.folders || [], parentId);
    if (!parent) {
      flash("No se encontró la carpeta de la petición.", true);
      return;
    }
    parent.requests = parent.requests.filter((item) => item.id !== id);
  } else {
    collection.value.requests = collection.value.requests.filter((item) => item.id !== id);
  }
  if (requestId.value === id) {
    requestId.value = "";
    response.value = null;
  }
  closeMenus();
  try {
    await persistCollection();
    flash("Petición eliminada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function saveRequest() {
  try {
    await persistCollection();
    flash("Petición guardada.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteRequest() {
  if (selectedRequest.value) await deleteRequestByID(requestId.value, folderId.value);
}

async function runRequest() {
  busy.value = true;
  response.value = null;
  try {
    await persistCollection();
    response.value = await backend().RunRequest(project.value.id, selectedRequest.value);
  } catch (error) {
    flash(error.message || String(error), true);
  } finally {
    busy.value = false;
  }
}

function readSelectedFile(event) {
  return new Promise((resolve, reject) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return resolve("");
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error("No se pudo leer el archivo seleccionado."));
    reader.readAsText(file);
  });
}

async function importProject(event) {
  try {
    const raw = await readSelectedFile(event);
    if (!raw) return;
    const imported = await backend().ImportProjectConfig(raw);
    await refreshProjects();
    openProject(imported.id);
    flash(`Proyecto «${imported.name}» importado.`);
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function createDemoProject() {
  try {
    const sample = await backend().CreateDemoProject();
    await refreshProjects();
    openProject(sample.id);
    const sampleCollection = sample.collections?.[0];
    if (sampleCollection) {
      selectCollection(sampleCollection);
      const sampleRequest = sampleCollection.requests?.[0];
      if (sampleRequest) selectRequest(sampleRequest.id);
    }
    flash("Proyecto de ejemplo listo. Pulsa Enviar para ver las diferencias.");
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function importPostman(event) {
  try {
    const raw = await readSelectedFile(event);
    if (!raw) return;
    const imported = await backend().ImportPostmanCollection(project.value.id, raw);
    await refreshProjects();
    selectCollection(imported.collection);
    if (imported.warnings?.length) {
      window.alert(`Colección importada con estas limitaciones:\n\n• ${imported.warnings.join("\n• ")}`);
    }
    flash(`Colección «${imported.collection.name}» importada.`);
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

function prettyBody(body) {
  try { return JSON.stringify(JSON.parse(body), null, 2); }
  catch { return body || "(respuesta vacía)"; }
}

function statusClass(item) {
  if (item.error) return "status-error";
  if (item.status_code >= 200 && item.status_code < 300) return "status-good";
  if (item.status_code >= 400) return "status-error";
  return "status-warn";
}

onMounted(async () => {
  setTheme(isDark.value);
  const savedSidebarWidth = Number(getPreference("sidebar-width"));
  if (Number.isFinite(savedSidebarWidth) && savedSidebarWidth > 0) {
    sidebarWidth.value = clampSidebarWidth(savedSidebarWidth);
  }
  try {
    await refreshProjects();
  } catch (error) {
    flash(error.message || String(error), true);
  }
});
</script>

<template>
  <div class="shell" @click="closeMenus">
    <header class="topbar">
      <div class="brand" @click="goHome" title="Inicio">
        <span class="brand-mark">T</span><span>DiffTwin</span>
      </div>
      <div class="top-divider"></div>
      <button v-if="project" class="nav-home" title="Volver al dashboard" aria-label="Volver al dashboard" @click="goHome">
        <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m2.8 9 7.2-6 7.2 6v8H12v-5H8v5H2.8V9Z"/></svg>
      </button>
      <div v-if="project" class="project-switch">
        <select v-model="projectId" aria-label="Proyecto" @change="openProject(projectId)">
          <option v-for="item in projects" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
        <span class="chevron" aria-hidden="true"><svg viewBox="0 0 16 16"><path d="m4.5 6 3.5 3.5L11.5 6" /></svg></span>
      </div>
      <div v-else class="top-context">Espacio de trabajo</div>
      <div class="top-spacer"></div>
      <template v-if="project">
        <button class="icon-button topbar-action" title="Configuración del proyecto" aria-label="Configuración del proyecto" @click="editProject">
          <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M8.5 2.8h3l.45 1.7c.47.15.9.4 1.3.7l1.7-.55 1.5 2.6-1.25 1.25c.1.5.1 1 0 1.5l1.25 1.25-1.5 2.6-1.7-.55c-.4.3-.83.55-1.3.7l-.45 1.7h-3l-.45-1.7a5.6 5.6 0 0 1-1.3-.7l-1.7.55-1.5-2.6 1.25-1.25a4.8 4.8 0 0 1 0-1.5L3.55 7.25l1.5-2.6 1.7.55c.4-.3.83-.55 1.3-.7l.45-1.7Z"/><circle cx="10" cy="9.25" r="2.3"/></svg>
        </button>
      </template>
      <button class="avatar" title="DiffTwin">DT</button>
    </header>

    <div v-if="notice || errorMessage" class="toast" :class="{ error: !!errorMessage }">
      <span>{{ errorMessage || notice }}</span><button @click="notice = ''; errorMessage = ''">×</button>
    </div>

    <main v-if="!project" class="dashboard">
      <div class="dashboard-heading">
        <div>
          <div class="eyebrow">TU ESPACIO DE TRABAJO</div>
          <h1>Tus proyectos</h1>
          <p>Organiza tus APIs y compara cambios entre entornos.</p>
          <p class="demo-caption">El ejemplo usa un servicio público de eco y requiere conexión a internet.</p>
        </div>
        <div class="dashboard-actions">
          <button class="button secondary" @click="createDemoProject">▷ Probar ejemplo</button>
          <button class="button secondary" @click="projectFileInput.click()">＋ Importar proyecto</button>
          <button class="button primary" @click="newProject">＋ Nuevo proyecto</button>
          <input ref="projectFileInput" type="file" accept=".json,application/json" hidden @change="importProject">
        </div>
      </div>
      <div v-if="projects.length" class="project-grid">
        <button v-for="item in projects" :key="item.id" class="project-card" @click="openProject(item.id)">
          <div class="project-card-top">
            <span class="project-logo">{{ item.name.slice(0, 1).toUpperCase() }}</span>
            <span class="card-arrow">↗</span>
          </div>
          <strong>{{ item.name }}</strong>
          <span class="card-meta">{{ item.collections.length }} colecciones · {{ item.collections.reduce((n, c) => n + countRequests(c.requests || [], c.folders || []), 0) }} peticiones</span>
          <div class="target-summary">
            <span><i class="target-dot stable"></i>{{ item.primary }}</span>
            <span><i class="target-dot candidate"></i>{{ item.candidate }}</span>
            <span v-if="item.secondary"><i class="target-dot stable"></i>{{ item.secondary }}</span>
          </div>
        </button>
      </div>
      <div v-else class="empty-state">
        <span class="empty-icon">◇</span>
        <h2>Empieza creando un proyecto</h2>
        <p>Configura tus destinos y guarda tus primeras peticiones de API.</p>
        <button class="button primary" @click="newProject">＋ Crear proyecto</button>
      </div>
    </main>

    <div v-else class="workspace" :class="{ 'sidebar-resizing': sidebarResizing }" :style="{ '--sidebar-width': `${sidebarWidth}px` }">
      <aside class="sidebar">
        <div class="sidebar-heading">
          <div v-if="!collectionSearchOpen" class="sidebar-heading-title">
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m10 2.8 6.7 3.6v7.2L10 17.2l-6.7-3.6V6.4L10 2.8Z"/><path d="m3.3 6.4 6.7 3.7 6.7-3.7M10 10.1v7.1"/></svg>
            <span>Colecciones</span>
          </div>
          <input
            v-else
            ref="collectionSearchInput"
            v-model="collectionSearch"
            class="collection-search"
            aria-label="Buscar colecciones y peticiones"
            placeholder="Buscar…"
            @keydown.esc="toggleCollectionSearch"
          >
          <div class="sidebar-heading-actions">
            <button class="icon-button" :title="collectionSearchOpen ? 'Cerrar búsqueda' : 'Buscar'" :aria-label="collectionSearchOpen ? 'Cerrar búsqueda' : 'Buscar'" @click="toggleCollectionSearch">
              <svg v-if="!collectionSearchOpen" viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.7" cy="8.7" r="5.6"/><path d="m12.8 12.8 4 4"/></svg>
              <svg v-else viewBox="0 0 20 20" aria-hidden="true"><path d="m5 5 10 10M15 5 5 15"/></svg>
            </button>
            <button class="icon-button" title="Nueva colección" aria-label="Nueva colección" @click="createCollection">＋</button>
            <button class="icon-button" title="Más opciones" aria-label="Más opciones" :aria-expanded="activeMenu === menuKey('import')" @click.stop="toggleMenu('import')">
              <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="4" cy="10" r="1"/><circle cx="10" cy="10" r="1"/><circle cx="16" cy="10" r="1"/></svg>
            </button>
            <div v-if="activeMenu === menuKey('import')" class="collection-menu sidebar-options-menu" @click.stop>
              <button @click="closeMenus(); postmanFileInput.click()">Importar colección de Postman</button>
            </div>
          </div>
          <input ref="postmanFileInput" type="file" accept=".json,application/json" hidden @change="importPostman">
        </div>
        <div class="collection-tree">
          <div v-for="item in visibleCollections" :key="item.id" class="collection-group">
            <div class="tree-row collection-row" :class="{ selected: collectionId === item.id && !folderId && !requestId, 'menu-open': activeMenu === menuKey('collection', item.id) }">
              <button
                class="tree-toggle"
                :aria-label="expandedCollections.has(item.id) ? 'Contraer colección' : 'Expandir colección'"
                :aria-expanded="expandedCollections.has(item.id)"
                @click="toggleCollection(item)"
              >
                <svg class="tree-chevron" :class="{ expanded: expandedCollections.has(item.id) }" viewBox="0 0 16 16" aria-hidden="true">
                  <path d="m5.5 6.5 2.5 2.5 2.5-2.5" />
                </svg>
              </button>
              <button class="tree-label" @click="selectCollection(item)">
                <span class="collection-icon">▤</span><span class="tree-name">{{ item.name }}</span>
              </button>
              <div class="tree-actions">
                <button
                  class="icon-button tiny"
                  title="Opciones de colección"
                  :aria-expanded="activeMenu === menuKey('collection', item.id)"
                  @click.stop="toggleMenu('collection', item.id)"
                >···</button>
              </div>
              <div v-if="activeMenu === menuKey('collection', item.id)" class="collection-menu" @click.stop>
                <button @click="selectCollection(item); createRequest()">＋ Nueva petición</button>
                <button @click="selectCollection(item); createFolder()">＋ Nueva carpeta</button>
                <div class="collection-menu-separator"></div>
                <button @click="selectCollection(item); renameCollection()">Renombrar colección</button>
                <button class="collection-menu-danger" @click="selectCollection(item); deleteCollection()">Eliminar colección</button>
              </div>
            </div>
            <div v-if="(expandedCollections.has(item.id) || collectionSearch.trim()) && collectionId === item.id" class="tree-children">
              <div v-for="request in (item.requests || []).filter((entry) => requestMatches(entry, collectionItemSearch(item).toLocaleLowerCase()))" :key="request.id" class="request-branch">
                <div
                  class="tree-row request-row"
                  :class="{ selected: requestId === request.id, 'menu-open': activeMenu === menuKey('request', request.id) }"
                  role="button"
                  tabindex="0"
                  @click="selectRequest(request.id)"
                  @keydown.enter="selectRequest(request.id)"
                >
                  <span class="tree-spacer"></span>
                  <span class="method-badge" :class="`method-${request.method.toLowerCase()}`">{{ request.method }}</span>
                  <span class="tree-name">{{ request.name }}</span>
                  <span class="tree-actions">
                    <button class="icon-button tiny" title="Opciones de petición" :aria-expanded="activeMenu === menuKey('request', request.id)" @click.stop="toggleMenu('request', request.id)">···</button>
                  </span>
                </div>
                <div v-if="activeMenu === menuKey('request', request.id)" class="collection-menu request-menu" @click.stop>
                  <button @click="closeMenus(); duplicateRequest(request)">Duplicar petición</button>
                  <button class="collection-menu-danger" @click="closeMenus(); deleteRequestByID(request.id)">Eliminar petición</button>
                </div>
              </div>
              <FolderTree
                v-for="folder in item.folders || []"
                :key="folder.id"
                :folder="folder"
                :selected-folder-id="folderId"
                :selected-request-id="requestId"
                :active-menu="activeMenu"
                :search-term="collectionItemSearch(item)"
                @select-folder="selectFolder"
                @select-request="selectRequest"
                @toggle-menu="toggleMenu"
                @close-menus="closeMenus"
                @duplicate-request="duplicateRequest"
                @delete-request="deleteRequestByID"
                @create-folder="createFolder"
                @create-request="createRequest"
                @rename-folder="renameFolder"
                @delete-folder="deleteFolder"
              />
            </div>
          </div>
          <div v-if="!visibleCollections.length" class="sidebar-empty">
            <p v-if="collectionSearch.trim()">No se encontraron coincidencias.</p>
            <template v-else>
            <p>Aún no hay colecciones.</p>
            <button class="button secondary small" @click="createCollection">Crear colección</button>
            </template>
          </div>
        </div>
        <div class="sidebar-bottom">
          <span>{{ requestCount }} peticiones</span>
          <span>DiffTwin</span>
        </div>
      </aside>

      <div
        class="sidebar-resizer"
        role="separator"
        aria-label="Cambiar ancho del panel lateral"
        aria-orientation="vertical"
        :aria-valuenow="sidebarWidth"
        :aria-valuemin="minSidebarWidth"
        :aria-valuemax="maxSidebarWidth()"
        tabindex="0"
        @pointerdown="startSidebarResize"
        @pointermove="moveSidebarResize"
        @pointerup="finishSidebarResize"
        @pointercancel="finishSidebarResize"
        @keydown="resizeSidebarWithKeyboard"
      ></div>

      <section class="main-panel">
        <div class="workspace-toolbar">
          <div class="breadcrumbs">
            <span>{{ project.name }}</span><span v-if="collection">/</span>
            <span v-if="collection">{{ collection.name }}</span>
            <span v-if="selectedFolder">/</span><span v-if="selectedFolder">{{ selectedFolder.name }}</span>
            <template v-if="selectedRequest"><span>/</span><strong>{{ selectedRequest.name }}</strong></template>
          </div>
        </div>

        <div class="target-strip" :class="{ 'target-strip-two': !project.secondary }">
          <button type="button" class="target-chip target-chip-button" aria-label="Editar estable principal" @click="editProject('primary')">
            <i class="target-dot stable"></i><span class="target-chip-content"><span>ESTABLE PRINCIPAL</span><strong>{{ project.primary }}</strong></span>
          </button>
          <button type="button" class="target-chip target-chip-button" aria-label="Editar candidata" @click="editProject('candidate')">
            <i class="target-dot candidate"></i><span class="target-chip-content"><span>CANDIDATA</span><strong>{{ project.candidate }}</strong></span>
          </button>
          <button v-if="project.secondary" type="button" class="target-chip target-chip-button" aria-label="Editar estable secundaria" @click="editProject('secondary')">
            <i class="target-dot stable"></i><span class="target-chip-content"><span>ESTABLE SECUNDARIA</span><strong>{{ project.secondary }}</strong></span>
          </button>
        </div>
        <details class="workspace-auth">
          <summary class="workspace-auth-summary">
            <span><strong>Configuración de destinos</strong><small>Autenticación, variables y opciones de comparación</small></span>
            <span class="settings-toggle">Configurar <span aria-hidden="true">⌄</span></span>
          </summary>
          <div class="workspace-auth-content">
          <div class="workspace-auth-heading">
            <div><strong>Autenticación y opciones</strong><span>Configura credenciales y opciones por proyecto.</span></div>
            <button class="button primary small" :disabled="authSaving || authLoadedProjectId !== project.id" @click="saveProjectAuth">
              {{ authSaving ? "Guardando…" : "Guardar opciones" }}
            </button>
          </div>
          <div class="auth-target-tabs">
            <button type="button" class="button small" :class="authTarget === 'primary' ? 'primary' : 'secondary'" @click="setAuthTarget('primary')">Estable principal</button>
            <button type="button" class="button small" :class="authTarget === 'candidate' ? 'primary' : 'secondary'" @click="setAuthTarget('candidate')">Candidata</button>
            <button v-if="project.secondary" type="button" class="button small" :class="authTarget === 'secondary' ? 'primary' : 'secondary'" @click="setAuthTarget('secondary')">Estable secundaria</button>
          </div>
          <p v-if="authLoadingProjectId === project.id" class="auth-storage-note">Cargando autenticación…</p>
          <div class="workspace-auth-fields">
            <label class="field-label">MÉTODO
              <select :value="authForm.auth[authTarget].type || 'none'" @change="setAuthType($event.target.value)">
                <option value="none">Sin autenticación</option>
                <option value="bearer">Bearer</option>
                <option value="basic">Basic</option>
                <option value="api_key">API key</option>
              </select>
            </label>
            <template v-if="authForm.auth[authTarget].type === 'bearer'">
              <label class="field-label">TOKEN<input v-model="authForm.credentials[authTarget].token" type="password" autocomplete="new-password" required></label>
            </template>
            <template v-else-if="authForm.auth[authTarget].type === 'basic'">
              <label class="field-label">USUARIO<input v-model="authForm.credentials[authTarget].username" autocomplete="off"></label>
              <label class="field-label">CONTRASEÑA<input v-model="authForm.credentials[authTarget].password" type="password" autocomplete="new-password" required></label>
            </template>
            <template v-else-if="authForm.auth[authTarget].type === 'api_key'">
              <label class="field-label">NOMBRE DE LA CLAVE<input v-model="authForm.auth[authTarget].api_key_name" required placeholder="X-API-Key"></label>
              <label class="field-label">ENVIAR EN
                <select v-model="authForm.auth[authTarget].api_key_location">
                  <option value="header">Header</option>
                  <option value="query">Query parameter</option>
                </select>
              </label>
              <label class="field-label">VALOR<input v-model="authForm.credentials[authTarget].api_key" type="password" autocomplete="new-password" required></label>
            </template>
          </div>
          <label class="field-label destination-variables-field">VARIABLES ESPECÍFICAS · JSON
            <textarea
              v-model="destinationVariableDrafts[authTarget]"
              spellcheck="false"
              placeholder='{ "region": "eu-west", "userId": "123" }'
            ></textarea>
          </label>
          <p class="auth-storage-note">Estas variables sobrescriben las comunes solo para este destino; las demás conservan el valor común. Se guardan en el JSON del proyecto sin cifrar: no pongas secretos aquí.</p>
          <p v-if="authForm.auth[authTarget].type === 'api_key' && authForm.auth[authTarget].api_key_location === 'query'" class="auth-security-warning">
            Las claves en la URL pueden quedar en registros del servidor o de proxies. Usa Header si la API lo admite.
          </p>
          <p v-if="authCredentialsMissing" class="auth-security-warning">Faltan credenciales guardadas en el sistema. Vuelve a introducirlas para este proyecto.</p>
          <p v-if="authLoadedProjectId !== project.id && authLoadingProjectId !== project.id" class="auth-security-warning">No se pudieron cargar las credenciales seguras. Comprueba que el almacén del sistema esté disponible.</p>
          <div class="comparison-settings">
            <label class="field-label">IGNORAR RUTAS JSON · UNA POR LÍNEA
              <textarea v-model="ignoredJSONPathsText" spellcheck="false" placeholder="$.meta.generatedAt&#10;$.items[*].id"></textarea>
            </label>
            <p>Admite rutas como <code>$.meta.timestamp</code>, <code>$["campo.con.puntos"]</code> y comodines <code>*</code>. Guardar autenticación también guarda estas opciones.</p>
          </div>
          <p class="auth-storage-note">Las credenciales se guardan en el almacén seguro del sistema y no se incluyen en la configuración JSON del proyecto.</p>
          </div>
        </details>

        <section v-if="selectedRequest" class="request-editor">
          <div class="request-sticky">
            <div class="request-titlebar">
              <div><span class="eyebrow">PETICIÓN</span><input v-model="selectedRequest.name" class="request-title-input" aria-label="Nombre de petición"></div>
              <button class="icon-button danger-icon" title="Eliminar petición" aria-label="Eliminar petición" @click="deleteRequest">
                <svg class="trash-icon" viewBox="0 0 16 16" aria-hidden="true">
                  <path d="M2.5 4.5h11M6 4.5V3h4v1.5m2.5 0-.7 9h-6L5 4.5m2.5 2.2v4.5m3-4.5v4.5" />
                </svg>
              </button>
            </div>
            <div class="request-urlbar">
              <select v-model="selectedRequest.method" class="method-select" :class="`method-${selectedRequest.method.toLowerCase()}`">
                <option v-for="method in ['GET','POST','PUT','PATCH','DELETE','HEAD','OPTIONS']" :key="method">{{ method }}</option>
              </select>
              <input v-model="selectedRequest.path" class="url-input" placeholder="/v1/users/{{userId}}">
              <button class="button primary send-button" :disabled="busy" @click="runRequest">
                <span v-if="busy" class="spinner"></span><span v-else>▶</span>{{ busy ? "Ejecutando…" : "Enviar" }}
              </button>
            </div>
          </div>
          <div class="editor-tabs">
            <button class="editor-tab" :class="{ active: activeEditorTab === 'params' }" @click="activeEditorTab = 'params'">Params <span class="tab-count">{{ queryParamCount }}</span></button>
            <button class="editor-tab" :class="{ active: activeEditorTab === 'headers' }" @click="activeEditorTab = 'headers'">Headers <span class="tab-count">{{ Object.keys(selectedRequest.headers || {}).length }}</span></button>
            <button class="editor-tab" :class="{ active: activeEditorTab === 'body' }" @click="activeEditorTab = 'body'">Body</button>
            <span class="tab-spacer"></span>
            <button class="button secondary small" @click="saveRequest">Guardar</button>
          </div>
          <div class="editor-grid single">
            <div v-if="activeEditorTab === 'params'" class="params-editor">
              <div class="params-heading"><span>PARÁMETROS DE CONSULTA</span><button class="button secondary small" @click="addQueryParam">＋ Añadir parámetro</button></div>
              <div v-if="queryParams.length" class="params-table">
                <div class="param-row param-column-head"><span>CLAVE</span><span>VALOR</span><span></span></div>
                <div v-for="(param, index) in queryParams" :key="index" class="param-row">
                  <input v-model="param.key" aria-label="Clave del parámetro" placeholder="page" @change="updateQueryPath">
                  <input v-model="param.value" aria-label="Valor del parámetro" placeholder="1" @change="updateQueryPath">
                  <button class="icon-button danger-icon" :aria-label="`Eliminar parámetro ${param.key || index + 1}`" title="Eliminar parámetro" @click="removeQueryParam(index)">×</button>
                </div>
              </div>
              <p v-else class="params-empty">No hay parámetros. Añade uno o escribe una query en la URL.</p>
            </div>
            <label v-if="activeEditorTab === 'headers'" class="field-label">HEADERS · JSON
              <textarea
                :value="JSON.stringify(selectedRequest.headers || {}, null, 2)"
                spellcheck="false"
                @change="(event) => { try { selectedRequest.headers = JSON.parse(event.target.value); } catch { flash('Headers debe ser JSON válido.', true); } }"
              ></textarea>
            </label>
            <label v-else-if="activeEditorTab === 'body'" class="field-label">BODY
              <textarea v-model="selectedRequest.body" spellcheck="false" placeholder='{ "example": true }'></textarea>
            </label>
          </div>
          <div v-if="response" class="response-section">
            <div class="response-heading"><span>RESULTADO DE LA COMPARACIÓN</span><span>Estable principal frente a candidata</span></div>
            <div class="verdict" :class="comparisonSummary.tone">
              <span class="verdict-icon">{{ comparisonSummary.icon }}</span>
              <strong>{{ comparisonSummary.title }}</strong>
              <span>{{ comparisonSummary.detail }}</span>
              <span v-if="response.secondary_enabled && response.noise_detected" class="noise-badge">Posible ruido entre estables</span>
            </div>
            <div class="latency-comparison" aria-label="Comparación de tiempos de respuesta">
              <span v-for="item in latencySummaries" :key="item.name">
                <strong>{{ item.name }}:</strong> {{ item.duration }} ms
                <small>({{ item.delta > 0 ? "+" : "" }}{{ item.delta }} ms frente a estable principal)</small>
              </span>
              <small>La latencia es informativa y no cambia el veredicto.</small>
            </div>
            <section
              v-for="panel in comparisonPanels"
              :key="panel.id"
              class="diff-panel"
              :class="{ 'diff-panel-noise': panel.id === 'stable' }"
              :aria-label="panel.title"
            >
              <header class="diff-heading">
                <strong>{{ panel.title }}</strong>
                <span>{{ Math.min(panel.diffs.length, 100) }}{{ panel.diffs.length > 100 ? "+" : "" }} {{ panel.diffs.length === 1 ? "cambio" : "cambios" }}</span>
              </header>
              <div class="diff-table">
                <div class="diff-row diff-column-head">
                  <span>Ruta</span><span>{{ panel.leftLabel }}</span><span>{{ panel.rightLabel }}</span>
                </div>
                <div v-for="(diff, index) in panel.diffs.slice(0, 100)" :key="`${diff.path}-${index}`" class="diff-row" :class="`diff-${diff.kind}`">
                  <code class="diff-path" :title="diff.path">{{ diff.path }}</code>
                  <code class="diff-value" :title="String(diff.before).slice(0, 1000)">{{ diff.before }}</code>
                  <code class="diff-value" :title="String(diff.after).slice(0, 1000)">{{ diff.after }}</code>
                </div>
              </div>
              <p v-if="panel.diffs.length > 100" class="diff-truncated">Se muestran los primeros 100 cambios.</p>
            </section>
            <div class="response-grid" :class="{ 'response-grid-two': !response.secondary_enabled }">
              <article v-for="[name, result] in responseCards" :key="name" class="response-card">
                <header><strong>{{ name }}</strong><span class="response-meta" :class="statusClass(result)">{{ result.error ? "Error" : `HTTP ${result.status_code}` }} · {{ result.duration_ms }} ms</span></header>
                <pre>{{ result.error || prettyBody(result.body) }}{{ result.truncated ? "\n[Respuesta truncada a 2 MiB]" : "" }}</pre>
              </article>
            </div>
          </div>
        </section>

        <div v-else-if="collection" class="content-empty">
          <div class="empty-icon">⌘</div>
          <h2>{{ selectedFolder ? selectedFolder.name : collection.name }}</h2>
          <p>Selecciona una petición o crea una nueva para empezar.</p>
          <div class="empty-actions">
            <button class="button primary" @click="createRequest(folderId)">＋ Nueva petición</button>
            <button class="button secondary" @click="createFolder(folderId)">▰ Nueva carpeta</button>
          </div>
        </div>
        <div v-else class="content-empty">
          <div class="empty-icon">⌘</div>
          <h2>Tu espacio de trabajo</h2>
          <p>Selecciona una colección o crea una nueva.</p>
          <button class="button primary" @click="createCollection">＋ Nueva colección</button>
        </div>

        <footer class="statusbar">
          <span><i class="online-dot"></i>Listo</span><span>{{ projectLabel }}</span>
          <button class="icon-button theme-footer" :title="isDark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro'" @click="toggleTheme">{{ isDark ? "☼" : "☾" }}</button>
        </footer>
      </section>
    </div>

    <footer v-if="!project" class="dashboard-footer">
      <span><span class="brand-mark mini">T</span> DiffTwin <span class="footer-muted">· Comparación de APIs</span></span>
      <button class="icon-button theme-footer" :title="isDark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro'" @click="toggleTheme">
        {{ isDark ? "☼" : "☾" }}
      </button>
    </footer>

    <div v-if="projectDialog" class="modal-backdrop" @click.self="projectDialog = false">
      <form class="modal-card" @submit.prevent="saveProject">
        <div class="modal-heading"><div><div class="eyebrow">CONFIGURACIÓN</div><h2>{{ editingProject ? "Editar proyecto" : "Nuevo proyecto" }}</h2></div><button type="button" class="icon-button" @click="projectDialog = false">×</button></div>
        <label class="field-label">NOMBRE<input v-model="projectForm.name" required placeholder="Mi API"></label>
        <label class="field-label">ESTABLE PRINCIPAL<input ref="primaryProjectInput" v-model="projectForm.primary" required placeholder="https://api-stable.example"></label>
        <label class="field-label">CANDIDATA<input ref="candidateProjectInput" v-model="projectForm.candidate" required placeholder="https://api-candidate.example"></label>
        <label class="field-label">ESTABLE SECUNDARIA · OPCIONAL<input ref="secondaryProjectInput" v-model="projectForm.secondary" placeholder="https://api-stable-2.example"></label>
        <label class="field-label">IGNORAR RUTAS JSON · UNA POR LÍNEA<textarea v-model="ignoredJSONPathsText" spellcheck="false" placeholder="$.meta.generatedAt&#10;$.items[*].id"></textarea></label>
        <label class="field-label">VARIABLES · JSON<textarea v-model="projectForm.variables" placeholder="{}"></textarea></label>
        <div v-if="editingProject" class="danger-zone">
          <div><strong>Eliminar proyecto</strong><p>Elimina el proyecto y todas sus colecciones y peticiones.</p></div>
          <button type="button" class="button danger-outline" @click="deleteProject">Eliminar</button>
        </div>
        <div class="modal-actions"><button type="button" class="button secondary" @click="projectDialog = false">Cancelar</button><button class="button primary">{{ editingProject ? "Guardar cambios" : "Crear proyecto" }}</button></div>
      </form>
    </div>
  </div>
</template>
