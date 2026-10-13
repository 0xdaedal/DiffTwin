<script setup>
import { computed, ref, watch } from "vue";

const props = defineProps({
  folder: { type: Object, required: true },
  selectedFolderId: { type: String, default: "" },
  selectedRequestId: { type: String, default: "" },
  activeMenu: { type: String, default: "" },
  searchTerm: { type: String, default: "" }
});
const emit = defineEmits(["select-folder", "select-request", "toggle-menu", "close-menus", "duplicate-request", "delete-request", "create-folder", "create-request", "rename-folder", "delete-folder"]);
const open = ref(true);
const selected = computed(() => props.selectedFolderId === props.folder.id);
const folderMenuKey = computed(() => `folder:${props.folder.id}`);
const menuOpen = computed(() => props.activeMenu === folderMenuKey.value);
function requestMenuKey(id) {
  return `request:${id}`;
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
    <div class="tree-row" :class="{ selected, 'menu-open': menuOpen }">
      <button class="tree-toggle" @click="open = !open" :aria-label="isExpanded ? 'Contraer carpeta' : 'Expandir carpeta'" :aria-expanded="isExpanded">
        <svg class="tree-chevron" :class="{ expanded: isExpanded }" viewBox="0 0 16 16" aria-hidden="true">
          <path d="m5.5 6.5 2.5 2.5 2.5-2.5" />
        </svg>
      </button>
      <button class="tree-label folder-label" @click="emit('select-folder', folder.id)">
        <span class="folder-icon">▰</span><span class="tree-name">{{ folder.name }}</span>
      </button>
      <div class="tree-actions">
        <button class="icon-button tiny" title="Opciones de carpeta" :aria-expanded="menuOpen" @click.stop="emit('toggle-menu', 'folder', folder.id)">···</button>
      </div>
      <div v-if="menuOpen" class="collection-menu folder-menu" @click.stop>
        <button @click="emit('close-menus'); emit('create-request', folder.id)">＋ Nueva petición</button>
        <button @click="emit('close-menus'); emit('create-folder', folder.id)">＋ Nueva subcarpeta</button>
        <div class="collection-menu-separator"></div>
        <button @click="emit('close-menus'); emit('rename-folder', folder.id)">Renombrar carpeta</button>
        <button class="collection-menu-danger" @click="emit('close-menus'); emit('delete-folder', folder.id)">Eliminar carpeta</button>
      </div>
    </div>
    <div v-if="isExpanded" class="tree-children">
      <div
        v-for="request in visibleRequests"
        :key="request.id"
        class="tree-row request-row"
        :class="{ selected: selectedRequestId === request.id, 'menu-open': activeMenu === requestMenuKey(request.id) }"
        role="button"
        tabindex="0"
        @click="emit('select-request', request.id, folder.id)"
        @keydown.enter="emit('select-request', request.id, folder.id)"
      >
        <span class="tree-spacer"></span>
        <span class="method-badge" :class="`method-${request.method.toLowerCase()}`">{{ request.method }}</span>
        <span class="tree-name">{{ request.name }}</span>
        <span class="tree-actions">
          <button class="icon-button tiny" title="Opciones de petición" :aria-expanded="activeMenu === requestMenuKey(request.id)" @click.stop="emit('toggle-menu', 'request', request.id)">···</button>
        </span>
        <div v-if="activeMenu === requestMenuKey(request.id)" class="collection-menu request-menu" @click.stop>
          <button @click="emit('close-menus'); emit('duplicate-request', request, folder.id)">Duplicar petición</button>
          <button class="collection-menu-danger" @click="emit('close-menus'); emit('delete-request', request.id, folder.id)">Eliminar petición</button>
        </div>
      </div>
      <FolderTree
        v-for="child in visibleFolders"
        :key="child.id"
        :folder="child"
        :selected-folder-id="selectedFolderId"
        :selected-request-id="selectedRequestId"
        :active-menu="activeMenu"
        :search-term="searchTerm"
        @select-folder="emit('select-folder', $event)"
        @select-request="(id, parentId) => emit('select-request', id, parentId)"
        @toggle-menu="(type, id) => emit('toggle-menu', type, id)"
        @close-menus="emit('close-menus')"
        @duplicate-request="(request, parentId) => emit('duplicate-request', request, parentId)"
        @delete-request="(id, parentId) => emit('delete-request', id, parentId)"
        @create-folder="(parentId) => emit('create-folder', parentId)"
        @create-request="(parentId) => emit('create-request', parentId)"
        @rename-folder="(id) => emit('rename-folder', id)"
        @delete-folder="(id) => emit('delete-folder', id)"
      />
    </div>
  </div>
</template>

<script>
export default { name: "FolderTree" };
</script>
