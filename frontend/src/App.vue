<script setup>
import { computed, nextTick, onMounted, ref } from "vue";
import * as AppBindings from "../wailsjs/go/main/App";
import CollectionSidebar from "./CollectionSidebar.vue";
import DestinationSettings from "./DestinationSettings.vue";
import RequestEditor from "./RequestEditor.vue";
import { detectLocale, translate } from "./i18n";
import { useProjectAuthentication } from "./useProjectAuthentication";
import { BrowserOpenURL } from "../wailsjs/runtime/runtime.js";

const backend = () => AppBindings;
const projects = ref([]);
const sidebarWidth = ref(268);
const sidebarResizing = ref(false);
const projectId = ref("");
const collectionId = ref("");
const folderId = ref("");
const requestId = ref("");
const requestEditorResetKey = ref(0);
const response = ref(null);
const busy = ref(false);
const collectionRunning = ref(false);
const collectionRun = ref(null);
const notice = ref("");
const errorMessage = ref("");
const updateInfo = ref(null);
const updateCheckError = ref("");
const updateCheckLoading = ref(false);
const updateNoticeDismissed = ref(false);
const updateDialogVisible = ref(false);
const appDialog = ref(null);
const appDialogInput = ref(null);
const isDark = ref(getPreference("theme") !== "light");
const storedLocale = getPreference("language");
const locale = ref(storedLocale === "en" || storedLocale === "es" ? storedLocale : detectLocale());
const t = (source, values) => translate(locale.value, source, values);
const projectDialog = ref(false);
const importConflict = ref(null);
const editingProject = ref(null);
const projectForm = ref(emptyProject());
const ignoredJSONPathsText = ref("");
const primaryProjectInput = ref(null);
const candidateProjectInput = ref(null);
const secondaryProjectInput = ref(null);
const projectFileInput = ref(null);
const expandedCollections = ref(new Set());
const activeMenu = ref("");
const draggedTreeItem = ref(null);
const treeDropTarget = ref("");
const movingTreeItem = ref(false);
let appDialogResolve = null;

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
const {
  authTarget,
  authForm,
  authSaving,
  authLoadedProjectId,
  authLoadingProjectId,
  authCredentialsMissing,
  destinationVariableDrafts,
  dirtyTargets: dirtyAuthTargets,
  hasUnsavedComparisonOptions,
  resetDestinationVariableDrafts,
  loadProjectAuth,
  saveProjectAuth,
  setAuthTarget,
  setAuthType,
  updateAuthSetting,
  updateCredential
} = useProjectAuthentication({
  project,
  projectId,
  ignoredPathsText: ignoredJSONPathsText,
  backend,
  refreshProjects,
  t,
  flash: (message, isError) => flash(message, isError),
  onSaved: () => {
    response.value = null;
    flash(t("Opciones guardadas."));
  }
});
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
const requestCount = computed(() => (project.value?.collections || []).reduce((sum, item) =>
  sum + countRequests(item.requests || [], item.folders || []), 0));
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

function showAppDialog(dialog) {
  return new Promise((resolve) => {
    appDialogResolve = resolve;
    appDialog.value = dialog;
    if (dialog.type === "prompt") {
      nextTick(() => appDialogInput.value?.focus());
    }
  });
}

function closeAppDialog(result = null) {
  if (!appDialog.value) return;
  const resolve = appDialogResolve;
  appDialogResolve = null;
  appDialog.value = null;
  resolve?.(result);
}

function confirmAction(message) {
  return showAppDialog({ type: "confirm", title: t("Confirmar acción"), message });
}

function promptForName(title) {
  return showAppDialog({ type: "prompt", title, value: "" });
}

