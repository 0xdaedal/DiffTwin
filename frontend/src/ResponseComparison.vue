<script setup>
import { computed, nextTick, ref } from "vue";
import { translate } from "./i18n";

const props = defineProps({
  cards: { type: Array, required: true },
  panels: { type: Array, required: true },
  locale: { type: String, required: true },
  secondaryEnabled: { type: Boolean, default: false }
});

const t = (source, values) => translate(props.locale, source, values);
const bodyMode = ref("diff");
const syncScroll = ref(true);
const expandedFoldPaths = ref(new Set());
const diffQueries = ref({});
const activeDiff = ref({ panelId: "", path: "", index: -1 });
const responsePanes = [];
let synchronizingScroll = false;

const bodyDiffs = computed(() => props.panels
  .flatMap((panel) => panel.diffs)
  .filter((diff) => diff.path.startsWith("$")));

const formattedCards = computed(() => props.cards.map(([name, result]) => ({
  name,
  result,
  ...formatBody(result)
})));

function formatJSONPath(path, key) {
  return /^[A-Za-z_$][\w$]*$/.test(key)
    ? `${path}.${key}`
    : `${path}[${JSON.stringify(key)}]`;
}

function formatJSONLines(value) {
  const lines = [];
  const append = (item, path, depth, prefix = "", comma = false) => {
    const indent = "  ".repeat(depth);
    const suffix = comma ? "," : "";
    if (Array.isArray(item)) {
      lines.push({ text: `${indent}${prefix}[`, path, structural: true });
      item.forEach((child, index) => append(child, `${path}[${index}]`, depth + 1, "", index < item.length - 1));
      lines.push({ text: `${indent}]${suffix}`, path, structural: true });
      return;
    }
    if (item !== null && typeof item === "object") {
      const entries = Object.entries(item);
      lines.push({ text: `${indent}${prefix}{`, path, structural: true });
      entries.forEach(([key, child], index) => {
        append(child, formatJSONPath(path, key), depth + 1, `${JSON.stringify(key)}: `, index < entries.length - 1);
      });
      lines.push({ text: `${indent}}${suffix}`, path, structural: true });
      return;
    }
    lines.push({ text: `${indent}${prefix}${JSON.stringify(item)}${suffix}`, path, structural: false });
  };
  append(value, "$", 0);
  return lines;
}

function formatBody(result) {
  if (result.error) {
    return { validJSON: false, lines: [{ text: result.error, path: "$error", structural: false }] };
  }
  let lines;
  let validJSON = false;
  try {
    lines = formatJSONLines(JSON.parse(result.body));
    validJSON = true;
  } catch {
    const body = result.body || t("(respuesta vacía)");
    lines = body.split(/\r?\n/).map((text, index) => ({ text, path: `$text[${index}]`, structural: false }));
  }
  if (result.truncated) {
    lines.push({ text: t("[Respuesta truncada a 2 MiB]"), path: "$truncated", structural: false });
  }
  return { validJSON, lines };
}

function pathContains(parent, child) {
  return child === parent || child.startsWith(`${parent}.`) || child.startsWith(`${parent}[`);
}

function diffKind(path) {
  return bodyDiffs.value.find((diff) => diff.path === path)?.kind || "";
}

function segmentsFor(card) {
  if (bodyMode.value === "full" || !card.validJSON || !bodyDiffs.value.length) {
    return card.lines.map((line, index) => ({ type: "line", line, index }));
  }

  const visible = new Set([0, card.lines.length - 1]);
  card.lines.forEach((line, index) => {
    const exact = bodyDiffs.value.some((diff) => diff.path === line.path);
    const structuralContext = line.structural && line.path !== "$" &&
      bodyDiffs.value.some((diff) => pathContains(line.path, diff.path));
    if (!exact && !structuralContext) return;
    visible.add(index);
    if (index > 0) visible.add(index - 1);
    if (index + 1 < card.lines.length) visible.add(index + 1);
  });

  const segments = [];
  let hidden = [];
  const flushHidden = () => {
    if (!hidden.length) return;
    segments.push({ type: "fold", lines: hidden });
    hidden = [];
  };
  card.lines.forEach((line, index) => {
    if (!visible.has(index)) {
      hidden.push({ line, index });
      return;
    }
    flushHidden();
    segments.push({ type: "line", line, index });
  });
  flushHidden();
  return segments;
}

