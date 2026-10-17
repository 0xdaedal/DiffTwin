<script setup>
import { computed, nextTick, ref } from "vue";
import { translate } from "./i18n";
import FolderTree from "./FolderTree.vue";

const props = defineProps({
  project: { type: Object, required: true },
  locale: { type: String, required: true },
  collectionId: { type: String, default: "" },
  folderId: { type: String, default: "" },
  requestId: { type: String, default: "" },
  requestCount: { type: Number, default: 0 },
  expandedCollections: { type: Object, required: true },
  activeMenu: { type: String, default: "" },
  draggedItem: { type: Object, default: null },
  dropTarget: { type: String, default: "" },
  busy: { type: Boolean, default: false }
});

const emit = defineEmits([
  "select-collection",
  "select-folder",
  "select-request",
  "toggle-collection",
  "create-collection",
  "create-folder",
  "create-request",
  "rename-collection",
  "delete-collection",
  "run-collection",
  "duplicate-request",
  "delete-request",
  "toggle-menu",
  "close-menus",
  "import-postman",
  "drag-start",
  "drag-end",
  "drag-over",
  "drop",
  "rename-folder",
  "delete-folder"
]);

const t = (source, values) => translate(props.locale, source, values);
const searchOpen = ref(false);
const searchTerm = ref("");
const searchInput = ref(null);
const postmanFileInput = ref(null);
const renamingCollectionId = ref("");
const renameDraft = ref("");
const renameInput = ref(null);

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

function countRequests(requests, folders) {
  return requests.length + folders.reduce((count, folder) =>
    count + countRequests(folder.requests || [], folder.folders || []), 0
  );
}

const visibleCollections = computed(() => {
  const query = searchTerm.value.trim().toLocaleLowerCase();
  return (props.project.collections || []).filter((item) =>
    !query || matchesSearch(item.name, query) ||
    (item.requests || []).some((request) => requestMatches(request, query)) ||
    (item.folders || []).some((folder) => folderMatches(folder, query))
  );
});

function collectionItemSearch(item) {
  const query = searchTerm.value.trim();
  return matchesSearch(item.name, query) ? "" : query;
}

function menuKey(type, id = "") {
  return id ? `${type}:${id}` : type;
}

function treeDropTargetKey(type, collectionID, folderID = "") {
  return type === "folder" ? `folder:${collectionID}:${folderID}` : `collection:${collectionID}`;
}

function toggleSearch() {
  emit("close-menus");
  searchOpen.value = !searchOpen.value;
  searchTerm.value = "";
  if (searchOpen.value) nextTick(() => searchInput.value?.focus());
}

function toggleCollection(item) {
  emit("toggle-collection", item);
}

function startRename(item) {
  emit("select-collection", item);
  renamingCollectionId.value = item.id;
  renameDraft.value = item.name;
  emit("close-menus");
  nextTick(() => {
    renameInput.value?.focus();
    renameInput.value?.select();
  });
}

function cancelRename(item) {
  if (renamingCollectionId.value !== item.id) return;
  renamingCollectionId.value = "";
  renameDraft.value = item.name;
}

function saveRename(item) {
  if (renamingCollectionId.value !== item.id) return;
  const name = renameDraft.value.trim();
  renamingCollectionId.value = "";
  if (!name) {
    emit("error", t("El nombre no puede estar vacío."));
    return;
  }
  emit("rename-collection", item, name);
}

function selectCollection(item) {
  emit("select-collection", item);
}

function createInCollection(item, action, parentId = "") {
  selectCollection(item);
  emit(action, parentId);
}

function startTreeDrag(event, item) {
  emit("drag-start", event, item);
}

function treeDragOver(event, collectionID, folderID = "") {
  emit("drag-over", event, collectionID, folderID);
}

function dropTreeItem(collectionID, folderID = "") {
  emit("drop", collectionID, folderID);
}

function duplicateInCollection(item, request, parentId = "") {
  selectCollection(item);
  emit("duplicate-request", request, parentId);
}

