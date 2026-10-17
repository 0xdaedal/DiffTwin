import assert from "node:assert/strict";
import test from "node:test";
import { computed, ref } from "vue";
import { useProjectAuthentication } from "./useProjectAuthentication.js";

function createAuthenticationState(saveProjectWithAuth) {
  const projectId = ref("project-1");
  const project = computed(() => ({
    id: projectId.value,
    primary: "https://stable.example",
    candidate: "https://candidate.example",
    secondary: ""
  }));
  const ignoredPathsText = ref("");
  const errors = [];
  let savedProject;
  let savedCredentials;
  let saveCount = 0;
  const settings = useProjectAuthentication({
    project,
    projectId,
    ignoredPathsText,
    backend: () => ({
      GetProjectAuth: async () => ({
        auth: { primary: { type: "none" }, candidate: { type: "none" } },
        credentials: {},
        credentials_missing: false
      }),
      SaveProjectWithAuth: async (payload, credentials) => {
        saveCount++;
        savedProject = payload;
        savedCredentials = credentials;
        return saveProjectWithAuth ? saveProjectWithAuth(payload, credentials) : { id: payload.id };
      }
    }),
    refreshProjects: async () => {},
    t: (message) => message,
    flash: (message, isError) => {
      if (isError) errors.push(message);
    },
    onSaved: () => {}
  });

  return {
    ...settings,
    ignoredPathsText,
    errors,
    get savedProject() { return savedProject; },
    get savedCredentials() { return savedCredentials; },
    get saveCount() { return saveCount; }
  };
}

test("destination settings track unsaved changes and clear them after saving", async () => {
  const state = createAuthenticationState();
  await state.loadProjectAuth("project-1");

  state.setAuthType("bearer", "candidate");
  state.updateCredential("candidate", "token", "example-value");
  state.ignoredPathsText.value = "$.metadata.generatedAt";

  assert.equal(state.dirtyTargets.value.candidate, true);
  assert.equal(state.dirtyTargets.value.primary, false);
  assert.equal(state.hasUnsavedComparisonOptions.value, true);

  await state.saveProjectAuth();

  assert.equal(state.saveCount, 1);
  assert.equal(state.dirtyTargets.value.candidate, false);
  assert.equal(state.hasUnsavedComparisonOptions.value, false);
  assert.equal(state.savedCredentials.candidate.token, "example-value");
  assert.equal("credentials" in state.savedProject, false);
  assert.deepEqual(state.errors, []);
});

test("destination settings retain unsaved state when saving fails", async () => {
  const state = createAuthenticationState(async () => {
    throw new Error("Storage unavailable");
  });
  await state.loadProjectAuth("project-1");
  state.setAuthType("bearer", "candidate");

  await state.saveProjectAuth();

  assert.equal(state.dirtyTargets.value.candidate, true);
  assert.equal(state.authSaving.value, false);
  assert.deepEqual(state.errors, ["Storage unavailable"]);
});
