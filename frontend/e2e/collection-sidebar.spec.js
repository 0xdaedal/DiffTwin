import { expect, test } from "@playwright/test";

const projectFixture = {
  id: "project-1",
  name: "Proyecto de prueba",
  primary: "https://stable.example",
  candidate: "https://candidate.example",
  secondary: "",
  auth: {},
  ignored_json_paths: [],
  destination_variables: {},
  variables: {},
  collections: [{
    id: "collection-1",
    name: "Colección de prueba",
    requests: [{
      id: "request-1",
      name: "Listar usuarios",
      method: "GET",
      path: "/users?page=1",
      headers: {},
      body: ""
    }],
    folders: [{
      id: "folder-1",
      name: "Administración",
      requests: [],
      folders: []
    }]
  }]
};

test.beforeEach(async ({ page }) => {
  await page.addInitScript((initialProject) => {
    const state = {
      projects: [structuredClone(initialProject)],
      auth: {},
      calls: [],
      runResponse: null
    };
    const clone = (value) => JSON.parse(JSON.stringify(value));
    const app = {
      GetProjects: async () => clone(state.projects),
      GetProjectAuth: async (projectId) => clone(state.auth[projectId] || {
        auth: { primary: { type: "none" }, candidate: { type: "none" }, secondary: { type: "none" } },
        credentials: {},
        credentials_missing: false
      }),
      SaveCollection: async (projectId, collection) => {
        state.calls.push({ method: "SaveCollection", collection: clone(collection) });
        const project = state.projects.find((item) => item.id === projectId);
        const index = project.collections.findIndex((item) => item.id === collection.id);
        if (index < 0) project.collections.push(clone(collection));
        else project.collections[index] = clone(collection);
        return clone(collection);
      },
      SaveProjectWithAuth: async (project, credentials) => {
        state.calls.push({
          method: "SaveProjectWithAuth",
          project: clone(project),
          credentials: clone(credentials)
        });
        const index = state.projects.findIndex((item) => item.id === project.id);
        state.projects[index] = clone(project);
        state.auth[project.id] = {
          auth: clone(project.auth),
          credentials: clone(credentials),
          credentials_missing: false
        };
        return clone(project);
      },
      MoveCollectionItem: async () => {},
      RunRequest: async (projectId, request) => {
        state.calls.push({ method: "RunRequest", projectId, request: clone(request) });
        return clone(state.runResponse || {
          primary: { status_code: 200, duration_ms: 20, body: '{"user":"stable"}', error: "", truncated: false },
          candidate: { status_code: 200, duration_ms: 24, body: '{"user":"candidate"}', error: "", truncated: false },
          secondary_enabled: false,
          candidate_differs: true
        });
      },
      DeleteCollection: async (projectId, collectionId) => {
        state.calls.push({ method: "DeleteCollection", projectId, collectionId });
        const project = state.projects.find((item) => item.id === projectId);
        project.collections = project.collections.filter((item) => item.id !== collectionId);
      },
      DeleteProject: async (projectId) => {
        state.calls.push({ method: "DeleteProject", projectId });
        state.projects = state.projects.filter((item) => item.id !== projectId);
        delete state.auth[projectId];
      },
      SaveProject: async (project) => {
        const saved = clone(project);
        if (!saved.id) saved.id = "project-created";
        state.calls.push({ method: "SaveProject", project: clone(saved) });
        const index = state.projects.findIndex((item) => item.id === saved.id);
        if (index < 0) state.projects.push(clone(saved));
        else state.projects[index] = clone(saved);
        return clone(saved);
      },
      ExportProject: async (projectId) => {
        state.calls.push({ method: "ExportProject", projectId });
        return "/tmp/Proyecto de prueba.json";
      },
      CreateDemoProject: async () => clone(state.projects[0]),
      ImportPostmanCollection: async () => ({})
    };
    window.go = { main: { App: app } };
    window.__difftwinTestState = state;
    localStorage.setItem("difftwin-language", "es");
  }, projectFixture);
});

