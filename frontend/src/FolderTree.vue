<script setup>
import { computed, nextTick, ref, watch } from "vue";
import { translate } from "./i18n";

const props = defineProps({
  folder: { type: Object, required: true },
  locale: { type: String, required: true },
  selectedFolderId: { type: String, default: "" },
  selectedRequestId: { type: String, default: "" },
  activeMenu: { type: String, default: "" },
  searchTerm: { type: String, default: "" },
  collectionId: { type: String, required: true },
  selectedCollectionId: { type: String, default: "" },
  parentFolderId: { type: String, default: "" },
  draggedItem: { type: Object, default: null },
  dropTarget: { type: String, default: "" }
});
const emit = defineEmits(["select-folder", "select-request", "toggle-menu", "close-menus", "duplicate-request", "delete-request", "create-folder", "create-request", "rename-folder", "delete-folder", "drag-start", "drag-end", "drag-over", "drop-on-folder"]);
const open = ref(true);
const isRenaming = ref(false);
const renameDraft = ref("");
const renameInput = ref(null);
const t = (source) => translate(props.locale, source);
const selected = computed(() => props.selectedCollectionId === props.collectionId && props.selectedFolderId === props.folder.id);
const folderMenuKey = computed(() => `folder:${props.folder.id}`);
const menuOpen = computed(() => props.activeMenu === folderMenuKey.value);
const folderDropTarget = computed(() => `folder:${props.collectionId}:${props.folder.id}`);
function requestMenuKey(id) {
  return `request:${id}`;
}
function startRename() {
  renameDraft.value = props.folder.name;
  isRenaming.value = true;
  emit("close-menus");
  nextTick(() => {
    renameInput.value?.focus();
    renameInput.value?.select();
  });
}
function cancelRename() {
  isRenaming.value = false;
  renameDraft.value = props.folder.name;
}
function commitRename() {
  if (!isRenaming.value) return;
  isRenaming.value = false;
  emit("rename-folder", props.folder.id, renameDraft.value);
}
const normalizedSearch = computed(() => props.searchTerm.trim().toLocaleLowerCase());
const visibleRequests = computed(() => (props.folder.requests || []).filter((request) =>
  !normalizedSearch.value || `${request.name} ${request.method} ${request.path}`.toLocaleLowerCase().includes(normalizedSearch.value)
));
const visibleFolders = computed(() => (props.folder.folders || []).filter((folder) => folderMatches(folder, normalizedSearch.value)));
const isExpanded = computed(() => open.value || !!normalizedSearch.value);
function folderMatches(folder, query) {
  return !query || folder.name.toLocaleLowerCase().includes(query) ||
    (folder.requests || []).some((request) => `${request.name} ${request.method} ${request.path}`.toLocaleLowerCase().includes(query)) ||
    (folder.folders || []).some((child) => folderMatches(child, query));
}
watch(
  () => [props.selectedFolderId, props.selectedRequestId, normalizedSearch.value],
  ([folderId, requestId, query]) => {
    if (query || folderId === props.folder.id || (requestId && (props.folder.requests || []).some((request) => request.id === requestId))) {
      open.value = true;
    }
  }
);
</script>

