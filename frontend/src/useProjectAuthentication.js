import { computed, ref, watch } from "vue";

const destinations = ["primary", "candidate", "secondary"];

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

export function emptyDestinationVariableDrafts(destinationVariables = {}) {
  return {
    primary: JSON.stringify(destinationVariables.primary || {}, null, 2),
    candidate: JSON.stringify(destinationVariables.candidate || {}, null, 2),
    secondary: JSON.stringify(destinationVariables.secondary || {}, null, 2)
  };
}

export const DEFAULT_LATENCY_THRESHOLDS = Object.freeze({ warn: 20, alert: 50, floor: 50 });

export function defaultLatencyThresholds() {
  return { ...DEFAULT_LATENCY_THRESHOLDS };
}

export function latencyThresholdsFrom(project) {
  return {
    warn: project?.latency_warn_percent || DEFAULT_LATENCY_THRESHOLDS.warn,
    alert: project?.latency_alert_percent || DEFAULT_LATENCY_THRESHOLDS.alert,
    floor: project?.latency_floor_ms || DEFAULT_LATENCY_THRESHOLDS.floor
  };
}

export function latencyThresholdsPayload(thresholds) {
  return {
    latency_warn_percent: Number(thresholds?.warn) || DEFAULT_LATENCY_THRESHOLDS.warn,
    latency_alert_percent: Number(thresholds?.alert) || DEFAULT_LATENCY_THRESHOLDS.alert,
    latency_floor_ms: Number(thresholds?.floor) || DEFAULT_LATENCY_THRESHOLDS.floor
  };
}

export function useProjectAuthentication({
  project,
  projectId,
  ignoredPathsText,
  latencyThresholds = ref(defaultLatencyThresholds()),
  backend,
  refreshProjects,
  t,
  flash,
  onSaved
}) {
  const authTarget = ref("primary");
  const authForm = ref(emptyProjectAuth());
  const authSaving = ref(false);
  const authLoadedProjectId = ref("");
  const authLoadingProjectId = ref("");
  const authCredentialsMissing = ref(false);
  const savedAuthTargetSnapshots = ref({});
  const savedIgnoredPathsText = ref("");
  const savedLatencyThresholds = ref(JSON.stringify(defaultLatencyThresholds()));
  const destinationVariableDrafts = ref(emptyDestinationVariableDrafts());

  const hasUnsavedComparisonOptions = computed(() =>
    ignoredPathsText.value !== savedIgnoredPathsText.value ||
    JSON.stringify(latencyThresholdsPayload(latencyThresholds.value)) !== savedLatencyThresholds.value
  );

  function authTargetSnapshot(
    target,
    auth = authForm.value.auth,
    credentials = authForm.value.credentials,
    variableDrafts = destinationVariableDrafts.value
  ) {
    return JSON.stringify({
      auth: auth[target] || {},
      credentials: credentials[target] || {},
      variables: variableDrafts[target] || ""
    });
  }

  function hasUnsavedAuthTarget(target) {
    return authLoadedProjectId.value === project.value?.id &&
      savedAuthTargetSnapshots.value[target] !== undefined &&
      savedAuthTargetSnapshots.value[target] !== authTargetSnapshot(target);
  }

  const dirtyTargets = computed(() => ({
    primary: hasUnsavedAuthTarget("primary"),
    candidate: hasUnsavedAuthTarget("candidate"),
    secondary: hasUnsavedAuthTarget("secondary")
  }));

  function resetDestinationVariableDrafts(destinationVariables = {}) {
    destinationVariableDrafts.value = emptyDestinationVariableDrafts(destinationVariables);
  }

  async function loadProjectAuth(id) {
    authLoadedProjectId.value = "";
    authLoadingProjectId.value = id;
    authCredentialsMissing.value = false;
    savedAuthTargetSnapshots.value = {};
    authForm.value = emptyProjectAuth();
    resetDestinationVariableDrafts(project.value?.destination_variables);
    savedIgnoredPathsText.value = ignoredPathsText.value;
    savedLatencyThresholds.value = JSON.stringify(latencyThresholdsPayload(latencyThresholds.value));
    try {
      const settings = await backend().GetProjectAuth(id);
      if (projectId.value !== id) return;
      authForm.value = emptyProjectAuth();
      Object.assign(authForm.value.auth, settings.auth || {});
      Object.assign(authForm.value.credentials, settings.credentials || {});
      authCredentialsMissing.value = settings.credentials_missing || false;
      authLoadedProjectId.value = id;
      savedAuthTargetSnapshots.value = Object.fromEntries(
        destinations.map((target) => [target, authTargetSnapshot(target)])
      );
    } catch (error) {
      if (projectId.value === id) {
        authForm.value.auth = { ...authForm.value.auth, ...(project.value?.auth || {}) };
        flash(error.message || String(error), true);
      }
    } finally {
      if (authLoadingProjectId.value === id) authLoadingProjectId.value = "";
    }
  }

  async function saveProjectAuth() {
    if (!project.value || authSaving.value || authLoadedProjectId.value !== project.value.id) return;
    authSaving.value = true;
    try {
      const authToSave = JSON.parse(JSON.stringify(authForm.value.auth));
      const credentialsToSave = JSON.parse(JSON.stringify(authForm.value.credentials));
      const variableDraftsToSave = { ...destinationVariableDrafts.value };
      const ignoredPathsToSave = ignoredPathsText.value;
      const latencyToSave = latencyThresholdsPayload(latencyThresholds.value);
      const destinationVariables = {};
      for (const target of destinations) {
        const parsed = JSON.parse(variableDraftsToSave[target] || "{}");
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
        auth: authToSave,
        ignored_json_paths: ignoredPathsToSave.split(/\r?\n/).map((path) => path.trim()).filter(Boolean),
        ...latencyToSave,
        destination_variables: destinationVariables
      }, credentialsToSave);
      await refreshProjects();
      if (projectId.value !== saved.id) return;
      authLoadedProjectId.value = saved.id;
      authCredentialsMissing.value = false;
      savedIgnoredPathsText.value = ignoredPathsToSave;
      savedLatencyThresholds.value = JSON.stringify(latencyToSave);
      savedAuthTargetSnapshots.value = Object.fromEntries(
        destinations.map((target) => [
          target,
          authTargetSnapshot(target, authToSave, credentialsToSave, variableDraftsToSave)
        ])
      );
      onSaved();
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

  function setAuthType(type, target = authTarget.value) {
    const current = authForm.value.auth[target];
    authForm.value.auth[target] = {
      ...current,
      ...(type === "api_key" && !current.api_key_location ? { api_key_location: "header" } : {}),
      type
    };
    authForm.value.credentials[target] = {};
  }

  function updateAuthSetting(target, key, value) {
    authForm.value.auth[target] = { ...authForm.value.auth[target], [key]: value };
  }

  function updateCredential(target, key, value) {
    authForm.value.credentials[target] = { ...authForm.value.credentials[target], [key]: value };
  }

  watch(() => project.value?.secondary, (secondary) => {
    if (!secondary && authTarget.value === "secondary") {
      authTarget.value = "primary";
      authForm.value.auth.secondary = { type: "none" };
      authForm.value.credentials.secondary = {};
    }
  });

  return {
    authTarget,
    authForm,
    authSaving,
    authLoadedProjectId,
    authLoadingProjectId,
    authCredentialsMissing,
    destinationVariableDrafts,
    dirtyTargets,
    hasUnsavedComparisonOptions,
    resetDestinationVariableDrafts,
    loadProjectAuth,
    saveProjectAuth,
    setAuthTarget,
    setAuthType,
    updateAuthSetting,
    updateCredential
  };
}