test("busca una petición y la abre desde la colección", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();

  await page.getByRole("button", { name: "Buscar" }).click();
  await page.getByRole("textbox", { name: "Buscar colecciones y peticiones" }).fill("Listar usuarios");
  await page.getByRole("button", { name: /Listar usuarios/ }).click();

  await expect(page.getByRole("textbox", { name: "Nombre de petición" })).toHaveValue("Listar usuarios");
  await expect(page.locator(".url-input")).toHaveValue("/users?page=1");
});

test("mantiene independientes el scroll del árbol y el editor principal", async ({ page }) => {
  await page.setViewportSize({ width: 1100, height: 650 });
  await page.addInitScript(() => {
    const collection = window.__difftwinTestState.projects[0].collections[0];
    collection.requests.push(...Array.from({ length: 45 }, (_, index) => ({
      id: `request-extra-${index}`,
      name: `Petición adicional ${index}`,
      method: "GET",
      path: `/additional/${index}`,
      headers: {},
      body: ""
    })));
    const primary = Object.fromEntries(Array.from({ length: 100 }, (_, index) => [`value${index}`, index]));
    const candidate = Object.fromEntries(Array.from({ length: 100 }, (_, index) => [`value${index}`, index + 1]));
    window.__difftwinTestState.runResponse = {
      primary: { status_code: 200, duration_ms: 20, body: JSON.stringify(primary), error: "", truncated: false },
      candidate: { status_code: 200, duration_ms: 24, body: JSON.stringify(candidate), error: "", truncated: false },
      secondary_enabled: false,
      candidate_differs: true
    };
  });
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: "Listar usuarios" }).click();
  await page.getByRole("button", { name: /Enviar/ }).click();

  const tree = page.locator(".collection-tree");
  const editor = page.locator(".request-editor");
  await expect(page.locator(".diff-panel .diff-row:not(.diff-column-head)")).toHaveCount(100);
  const dimensions = await page.evaluate(() => ({
    viewportHeight: window.innerHeight,
    documentHeight: document.documentElement.scrollHeight,
    treeClientHeight: document.querySelector(".collection-tree").clientHeight,
    treeScrollHeight: document.querySelector(".collection-tree").scrollHeight,
    editorClientHeight: document.querySelector(".request-editor").clientHeight,
    editorScrollHeight: document.querySelector(".request-editor").scrollHeight
  }));
  expect(dimensions.documentHeight).toBe(dimensions.viewportHeight);
  expect(dimensions.treeScrollHeight).toBeGreaterThan(dimensions.treeClientHeight);
  expect(dimensions.editorScrollHeight).toBeGreaterThan(dimensions.editorClientHeight);

  await editor.evaluate((element) => { element.scrollTop = 200; });
  await expect.poll(() => editor.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect.poll(() => tree.evaluate((element) => element.scrollTop)).toBe(0);
  await tree.evaluate((element) => { element.scrollTop = 200; });
  await expect.poll(() => tree.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
  await expect.poll(() => editor.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
});

test("guarda la edición de parámetros desde el editor de peticiones", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();

  await page.getByRole("textbox", { name: "Valor del parámetro" }).fill("2");
  await page.getByRole("button", { name: "Guardar", exact: true }).click();

  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "SaveCollection" && call.collection.requests[0].path === "/users?page=2"
    )
  )).toBe(true);
});

test("renombra una colección desde su menú contextual", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator(".collection-row").hover();
  await page.getByRole("button", { name: "Opciones de colección" }).click();
  await page.getByRole("button", { name: "Renombrar colección" }).click();

  const nameInput = page.getByRole("textbox", { name: "Nombre de la colección" });
  await nameInput.fill("Colección renombrada");
  await nameInput.press("Enter");

  await expect(page.getByRole("button", { name: "Colección renombrada" })).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "SaveCollection" && call.collection.name === "Colección renombrada"
    )
  )).toBe(true);
});