<template>
  <div class="tree-branch folder-branch">
    <div
      class="tree-row"
      :class="{ selected, 'menu-open': menuOpen, 'drop-target': dropTarget === folderDropTarget }"
      :title="t('Arrastra la carpeta a otra carpeta o colección para moverla')"
      draggable="true"
      @dragstart.stop="emit('drag-start', $event, { type: 'folder', id: folder.id, collectionId, parentId: parentFolderId })"
      @dragend="emit('drag-end')"
      @dragover.stop="emit('drag-over', $event, collectionId, folder.id)"
      @drop.prevent.stop="emit('drop-on-folder', $event, collectionId, folder.id)"
    >
      <button class="tree-toggle" @click="open = !open" :aria-label="t(isExpanded ? 'Contraer carpeta' : 'Expandir carpeta')" :aria-expanded="isExpanded">
        <svg class="tree-chevron" :class="{ expanded: isExpanded }" viewBox="0 0 16 16" aria-hidden="true">
          <path d="m5.5 6.5 2.5 2.5 2.5-2.5" />
        </svg>
      </button>
      <div v-if="isRenaming" class="tree-label folder-label tree-label-editing">
        <span class="folder-icon">▰</span>
        <input
          ref="renameInput"
          v-model="renameDraft"
          class="tree-rename-input"
          :aria-label="t('Nombre de la carpeta')"
          :title="t('Pulsa Enter para guardar o Escape para cancelar.')"
          @click.stop
          @dragstart.stop.prevent
          @keydown.enter.prevent.stop="commitRename"
          @keydown.esc.prevent.stop="cancelRename"
          @blur="commitRename"
        >
      </div>
      <button v-else class="tree-label folder-label" @click="emit('select-folder', folder.id)">
        <span class="folder-icon">▰</span><span class="tree-name">{{ folder.name }}</span>
      </button>
      <div class="tree-actions">
        <button class="icon-button tiny" :title="t('Opciones de carpeta')" :aria-expanded="menuOpen" @click.stop="emit('toggle-menu', 'folder', folder.id)">···</button>
      </div>
      <div v-if="menuOpen" class="collection-menu folder-menu" @click.stop>
        <button @click="emit('close-menus'); emit('create-request', folder.id)">＋ {{ t("Nueva petición") }}</button>
        <button @click="emit('close-menus'); emit('create-folder', folder.id)">＋ {{ t("Nueva subcarpeta") }}</button>
        <div class="collection-menu-separator"></div>
        <button @click="startRename">{{ t("Renombrar carpeta") }}</button>
        <button class="collection-menu-danger" @click="emit('close-menus'); emit('delete-folder', folder.id)">{{ t("Eliminar carpeta") }}</button>
      </div>
    </div>
    <div v-if="isExpanded" class="tree-children">
      <div
        v-for="request in visibleRequests"
        :key="request.id"
        class="tree-row request-row"
        :class="{ selected: selectedCollectionId === collectionId && selectedRequestId === request.id, 'menu-open': activeMenu === requestMenuKey(request.id) }"
        role="button"
        tabindex="0"
        :title="t('Arrastra la petición a una carpeta o colección para moverla')"
        draggable="true"
        @click="emit('select-request', request.id, folder.id)"
        @keydown.enter="emit('select-request', request.id, folder.id)"
        @dragstart.stop="emit('drag-start', $event, { type: 'request', id: request.id, collectionId, parentId: folder.id })"
        @dragend="emit('drag-end')"
      >
        <span class="tree-spacer"></span>
        <span class="method-badge" :class="`method-${request.method.toLowerCase()}`">{{ request.method }}</span>
        <span class="tree-name">{{ request.name }}</span>
        <span class="tree-actions">
          <button class="icon-button tiny" :title="t('Opciones de petición')" :aria-expanded="activeMenu === requestMenuKey(request.id)" @click.stop="emit('toggle-menu', 'request', request.id)">···</button>
        </span>
        <div v-if="activeMenu === requestMenuKey(request.id)" class="collection-menu request-menu" @click.stop>
          <button @click="emit('close-menus'); emit('duplicate-request', request, folder.id)">{{ t("Duplicar petición") }}</button>
          <button class="collection-menu-danger" @click="emit('close-menus'); emit('delete-request', request.id, folder.id)">{{ t("Eliminar petición") }}</button>
        </div>
      </div>
      <FolderTree
        v-for="child in visibleFolders"
        :key="child.id"
        :folder="child"
        :locale="locale"
        :collection-id="collectionId"
        :selected-collection-id="selectedCollectionId"
        :parent-folder-id="folder.id"
        :selected-folder-id="selectedFolderId"
        :selected-request-id="selectedRequestId"
        :active-menu="activeMenu"
        :search-term="searchTerm"
        :dragged-item="draggedItem"
        :drop-target="dropTarget"
        @select-folder="emit('select-folder', $event)"
        @select-request="(id, parentId) => emit('select-request', id, parentId)"
        @toggle-menu="(type, id) => emit('toggle-menu', type, id)"
        @close-menus="emit('close-menus')"
        @duplicate-request="(request, parentId) => emit('duplicate-request', request, parentId)"
        @delete-request="(id, parentId) => emit('delete-request', id, parentId)"
        @create-folder="(parentId) => emit('create-folder', parentId)"
        @create-request="(parentId) => emit('create-request', parentId)"
        @rename-folder="(id, name) => emit('rename-folder', id, name)"
        @delete-folder="(id) => emit('delete-folder', id)"
        @drag-start="(event, item) => emit('drag-start', event, item)"
        @drag-end="emit('drag-end')"
        @drag-over="(event, targetCollectionId, id) => emit('drag-over', event, targetCollectionId, id)"
        @drop-on-folder="(event, targetCollectionId, id) => emit('drop-on-folder', event, targetCollectionId, id)"
      />
    </div>
  </div>
</template>

<script>
export default { name: "FolderTree" };
</script>
