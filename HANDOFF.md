# DiffTwin handoff

## Current state

DiffTwin is a Go/Wails desktop application for comparing HTTP requests against a primary stable destination and a candidate, with an optional secondary stable destination to detect response noise. The frontend uses Vue 3 and Vite; Wails exposes project, collection, Postman import, and comparison operations.

The application includes a dashboard, project selector, nested collection/folder navigation, request editor with Params/Headers/Body tabs, and comparisons across up to three destinations. Comparison details include JSON paths that differ between the two stable destinations. The verdict considers status, response headers (excluding volatile/transport headers such as `Date`, `Content-Length`, and `CF-Ray`; `NEL` and `Report-To` remain comparable), and body. Latency is displayed but does not affect the verdict. JSON paths can be ignored with exact or wildcard patterns in project options. Shared variables can be overridden per destination for paths, query parameters, headers, and bodies. Authentication is independent per destination (none, Bearer, Basic, or API key in header/query); secrets are stored in the operating system credential store, not in project JSON. Postman v2.0/v2.1 import preserves folder hierarchy, paths/query, headers, raw bodies, and collection variables, and reports known unsupported items.

Collections can be executed sequentially in displayed order. The workspace shows progress and per-request outcomes, continues after a request fails, and allows opening each completed comparison. The existing single-request flow remains available.

The **Probar ejemplo** dashboard action idempotently creates or opens the `Demo: comparación de APIs` project, selects its request, and uses HTTPBin to demonstrate a JSON difference at `$.url`. It requires an internet connection; do not send sensitive data to the public service.

## Code layout

- `internal/domain`: entities, validation, cloning, and recursive collection/folder operations.
- `internal/comparison`: HTTP client and response comparison.
- `internal/credentials`: operating system credential-store access.
- `internal/storage`: project JSON storage and migration.
- `app.go`: application use cases exposed to Wails.
- `frontend/src`: Vue interface and styles; `frontend/dist` contains assets embedded in the app.
- `main.go`: Wails startup and legacy HTTP handlers retained for compatibility/tests.

## Next improvement

**Comparison history:** browse previous runs and inspect their responses and differences. Define its UX and persistence before implementation; keep credentials out of any stored run data.


## Development workflow

See the [native development workflow in README](README.md#native-development-workflow) for detailed platform requirements and steps. In short:

- Use the Dev Container from any host to edit, run Go tests with `-tags ci`, build Vue, and build Linux. It does not show the desktop window or share the host credential store.
- To visually test the app and build a native executable, install platform dependencies and run `wails dev` on Linux, macOS, or Windows.
- Stop `wails dev` and other foreground servers in their terminals with `Ctrl+C`; close the application window. To disconnect VS Code from the container, run **Dev Containers: Reopen Folder Locally** or **Close Remote Connection**.
- Before publishing, run tests (`go test -tags ci ./...`, `go vet -tags ci ./...`, `npm --prefix frontend run build`) and make sure no secrets or local project JSON are included.

## Validation and known limitations

- Go tests pass with `go test -count=1 -race -tags ci ./...`; `go vet -tags ci ./...` passes.
- `npm run build` in `frontend/` generates `frontend/dist`.
- A Linux native build was verified with WebKitGTK 4.1 and tags `production webkit2_41`.
- Native builds and interactive GUI testing should be done on their target operating systems. Installer generation and automated publishing are not configured.
- The project uses Go 1.27.2 in its Dev Container and documented setup.

## Working on another machine

The public repository is [github.com/jrcasanya/DiffTwin](https://github.com/jrcasanya/DiffTwin). Clone it and use either the Dev Container or the native workflow described in README. To transfer projects, import their JSON or copy the local data (`~/.config/DiffTwin/projects/` on Linux; Go's `os.UserConfigDir` determines user-specific paths on macOS/Windows). Credentials remain in each machine's secure system store and must be configured again.