test("crea una petición desde el menú de la colección", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator(".collection-row").hover();
  await page.getByRole("button", { name: "Opciones de colección" }).click();
  await page.getByRole("button", { name: /Nueva petición/ }).click();

  await expect(page.getByRole("textbox", { name: "Nombre de petición" })).toHaveValue("Nueva petición");
  await expect(page.locator(".url-input")).toHaveValue("/");
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].requests.length
  )).toBe(2);
});

test("duplica una petición desde sus opciones", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.locator(".request-row").hover();
  await page.locator(".request-row .tree-actions button").click();
  await page.getByRole("button", { name: "Duplicar petición" }).click();

  await expect(page.getByRole("textbox", { name: "Nombre de petición" })).toHaveValue("Listar usuarios (copia)");
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].requests.map((request) => request.name)
  )).toEqual(["Listar usuarios", "Listar usuarios (copia)"]);
});

test("ejecuta una petición y muestra el resultado de comparación", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();
  await page.getByRole("button", { name: /Enviar/ }).click();

  await expect(page.getByText("Hay diferencias", { exact: true })).toBeVisible();
  await expect(page.getByText("RESULTADO DE LA COMPARACIÓN")).toBeVisible();
  await expect(page.locator(".response-card pre").filter({ hasText: '"stable"' })).toBeVisible();
  await expect(page.locator(".response-card pre").filter({ hasText: '"candidate"' })).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "RunRequest" && call.projectId === "project-1" && call.request.path === "/users?page=1"
    )
  )).toBe(true);
});

test("elimina una petición tras confirmar y persiste el cambio", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();
  await page.getByRole("button", { name: "Eliminar petición" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("¿Eliminar la petición «Listar usuarios»?");
  await dialog.getByRole("button", { name: "Eliminar", exact: true }).click();

  await expect(page.getByText("Selecciona una petición o crea una nueva para empezar.")).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].requests
  )).toEqual([]);
});

test("guarda la autenticación sin incluir el token en la configuración del proyecto", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByLabel("MÉTODO DE AUTENTICACIÓN").selectOption("bearer");
  await page.getByLabel("TOKEN").fill("e2e-fake-token");
  await page.getByRole("button", { name: "Guardar configuración" }).click();

  await expect.poll(() => page.evaluate(() => {
    const call = window.__difftwinTestState.calls.find((entry) => entry.method === "SaveProjectWithAuth");
    return call ? {
      authType: call.project.auth.primary.type,
      credential: call.credentials.primary.token,
      containsSecretInProject: JSON.stringify(call.project).includes("e2e-fake-token")
    } : null;
  })).toEqual({
    authType: "bearer",
    credential: "e2e-fake-token",
    containsSecretInProject: false
  });
});

test("no guarda variables del destino con JSON inválido", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator("details.advanced-settings > summary").click();
  await page.getByLabel("VARIABLES ESPECÍFICAS · JSON").fill("{ inválido");
  await page.getByRole("button", { name: "Guardar configuración" }).click();

  await expect(page.locator(".toast.error")).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.filter((call) => call.method === "SaveProjectWithAuth").length
  )).toBe(0);
});

test("crea una carpeta y guarda el cambio en la colección", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator(".collection-row").hover();
  await page.getByRole("button", { name: "Opciones de colección" }).click();
  await page.getByRole("button", { name: /Nueva carpeta/ }).click();

  const dialog = page.getByRole("dialog");
  await dialog.getByRole("textbox", { name: "Nombre de la carpeta" }).fill("Operaciones");
  await dialog.getByRole("button", { name: "Aceptar" }).click();

  await expect(page.getByRole("button", { name: "▰ Operaciones" })).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].folders.map((folder) => folder.name)
  )).toEqual(["Administración", "Operaciones"]);
});

