<script setup>
import { computed } from "vue";
import { translate } from "./i18n";

const props = defineProps({
  project: { type: Object, required: true },
  locale: { type: String, required: true },
  authTarget: { type: String, required: true },
  authForm: { type: Object, required: true },
  destinationVariableDrafts: { type: Object, required: true },
  authLoadedProjectId: { type: String, default: "" },
  authLoadingProjectId: { type: String, default: "" },
  authCredentialsMissing: { type: Boolean, default: false },
  authSaving: { type: Boolean, default: false },
  dirtyTargets: { type: Object, required: true },
  comparisonDirty: { type: Boolean, default: false },
  ignoredPathsText: { type: String, required: true },
  latencyThresholds: { type: Object, required: true }
});

const emit = defineEmits([
  "update:authTarget",
  "update:ignoredPathsText",
  "update:latencyThresholds",
  "update:auth-setting",
  "update:credential",
  "update:destination-variables",
  "auth-type",
  "save"
]);

const t = (source, values) => translate(props.locale, source, values);
const authTargetLabel = computed(() => ({
  primary: t("Estable principal"),
  candidate: t("Candidata"),
  secondary: t("Estable secundaria")
}[props.authTarget]));
const panelTitle = computed(() => t("Configuración de {destination}", { destination: authTargetLabel.value }));
const authMethodHint = computed(() => ({
  none: t("Este destino no enviará credenciales."),
  bearer: t("El token se enviará como una cabecera Authorization: Bearer."),
  basic: t("El usuario y la contraseña se enviarán mediante autenticación HTTP Basic."),
  api_key: t("La clave se enviará en la ubicación que elijas.")
}[props.authForm.auth[props.authTarget].type || "none"]));

function updateAuthSetting(target, key, event) {
  emit("update:auth-setting", target, key, event.target.value);
}

function updateCredential(target, key, event) {
  emit("update:credential", target, key, event.target.value);
}

function updateLatencyThreshold(key, event) {
  const value = Number(event.target.value);
  emit("update:latencyThresholds", {
    ...props.latencyThresholds,
    [key]: Number.isFinite(value) && value > 0 ? Math.round(value) : ""
  });
}
</script>