function foldExpanded(segment) {
  return segment.lines.some((item) => expandedFoldPaths.value.has(item.line.path));
}

function toggleFold(segment, expanded) {
  const next = new Set(expandedFoldPaths.value);
  for (const item of segment.lines) {
    if (expanded) next.add(item.line.path);
    else next.delete(item.line.path);
  }
  expandedFoldPaths.value = next;
}

function setBodyMode(mode) {
  bodyMode.value = mode;
  expandedFoldPaths.value = new Set();
}

function setQuery(panelId, value) {
  diffQueries.value = { ...diffQueries.value, [panelId]: value };
}

function visibleDiffs(panel) {
  const query = (diffQueries.value[panel.id] || "").trim().toLowerCase();
  const diffs = panel.diffs.slice(0, 100);
  if (!query) return diffs;
  return diffs.filter((diff) =>
    diff.path.toLowerCase().includes(query) ||
    String(diff.before).toLowerCase().includes(query) ||
    String(diff.after).toLowerCase().includes(query)
  );
}

function activePosition(panel) {
  if (activeDiff.value.panelId !== panel.id) return -1;
  return visibleDiffs(panel).findIndex((diff) => diff.path === activeDiff.value.path);
}

function scrollToPath(path) {
  if (!path.startsWith("$")) return;
  for (const pane of responsePanes) {
    if (!pane) continue;
    const line = [...pane.querySelectorAll("[data-json-path]")]
      .find((item) => item.dataset.jsonPath === path);
    if (!line) continue;
    pane.scrollTo({ top: Math.max(0, line.offsetTop - pane.clientHeight / 3), behavior: "smooth" });
  }
}

async function selectDiff(panel, diff, index) {
  activeDiff.value = { panelId: panel.id, path: diff.path, index };
  await nextTick();
  scrollToPath(diff.path);
}

function moveDiff(panel, delta) {
  const diffs = visibleDiffs(panel);
  if (!diffs.length) return;
  const current = activePosition(panel);
  const next = current < 0
    ? (delta > 0 ? 0 : diffs.length - 1)
    : (current + delta + diffs.length) % diffs.length;
  selectDiff(panel, diffs[next], next);
}

function setResponsePane(element, index) {
  responsePanes[index] = element;
}

function firstVisibleLine(pane) {
  const top = pane.scrollTop;
  return [...pane.querySelectorAll(".response-body-line")]
    .find((line) => line.offsetTop + line.offsetHeight > top + 1);
}

function synchronizeResponseScroll(event) {
  if (!syncScroll.value || synchronizingScroll) return;
  const source = event.currentTarget;
  const sourceLine = firstVisibleLine(source);
  const sourceOffset = sourceLine ? sourceLine.offsetTop - source.scrollTop : 0;
  synchronizingScroll = true;

  for (const pane of responsePanes) {
    if (!pane || pane === source) continue;
    const matchingLine = sourceLine
      ? [...pane.querySelectorAll(".response-body-line")]
        .find((line) => line.dataset.jsonPath === sourceLine.dataset.jsonPath)
      : null;
    if (matchingLine) {
      pane.scrollTop = Math.max(0, matchingLine.offsetTop - sourceOffset);
    } else {
      const sourceRange = source.scrollHeight - source.clientHeight;
      const targetRange = pane.scrollHeight - pane.clientHeight;
      pane.scrollTop = sourceRange > 0 ? (source.scrollTop / sourceRange) * targetRange : 0;
    }
  }
  requestAnimationFrame(() => { synchronizingScroll = false; });
}

function statusClass(result) {
  if (result.error) return "status-error";
  if (result.status_code >= 200 && result.status_code < 300) return "status-good";
  if (result.status_code >= 400) return "status-error";
  return "status-warn";
}
</script>