test("renombra una carpeta desde sus opciones", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  const folderRow = page.locator(".folder-branch > .tree-row");
  await folderRow.hover();
  await folderRow.getByRole("button", { name: "Opciones de carpeta" }).click();
  await page.getByRole("button", { name: "Renombrar carpeta" }).click();

  const nameInput = page.getByRole("textbox", { name: "Nombre de la carpeta" });
  await nameInput.fill("Operaciones");
  await nameInput.press("Enter");

  await expect(page.getByRole("button", { name: "▰ Operaciones" })).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].folders[0].name
  )).toBe("Operaciones");
});

test("elimina una colección tras confirmar y persiste el cambio", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator(".collection-row").hover();
  await page.getByRole("button", { name: "Opciones de colección" }).click();
  await page.getByRole("button", { name: "Eliminar colección" }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("¿Eliminar «Colección de prueba» y todo su contenido?");
  await dialog.getByRole("button", { name: "Eliminar", exact: true }).click();

  await expect(page.getByText("Tu espacio de trabajo")).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections
  )).toEqual([]);
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "DeleteCollection" && call.collectionId === "collection-1"
    )
  )).toBe(true);
});

test("crea un proyecto y lo muestra en el panel", async ({ page }) => {
  await page.goto("/");
  await page.getByRole("button", { name: /Nuevo proyecto/ }).click();

  const form = page.locator("form.modal-card");
  await form.getByLabel("NOMBRE").fill("API de inventario");
  await form.getByLabel("ESTABLE PRINCIPAL").fill("https://inventory.example");
  await form.getByLabel("CANDIDATA").fill("https://inventory-next.example");
  await form.getByRole("button", { name: "Crear proyecto" }).click();

  await page.getByRole("button", { name: "Volver al dashboard" }).click();
  await expect(page.locator(".project-card").filter({ hasText: "API de inventario" })).toBeVisible();
  await expect.poll(() => page.evaluate(() => {
    const call = window.__difftwinTestState.calls.find((entry) => entry.method === "SaveProject");
    return call && {
      name: call.project.name,
      primary: call.project.primary,
      candidate: call.project.candidate
    };
  })).toEqual({
    name: "API de inventario",
    primary: "https://inventory.example",
    candidate: "https://inventory-next.example"
  });
});

test("edita un proyecto y conserva los destinos actualizados", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Configuración del proyecto" }).click();

  const form = page.locator("form.modal-card");
  await form.getByLabel("NOMBRE").fill("Proyecto actualizado");
  await form.getByLabel("CANDIDATA").fill("https://candidate-v2.example");
  await form.getByRole("button", { name: "Guardar cambios" }).click();

  await page.getByRole("button", { name: "Volver al dashboard" }).click();
  const updatedCard = page.locator(".project-card").filter({ hasText: "Proyecto actualizado" });
  await expect(updatedCard).toContainText("https://candidate-v2.example");
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].candidate
  )).toBe("https://candidate-v2.example");
});

test("elimina un proyecto tras confirmar", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-actions > summary").click();
  await page.getByRole("button", { name: "Eliminar", exact: true }).click();

  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("¿Eliminar el proyecto «Proyecto de prueba»?");
  await dialog.getByRole("button", { name: "Eliminar", exact: true }).click();

  await expect(page.getByText("Empieza creando un proyecto")).toBeVisible();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects
  )).toEqual([]);
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "DeleteProject" && call.projectId === "project-1"
    )
  )).toBe(true);
});

test("exporta un proyecto e indica la ruta devuelta por el backend", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-actions > summary").click();
  await page.getByRole("button", { name: "Exportar proyecto", exact: true }).click();

  await expect(page.locator(".toast")).toContainText("/tmp/Proyecto de prueba.json");
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.calls.some((call) =>
      call.method === "ExportProject" && call.projectId === "project-1"
    )
  )).toBe(true);
});