<template>
  <div class="target-strip" :class="{ 'target-strip-two': !project.secondary }" role="group" :aria-label="t('Destino para configurar')">
    <button
      type="button"
      class="target-chip target-chip-button"
      :class="{ selected: authTarget === 'primary' }"
      :aria-pressed="authTarget === 'primary'"
      :aria-label="`${t('Configurar autenticación de estable principal')}${dirtyTargets.primary ? ` · ${t('Sin guardar')}` : ''}`"
      @click="emit('update:authTarget', 'primary')"
    >
      <i class="target-dot stable"></i>
      <span class="target-chip-content">
        <span>{{ t("ESTABLE PRINCIPAL") }}</span>
        <strong>{{ project.primary }}</strong>
        <small v-if="dirtyTargets.primary" class="target-pending">{{ t("Sin guardar") }}</small>
      </span>
      <span v-if="authTarget === 'primary'" class="auth-target-check">✓</span>
    </button>
    <button
      type="button"
      class="target-chip target-chip-button"
      :class="{ selected: authTarget === 'candidate' }"
      :aria-pressed="authTarget === 'candidate'"
      :aria-label="`${t('Configurar autenticación de candidata')}${dirtyTargets.candidate ? ` · ${t('Sin guardar')}` : ''}`"
      @click="emit('update:authTarget', 'candidate')"
    >
      <i class="target-dot candidate"></i>
      <span class="target-chip-content">
        <span>{{ t("CANDIDATA") }}</span>
        <strong>{{ project.candidate }}</strong>
        <small v-if="dirtyTargets.candidate" class="target-pending">{{ t("Sin guardar") }}</small>
      </span>
      <span v-if="authTarget === 'candidate'" class="auth-target-check">✓</span>
    </button>
    <button
      v-if="project.secondary"
      type="button"
      class="target-chip target-chip-button"
      :class="{ selected: authTarget === 'secondary' }"
      :aria-pressed="authTarget === 'secondary'"
      :aria-label="`${t('Configurar autenticación de estable secundaria')}${dirtyTargets.secondary ? ` · ${t('Sin guardar')}` : ''}`"
      @click="emit('update:authTarget', 'secondary')"
    >
      <i class="target-dot stable"></i>
      <span class="target-chip-content">
        <span>{{ t("ESTABLE SECUNDARIA") }}</span>
        <strong>{{ project.secondary }}</strong>
        <small v-if="dirtyTargets.secondary" class="target-pending">{{ t("Sin guardar") }}</small>
      </span>
      <span v-if="authTarget === 'secondary'" class="auth-target-check">✓</span>
    </button>
  </div>

  <details class="workspace-auth" open>
    <summary class="workspace-auth-summary">
      <strong>{{ panelTitle }}</strong>
      <span v-if="dirtyTargets[authTarget]" class="auth-unsaved-badge">{{ t("Sin guardar") }}</span>
      <span class="settings-toggle" aria-hidden="true"><span>⌄</span></span>
    </summary>
    <div class="workspace-auth-content">
      <section class="auth-configuration">
        <p v-if="authLoadingProjectId === project.id" class="auth-storage-note">{{ t("Cargando autenticación…") }}</p>
        <div class="auth-method-row">
          <label class="field-label">{{ t("MÉTODO DE AUTENTICACIÓN") }}
            <select :value="authForm.auth[authTarget].type || 'none'" @change="emit('auth-type', $event.target.value, authTarget)">
              <option value="none">{{ t("Sin autenticación") }}</option>
              <option value="bearer">Bearer</option>
              <option value="basic">Basic</option>
              <option value="api_key">API key</option>
            </select>
          </label>
          <p class="auth-method-hint">{{ authMethodHint }}</p>
        </div>
        <div class="workspace-auth-fields">
          <template v-if="authForm.auth[authTarget].type === 'bearer'">
            <label class="field-label">{{ t("TOKEN") }}<input :value="authForm.credentials[authTarget].token || ''" type="password" autocomplete="new-password" required @input="updateCredential(authTarget, 'token', $event)"></label>
          </template>
          <template v-else-if="authForm.auth[authTarget].type === 'basic'">
            <label class="field-label">{{ t("USUARIO") }}<input :value="authForm.credentials[authTarget].username || ''" autocomplete="off" @input="updateCredential(authTarget, 'username', $event)"></label>
            <label class="field-label">{{ t("CONTRASEÑA") }}<input :value="authForm.credentials[authTarget].password || ''" type="password" autocomplete="new-password" required @input="updateCredential(authTarget, 'password', $event)"></label>
          </template>
          <template v-else-if="authForm.auth[authTarget].type === 'api_key'">
            <label class="field-label">{{ t("NOMBRE DE LA CLAVE") }}<input :value="authForm.auth[authTarget].api_key_name || ''" required placeholder="X-API-Key" @input="updateAuthSetting(authTarget, 'api_key_name', $event)"></label>
            <label class="field-label">{{ t("ENVIAR EN") }}
              <select :value="authForm.auth[authTarget].api_key_location || 'header'" @change="updateAuthSetting(authTarget, 'api_key_location', $event)">
                <option value="header">Header</option>
                <option value="query">{{ t("Parámetro de consulta") }}</option>
              </select>
            </label>
            <label class="field-label">{{ t("VALOR") }}<input :value="authForm.credentials[authTarget].api_key || ''" type="password" autocomplete="new-password" required @input="updateCredential(authTarget, 'api_key', $event)"></label>
          </template>
        </div>
        <p v-if="authForm.auth[authTarget].type === 'api_key' && authForm.auth[authTarget].api_key_location === 'query'" class="auth-security-warning">
          {{ t("Las claves en la URL pueden quedar en registros del servidor o de proxies. Usa Header si la API lo admite.") }}
        </p>
        <p v-if="authCredentialsMissing" class="auth-security-warning">{{ t("Faltan credenciales guardadas en el sistema. Vuelve a introducirlas para este proyecto.") }}</p>
        <p v-if="authLoadedProjectId !== project.id && authLoadingProjectId !== project.id" class="auth-security-warning">{{ t("No se pudieron cargar las credenciales seguras. Comprueba que el almacén del sistema esté disponible.") }}</p>
        <p class="auth-storage-note auth-storage-note-emphasis">🔒 {{ t("Las credenciales se guardan en el almacén seguro del sistema y no se incluyen en la configuración JSON del proyecto.") }}</p>
      </section>
      <details class="advanced-settings">
        <summary><span><strong>{{ t("Variables del destino") }}</strong><small>{{ t("Variables específicas de este destino") }}</small></span><span class="advanced-settings-toggle">⌄</span></summary>
        <div class="advanced-settings-content">
          <label class="field-label destination-variables-field">{{ t("VARIABLES ESPECÍFICAS · JSON") }}
            <textarea
              :value="destinationVariableDrafts[authTarget]"
              spellcheck="false"
              placeholder='{ "region": "eu-west", "userId": "123" }'
              @input="emit('update:destination-variables', authTarget, $event.target.value)"
            ></textarea>
          </label>
          <p class="auth-storage-note">{{ t("Estas variables sobrescriben las comunes solo para este destino; las demás conservan el valor común. Se guardan en el JSON del proyecto sin cifrar: no pongas secretos aquí.") }}</p>
        </div>
      </details>
    </div>
  </details>

  <details class="comparison-rules">
    <summary class="workspace-auth-summary">
      <strong>{{ t("Reglas de comparación") }}</strong>
      <span v-if="comparisonDirty" class="auth-unsaved-badge">{{ t("Sin guardar") }}</span>
      <span class="settings-toggle" aria-hidden="true"><span>⌄</span></span>
    </summary>
    <div class="comparison-rules-content">
      <div class="comparison-settings">
        <label class="field-label">{{ t("IGNORAR RUTAS JSON · UNA POR LÍNEA") }}
          <textarea :value="ignoredPathsText" spellcheck="false" placeholder="$.meta.generatedAt&#10;$.items[*].id" @input="emit('update:ignoredPathsText', $event.target.value)"></textarea>
        </label>
        <p>{{ t("Admite rutas como $.meta.timestamp, $[\"campo.con.puntos\"] y comodines *. Guardar configuración también guarda estas opciones.") }}</p>
      </div>
      <div class="comparison-settings latency-settings">
        <strong class="field-label">{{ t("AVISO DE LATENCIA") }}</strong>
        <div class="latency-threshold-grid">
          <label class="field-label">{{ t("AVISO · % MÁS LENTA") }}
            <input
              type="number"
              min="1"
              max="10000"
              :value="latencyThresholds.warn"
              @input="updateLatencyThreshold('warn', $event)"
            >
          </label>
          <label class="field-label">{{ t("ALERTA · % MÁS LENTA") }}
            <input
              type="number"
              min="1"
              max="10000"
              :value="latencyThresholds.alert"
              @input="updateLatencyThreshold('alert', $event)"
            >
          </label>
          <label class="field-label">{{ t("MARGEN MÍNIMO · MS") }}
            <input
              type="number"
              min="1"
              max="600000"
              :value="latencyThresholds.floor"
              @input="updateLatencyThreshold('floor', $event)"
            >
          </label>
        </div>
        <p>{{ t("Solo se avisa cuando la candidata es más lenta que la estable principal y supera a la vez el margen mínimo y el porcentaje. Si hay estable secundaria, su desviación se usa como ruido esperado. La latencia nunca cambia el veredicto.") }}</p>
      </div>
    </div>
  </details>
  <div class="workspace-settings-footer">
    <button class="button primary small" :disabled="authSaving || authLoadedProjectId !== project.id" @click="emit('save')">
      {{ authSaving ? t("Guardando…") : t("Guardar configuración") }}
    </button>
  </div>
</template>
