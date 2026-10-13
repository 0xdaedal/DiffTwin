# Copilot instructions

- Before making changes, inspect the relevant code and preserve existing behavior unless the task explicitly requests a change.
- Keep changes focused, follow existing Go/Vue patterns, and add or update tests for behavior changes.
- Keep user-facing text and documentation in Spanish.
- Never put authentication secrets in project JSON, logs, errors, or imported/exported project configuration. Use the operating-system credential store.
- Regenerate Wails bindings with `wails generate module` when Wails-bound Go methods or models change.
- Rebuild `frontend/dist` after frontend changes because it is embedded in the desktop app.
- See [the DiffTwin skill](skills/difftwin/SKILL.md) for architecture, domain-specific behavior, and validation commands.