test("ofrece acciones claras al abrir el menú de un proyecto", async ({ page }) => {
  await page.goto("/");
  const card = page.locator(".project-card");

  await expect(card.getByRole("button", { name: "Abrir proyecto Proyecto de prueba" })).toBeVisible();
  await expect(card).toContainText("Estable principal");
  await expect(card).toContainText("Candidata");
  await expect(card).toContainText("1 colección · 1 petición");
  await expect(page.getByRole("region", { name: "Tu próxima comparación, en tres pasos" })).toBeVisible();
  await expect(page.locator(".dashboard-steps li")).toHaveCount(3);
  await expect(card.locator(".project-actions > summary")).toHaveAttribute(
    "aria-label",
    "Opciones de proyecto Proyecto de prueba"
  );

  await card.locator(".project-actions > summary").click();
  await expect(card.getByRole("button", { name: "Exportar proyecto", exact: true })).toBeVisible();
  await expect(card.getByRole("button", { name: "Eliminar", exact: true })).toBeVisible();
  await card.locator(".project-actions > summary").press("Escape");
  await expect(card.locator(".project-actions")).not.toHaveAttribute("open", "");
  await expect(card.locator(".project-actions > summary")).toBeFocused();
});

test("muestra crear, importar y probar ejemplo cuando no hay proyectos", async ({ page }) => {
  await page.addInitScript(() => {
    window.__difftwinTestState.projects = [];
  });
  await page.goto("/");

  await expect(page.getByText("Empieza creando un proyecto")).toBeVisible();
  await expect(page.getByRole("button", { name: "Crear proyecto" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Importar proyecto" })).toBeVisible();
  await expect(page.getByRole("button", { name: /Probar ejemplo/ })).toBeVisible();
  await expect(page.locator('input[type="file"][accept=".json,application/json"]')).toBeAttached();
  const chooser = page.waitForEvent("filechooser");
  await page.getByRole("button", { name: "Importar proyecto" }).click();
  await chooser;
  await page.getByRole("button", { name: "Crear proyecto" }).click();
  await expect(page.locator("form.modal-card")).toBeVisible();
});

for (const locale of ["es", "en"]) {
  for (const theme of ["light", "dark"]) {
    test(`ajusta el dashboard con varios proyectos: ${locale}, ${theme}`, async ({ page }) => {
      await page.setViewportSize({ width: 800, height: 650 });
      await page.addInitScript(({ locale, theme }) => {
        localStorage.setItem("difftwin-language", locale);
        localStorage.setItem("difftwin-theme", theme);
        const initial = window.__difftwinTestState.projects[0];
        window.__difftwinTestState.projects = Array.from({ length: 12 }, (_, index) => ({
          ...structuredClone(initial),
          id: `dashboard-project-${index}`,
          name: `API ${index} — ${"Inventario ".repeat(12)}`,
          secondary: "https://secondary.example"
        }));
      }, { locale, theme });
      await page.goto("/");
      await expect(page.locator(".project-card")).toHaveCount(12);
      await expect(page.locator(".dashboard-guide")).toHaveCount(0);
      await expect(page.locator(".project-actions > summary").first()).toHaveAttribute(
        "aria-label", new RegExp(locale === "es" ? "^Opciones de proyecto" : "^Options for project")
      );
      await expect(page.locator(".target-label").first()).toHaveText(
        locale === "es" ? "Estable principal" : "Primary stable"
      );
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBe(800);
      expect(await page.evaluate(() => document.documentElement.scrollHeight)).toBe(650);
      await expect(page.locator("html")).toHaveAttribute("data-theme", theme);
      const dashboard = page.locator(".dashboard");
      expect(await dashboard.evaluate((element) => element.scrollHeight > element.clientHeight)).toBe(true);
      await page.locator(".project-actions > summary").last().scrollIntoViewIfNeeded();
      await page.locator(".project-actions > summary").last().click();
      await expect(page.locator(".project-menu-delete").last()).toBeInViewport();
    });
  }
}


test("marca cambios por destino y guarda también las reglas de comparación", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Configurar autenticación de candidata" }).click();
  await page.getByLabel("MÉTODO DE AUTENTICACIÓN").selectOption("bearer");

  const candidateButton = page.getByRole("button", { name: /Configurar autenticación de candidata/ });
  await expect(candidateButton).toHaveAccessibleName(/Sin guardar/);

  await page.locator("details.comparison-rules > summary").click();
  await page.getByLabel("IGNORAR RUTAS JSON · UNA POR LÍNEA").fill("$.meta.generatedAt");
  await page.getByRole("button", { name: "Guardar configuración" }).click();

  await expect(candidateButton).toHaveAccessibleName("Configurar autenticación de candidata");
  await expect.poll(() => page.evaluate(() => {
    const call = window.__difftwinTestState.calls.find((entry) => entry.method === "SaveProjectWithAuth");
    return call && {
      authType: call.project.auth.candidate.type,
      ignoredPaths: call.project.ignored_json_paths
    };
  })).toEqual({
    authType: "bearer",
    ignoredPaths: ["$.meta.generatedAt"]
  });
});

test("actualiza parámetros de consulta sin perder el fragmento de la URL", async ({ page }) => {
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();

  await page.locator(".url-input").fill("/users?page=1#profile");
  await page.getByRole("textbox", { name: "Valor del parámetro" }).fill("3");
  await page.getByRole("textbox", { name: "Valor del parámetro" }).press("Tab");
  await expect(page.locator(".url-input")).toHaveValue("/users?page=3#profile");

  await page.getByRole("button", { name: "Guardar", exact: true }).click();
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].collections[0].requests[0].path
  )).toBe("/users?page=3#profile");
});