<template>
  <section
    v-for="panel in panels"
    :key="panel.id"
    class="diff-panel"
    :class="{ 'diff-panel-noise': panel.id === 'stable' }"
    :aria-label="panel.title"
  >
    <header class="diff-heading">
      <strong>{{ panel.title }}</strong>
      <div class="diff-navigation">
        <input
          :value="diffQueries[panel.id] || ''"
          :aria-label="t('Filtrar por ruta')"
          :placeholder="t('Filtrar por ruta')"
          @input="setQuery(panel.id, $event.target.value)"
          @keydown.up.prevent="moveDiff(panel, -1)"
          @keydown.down.prevent="moveDiff(panel, 1)"
        >
        <span>{{ Math.max(0, activePosition(panel) + 1) }} / {{ visibleDiffs(panel).length }}</span>
        <button type="button" :title="t('Diferencia anterior')" :aria-label="t('Diferencia anterior')" @click="moveDiff(panel, -1)">↑</button>
        <button type="button" :title="t('Diferencia siguiente')" :aria-label="t('Diferencia siguiente')" @click="moveDiff(panel, 1)">↓</button>
      </div>
    </header>
    <div class="diff-table">
      <div class="diff-row diff-column-head">
        <span>{{ t("Ruta") }}</span><span>{{ panel.leftLabel }}</span><span>{{ panel.rightLabel }}</span>
      </div>
      <button
        v-for="(diff, index) in visibleDiffs(panel)"
        :key="`${diff.path}-${index}`"
        type="button"
        class="diff-row diff-row-button"
        :class="[`diff-${diff.kind}`, { 'diff-current': activeDiff.panelId === panel.id && activeDiff.path === diff.path }]"
        @click="selectDiff(panel, diff, index)"
      >
        <code class="diff-path" :title="diff.path">{{ diff.path }}</code>
        <code class="diff-value" :title="String(diff.before).slice(0, 1000)">{{ diff.before }}</code>
        <code class="diff-value" :title="String(diff.after).slice(0, 1000)">{{ diff.after }}</code>
      </button>
    </div>
    <p v-if="panel.diffs.length > 100" class="diff-truncated">{{ t("Se muestran los primeros 100 cambios.") }}</p>
  </section>

  <div class="response-view-toolbar">
    <div class="response-mode-toggle" role="group" :aria-label="t('Visualización de las respuestas')">
      <button type="button" :class="{ active: bodyMode === 'diff' }" @click="setBodyMode('diff')">{{ t("Solo cambios") }}</button>
      <button type="button" :class="{ active: bodyMode === 'full' }" @click="setBodyMode('full')">{{ t("Respuesta completa") }}</button>
    </div>
    <label class="response-sync-toggle">
      <input v-model="syncScroll" type="checkbox">
      {{ t("Scroll sincronizado") }}
    </label>
  </div>

  <div class="response-grid" :class="{ 'response-grid-two': !secondaryEnabled }">
    <article v-for="(card, cardIndex) in formattedCards" :key="card.name" class="response-card">
      <header>
        <strong>{{ card.name }}</strong>
        <span class="response-meta" :class="statusClass(card.result)">
          {{ card.result.error ? t("Error") : `HTTP ${card.result.status_code}` }} · {{ card.result.duration_ms }} ms
        </span>
      </header>
      <div
        :ref="(element) => setResponsePane(element, cardIndex)"
        class="response-body"
        @scroll="synchronizeResponseScroll"
      >
        <template v-for="(segment, segmentIndex) in segmentsFor(card)" :key="`${segment.type}-${segmentIndex}`">
          <div
            v-if="segment.type === 'line'"
            class="response-body-line"
            :class="[
              diffKind(segment.line.path) ? `response-line-${diffKind(segment.line.path)}` : '',
              { 'response-line-active': activeDiff.path === segment.line.path }
            ]"
            :data-json-path="segment.line.path"
          >
            <span class="response-line-number">{{ segment.index + 1 }}</span>
            <code>{{ segment.line.text }}</code>
          </div>
          <template v-else>
            <button
              v-if="!foldExpanded(segment)"
              type="button"
              class="response-fold"
              @click="toggleFold(segment, true)"
            >
              <span>▸</span>{{ t("Mostrar {count} líneas idénticas", { count: segment.lines.length }) }}
            </button>
            <div v-else class="response-fold-expanded">
              <button
                type="button"
                class="response-fold-collapse"
                :title="t('Volver a plegar este tramo')"
                :aria-label="t('Volver a plegar este tramo')"
                @click="toggleFold(segment, false)"
              >▴</button>
              <div
                v-for="item in segment.lines"
                :key="item.index"
                class="response-body-line"
                :data-json-path="item.line.path"
              >
                <span class="response-line-number">{{ item.index + 1 }}</span>
                <code>{{ item.line.text }}</code>
              </div>
            </div>
          </template>
        </template>
      </div>
    </article>
  </div>
</template>
