# DiffTwin

DiffTwin is a desktop application for organizing HTTP requests and comparing two or three API destinations. A primary stable destination and a candidate are required; an optional secondary stable destination helps identify response noise. It is built with Go and Wails, with a Vue 3 and Vite frontend.

For project context when resuming work in another session or on another machine, see [HANDOFF.md](HANDOFF.md).

## Requirements

- Go 1.27.2.
- Node.js 20 or later and npm to modify or rebuild the frontend.
- Platform-specific Wails dependencies:
  - **Linux (Ubuntu/Debian):** `build-essential`, `pkg-config`, `libgtk-3-dev`, and WebKitGTK. Ubuntu 24.04 uses WebKitGTK 4.1; install `libwebkit2gtk-4.1-dev` and pass the `webkit2_41` build tag. On distributions with WebKitGTK 4.0, omit that tag.
  - **macOS:** Xcode Command Line Tools (`xcode-select --install`).
  - **Windows:** WebView2 Runtime and a C/C++ compiler supported by Wails, such as MSYS2/MinGW-w64.

See the [official Wails v2 installation requirements](https://wails.io/docs/gettingstarted/installation) for platform-specific setup. Run `wails doctor` to diagnose missing dependencies.

## VS Code Dev Containers

The `.devcontainer/` configuration provides a reproducible Linux environment with Go 1.27.2, Node.js 20, Wails v2.16.0, and the dependencies for Linux builds. It works with VS Code's **Dev Containers** extension and Docker Desktop/Engine on Linux, Windows, or macOS:

1. Clone the repository and open it in VS Code:

   ```sh
   git clone https://github.com/jrcasanya/DiffTwin.git
   code DiffTwin
   ```

2. Run **Dev Containers: Reopen in Container** from the Command Palette.
3. Wait for Go, Node.js, Wails CLI, and project dependencies to be installed. Use the container to edit, run tests, build the frontend, and build the Linux app:

   ```sh
   go test -count=1 -race -tags ci ./...
   go vet -tags ci ./...
   npm --prefix frontend run build
   wails build -tags webkit2_41
   ```

The container is Linux regardless of whether VS Code runs on Windows or macOS. It does not forward the desktop window or provide access to the host's secure credential store. Use the host system to interact with the GUI, test its native credential store, or build native Windows/macOS binaries. The Dev Container is optional; you can also follow the native workflow below.

## Native development workflow

Choose either the **Dev Container** or the **native host** workflow. Both use the same project folder; avoid maintaining separate working copies for editing and testing.

### Prepare the project

Clone the repository and enter its folder:

```sh
git clone https://github.com/jrcasanya/DiffTwin.git
cd DiffTwin
```

Install Go 1.27.2 and Node.js 20 or later if they are not already installed. Install frontend dependencies and the Wails v2.16.0 CLI:

```sh
npm ci --prefix frontend
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

Ensure `wails` is on `PATH`: `$HOME/go/bin` on macOS/Linux or `%USERPROFILE%\go\bin` on Windows.

### Platform-specific setup and commands

#### Linux (Ubuntu/Debian)

Install native dependencies:

```sh
sudo apt update
sudo apt install -y build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

Install Go 1.27.2 and Node.js 20 or later from their official download pages; distribution packages may be older.

Run the app with hot reload during development:

```sh
wails dev -tags webkit2_41
```

Run a normal desktop build using the embedded frontend assets:

```sh
go run -tags 'production webkit2_41' .
```

If your distribution uses WebKitGTK 4.0, omit `webkit2_41` from Wails commands.

#### macOS

Install the Xcode Command Line Tools once:

```sh
xcode-select --install
```

Then run the app with hot reload:

```sh
wails dev
```

#### Windows (PowerShell)

Install Go 1.27.2, Node.js 20 or later, WebView2 Runtime if it is not already installed, and MSYS2/MinGW-w64 as required by the [official Wails installation guide](https://wails.io/docs/gettingstarted/installation). Ensure `go`, `npm`, and `wails` are on `PATH`; if `wails` is not found, add `%USERPROFILE%\go\bin` to the user `Path` and open a new terminal.

Run the app with hot reload:

```powershell
wails dev
```

### Iterate, test, and stop

1. Create a branch for your change (`git switch -c your-change`) and edit Go code and/or files under `frontend/src`.
2. Keep `wails dev` running in its terminal while developing the backend or UI. The app reloads changes; close its window when you finish interacting with it.
3. Stop a foreground development command by returning to its terminal and pressing **Ctrl+C**. If you started Vite manually (`npm run dev --prefix frontend`), stop that terminal too. Close the app window to stop a normal app run.
4. Validate the backend and frontend:

   ```sh
   go test -count=1 -tags ci ./...
   go vet -tags ci ./...
   npm --prefix frontend run build
   ```

   On Linux or in the Dev Container, you can also run `go test -count=1 -race -tags ci ./...`. On Windows, the race detector requires a compatible C compiler.

5. Test the desktop GUI with `wails dev` on the system where you intend to use it. **Probar ejemplo** exercises the app without real credentials.
6. Review the changes with `git diff`. Ensure they contain no secrets or local project data before committing and pushing. Never commit project JSON files or personal credentials.

To disconnect VS Code from the container, run **Dev Containers: Reopen Folder Locally** or **Close Remote Connection** from the Command Palette. This disconnects VS Code but does not stop host applications or terminals. To stop a still-running container, stop it from Docker Desktop or use Docker Engine.

Build native executables on their target operating systems: run `wails build` on macOS or Windows; on Ubuntu 24.04, run `wails build -tags webkit2_41`. Installer generation and automated publishing are not configured yet.

## Features

The dashboard displays projects as cards. **Probar ejemplo** creates or opens the idempotent `Demo: comparación de APIs` project and selects its request. It calls HTTPBin, a public echo service: the primary and secondary stable destinations use the same route, while the candidate uses a different route, producing a difference at `$.url`. An internet connection is required. Do not send secrets or sensitive data to this public service.

Projects contain collections, nested folders, and requests. Drag requests or folders onto a folder or collection in the workspace tree to move them, including between collections in the same project. Run a whole collection sequentially in its displayed order; DiffTwin shows progress and per-request outcomes, continues after individual failures, and lets you open each completed comparison. Each request still runs against its configured destinations in parallel. Authentication is configured independently for the primary stable, candidate, and optional secondary stable destinations: none, Bearer, Basic, or API key in a header/query parameter. The request editor has Params, Headers, and Body tabs; query parameters can be edited in a table or directly in the URL.

DiffTwin compares HTTP status codes, response headers, and bodies, normalizing JSON object key order. Date, content-length, and hop-by-hop headers are excluded because they commonly vary by transport or formatting; `CF-Ray`, Cloudflare's per-request identifier, is also excluded. Other headers such as `NEL` and `Report-To` are compared because they may contain meaningful server configuration. Latency is shown for context but does not affect the verdict.

Project options can exclude exact or wildcard JSON paths (`$.meta.generatedAt`, `$.items[*].id`, `$["field.with.dots"]`); array indexes are preserved when values are excluded. When the two stable responses differ, their paths and values are also shown to help distinguish noise from candidate changes. Shared variables can be overridden per destination; destination values take precedence and unspecified values fall back to shared variables. `{{name}}` interpolation applies to paths, query parameters, headers, and bodies.

Postman v2.0/v2.1 collections can be imported from the workspace. Nested folders, methods, paths/query, headers, raw bodies, and collection variables are retained. Postman hosts are discarded in favor of the project's configured destinations. Variable conflicts preserve existing values, and unsupported scripts, collection authentication, and body formats are reported.

## Data and credentials

Each project is stored as a JSON file in the user's configuration directory (for example, `~/.config/DiffTwin/projects/` on Linux). `DIFFTWIN_DATA` selects a custom path; `DIFFBENCH_DATA` remains supported for compatibility. If the previous DiffBench data directory exists, DiffTwin reuses it to preserve existing projects. Secure credentials remain accessible through the existing system credential-store service name.

If the selected data path points to a legacy JSON file, data is migrated to a sibling directory with the `.d` suffix. The original file remains as a backup, and a marker prevents deleted projects from being restored. Projects can also be imported from the dashboard.

Authentication secrets are stored in the operating system's credential store (Secret Service on Linux, Keychain on macOS, or Credential Manager on Windows). They are not written to project JSON and are not included when importing a project on another machine; configure them again on each machine. Saving or executing authenticated requests fails explicitly if the credential store is unavailable. Variables, request headers, and other project data are stored unencrypted in JSON. API keys in query parameters may be recorded by servers or proxies; prefer headers when the API allows them.

The light/dark theme preference is stored in the frontend's local storage.

## Code layout

- `internal/domain`: models, validation, and collection/folder operations.
- `internal/comparison`: parallel HTTP requests and response comparison.
- `internal/credentials`: operating-system credential-store access.
- `internal/storage`: JSON persistence and migration.
- `app.go`: Wails-exposed methods and application use-case coordination.
- `frontend/src`: Vue interface; `frontend/dist` contains assets embedded in the desktop executable.
- `main.go`: Wails startup and legacy HTTP handlers retained for compatibility/tests.

Internal packages use concrete dependencies where sufficient; interfaces will be added when there is a real need for decoupling.

## Testing without native WebKit

In CI environments without WebKit/GTK, use the `ci` build tag to replace only the graphical startup:

```sh
go test -count=1 -race -tags ci ./...
go vet -tags ci ./...
```

## License

DiffTwin is distributed under the [MIT License](LICENSE).