test("muestra que las respuestas coinciden cuando tienen el mismo contenido", async ({ page }) => {
  await page.addInitScript(() => {
    window.__difftwinTestState.runResponse = {
      primary: { status_code: 200, duration_ms: 20, body: '{"user":"same"}', error: "", truncated: false },
      candidate: { status_code: 200, duration_ms: 24, body: '{"user":"same"}', error: "", truncated: false },
      secondary_enabled: false,
      candidate_differs: false
    };
  });
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();
  await page.getByRole("button", { name: /Enviar/ }).click();

  await expect(page.getByText("Las respuestas coinciden", { exact: true })).toBeVisible();
  await expect(page.locator(".diff-panel")).toHaveCount(0);
});

test("aplica una ruta JSON ignorada a la comparación", async ({ page }) => {
  await page.addInitScript(() => {
    window.__difftwinTestState.runResponse = {
      primary: {
        status_code: 200,
        duration_ms: 20,
        body: '{"meta":{"generatedAt":"2026-10-16"},"user":"stable"}',
        error: "",
        truncated: false
      },
      candidate: {
        status_code: 200,
        duration_ms: 24,
        body: '{"meta":{"generatedAt":"2026-10-17"},"user":"candidate"}',
        error: "",
        truncated: false
      },
      secondary_enabled: false,
      candidate_differs: true
    };
  });
  await page.goto("/");
  await page.locator(".project-card-open").click();
  await page.locator("details.comparison-rules > summary").click();
  await page.getByLabel("IGNORAR RUTAS JSON · UNA POR LÍNEA").fill("$.meta.generatedAt");
  await page.getByRole("button", { name: "Guardar configuración" }).click();

  await page.getByRole("button", { name: "Expandir colección" }).click();
  await page.getByRole("button", { name: /Listar usuarios/ }).click();
  await page.getByRole("button", { name: /Enviar/ }).click();

  await expect(page.getByText("Hay diferencias", { exact: true })).toBeVisible();
  const candidatePanel = page.locator(".diff-panel[aria-label='Candidata frente a estable principal']");
  await expect(candidatePanel).toBeVisible();
  await expect(candidatePanel.locator(".diff-row:not(.diff-column-head)")).toHaveCount(1);
  await expect(candidatePanel).toContainText("$.user");
  await expect(candidatePanel).not.toContainText("$.meta.generatedAt");
  await expect.poll(() => page.evaluate(() =>
    window.__difftwinTestState.projects[0].ignored_json_paths
  )).toEqual(["$.meta.generatedAt"]);
});
