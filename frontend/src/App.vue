<script setup>
import { computed, nextTick, onMounted, ref, watch } from "vue";
import * as AppBindings from "../wailsjs/go/main/App";
import FolderTree from "./FolderTree.vue";
import { detectLocale, translate } from "./i18n";

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
const collectionRunning = ref(false);
const collectionRun = ref(null);
const activeEditorTab = ref("params");
const queryParams = ref([]);
const notice = ref("");
const errorMessage = ref("");
const isDark = ref(getPreference("theme") !== "light");
const storedLocale = getPreference("language");
const locale = ref(storedLocale === "en" || storedLocale === "es" ? storedLocale : detectLocale());
const t = (source, values) => translate(locale.value, source, values);
const projectDialog = ref(false);
const importConflict = ref(null);
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
const renamingCollectionId = ref("");
const collectionRenameDraft = ref("");
const collectionRenameInput = ref(null);
const draggedTreeItem = ref(null);
const treeDropTarget = ref("");
const movingTreeItem = ref(false);
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
const projectLabel = computed(() => project.value?.name || t("Selecciona un proyecto"));
const visibleCollectionRun = computed(() =>
  collectionRun.value?.collectionId === collection.value?.id ? collectionRun.value : null
);
const latencySummaries = computed(() => {
  if (!response.value) return [];
  const primaryDuration = response.value.primary.duration_ms;
  const destinations = [[t("Candidata"), response.value.candidate]];
  if (response.value.secondary_enabled) destinations.push([t("Estable secundaria"), response.value.secondary]);
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
  if (failed) return { tone: "error", icon: "!", title: t("Error al consultar"), detail: t("Una o más respuestas no pudieron completarse.") };
  if (response.value.candidate_differs) {
    return { tone: "different", icon: "≠", title: t("Hay diferencias"), detail: t("La candidata no coincide con la estable principal.") };
  }
  return { tone: "same", icon: "✓", title: t("Las respuestas coinciden"), detail: t("La candidata coincide con la estable principal.") };
});
const responseCards = computed(() => {
  if (!response.value) return [];
  const cards = [
    [t("Estable principal"), response.value.primary],
    [t("Candidata"), response.value.candidate]
  ];
  if (response.value.secondary_enabled) cards.push([t("Estable secundaria"), response.value.secondary]);
  return cards;
});
function getResponseDiffs(primary, compared) {
  const diffs = [];
  const addDiff = (path, before, after, kind = "changed") => {
    if (diffs.length < 101) diffs.push({ path, before, after, kind });
  };

  if (primary.status_code !== compared.status_code) {
    addDiff("HTTP status", primary.status_code || t("(sin respuesta)"), compared.status_code || t("(sin respuesta)"));
  }
  if (primary.error !== compared.error) {
    addDiff(t("Error de transporte"), primary.error || t("(sin error)"), compared.error || t("(sin error)"));
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
            if (index >= left.length) addDiff(`${path}[${index}]`, t("(no existe)"), formatValue(right[index]), "added");
            else if (index >= right.length) addDiff(`${path}[${index}]`, formatValue(left[index]), t("(no existe)"), "removed");
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
            if (!Object.hasOwn(left, key)) addDiff(childPath, t("(no existe)"), formatValue(right[key]), "added");
            else if (!Object.hasOwn(right, key)) addDiff(childPath, formatValue(left[key]), t("(no existe)"), "removed");
            else compare(left[key], right[key], childPath, depth + 1, [...segments, { key }]);
            if (diffs.length >= 101) break;
          }
          return;
        }
        addDiff(path, formatValue(left), formatValue(right));
      };
      if (leftBody !== removed || rightBody !== removed) compare(leftBody, rightBody);
    } else {
      addDiff(t("Cuerpo (texto)"), primary.body || t("(vacío)"), compared.body || t("(vacío)"));
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
      title: t("Candidata frente a estable principal"),
      leftLabel: t("Estable principal"),
      rightLabel: t("Candidata"),
      diffs: getResponseDiffs(response.value.primary, response.value.candidate)
    });
  }
  if (response.value.secondary_enabled && response.value.noise_detected) {
    panels.push({
      id: "stable",
      title: t("Variación entre instancias estables"),
      leftLabel: t("Estable principal"),
      rightLabel: t("Estable secundaria"),
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

function projectSummary(item) {
  const requestCount = item.collections.reduce((sum, entry) =>
    sum + countRequests(entry.requests || [], entry.folders || []), 0);
  const collectionLabel = item.collections.length === 1 ? "{count} colección" : "{count} colecciones";
  const requestLabel = requestCount === 1 ? "{count} petición" : "{count} peticiones";
  return `${t(collectionLabel, { count: item.collections.length })} · ${t(requestLabel, { count: requestCount })}`;
}

function collectCollectionRequests(item) {
  const requests = (item.requests || []).map((request) => ({
    request: JSON.parse(JSON.stringify(request)),
    folderId: ""
  }));
  const visitFolders = (folders) => {
    for (const folder of folders || []) {
      for (const request of folder.requests || []) {
        requests.push({
          request: JSON.parse(JSON.stringify(request)),
          folderId: folder.id
        });
      }
      visitFolders(folder.folders || []);
    }
  };
  visitFolders(item.folders || []);
  return requests;
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

function treeDropTargetKey(type, collectionID, folderID = "") {
  return type === "folder" ? `folder:${collectionID}:${folderID}` : `collection:${collectionID}`;
}

function startTreeDrag(event, item) {
  draggedTreeItem.value = item;
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = "move";
    event.dataTransfer.setData("text/plain", `${item.type}:${item.id}`);
  }
  closeMenus();
}

function endTreeDrag() {
  draggedTreeItem.value = null;
  treeDropTarget.value = "";
}

function sourceFolderContains(folder, targetID) {
  return (folder.folders || []).some((child) =>
    child.id === targetID || sourceFolderContains(child, targetID)
  );
}

function canDropTreeItem(targetCollectionID, targetFolderID = "") {
  const item = draggedTreeItem.value;
  if (!item || movingTreeItem.value) return false;
  if (item.type === "request") {
    return item.collectionId !== targetCollectionID || item.parentId !== targetFolderID;
  }
  if (item.type !== "folder") return false;
  if (item.collectionId !== targetCollectionID) return true;
  if (item.id === targetFolderID || item.parentId === targetFolderID) return false;
  const sourceCollection = project.value?.collections.find((entry) => entry.id === item.collectionId);
  const sourceFolder = findFolder(sourceCollection?.folders || [], item.id);
  return !sourceFolder || !sourceFolderContains(sourceFolder, targetFolderID);
}

function treeDragOver(event, targetCollectionID, targetFolderID = "") {
  if (!canDropTreeItem(targetCollectionID, targetFolderID)) {
    treeDropTarget.value = "";
    return;
  }
  event.preventDefault();
  if (event.dataTransfer) event.dataTransfer.dropEffect = "move";
  treeDropTarget.value = treeDropTargetKey(targetFolderID ? "folder" : "collection", targetCollectionID, targetFolderID);
}

async function dropTreeItem(targetCollectionID, targetFolderID = "") {
  if (!canDropTreeItem(targetCollectionID, targetFolderID)) return;
  const item = draggedTreeItem.value;
  if (!item || !project.value) return;
  movingTreeItem.value = true;
  try {
    if (collection.value) await persistCollection();
    await backend().MoveCollectionItem(
      project.value.id,
      item.collectionId,
      item.parentId,
      item.type,
      item.id,
      targetCollectionID,
      targetFolderID
    );
    await refreshProjects();
    const destination = project.value?.collections.find((entry) => entry.id === targetCollectionID);
    if (destination) {
      collectionId.value = destination.id;
      expandedCollections.value.add(destination.id);
      folderId.value = item.type === "folder" ? item.id : targetFolderID;
      requestId.value = item.type === "request" ? item.id : "";
      response.value = null;
      flash(t(item.type === "folder" ? "Carpeta movida." : "Petición movida."));
    }
  } catch (error) {
    flash(error.message || String(error), true);
  } finally {
    movingTreeItem.value = false;
    endTreeDrag();
  }
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

function setLocale(value) {
  locale.value = value === "en" ? "en" : "es";
  setPreference("language", locale.value);
  document.documentElement.lang = locale.value;
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
    flash(t(editingProject.value ? "Proyecto actualizado." : "Proyecto creado."));
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
        throw new Error(t("Las variables de {destination} deben ser un objeto JSON con valores de texto.", {
          destination: destinationLabel(target)
        }));
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
    flash(t("Opciones guardadas."));
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
    primary: t("la estable principal"),
    candidate: t("la candidata"),
    secondary: t("la estable secundaria")
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

async function deleteProject(item = project.value) {
  if (!item || !window.confirm(t("¿Eliminar el proyecto «{name}»?\n\nSe eliminarán también sus colecciones, peticiones y credenciales guardadas. Esta acción no se puede deshacer.", { name: item.name }))) return;
  try {
    await backend().DeleteProject(item.id);
    if (projectId.value === item.id) {
      projectDialog.value = false;
      goHome();
    }
    await refreshProjects();
    flash(t("Proyecto «{name}» eliminado.", { name: item.name }));
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
  const name = window.prompt(t("Nombre de la colección"));
  if (!name?.trim()) return;
  try {
    const saved = await backend().SaveCollection(project.value.id, {
      id: "", name: name.trim(), requests: [], folders: []
    });
    await refreshProjects();
    selectCollection(saved);
    flash(t("Colección creada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

function startCollectionRename(item) {
  renamingCollectionId.value = item.id;
  collectionRenameDraft.value = item.name;
  closeMenus();
  nextTick(() => {
    collectionRenameInput.value?.focus();
    collectionRenameInput.value?.select();
  });
}

function cancelCollectionRename(item) {
  if (renamingCollectionId.value !== item.id) return;
  renamingCollectionId.value = "";
  collectionRenameDraft.value = item.name;
}

async function saveCollectionRename(item) {
  if (renamingCollectionId.value !== item.id) return;
  const name = collectionRenameDraft.value.trim();
  if (!name) {
    cancelCollectionRename(item);
    flash(t("El nombre no puede estar vacío."), true);
    return;
  }
  renamingCollectionId.value = "";
  if (name === item.name) return;

  try {
    const updatedCollection = JSON.parse(JSON.stringify(item));
    updatedCollection.name = name;
    await backend().SaveCollection(project.value.id, updatedCollection);
    await refreshProjects();
    flash(t("Colección renombrada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteCollection() {
  if (!window.confirm(t("¿Eliminar «{name}» y todo su contenido?", { name: collection.value.name }))) return;
  try {
    await backend().DeleteCollection(project.value.id, collection.value.id);
    collectionId.value = "";
    folderId.value = "";
    requestId.value = "";
    await refreshProjects();
    flash(t("Colección eliminada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function createFolder(parentId = "") {
  if (!collection.value) {
    flash(t("Selecciona una colección antes de crear una carpeta."), true);
    return;
  }
  const name = window.prompt(t(parentId ? "Nombre de la subcarpeta" : "Nombre de la carpeta"));
  if (!name?.trim()) return;
  const folder = { id: crypto.randomUUID().replaceAll("-", ""), name: name.trim(), requests: [], folders: [] };
  collection.value.folders ||= [];
  let siblings = collection.value.folders;
  if (parentId) {
    const parent = findFolder(collection.value.folders, parentId);
    if (!parent) {
      flash(t("No se encontró la carpeta donde crear la subcarpeta."), true);
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
    flash(t("Carpeta creada."));
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

function renameFolderInTree(folders, id, name) {
  for (const folder of folders || []) {
    if (folder.id === id) {
      folder.name = name;
      return true;
    }
    if (renameFolderInTree(folder.folders || [], id, name)) return true;
  }
  return false;
}

async function renameFolder(collectionID, id, rawName) {
  const targetCollection = project.value?.collections.find((item) => item.id === collectionID);
  if (!targetCollection) {
    flash(t("No se encontró la carpeta que se iba a renombrar."), true);
    return;
  }
  const name = rawName.trim();
  if (!name) {
    flash(t("El nombre no puede estar vacío."), true);
    return;
  }
  const updatedCollection = JSON.parse(JSON.stringify(targetCollection));
  const folder = findFolder(updatedCollection.folders || [], id);
  if (!folder) {
    flash(t("No se encontró la carpeta que se iba a renombrar."), true);
    return;
  }
  if (name === folder.name) return;
  renameFolderInTree(updatedCollection.folders, id, name);
  try {
    await backend().SaveCollection(project.value.id, updatedCollection);
    await refreshProjects();
    flash(t("Carpeta renombrada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

function deleteFolder(id) {
  const target = findFolder(collection.value.folders || [], id);
  if (!window.confirm(t("¿Eliminar «{name}» y todo su contenido?", { name: target?.name || "" }))) return;
  updateFolder(collection.value.folders, id, (siblings, index) => siblings.splice(index, 1));
  if (folderId.value === id || !findFolder(collection.value.folders, folderId.value)) {
    folderId.value = "";
    requestId.value = "";
  }
  persistCollection().then(() => flash(t("Carpeta eliminada.")))
    .catch((error) => flash(error.message || String(error), true));
}

async function createRequest(parentId = "") {
  if (!collection.value) {
    flash(t("Selecciona una colección antes de crear una petición."), true);
    return;
  }
  const request = {
    id: crypto.randomUUID().replaceAll("-", ""),
    name: t("Nueva petición"), method: "GET", path: "/", headers: {}, body: ""
  };
  collection.value.requests ||= [];
  let siblings = collection.value.requests;
  if (parentId) {
    const parent = findFolder(collection.value.folders || [], parentId);
    if (!parent) {
      flash(t("No se encontró la carpeta donde crear la petición."), true);
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
    flash(t("Petición creada."));
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
    name: t("{name} (copia)", { name: request.name })
  };
  if (parentId) {
    const parent = findFolder(collection.value?.folders || [], parentId);
    if (!parent) {
      flash(t("No se encontró la carpeta donde duplicar la petición."), true);
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
    flash(t("Petición duplicada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteRequestByID(id, parentId = "") {
  const request = findRequest(collection.value, id);
  if (!request || !window.confirm(t("¿Eliminar la petición «{name}»?", { name: request.name }))) return;
  if (parentId) {
    const parent = findFolder(collection.value?.folders || [], parentId);
    if (!parent) {
      flash(t("No se encontró la carpeta de la petición."), true);
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
    flash(t("Petición eliminada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function saveRequest() {
  try {
    await persistCollection();
    flash(t("Petición guardada."));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function deleteRequest() {
  if (selectedRequest.value) await deleteRequestByID(requestId.value, folderId.value);
}

async function runRequest() {
  if (busy.value || !project.value || !selectedRequest.value) return;
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

function collectionResultStatus(item) {
  if (item.status === "pending") return t("Pendiente");
  if (item.status === "running") return t("Ejecutando…");
  if (item.status === "error") return "Error";
  return item.status === "different" ? t("Hay diferencias") : t("Coincide");
}

async function runCollection() {
  if (busy.value || !project.value || !collection.value) return;
  const runProjectId = project.value.id;
  const runCollectionId = collection.value.id;
  busy.value = true;
  collectionRunning.value = true;
  try {
    await persistCollection();
    const savedCollection = project.value?.collections.find((item) => item.id === runCollectionId);
    const requests = savedCollection ? collectCollectionRequests(savedCollection) : [];
    if (!requests.length) {
      flash(t("La colección no tiene peticiones para ejecutar."), true);
      return;
    }

    const run = {
      projectId: runProjectId,
      collectionId: runCollectionId,
      collectionName: savedCollection.name,
      completed: 0,
      total: requests.length,
      items: requests.map((item) => ({ ...item, status: "pending", result: null, error: "" }))
    };
    collectionRun.value = run;
    response.value = null;
    const currentRun = collectionRun.value;

    for (const item of currentRun.items) {
      item.status = "running";
      await nextTick();
      try {
        item.result = await backend().RunRequest(runProjectId, item.request);
        const responses = [item.result.primary, item.result.candidate];
        if (item.result.secondary_enabled) responses.push(item.result.secondary);
        if (responses.some((result) => result.error)) {
          item.status = "error";
        } else {
          item.status = item.result.candidate_differs ? "different" : "same";
        }
      } catch (error) {
        item.error = error.message || String(error);
        item.status = "error";
      } finally {
        currentRun.completed += 1;
      }
    }
  } catch (error) {
    flash(error.message || String(error), true);
  } finally {
    collectionRunning.value = false;
    busy.value = false;
  }
}

async function runCollectionFromMenu(item) {
  selectCollection(item);
  await nextTick();
  await runCollection();
}

function showCollectionRunResult(item) {
  const run = collectionRun.value;
  if (!run) return;
  if (projectId.value !== run.projectId) openProject(run.projectId);
  const runCollection = project.value?.collections.find((entry) => entry.id === run.collectionId);
  if (!runCollection || !findRequest(runCollection, item.request.id)) {
    flash(t("La petición de este resultado ya no está en el proyecto."), true);
    return;
  }
  selectCollection(runCollection);
  selectRequest(item.request.id, item.folderId);
  response.value = item.result;
}

function readSelectedFile(event) {
  return new Promise((resolve, reject) => {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return resolve("");
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result || ""));
    reader.onerror = () => reject(new Error(t("No se pudo leer el archivo seleccionado.")));
    reader.readAsText(file);
  });
}

async function importProject(event) {
  try {
    const raw = await readSelectedFile(event);
    if (!raw) return;
    let config;
    try {
      config = JSON.parse(raw);
    } catch {
      await completeProjectImport(raw);
      return;
    }
    const existing = config && typeof config === "object"
      ? projects.value.find((item) => item.id && item.id === config.id)
      : null;
    if (existing) {
      importConflict.value = {
        raw,
        existing,
        importedName: typeof config.name === "string" && config.name ? config.name : t("Proyecto sin nombre")
      };
      return;
    }
    await completeProjectImport(raw);
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function completeProjectImport(raw, conflictAction = "") {
  const imported = conflictAction
    ? await backend().ImportProjectConfigWithAction(raw, conflictAction)
    : await backend().ImportProjectConfig(raw);
  await refreshProjects();
  openProject(imported.id);
  if (conflictAction === "replace") {
    flash(t("Proyecto «{name}» reemplazado. Vuelve a configurar la autenticación; las credenciales no se incluyen en la importación.", { name: imported.name }));
  } else if (conflictAction === "copy") {
    flash(t("Proyecto importado como «{name}». Las credenciales no se incluyen.", { name: imported.name }));
  } else {
    flash(t("Proyecto «{name}» importado.", { name: imported.name }));
  }
}

async function resolveImportConflict(action) {
  const conflict = importConflict.value;
  importConflict.value = null;
  if (!conflict || action === "cancel") return;
  try {
    await completeProjectImport(conflict.raw, action);
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

async function exportProject(item) {
  try {
    const path = await backend().ExportProject(item.id);
    if (!path) return;
    flash(t("Proyecto «{name}» exportado a {path}. Incluye destinos, variables, colecciones y peticiones; las credenciales no se incluyen.", { name: item.name, path }));
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
    flash(t("Proyecto de ejemplo listo. Pulsa Enviar para ver las diferencias."));
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
      window.alert(t("Colección importada con estas limitaciones:\n\n• {warnings}", { warnings: imported.warnings.join("\n• ") }));
    }
    flash(t("Colección «{name}» importada.", { name: imported.collection.name }));
  } catch (error) {
    flash(error.message || String(error), true);
  }
}

function prettyBody(body) {
  try { return JSON.stringify(JSON.parse(body), null, 2); }
  catch { return body || t("(respuesta vacía)"); }
}

function statusClass(item) {
  if (item.error) return "status-error";
  if (item.status_code >= 200 && item.status_code < 300) return "status-good";
  if (item.status_code >= 400) return "status-error";
  return "status-warn";
}

onMounted(async () => {
  setTheme(isDark.value);
  document.documentElement.lang = locale.value;
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
      <div class="brand" @click="goHome" :title="t('Inicio')">
        <span class="brand-mark">T</span><span>DiffTwin</span>
      </div>
      <div class="top-divider"></div>
      <button v-if="project" class="nav-home" :title="t('Volver al dashboard')" :aria-label="t('Volver al dashboard')" @click="goHome">
        <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m2.8 9 7.2-6 7.2 6v8H12v-5H8v5H2.8V9Z"/></svg>
      </button>
      <div v-if="project" class="project-switch">
        <select v-model="projectId" :aria-label="t('Proyecto')" @change="openProject(projectId)">
          <option v-for="item in projects" :key="item.id" :value="item.id">{{ item.name }}</option>
        </select>
        <span class="chevron" aria-hidden="true"><svg viewBox="0 0 16 16"><path d="m4.5 6 3.5 3.5L11.5 6" /></svg></span>
      </div>
      <div v-else class="top-context">{{ t("Espacio de trabajo") }}</div>
      <div class="top-spacer"></div>
      <template v-if="project">
        <button class="icon-button topbar-action" :title="t('Configuración del proyecto')" :aria-label="t('Configuración del proyecto')" @click="editProject">
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
          <div class="eyebrow">{{ t("TU ESPACIO DE TRABAJO") }}</div>
          <h1>{{ t("Tus proyectos") }}</h1>
          <p>{{ t("Organiza tus APIs y compara cambios entre entornos.") }}</p>
          <p class="demo-caption">{{ t("El ejemplo usa un servicio público de eco y requiere conexión a internet.") }}</p>
        </div>
        <div class="dashboard-actions">
          <button class="button secondary" @click="createDemoProject">▷ {{ t("Probar ejemplo") }}</button>
          <button class="button secondary project-action-button" @click="projectFileInput.click()">
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 12V3m0 0L6.5 6.5M10 3l3.5 3.5M4 11v5h12v-5"/></svg>
            {{ t("Importar proyecto") }}
          </button>
          <button class="button primary" @click="newProject">＋ {{ t("Nuevo proyecto") }}</button>
          <input ref="projectFileInput" type="file" accept=".json,application/json" hidden @change="importProject">
        </div>
      </div>
      <div v-if="projects.length" class="project-grid">
        <article v-for="item in projects" :key="item.id" class="project-card">
          <button class="project-card-open" @click="openProject(item.id)">
            <div class="project-card-top">
              <span class="project-logo">{{ item.name.slice(0, 1).toUpperCase() }}</span>
              <span class="card-arrow">↗</span>
            </div>
            <strong>{{ item.name }}</strong>
            <span class="card-meta">{{ projectSummary(item) }}</span>
            <div class="target-summary">
              <span><i class="target-dot stable"></i>{{ item.primary }}</span>
              <span><i class="target-dot candidate"></i>{{ item.candidate }}</span>
              <span v-if="item.secondary"><i class="target-dot stable"></i>{{ item.secondary }}</span>
            </div>
          </button>
          <div class="project-card-actions">
            <button class="button quiet project-action-button" :aria-label="`${t('Exportar proyecto')} ${item.name}`" @click="exportProject(item)">
              <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 8v9m0 0 3.5-3.5M10 17l-3.5-3.5M4 9V4h12v5"/></svg>
              {{ t("Exportar proyecto") }}
            </button>
            <button class="button quiet project-card-delete project-action-button" :aria-label="`${t('Eliminar')} ${t('proyecto')} ${item.name}`" @click="deleteProject(item)">
              <svg class="project-delete-icon" viewBox="0 0 20 20" aria-hidden="true">
                <path d="M5 6.25h10l-.7 9.1a1 1 0 0 1-1 .9H6.7a1 1 0 0 1-1-.9L5 6.25Z" />
                <path d="M3.75 4.75h12.5M8 4.75v-1a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1m-4 3.5v5m3-5v5" />
              </svg>
              {{ t("Eliminar") }}
            </button>
          </div>
        </article>
      </div>
      <div v-else class="empty-state">
        <span class="empty-icon">◇</span>
        <h2>{{ t("Empieza creando un proyecto") }}</h2>
        <p>{{ t("Configura tus destinos y guarda tus primeras peticiones de API.") }}</p>
        <button class="button primary" @click="newProject">＋ {{ t("Crear proyecto") }}</button>
      </div>
    </main>

    <div v-else class="workspace" :class="{ 'sidebar-resizing': sidebarResizing }" :style="{ '--sidebar-width': `${sidebarWidth}px` }">
      <aside class="sidebar">
        <div class="sidebar-heading">
          <div v-if="!collectionSearchOpen" class="sidebar-heading-title">
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m10 2.8 6.7 3.6v7.2L10 17.2l-6.7-3.6V6.4L10 2.8Z"/><path d="m3.3 6.4 6.7 3.7 6.7-3.7M10 10.1v7.1"/></svg>
            <span>{{ t("Colecciones") }}</span>
          </div>
          <input
            v-else
            ref="collectionSearchInput"
            v-model="collectionSearch"
            class="collection-search"
            :aria-label="t('Buscar colecciones y peticiones')"
            :placeholder="t('Buscar…')"
            @keydown.esc="toggleCollectionSearch"
          >
          <div class="sidebar-heading-actions">
            <button class="icon-button" :title="t(collectionSearchOpen ? 'Cerrar búsqueda' : 'Buscar')" :aria-label="t(collectionSearchOpen ? 'Cerrar búsqueda' : 'Buscar')" @click="toggleCollectionSearch">
              <svg v-if="!collectionSearchOpen" viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.7" cy="8.7" r="5.6"/><path d="m12.8 12.8 4 4"/></svg>
              <svg v-else viewBox="0 0 20 20" aria-hidden="true"><path d="m5 5 10 10M15 5 5 15"/></svg>
            </button>
            <button class="icon-button" :title="t('Nueva colección')" :aria-label="t('Nueva colección')" @click="createCollection">＋</button>
            <button class="icon-button" :title="t('Más opciones')" :aria-label="t('Más opciones')" :aria-expanded="activeMenu === menuKey('import')" @click.stop="toggleMenu('import')">
              <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="4" cy="10" r="1"/><circle cx="10" cy="10" r="1"/><circle cx="16" cy="10" r="1"/></svg>
            </button>
            <div v-if="activeMenu === menuKey('import')" class="collection-menu sidebar-options-menu" @click.stop>
              <button @click="closeMenus(); postmanFileInput.click()">{{ t("Importar colección de Postman") }}</button>
            </div>
          </div>
          <input ref="postmanFileInput" type="file" accept=".json,application/json" hidden @change="importPostman">
        </div>
        <div class="collection-tree">
          <div v-for="item in visibleCollections" :key="item.id" class="collection-group">
            <div
              class="tree-row collection-row"
              :class="{ selected: collectionId === item.id && !folderId && !requestId, 'menu-open': activeMenu === menuKey('collection', item.id), 'drop-target': treeDropTarget === treeDropTargetKey('collection', item.id) }"
              :title="t('Suelta aquí para mover a la colección')"
              @dragover="treeDragOver($event, item.id)"
              @drop.prevent.stop="dropTreeItem(item.id)"
            >
              <button
                class="tree-toggle"
                :aria-label="t(expandedCollections.has(item.id) ? 'Contraer colección' : 'Expandir colección')"
                :aria-expanded="expandedCollections.has(item.id)"
                @click="toggleCollection(item)"
              >
                <svg class="tree-chevron" :class="{ expanded: expandedCollections.has(item.id) }" viewBox="0 0 16 16" aria-hidden="true">
                  <path d="m5.5 6.5 2.5 2.5 2.5-2.5" />
                </svg>
              </button>
              <div v-if="renamingCollectionId === item.id" class="tree-label tree-label-editing">
                <span class="collection-icon">▤</span>
                <input
                  ref="collectionRenameInput"
                  v-model="collectionRenameDraft"
                  class="tree-rename-input"
                  :aria-label="t('Nombre de la colección')"
                  :title="t('Pulsa Enter para guardar o Escape para cancelar.')"
                  @click.stop
                  @keydown.enter.prevent="saveCollectionRename(item)"
                  @keydown.esc.prevent="cancelCollectionRename(item)"
                  @blur="saveCollectionRename(item)"
                >
              </div>
              <button v-else class="tree-label" @click="selectCollection(item)">
                <span class="collection-icon">▤</span><span class="tree-name">{{ item.name }}</span>
              </button>
              <div class="tree-actions">
                <button
                  class="icon-button tiny"
                  :title="t('Opciones de colección')"
                  :aria-expanded="activeMenu === menuKey('collection', item.id)"
                  @click.stop="toggleMenu('collection', item.id)"
                >···</button>
              </div>
              <div v-if="activeMenu === menuKey('collection', item.id)" class="collection-menu" @click.stop>
                <button @click="selectCollection(item); createRequest()">＋ {{ t("Nueva petición") }}</button>
                <button @click="selectCollection(item); createFolder()">＋ {{ t("Nueva carpeta") }}</button>
                <div class="collection-menu-separator"></div>
                <button
                  :disabled="busy || countRequests(item.requests || [], item.folders || []) === 0"
                  @click="closeMenus(); runCollectionFromMenu(item)"
                >▶ {{ t("Ejecutar colección ({count})", { count: countRequests(item.requests || [], item.folders || []) }) }}</button>
                <div class="collection-menu-separator"></div>
                <button @click="selectCollection(item); startCollectionRename(item)">{{ t("Renombrar colección") }}</button>
                <button class="collection-menu-danger" @click="selectCollection(item); deleteCollection()">{{ t("Eliminar colección") }}</button>
              </div>
            </div>
            <div v-if="expandedCollections.has(item.id) || collectionSearch.trim()" class="tree-children">
              <div v-for="request in (item.requests || []).filter((entry) => requestMatches(entry, collectionItemSearch(item).toLocaleLowerCase()))" :key="request.id" class="request-branch">
                <div
                  class="tree-row request-row"
                  :class="{ selected: collectionId === item.id && requestId === request.id, 'menu-open': activeMenu === menuKey('request', request.id) }"
                  role="button"
                  tabindex="0"
                  :title="t('Arrastra la petición a una carpeta o colección para moverla')"
                  draggable="true"
                  @click="selectRequest(request.id)"
                  @keydown.enter="selectRequest(request.id)"
                  @dragstart.stop="startTreeDrag($event, { type: 'request', id: request.id, collectionId: item.id, parentId: '' })"
                  @dragend="endTreeDrag"
                >
                  <span class="tree-spacer"></span>
                  <span class="method-badge" :class="`method-${request.method.toLowerCase()}`">{{ request.method }}</span>
                  <span class="tree-name">{{ request.name }}</span>
                  <span class="tree-actions">
                    <button class="icon-button tiny" :title="t('Opciones de petición')" :aria-expanded="activeMenu === menuKey('request', request.id)" @click.stop="toggleMenu('request', request.id)">···</button>
                  </span>
                </div>
                <div v-if="activeMenu === menuKey('request', request.id)" class="collection-menu request-menu" @click.stop>
                  <button @click="closeMenus(); duplicateRequest(request)">{{ t("Duplicar petición") }}</button>
                  <button class="collection-menu-danger" @click="closeMenus(); deleteRequestByID(request.id)">{{ t("Eliminar petición") }}</button>
                </div>
              </div>
              <FolderTree
                v-for="folder in item.folders || []"
                :key="folder.id"
                :folder="folder"
                :locale="locale"
                :collection-id="item.id"
                :selected-collection-id="collectionId"
                :selected-folder-id="folderId"
                :selected-request-id="requestId"
                :active-menu="activeMenu"
                :search-term="collectionItemSearch(item)"
                :dragged-item="draggedTreeItem"
                :drop-target="treeDropTarget"
                @select-folder="selectFolder"
                @select-request="selectRequest"
                @toggle-menu="toggleMenu"
                @close-menus="closeMenus"
                @duplicate-request="duplicateRequest"
                @delete-request="deleteRequestByID"
                @create-folder="createFolder"
                @create-request="createRequest"
                @rename-folder="(id, name) => renameFolder(item.id, id, name)"
                @delete-folder="deleteFolder"
                @drag-start="startTreeDrag"
                @drag-end="endTreeDrag"
                @drag-over="(event, targetCollectionID, targetFolderID) => treeDragOver(event, targetCollectionID, targetFolderID)"
                @drop-on-folder="(event, targetCollectionID, targetFolderID) => dropTreeItem(targetCollectionID, targetFolderID)"
              />
            </div>
          </div>
          <div v-if="!visibleCollections.length" class="sidebar-empty">
            <p v-if="collectionSearch.trim()">{{ t("No se encontraron coincidencias.") }}</p>
            <template v-else>
            <p>{{ t("Aún no hay colecciones.") }}</p>
            <button class="button secondary small" @click="createCollection">{{ t("Crear colección") }}</button>
            </template>
          </div>
        </div>
        <div class="sidebar-bottom">
          <span>{{ t(requestCount === 1 ? "{count} petición" : "{count} peticiones", { count: requestCount }) }}</span>
          <span>DiffTwin</span>
        </div>
      </aside>

      <div
        class="sidebar-resizer"
        role="separator"
        :aria-label="t('Cambiar ancho del panel lateral')"
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
        <section v-if="visibleCollectionRun" class="collection-run-panel" aria-live="polite">
          <header class="collection-run-heading">
            <strong>{{ t("RESULTADOS · {name}", { name: visibleCollectionRun.collectionName }) }}</strong>
            <span>{{ t("{completed}/{total} completadas", { completed: visibleCollectionRun.completed, total: visibleCollectionRun.total }) }}</span>
          </header>
          <div class="collection-run-list">
            <button
              v-for="(item, index) in visibleCollectionRun.items"
              :key="`${item.request.id}-${index}`"
              class="collection-run-item"
              :class="`collection-run-${item.status}`"
              :disabled="item.status === 'pending' || item.status === 'running'"
              @click="showCollectionRunResult(item)"
            >
              <span class="collection-run-name">{{ item.request.name }}</span>
              <span class="collection-run-status">{{ collectionResultStatus(item) }}</span>
              <span v-if="item.error" class="collection-run-message-error" :title="item.error">{{ item.error }}</span>
            </button>
          </div>
        </section>
        <div class="target-strip" :class="{ 'target-strip-two': !project.secondary }">
          <button type="button" class="target-chip target-chip-button" :aria-label="t('Editar estable principal')" @click="editProject('primary')">
            <i class="target-dot stable"></i><span class="target-chip-content"><span>{{ t("ESTABLE PRINCIPAL") }}</span><strong>{{ project.primary }}</strong></span>
          </button>
          <button type="button" class="target-chip target-chip-button" :aria-label="t('Editar candidata')" @click="editProject('candidate')">
            <i class="target-dot candidate"></i><span class="target-chip-content"><span>{{ t("CANDIDATA") }}</span><strong>{{ project.candidate }}</strong></span>
          </button>
          <button v-if="project.secondary" type="button" class="target-chip target-chip-button" :aria-label="t('Editar estable secundaria')" @click="editProject('secondary')">
            <i class="target-dot stable"></i><span class="target-chip-content"><span>{{ t("ESTABLE SECUNDARIA") }}</span><strong>{{ project.secondary }}</strong></span>
          </button>
        </div>
        <details class="workspace-auth">
          <summary class="workspace-auth-summary">
            <span><strong>{{ t("Configuración de destinos") }}</strong><small>{{ t("Autenticación, variables y opciones de comparación") }}</small></span>
            <span class="settings-toggle">{{ t("Configurar") }} <span aria-hidden="true">⌄</span></span>
          </summary>
          <div class="workspace-auth-content">
          <div class="workspace-auth-heading">
            <div><strong>{{ t("Autenticación y opciones") }}</strong><span>{{ t("Configura credenciales y opciones por proyecto.") }}</span></div>
            <button class="button primary small" :disabled="authSaving || authLoadedProjectId !== project.id" @click="saveProjectAuth">
              {{ authSaving ? t("Guardando…") : t("Guardar opciones") }}
            </button>
          </div>
          <div class="auth-target-tabs">
            <button type="button" class="button small" :class="authTarget === 'primary' ? 'primary' : 'secondary'" @click="setAuthTarget('primary')">{{ t("Estable principal") }}</button>
            <button type="button" class="button small" :class="authTarget === 'candidate' ? 'primary' : 'secondary'" @click="setAuthTarget('candidate')">{{ t("Candidata") }}</button>
            <button v-if="project.secondary" type="button" class="button small" :class="authTarget === 'secondary' ? 'primary' : 'secondary'" @click="setAuthTarget('secondary')">{{ t("Estable secundaria") }}</button>
          </div>
          <p v-if="authLoadingProjectId === project.id" class="auth-storage-note">{{ t("Cargando autenticación…") }}</p>
          <div class="workspace-auth-fields">
            <label class="field-label">{{ t("MÉTODO") }}
              <select :value="authForm.auth[authTarget].type || 'none'" @change="setAuthType($event.target.value)">
                <option value="none">{{ t("Sin autenticación") }}</option>
                <option value="bearer">Bearer</option>
                <option value="basic">Basic</option>
                <option value="api_key">API key</option>
              </select>
            </label>
            <template v-if="authForm.auth[authTarget].type === 'bearer'">
              <label class="field-label">{{ t("TOKEN") }}<input v-model="authForm.credentials[authTarget].token" type="password" autocomplete="new-password" required></label>
            </template>
            <template v-else-if="authForm.auth[authTarget].type === 'basic'">
              <label class="field-label">{{ t("USUARIO") }}<input v-model="authForm.credentials[authTarget].username" autocomplete="off"></label>
              <label class="field-label">{{ t("CONTRASEÑA") }}<input v-model="authForm.credentials[authTarget].password" type="password" autocomplete="new-password" required></label>
            </template>
            <template v-else-if="authForm.auth[authTarget].type === 'api_key'">
              <label class="field-label">{{ t("NOMBRE DE LA CLAVE") }}<input v-model="authForm.auth[authTarget].api_key_name" required placeholder="X-API-Key"></label>
              <label class="field-label">{{ t("ENVIAR EN") }}
                <select v-model="authForm.auth[authTarget].api_key_location">
                  <option value="header">Header</option>
                  <option value="query">{{ t("Parámetro de consulta") }}</option>
                </select>
              </label>
              <label class="field-label">{{ t("VALOR") }}<input v-model="authForm.credentials[authTarget].api_key" type="password" autocomplete="new-password" required></label>
            </template>
          </div>
          <label class="field-label destination-variables-field">{{ t("VARIABLES ESPECÍFICAS · JSON") }}
            <textarea
              v-model="destinationVariableDrafts[authTarget]"
              spellcheck="false"
              placeholder='{ "region": "eu-west", "userId": "123" }'
            ></textarea>
          </label>
          <p class="auth-storage-note">{{ t("Estas variables sobrescriben las comunes solo para este destino; las demás conservan el valor común. Se guardan en el JSON del proyecto sin cifrar: no pongas secretos aquí.") }}</p>
          <p v-if="authForm.auth[authTarget].type === 'api_key' && authForm.auth[authTarget].api_key_location === 'query'" class="auth-security-warning">
            {{ t("Las claves en la URL pueden quedar en registros del servidor o de proxies. Usa Header si la API lo admite.") }}
          </p>
          <p v-if="authCredentialsMissing" class="auth-security-warning">{{ t("Faltan credenciales guardadas en el sistema. Vuelve a introducirlas para este proyecto.") }}</p>
          <p v-if="authLoadedProjectId !== project.id && authLoadingProjectId !== project.id" class="auth-security-warning">{{ t("No se pudieron cargar las credenciales seguras. Comprueba que el almacén del sistema esté disponible.") }}</p>
          <div class="comparison-settings">
            <label class="field-label">{{ t("IGNORAR RUTAS JSON · UNA POR LÍNEA") }}
              <textarea v-model="ignoredJSONPathsText" spellcheck="false" placeholder="$.meta.generatedAt&#10;$.items[*].id"></textarea>
            </label>
            <p>{{ t("Admite rutas como $.meta.timestamp, $[\"campo.con.puntos\"] y comodines *. Guardar autenticación también guarda estas opciones.") }}</p>
          </div>
          <p class="auth-storage-note">{{ t("Las credenciales se guardan en el almacén seguro del sistema y no se incluyen en la configuración JSON del proyecto.") }}</p>
          </div>
        </details>

        <section v-if="selectedRequest" class="request-editor">
          <div class="request-sticky">
            <div class="request-titlebar">
              <div><span class="eyebrow">{{ t("PETICIÓN") }}</span><input v-model="selectedRequest.name" class="request-title-input" :aria-label="t('Nombre de petición')"></div>
              <button class="icon-button danger-icon" :title="t('Eliminar petición')" :aria-label="t('Eliminar petición')" @click="deleteRequest">
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
                <span v-if="busy" class="spinner"></span><span v-else>▶</span>{{ busy ? t("Ejecutando…") : t("Enviar") }}
              </button>
            </div>
          </div>
          <div class="editor-tabs">
            <button class="editor-tab" :class="{ active: activeEditorTab === 'params' }" @click="activeEditorTab = 'params'">Params <span class="tab-count">{{ queryParamCount }}</span></button>
            <button class="editor-tab" :class="{ active: activeEditorTab === 'headers' }" @click="activeEditorTab = 'headers'">Headers <span class="tab-count">{{ Object.keys(selectedRequest.headers || {}).length }}</span></button>
            <button class="editor-tab" :class="{ active: activeEditorTab === 'body' }" @click="activeEditorTab = 'body'">Body</button>
            <span class="tab-spacer"></span>
            <button class="button secondary small" @click="saveRequest">{{ t("Guardar") }}</button>
          </div>
          <div class="editor-grid single">
            <div v-if="activeEditorTab === 'params'" class="params-editor">
              <div class="params-heading"><span>{{ t("PARÁMETROS DE CONSULTA") }}</span><button class="button secondary small" @click="addQueryParam">{{ t("＋ Añadir parámetro") }}</button></div>
              <div v-if="queryParams.length" class="params-table">
                <div class="param-row param-column-head"><span>{{ t("CLAVE") }}</span><span>{{ t("VALOR") }}</span><span></span></div>
                <div v-for="(param, index) in queryParams" :key="index" class="param-row">
                  <input v-model="param.key" :aria-label="t('Clave del parámetro')" placeholder="page" @change="updateQueryPath">
                  <input v-model="param.value" :aria-label="t('Valor del parámetro')" placeholder="1" @change="updateQueryPath">
                  <button class="icon-button danger-icon" :aria-label="t('Eliminar parámetro {name}', { name: param.key || index + 1 })" :title="t('Eliminar parámetro')" @click="removeQueryParam(index)">×</button>
                </div>
              </div>
              <p v-else class="params-empty">{{ t("No hay parámetros. Añade uno o escribe una query en la URL.") }}</p>
            </div>
            <label v-if="activeEditorTab === 'headers'" class="field-label">{{ t("HEADERS · JSON") }}
              <textarea
                :value="JSON.stringify(selectedRequest.headers || {}, null, 2)"
                spellcheck="false"
                @change="(event) => { try { selectedRequest.headers = JSON.parse(event.target.value); } catch { flash(t('Headers debe ser JSON válido.'), true); } }"
              ></textarea>
            </label>
            <label v-else-if="activeEditorTab === 'body'" class="field-label">{{ t("BODY") }}
              <textarea v-model="selectedRequest.body" spellcheck="false" placeholder='{ "example": true }'></textarea>
            </label>
          </div>
          <div v-if="response" class="response-section">
            <div class="response-heading"><span>{{ t("RESULTADO DE LA COMPARACIÓN") }}</span><span>{{ t("Candidata frente a estable principal") }}</span></div>
            <div class="verdict" :class="comparisonSummary.tone">
              <span class="verdict-icon">{{ comparisonSummary.icon }}</span>
              <strong>{{ comparisonSummary.title }}</strong>
              <span>{{ comparisonSummary.detail }}</span>
              <span v-if="response.secondary_enabled && response.noise_detected" class="noise-badge">{{ t("Posible ruido entre estables") }}</span>
            </div>
            <div class="latency-comparison" :aria-label="t('Comparación de tiempos de respuesta')">
              <span v-for="item in latencySummaries" :key="item.name">
                <strong>{{ item.name }}:</strong> {{ item.duration }} ms
                <small>({{ t("{delta} ms frente a estable principal", { delta: `${item.delta > 0 ? "+" : ""}${item.delta}` }) }})</small>
              </span>
              <small>{{ t("La latencia es informativa y no cambia el veredicto.") }}</small>
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
                <span>{{ t(panel.diffs.length === 1 ? "{count} cambio" : "{count} cambios", { count: Math.min(panel.diffs.length, 100) + (panel.diffs.length > 100 ? "+" : "") }) }}</span>
              </header>
              <div class="diff-table">
                <div class="diff-row diff-column-head">
                  <span>{{ t("Ruta") }}</span><span>{{ panel.leftLabel }}</span><span>{{ panel.rightLabel }}</span>
                </div>
                <div v-for="(diff, index) in panel.diffs.slice(0, 100)" :key="`${diff.path}-${index}`" class="diff-row" :class="`diff-${diff.kind}`">
                  <code class="diff-path" :title="diff.path">{{ diff.path }}</code>
                  <code class="diff-value" :title="String(diff.before).slice(0, 1000)">{{ diff.before }}</code>
                  <code class="diff-value" :title="String(diff.after).slice(0, 1000)">{{ diff.after }}</code>
                </div>
              </div>
              <p v-if="panel.diffs.length > 100" class="diff-truncated">{{ t("Se muestran los primeros 100 cambios.") }}</p>
            </section>
            <div class="response-grid" :class="{ 'response-grid-two': !response.secondary_enabled }">
              <article v-for="[name, result] in responseCards" :key="name" class="response-card">
                <header><strong>{{ name }}</strong><span class="response-meta" :class="statusClass(result)">{{ result.error ? t("Error") : `HTTP ${result.status_code}` }} · {{ result.duration_ms }} ms</span></header>
                <pre>{{ result.error || prettyBody(result.body) }}{{ result.truncated ? `\n${t("[Respuesta truncada a 2 MiB]")}` : "" }}</pre>
              </article>
            </div>
          </div>
        </section>

        <div v-else-if="collection" class="content-empty">
          <div class="empty-icon">⌘</div>
          <h2>{{ selectedFolder ? selectedFolder.name : collection.name }}</h2>
          <p>{{ t("Selecciona una petición o crea una nueva para empezar.") }}</p>
          <div class="empty-actions">
            <button class="button primary" @click="createRequest(folderId)">＋ {{ t("Nueva petición") }}</button>
            <button class="button secondary" @click="createFolder(folderId)">▰ {{ t("Nueva carpeta") }}</button>
          </div>
        </div>
        <div v-else class="content-empty">
          <div class="empty-icon">⌘</div>
          <h2>{{ t("Tu espacio de trabajo") }}</h2>
          <p>{{ t("Selecciona una colección o crea una nueva.") }}</p>
          <button class="button primary" @click="createCollection">＋ {{ t("Nueva colección") }}</button>
        </div>

        <footer class="statusbar">
          <span><i class="online-dot"></i>{{ t("Listo") }}</span><span>{{ projectLabel }}</span>
          <div class="footer-preferences">
            <label class="language-control">
              <span class="visually-hidden">{{ t("Idioma") }}</span>
              <select :value="locale" :aria-label="t('Cambiar idioma')" @change="setLocale($event.target.value)">
                <option value="es">Español</option>
                <option value="en">English</option>
              </select>
            </label>
          <button class="icon-button theme-footer" :title="t(isDark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro')" @click="toggleTheme">{{ isDark ? "☼" : "☾" }}</button>
          </div>
        </footer>
      </section>
    </div>

    <footer v-if="!project" class="dashboard-footer">
      <span><span class="brand-mark mini">T</span> DiffTwin <span class="footer-muted">{{ t("· Comparación de APIs") }}</span></span>
      <div class="footer-preferences">
        <label class="language-control">
          <span class="visually-hidden">{{ t("Idioma") }}</span>
          <select :value="locale" :aria-label="t('Cambiar idioma')" @change="setLocale($event.target.value)">
            <option value="es">Español</option>
            <option value="en">English</option>
          </select>
        </label>
        <button class="icon-button theme-footer" :title="t(isDark ? 'Cambiar a tema claro' : 'Cambiar a tema oscuro')" @click="toggleTheme">
          {{ isDark ? "☼" : "☾" }}
        </button>
      </div>
    </footer>

    <div v-if="importConflict" class="modal-backdrop" @click.self="resolveImportConflict('cancel')">
      <section class="modal-card import-conflict-modal" role="dialog" aria-modal="true" aria-labelledby="import-conflict-title">
        <div class="modal-heading">
          <div><div class="eyebrow">{{ t("CONFLICTO DE IMPORTACIÓN") }}</div><h2 id="import-conflict-title">{{ t("El proyecto ya existe") }}</h2></div>
          <button type="button" class="icon-button" :aria-label="t('Cancelar importación')" @click="resolveImportConflict('cancel')">×</button>
        </div>
        <p>{{ t("El archivo «{imported}» tiene el mismo identificador que «{existing}». Si lo reemplazas, sus colecciones y peticiones se sustituirán por las del archivo.", { imported: importConflict.importedName, existing: importConflict.existing.name }) }}</p>
        <div class="import-conflict-warning">
          <strong>{{ t("Las credenciales no se incluyen en los archivos exportados.") }}</strong>
          <span>{{ t("Si reemplazas el proyecto, tendrás que volver a configurar su autenticación.") }}</span>
        </div>
        <div class="import-conflict-actions">
          <button type="button" class="button secondary" @click="resolveImportConflict('cancel')">{{ t("Cancelar") }}</button>
          <button type="button" class="button secondary" @click="resolveImportConflict('copy')">{{ t("Importar como copia") }}</button>
          <button type="button" class="button project-import-replace" @click="resolveImportConflict('replace')">{{ t("Reemplazar proyecto") }}</button>
        </div>
      </section>
    </div>

    <div v-if="projectDialog" class="modal-backdrop" @click.self="projectDialog = false">
      <form class="modal-card" @submit.prevent="saveProject">
        <div class="modal-heading"><div><div class="eyebrow">{{ t("CONFIGURACIÓN") }}</div><h2>{{ t(editingProject ? "Editar proyecto" : "Nuevo proyecto") }}</h2></div><button type="button" class="icon-button" :aria-label="t('Cerrar')" @click="projectDialog = false">×</button></div>
        <label class="field-label">{{ t("NOMBRE") }}<input v-model="projectForm.name" required :placeholder="t('Mi API')"></label>
        <label class="field-label">{{ t("ESTABLE PRINCIPAL") }}<input ref="primaryProjectInput" v-model="projectForm.primary" required placeholder="https://api-stable.example"></label>
        <label class="field-label">{{ t("CANDIDATA") }}<input ref="candidateProjectInput" v-model="projectForm.candidate" required placeholder="https://api-candidate.example"></label>
        <label class="field-label">{{ t("ESTABLE SECUNDARIA · OPCIONAL") }}<input ref="secondaryProjectInput" v-model="projectForm.secondary" placeholder="https://api-stable-2.example"></label>
        <label class="field-label">{{ t("IGNORAR RUTAS JSON · UNA POR LÍNEA") }}<textarea v-model="ignoredJSONPathsText" spellcheck="false" placeholder="$.meta.generatedAt&#10;$.items[*].id"></textarea></label>
        <label class="field-label">{{ t("VARIABLES · JSON") }}<textarea v-model="projectForm.variables" placeholder="{}"></textarea></label>
        <div v-if="editingProject" class="danger-zone">
          <div><strong>{{ t("Eliminar proyecto") }}</strong><p>{{ t("Elimina el proyecto y todas sus colecciones y peticiones.") }}</p></div>
          <button type="button" class="button danger-outline" @click="deleteProject">{{ t("Eliminar") }}</button>
        </div>
        <div class="modal-actions"><button type="button" class="button secondary" @click="projectDialog = false">{{ t("Cancelar") }}</button><button class="button primary">{{ t(editingProject ? "Guardar cambios" : "Crear proyecto") }}</button></div>
      </form>
    </div>
  </div>
</template>
