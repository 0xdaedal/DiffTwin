<script setup>
import { computed, ref, watch } from "vue";
import { translate } from "./i18n";

const props = defineProps({
  request: { type: Object, required: true },
  locale: { type: String, required: true },
  busy: { type: Boolean, default: false },
  resetKey: { type: Number, default: 0 }
});

const emit = defineEmits(["save", "run", "delete", "error"]);
const t = (source, values) => translate(props.locale, source, values);
const activeTab = ref("params");
const queryParams = ref([]);
const queryParamCount = computed(() => queryParams.value.filter((param) => param.key !== "" || param.value !== "").length);
const headerCount = computed(() => Object.keys(props.request.headers || {}).length);

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
  queryParams.value = parseQueryParams(props.request.path || "");
}

function updateQueryPath() {
  const path = props.request.path || "";
  const hashIndex = path.indexOf("#");
  const queryIndex = path.indexOf("?");
  const hasQuery = queryIndex >= 0 && (hashIndex < 0 || queryIndex < hashIndex);
  const prefix = hasQuery ? path.slice(0, queryIndex) : hashIndex < 0 ? path : path.slice(0, hashIndex);
  const fragment = hashIndex >= 0 ? path.slice(hashIndex) : "";
  const values = queryParams.value
    .filter((param) => param.key !== "" || param.value !== "")
    .map((param) => `${encodeQueryPart(param.key)}${param.hasEquals || param.value !== "" ? `=${encodeQueryPart(param.value)}` : ""}`);
  props.request.path = `${prefix}${values.length ? `?${values.join("&")}` : hasQuery && !queryParams.value.length ? "?" : ""}${fragment}`;
}

function addQueryParam() {
  queryParams.value.push({ key: "", value: "", hasEquals: true });
}

function removeQueryParam(index) {
  queryParams.value.splice(index, 1);
  updateQueryPath();
}

function updateHeaders(event) {
  try {
    props.request.headers = JSON.parse(event.target.value);
  } catch {
    emit("error", t("Headers debe ser JSON válido."));
  }
}

watch(
  [() => props.request.id, () => props.request.path],
  ([requestId], [previousRequestId] = []) => {
    if (requestId !== previousRequestId) activeTab.value = "params";
    syncQueryParams();
  },
  { immediate: true }
);

watch(() => props.resetKey, () => {
  activeTab.value = "params";
});
</script>

<template>
  <div class="request-sticky">
    <div class="request-titlebar">
      <div><span class="eyebrow">{{ t("PETICIÓN") }}</span><input v-model="request.name" class="request-title-input" :aria-label="t('Nombre de petición')"></div>
      <button class="icon-button danger-icon" :title="t('Eliminar petición')" :aria-label="t('Eliminar petición')" @click="emit('delete')">
        <svg class="trash-icon" viewBox="0 0 16 16" aria-hidden="true">
          <path d="M2.5 4.5h11M6 4.5V3h4v1.5m2.5 0-.7 9h-6L5 4.5m2.5 2.2v4.5m3-4.5v4.5" />
        </svg>
      </button>
    </div>
    <div class="request-urlbar">
      <select v-model="request.method" class="method-select" :class="`method-${request.method.toLowerCase()}`">
        <option v-for="method in ['GET','POST','PUT','PATCH','DELETE','HEAD','OPTIONS']" :key="method">{{ method }}</option>
      </select>
      <input v-model="request.path" class="url-input" placeholder="/v1/users/{{userId}}">
      <button class="button primary send-button" :disabled="busy" @click="emit('run')">
        <span v-if="busy" class="spinner"></span><span v-else>▶</span>{{ busy ? t("Ejecutando…") : t("Enviar") }}
      </button>
    </div>
  </div>
  <div class="editor-tabs">
    <button class="editor-tab" :class="{ active: activeTab === 'params' }" @click="activeTab = 'params'">Params <span class="tab-count">{{ queryParamCount }}</span></button>
    <button class="editor-tab" :class="{ active: activeTab === 'headers' }" @click="activeTab = 'headers'">Headers <span class="tab-count">{{ headerCount }}</span></button>
    <button class="editor-tab" :class="{ active: activeTab === 'body' }" @click="activeTab = 'body'">Body</button>
    <span class="tab-spacer"></span>
    <button class="button secondary small" @click="emit('save')">{{ t("Guardar") }}</button>
  </div>
  <div class="editor-grid single">
    <div v-if="activeTab === 'params'" class="params-editor">
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
    <label v-if="activeTab === 'headers'" class="field-label">{{ t("HEADERS · JSON") }}
      <textarea
        :value="JSON.stringify(request.headers || {}, null, 2)"
        spellcheck="false"
        @change="updateHeaders"
      ></textarea>
    </label>
    <label v-else-if="activeTab === 'body'" class="field-label">{{ t("BODY") }}
      <textarea v-model="request.body" spellcheck="false" placeholder='{ "example": true }'></textarea>
    </label>
  </div>
</template>