function showImportWarnings(message) {
  return showAppDialog({ type: "message", title: t("Limitaciones de importación"), message });
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

async function checkForUpdates() {
  updateCheckLoading.value = true;
  updateCheckError.value = "";
  try {
    updateInfo.value = await backend().CheckForUpdates();
    updateDialogVisible.value = !!updateInfo.value?.available;
  } catch (error) {
    updateCheckError.value = error.message || String(error);
  } finally {
    updateCheckLoading.value = false;
  }
}

function openLatestRelease() {
  if (updateInfo.value?.release_url) BrowserOpenURL(updateInfo.value.release_url);
}

function downloadUpdate() {
  openLatestRelease();
  dismissUpdateDialog();
}

function dismissUpdateDialog() {
  updateDialogVisible.value = false;
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
  authTarget.value = "primary";
  closeMenus();
  loadProjectAuth(id);
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

function selectFolder(targetCollectionID, id) {
  collectionId.value = targetCollectionID;
  folderId.value = id;
  requestId.value = "";
  response.value = null;
  expandedCollections.value.add(targetCollectionID);
}

function selectRequest(targetCollectionID, id, parentFolderId = "") {
  collectionId.value = targetCollectionID;
  requestId.value = id;
  folderId.value = parentFolderId;
  response.value = null;
  expandedCollections.value.add(targetCollectionID);
  closeMenus();
  requestEditorResetKey.value += 1;
}

function newProject() {
  editingProject.value = false;
  projectForm.value = emptyProject();
  ignoredJSONPathsText.value = "";
  resetDestinationVariableDrafts();
  projectDialog.value = true;
}

async function editProject(target = "") {
  editingProject.value = true;
  projectForm.value = JSON.parse(JSON.stringify(project.value));
  projectForm.value.variables = JSON.stringify(project.value.variables || {}, null, 2);
  ignoredJSONPathsText.value = (project.value.ignored_json_paths || []).join("\n");
  resetDestinationVariableDrafts(project.value.destination_variables);
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

async function deleteProject(item = project.value) {
  if (!item || !(await confirmAction(t("¿Eliminar el proyecto «{name}»?\n\nSe eliminarán también sus colecciones, peticiones y credenciales guardadas. Esta acción no se puede deshacer.", { name: item.name })))) return;
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
  const name = await promptForName(t("Nombre de la colección"));
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

async function saveCollectionRename(item, name) {
  if (!name) {
    flash(t("El nombre no puede estar vacío."), true);
    return;
  }
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
  if (!(await confirmAction(t("¿Eliminar «{name}» y todo su contenido?", { name: collection.value.name })))) return;
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
  const name = await promptForName(t(parentId ? "Nombre de la subcarpeta" : "Nombre de la carpeta"));
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

async function deleteFolder(id) {
  const target = findFolder(collection.value.folders || [], id);
  if (!target || !(await confirmAction(t("¿Eliminar «{name}» y todo su contenido?", { name: target.name })))) return;
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
  if (!request || !(await confirmAction(t("¿Eliminar la petición «{name}»?", { name: request.name })))) return;
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
  selectRequest(run.collectionId, item.request.id, item.folderId);
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
      if (sampleRequest) selectRequest(sampleCollection.id, sampleRequest.id);
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
      await showImportWarnings(t("Colección importada con estas limitaciones:\n\n• {warnings}", { warnings: imported.warnings.join("\n• ") }));
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
  void checkForUpdates();
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
      <div v-if="updateInfo?.available && !updateNoticeDismissed" class="update-banner">
        <span class="update-banner-icon" aria-hidden="true">↗</span>
        <div>
          <strong>{{ t("Nueva versión disponible: {version}", { version: updateInfo.latest_version }) }}</strong>
          <span>{{ t("Descarga la última versión estable de DiffTwin.") }}</span>
        </div>
        <button class="button primary small" @click="downloadUpdate">
          {{ t("Descargar actualización") }}
        </button>
        <button class="icon-button tiny" :title="t('Ocultar aviso')" :aria-label="t('Ocultar aviso')" @click="updateNoticeDismissed = true">×</button>
      </div>
      <button
        v-else-if="updateCheckError"
        class="button quiet update-check-error"
        :disabled="updateCheckLoading"
        :title="updateCheckError"
        @click="checkForUpdates"
      >{{ updateCheckLoading ? t("Comprobando…") : t("No se pudo comprobar · Reintentar") }}</button>
      <button class="avatar" title="DiffTwin">DT</button>
    </header>

    <div v-if="updateDialogVisible && updateInfo?.available" class="update-dialog-layer">
      <section class="modal-card update-dialog" role="dialog" aria-modal="false" aria-labelledby="update-dialog-title">
        <div class="update-dialog-icon" aria-hidden="true">↗</div>
        <div class="eyebrow">{{ t("ACTUALIZACIÓN DISPONIBLE") }}</div>
        <h2 id="update-dialog-title">{{ t("Ya está disponible DiffTwin {version}", { version: updateInfo.latest_version }) }}</h2>
        <p>{{ t("Hay una versión estable más reciente. Descárgala desde GitHub cuando te venga bien.") }}</p>
        <div class="modal-actions">
          <button type="button" class="button secondary" @click="dismissUpdateDialog">{{ t("Ahora no") }}</button>
          <button type="button" class="button primary" @click="downloadUpdate">{{ t("Descargar actualización") }}</button>
        </div>
      </section>
    </div>

    <div v-if="notice || errorMessage" class="toast" :class="{ error: !!errorMessage }">
      <span>{{ errorMessage || notice }}</span><button @click="notice = ''; errorMessage = ''">×</button>
    </div>

    <main v-if="!project" class="dashboard">
      <div class="dashboard-heading">
        <div>
          <div class="eyebrow">{{ t("TU ESPACIO DE TRABAJO") }}</div>
          <h1>{{ t("Compara cambios. Encuentra diferencias.") }}</h1>
          <p>{{ t("Organiza tus APIs y compara cambios entre entornos.") }}</p>
          <p v-if="projects.length" class="demo-caption">{{ t("El ejemplo usa un servicio público de eco y requiere conexión a internet.") }}</p>
        </div>
        <div v-if="projects.length" class="dashboard-actions">
          <button class="button primary" @click="newProject">＋ {{ t("Nuevo proyecto") }}</button>
          <button class="button secondary project-action-button" @click="projectFileInput.click()">
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 12V3m0 0L6.5 6.5M10 3l3.5 3.5M4 11v5h12v-5"/></svg>
            {{ t("Importar proyecto") }}
          </button>
          <button class="button quiet" @click="createDemoProject">▷ {{ t("Probar ejemplo") }}</button>
        </div>
        <input ref="projectFileInput" type="file" accept=".json,application/json" hidden @change="importProject">
      </div>
      <section v-if="projects.length" class="dashboard-projects" aria-labelledby="dashboard-projects-title">
        <div class="dashboard-section-heading">
          <h2 id="dashboard-projects-title">{{ t("Tus proyectos") }}</h2>
          <p>{{ t("Abre un proyecto para continuar comparando tus APIs.") }}</p>
        </div>
      <div class="project-grid">
        <article v-for="item in projects" :key="item.id" class="project-card">
          <div class="project-card-top">
            <button class="project-card-open" :aria-label="t('Abrir proyecto {name}', { name: item.name })" @click="openProject(item.id)">
              <span class="project-logo">{{ item.name.slice(0, 1).toUpperCase() }}</span>
              <span class="project-card-heading">
                <strong>{{ item.name }}</strong>
                <span class="card-meta">{{ projectSummary(item) }}</span>
              </span>
              <span class="card-arrow">↗</span>
            </button>
            <details class="project-actions" @keydown.esc.prevent="$event.currentTarget.open = false; $event.currentTarget.querySelector('summary').focus()">
              <summary class="icon-button project-actions-trigger" :aria-label="t('Opciones de proyecto {name}', { name: item.name })" :title="t('Opciones de proyecto {name}', { name: item.name })">···</summary>
              <div class="project-actions-menu">
                <button type="button" class="project-menu-item" @click="exportProject(item); $event.currentTarget.closest('details').open = false">
                  <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 8v9m0 0 3.5-3.5M10 17l-3.5-3.5M4 9V4h12v5"/></svg>
                  {{ t("Exportar proyecto") }}
                </button>
                <button type="button" class="project-menu-item project-menu-delete" @click="deleteProject(item); $event.currentTarget.closest('details').open = false">
                  <svg class="project-delete-icon" viewBox="0 0 20 20" aria-hidden="true">
                    <path d="M5 6.25h10l-.7 9.1a1 1 0 0 1-1 .9H6.7a1 1 0 0 1-1-.9L5 6.25Z" />
                    <path d="M3.75 4.75h12.5M8 4.75v-1a1 1 0 0 1 1-1h2a1 1 0 0 1 1 1v1m-4 3.5v5m3-5v5" />
                  </svg>
                  {{ t("Eliminar") }}
                </button>
              </div>
            </details>
          </div>
          <div class="target-summary">
            <div class="target-row">
              <i class="target-dot stable"></i><span class="target-label">{{ t("Estable principal") }}</span><code :title="item.primary">{{ item.primary }}</code>
            </div>
            <div class="target-row">
              <i class="target-dot candidate"></i><span class="target-label">{{ t("Candidata") }}</span><code :title="item.candidate">{{ item.candidate }}</code>
            </div>
            <div v-if="item.secondary" class="target-row">
              <i class="target-dot stable"></i><span class="target-label">{{ t("Estable secundaria") }}</span><code :title="item.secondary">{{ item.secondary }}</code>
            </div>
          </div>
        </article>
      </div>
      </section>
      <div v-else class="empty-state">
        <span class="empty-icon">◇</span>
        <h2>{{ t("Empieza creando un proyecto") }}</h2>
        <p>{{ t("Crea tu primer proyecto o importa uno existente para empezar.") }}</p>
        <div class="empty-state-actions">
          <button class="button primary" @click="newProject">＋ {{ t("Crear proyecto") }}</button>
          <button class="button secondary project-action-button" @click="projectFileInput.click()">
            <svg viewBox="0 0 20 20" aria-hidden="true"><path d="M10 12V3m0 0L6.5 6.5M10 3l3.5 3.5M4 11v5h12v-5"/></svg>
            {{ t("Importar proyecto") }}
          </button>
          <button class="button quiet" @click="createDemoProject">▷ {{ t("Probar ejemplo") }}</button>
        </div>
        <p class="empty-state-note">{{ t("El ejemplo usa un servicio público de eco y requiere conexión a internet.") }}</p>
      </div>
      <section v-if="projects.length === 1" class="dashboard-guide" aria-labelledby="dashboard-guide-title">
        <div class="dashboard-guide-intro">
          <span class="eyebrow">{{ t("DE LA PETICIÓN AL RESULTADO") }}</span>
          <h2 id="dashboard-guide-title">{{ t("Tu próxima comparación, en tres pasos") }}</h2>
          <p>{{ t("Abre tu proyecto y sigue este recorrido.") }}</p>
        </div>
        <ol class="dashboard-steps">
          <li><span class="dashboard-step-number">1</span><div><h3>{{ t("Configura los destinos") }}</h3><p>{{ t("Define la estable principal y la candidata. Añade otra estable para detectar ruido.") }}</p></div></li>
          <li><span class="dashboard-step-number">2</span><div><h3>{{ t("Prepara una petición") }}</h3><p>{{ t("Elige una petición de tu colección o crea una con su ruta, parámetros y cabeceras.") }}</p></div></li>
          <li><span class="dashboard-step-number">3</span><div><h3>{{ t("Envía y compara") }}</h3><p>{{ t("Revisa el estado HTTP, los valores que cambian y las respuestas de cada destino.") }}</p></div></li>
        </ol>
      </section>
    </main>

    <div v-else class="workspace" :class="{ 'sidebar-resizing': sidebarResizing }" :style="{ '--sidebar-width': `${sidebarWidth}px` }">
      <CollectionSidebar
        :project="project"
        :locale="locale"
        :collection-id="collectionId"
        :folder-id="folderId"
        :request-id="requestId"
        :request-count="requestCount"
        :expanded-collections="expandedCollections"
        :active-menu="activeMenu"
        :dragged-item="draggedTreeItem"
        :drop-target="treeDropTarget"
        :busy="busy"
        @select-collection="selectCollection"
        @select-folder="selectFolder"
        @select-request="selectRequest"
        @toggle-collection="toggleCollection"
        @create-collection="createCollection"
        @create-folder="createFolder"
        @create-request="createRequest"
        @rename-collection="saveCollectionRename"
        @delete-collection="deleteCollection"
        @run-collection="runCollectionFromMenu"
        @duplicate-request="duplicateRequest"
        @delete-request="deleteRequestByID"
        @toggle-menu="toggleMenu"
        @close-menus="closeMenus"
        @import-postman="importPostman"
        @drag-start="startTreeDrag"
        @drag-end="endTreeDrag"
        @drag-over="treeDragOver"
        @drop="dropTreeItem"
        @rename-folder="renameFolder"
        @delete-folder="deleteFolder"
        @error="(message) => flash(message, true)"
      />

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
        <DestinationSettings
          :project="project"
          :locale="locale"
          :auth-target="authTarget"
          :auth-form="authForm"
          :destination-variable-drafts="destinationVariableDrafts"
          :auth-loaded-project-id="authLoadedProjectId"
          :auth-loading-project-id="authLoadingProjectId"
          :auth-credentials-missing="authCredentialsMissing"
          :auth-saving="authSaving"
          :dirty-targets="dirtyAuthTargets"
          :comparison-dirty="hasUnsavedComparisonOptions"
          :ignored-paths-text="ignoredJSONPathsText"
          @update:auth-target="setAuthTarget"
          @update:ignored-paths-text="ignoredJSONPathsText = $event"
          @update:auth-setting="updateAuthSetting"
          @update:credential="updateCredential"
          @update:destination-variables="(target, value) => destinationVariableDrafts[target] = value"
          @auth-type="setAuthType"
          @save="saveProjectAuth"
        />

        <section v-if="selectedRequest" class="request-editor">
          <RequestEditor
            :request="selectedRequest"
            :locale="locale"
            :busy="busy"
            :reset-key="requestEditorResetKey"
            @save="saveRequest"
            @run="runRequest"
            @delete="deleteRequest"
            @error="(message) => flash(message, true)"
          />
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

    <div v-if="appDialog" class="modal-backdrop app-dialog-backdrop" @click.self="closeAppDialog()">
      <section
        class="modal-card app-dialog"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="'app-dialog-title'"
        @keydown.esc.stop.prevent="closeAppDialog()"
      >
        <div class="modal-heading">
          <div><div class="eyebrow">{{ t(appDialog.type === "confirm" ? "CONFIRMAR ACCIÓN" : appDialog.type === "prompt" ? "NUEVO ELEMENTO" : "IMPORTACIÓN") }}</div><h2 id="app-dialog-title">{{ appDialog.title }}</h2></div>
          <button type="button" class="icon-button" :aria-label="t('Cerrar')" @click="closeAppDialog()">×</button>
        </div>
        <p v-if="appDialog.message" class="app-dialog-message">{{ appDialog.message }}</p>
        <input
          v-if="appDialog.type === 'prompt'"
          ref="appDialogInput"
          v-model="appDialog.value"
          class="app-dialog-input"
          :aria-label="appDialog.title"
          @keydown.enter.prevent="closeAppDialog(appDialog.value)"
        >
        <div class="modal-actions">
          <button v-if="appDialog.type !== 'message'" type="button" class="button secondary" @click="closeAppDialog()">{{ t("Cancelar") }}</button>
          <button
            type="button"
            class="button"
            :class="appDialog.type === 'confirm' ? 'danger-outline' : 'primary'"
            @click="closeAppDialog(appDialog.type === 'prompt' ? appDialog.value : appDialog.type === 'confirm')"
          >{{ t(appDialog.type === "confirm" ? "Eliminar" : appDialog.type === "prompt" ? "Aceptar" : "Cerrar") }}</button>
        </div>
      </section>
    </div>
  </div>
</template>