function deleteInCollection(item, id, parentId = "") {
  selectCollection(item);
  emit("delete-request", id, parentId);
}
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar-heading">
      <div v-if="!searchOpen" class="sidebar-heading-title">
        <svg viewBox="0 0 20 20" aria-hidden="true"><path d="m10 2.8 6.7 3.6v7.2L10 17.2l-6.7-3.6V6.4L10 2.8Z"/><path d="m3.3 6.4 6.7 3.7 6.7-3.7M10 10.1v7.1"/></svg>
        <span>{{ t("Colecciones") }}</span>
      </div>
      <input
        v-else
        ref="searchInput"
        v-model="searchTerm"
        class="collection-search"
        :aria-label="t('Buscar colecciones y peticiones')"
        :placeholder="t('Buscar…')"
        @keydown.esc="toggleSearch"
      >
      <div class="sidebar-heading-actions">
        <button class="icon-button" :title="t(searchOpen ? 'Cerrar búsqueda' : 'Buscar')" :aria-label="t(searchOpen ? 'Cerrar búsqueda' : 'Buscar')" @click="toggleSearch">
          <svg v-if="!searchOpen" viewBox="0 0 20 20" aria-hidden="true"><circle cx="8.7" cy="8.7" r="5.6"/><path d="m12.8 12.8 4 4"/></svg>
          <svg v-else viewBox="0 0 20 20" aria-hidden="true"><path d="m5 5 10 10M15 5 5 15"/></svg>
        </button>
        <button class="icon-button" :title="t('Nueva colección')" :aria-label="t('Nueva colección')" @click="emit('create-collection')">＋</button>
        <button class="icon-button" :title="t('Más opciones')" :aria-label="t('Más opciones')" :aria-expanded="activeMenu === menuKey('import')" @click.stop="emit('toggle-menu', 'import')">
          <svg viewBox="0 0 20 20" aria-hidden="true"><circle cx="4" cy="10" r="1"/><circle cx="10" cy="10" r="1"/><circle cx="16" cy="10" r="1"/></svg>
        </button>
        <div v-if="activeMenu === menuKey('import')" class="collection-menu sidebar-options-menu" @click.stop>
          <button @click="emit('close-menus'); postmanFileInput.click()">{{ t("Importar colección de Postman") }}</button>
        </div>
      </div>
      <input ref="postmanFileInput" type="file" accept=".json,application/json" hidden @change="emit('import-postman', $event)">
    </div>
    <div class="collection-tree">
      <div v-for="item in visibleCollections" :key="item.id" class="collection-group">
        <div
          class="tree-row collection-row"
          :class="{ selected: collectionId === item.id && !folderId && !requestId, 'menu-open': activeMenu === menuKey('collection', item.id), 'drop-target': dropTarget === treeDropTargetKey('collection', item.id) }"
          :title="t('Suelta aquí para mover a la colección')"
          @dragover="treeDragOver($event, item.id)"
          @drop.prevent.stop="dropTreeItem(item.id)"
        >
          <button class="tree-toggle" :aria-label="t(expandedCollections.has(item.id) ? 'Contraer colección' : 'Expandir colección')" :aria-expanded="expandedCollections.has(item.id)" @click="toggleCollection(item)">
            <svg class="tree-chevron" :class="{ expanded: expandedCollections.has(item.id) }" viewBox="0 0 16 16" aria-hidden="true"><path d="m5.5 6.5 2.5 2.5 2.5-2.5" /></svg>
          </button>
          <div v-if="renamingCollectionId === item.id" class="tree-label tree-label-editing">
            <span class="collection-icon">▤</span>
            <input
              ref="renameInput"
              v-model="renameDraft"
              class="tree-rename-input"
              :aria-label="t('Nombre de la colección')"
              :title="t('Pulsa Enter para guardar o Escape para cancelar.')"
              @click.stop
              @keydown.enter.prevent="saveRename(item)"
              @keydown.esc.prevent="cancelRename(item)"
              @blur="saveRename(item)"
            >
          </div>
          <button v-else class="tree-label" @click="selectCollection(item)">
            <span class="collection-icon">▤</span><span class="tree-name">{{ item.name }}</span>
          </button>
          <div class="tree-actions">
            <button class="icon-button tiny" :title="t('Opciones de colección')" :aria-label="t('Opciones de colección')" :aria-expanded="activeMenu === menuKey('collection', item.id)" @click.stop="emit('toggle-menu', 'collection', item.id)">···</button>
          </div>
          <div v-if="activeMenu === menuKey('collection', item.id)" class="collection-menu" @click.stop>
            <button @click="createInCollection(item, 'create-request')">＋ {{ t("Nueva petición") }}</button>
            <button @click="createInCollection(item, 'create-folder')">＋ {{ t("Nueva carpeta") }}</button>
            <div class="collection-menu-separator"></div>
            <button :disabled="busy || countRequests(item.requests || [], item.folders || []) === 0" @click="emit('close-menus'); emit('run-collection', item)">▶ {{ t("Ejecutar colección ({count})", { count: countRequests(item.requests || [], item.folders || []) }) }}</button>
            <div class="collection-menu-separator"></div>
            <button @click="startRename(item)">{{ t("Renombrar colección") }}</button>
            <button class="collection-menu-danger" @click="selectCollection(item); emit('delete-collection')">{{ t("Eliminar colección") }}</button>
          </div>
        </div>
        <div v-if="expandedCollections.has(item.id) || searchTerm.trim()" class="tree-children">
          <div v-for="request in (item.requests || []).filter((entry) => requestMatches(entry, collectionItemSearch(item).toLocaleLowerCase()))" :key="request.id" class="request-branch">
            <div
              class="tree-row request-row"
              :class="{ selected: collectionId === item.id && requestId === request.id, 'menu-open': activeMenu === menuKey('request', request.id) }"
              role="button"
              tabindex="0"
              :title="t('Arrastra la petición a una carpeta o colección para moverla')"
              draggable="true"
              @click="emit('select-request', item.id, request.id)"
              @keydown.enter="emit('select-request', item.id, request.id)"
              @dragstart.stop="startTreeDrag($event, { type: 'request', id: request.id, collectionId: item.id, parentId: '' })"
              @dragend="emit('drag-end')"
            >
              <span class="tree-spacer"></span>
              <span class="method-badge" :class="`method-${request.method.toLowerCase()}`">{{ request.method }}</span>
              <span class="tree-name">{{ request.name }}</span>
              <span class="tree-actions"><button class="icon-button tiny" :title="t('Opciones de petición')" :aria-expanded="activeMenu === menuKey('request', request.id)" @click.stop="emit('toggle-menu', 'request', request.id)">···</button></span>
            </div>
            <div v-if="activeMenu === menuKey('request', request.id)" class="collection-menu request-menu" @click.stop>
              <button @click="emit('close-menus'); duplicateInCollection(item, request)">{{ t("Duplicar petición") }}</button>
              <button class="collection-menu-danger" @click="emit('close-menus'); deleteInCollection(item, request.id)">{{ t("Eliminar petición") }}</button>
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
            :dragged-item="draggedItem"
            :drop-target="dropTarget"
            @select-folder="emit('select-folder', item.id, $event)"
            @select-request="(id, parentFolderId) => emit('select-request', item.id, id, parentFolderId)"
            @toggle-menu="(type, id) => emit('toggle-menu', type, id)"
            @close-menus="emit('close-menus')"
            @duplicate-request="(request, parentId) => duplicateInCollection(item, request, parentId)"
            @delete-request="(id, parentId) => deleteInCollection(item, id, parentId)"
            @create-folder="(parentId) => { selectCollection(item); emit('create-folder', parentId); }"
            @create-request="(parentId) => { selectCollection(item); emit('create-request', parentId); }"
            @rename-folder="(id, name) => emit('rename-folder', item.id, id, name)"
            @delete-folder="(id) => { selectCollection(item); emit('delete-folder', id); }"
            @drag-start="(event, dragged) => emit('drag-start', event, dragged)"
            @drag-end="emit('drag-end')"
            @drag-over="(event, collectionID, folderID) => treeDragOver(event, collectionID, folderID)"
            @drop-on-folder="(event, collectionID, folderID) => dropTreeItem(collectionID, folderID)"
          />
        </div>
      </div>
      <div v-if="!visibleCollections.length" class="sidebar-empty">
        <p v-if="searchTerm.trim()">{{ t("No se encontraron coincidencias.") }}</p>
        <template v-else>
          <p>{{ t("Aún no hay colecciones.") }}</p>
          <button class="button secondary small" @click="emit('create-collection')">{{ t("Crear colección") }}</button>
        </template>
      </div>
    </div>
    <div class="sidebar-bottom">
      <span>{{ t(requestCount === 1 ? "{count} petición" : "{count} peticiones", { count: requestCount }) }}</span>
      <span>DiffTwin</span>
    </div>
  </aside>
</template>
