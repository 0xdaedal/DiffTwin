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
