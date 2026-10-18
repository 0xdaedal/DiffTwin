import assert from "node:assert/strict";
import test from "node:test";
import { detectLocale, translate } from "./i18n.js";

test("detectLocale selects English when an English system language is available", () => {
  assert.equal(detectLocale(["fr-FR", "en-GB"]), "en");
});

test("detectLocale defaults to Spanish for non-English system languages", () => {
  assert.equal(detectLocale(["es-ES"]), "es");
  assert.equal(detectLocale(["fr-FR"]), "es");
});

test("translate formats interpolated English messages", () => {
  assert.equal(
    translate("en", "Proyecto «{name}» eliminado.", { name: "Demo" }),
    "Project “Demo” deleted."
  );
});

test("translate keeps Spanish messages and falls back to unknown source text", () => {
  assert.equal(translate("es", "Proyecto creado."), "Proyecto creado.");
  assert.equal(translate("en", "An untranslated message."), "An untranslated message.");
});

test("rename keyboard guidance is localized", () => {
  assert.equal(
    translate("en", "Pulsa Enter para guardar o Escape para cancelar."),
    "Press Enter to save or Escape to cancel."
  );
  assert.equal(
    translate("es", "Pulsa Enter para guardar o Escape para cancelar."),
    "Pulsa Enter para guardar o Escape para cancelar."
  );
});

test("custom dialogs use localized labels", () => {
  assert.equal(translate("en", "Confirmar acción"), "Confirm action");
  assert.equal(translate("en", "Limitaciones de importación"), "Import limitations");
  assert.equal(translate("en", "Aceptar"), "OK");
});

test("destination settings labels are localized", () => {
  assert.equal(
    translate("en", "Configuración de {destination}", { destination: "Candidate" }),
    "Candidate settings"
  );
  assert.equal(translate("en", "Reglas de comparación"), "Comparison rules");
  assert.equal(translate("en", "Sin guardar"), "Unsaved");
});

test("dashboard actions and project menus are localized", () => {
  assert.equal(
    translate("en", "Abrir proyecto {name}", { name: "Inventory API" }),
    "Open project Inventory API"
  );
  assert.equal(
    translate("en", "Opciones de proyecto {name}", { name: "Inventory API" }),
    "Options for project Inventory API"
  );
  assert.equal(
    translate("en", "Crea tu primer proyecto o importa uno existente para empezar."),
    "Create your first project or import an existing one to get started."
  );
});

test("update notifications are localized", () => {
  assert.equal(
    translate("en", "Nueva versión disponible: {version}", { version: "v0.2.0" }),
    "New version available: v0.2.0"
  );
  assert.equal(translate("en", "No se pudo comprobar · Reintentar"), "Could not check · Retry");
});
