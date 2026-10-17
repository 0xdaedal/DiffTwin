# Development guide

This guide covers building, testing, and releasing DiffTwin. For a description of the application and instructions for using it, see the [English](user-manual.md) or [Spanish](user-manual.es.md) user manual.

## Requirements

- Go 1.27.2.
- Node.js 20 or later and npm.
- Wails CLI v2.16.0.
- Platform-specific Wails dependencies:
  - **Linux (Ubuntu/Debian):** `build-essential`, `pkg-config`, `libgtk-3-dev`, and WebKitGTK. Ubuntu 24.04 uses WebKitGTK 4.1; install `libwebkit2gtk-4.1-dev` and pass the `webkit2_41` build tag. On distributions with WebKitGTK 4.0, omit that tag.
  - **macOS:** Xcode Command Line Tools (`xcode-select --install`).
  - **Windows:** WebView2 Runtime and a C/C++ compiler supported by Wails, such as MSYS2/MinGW-w64.

See the [official Wails v2 installation requirements](https://wails.io/docs/gettingstarted/installation) for platform-specific setup. Run `wails doctor` to diagnose missing dependencies.

## VS Code Dev Container

The `.devcontainer/` configuration provides a reproducible Linux environment with Go 1.27.2, Node.js 20, Wails v2.16.0, and the dependencies for Linux builds. It works with VS Code's **Dev Containers** extension and Docker Desktop/Engine on Linux, Windows, or macOS:

1. Clone the repository and open it in VS Code:

   ```sh
   git clone https://github.com/0xdaedal/DiffTwin.git
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

## Native development

Choose either the **Dev Container** or the **native host** workflow. Both use the same project folder; avoid maintaining separate working copies for editing and testing.

### Prepare the project

Clone the repository and enter its folder:

```sh
git clone https://github.com/0xdaedal/DiffTwin.git
cd DiffTwin
```

Install Go 1.27.2 and Node.js 20 or later if they are not already installed. Install frontend dependencies and the Wails v2.16.0 CLI:

```sh
npm ci --prefix frontend
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
```

Ensure `wails` is on `PATH`: `$HOME/go/bin` on macOS/Linux or `%USERPROFILE%\go\bin` on Windows.

### Linux (Ubuntu/Debian)

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

### macOS

Install the Xcode Command Line Tools once:

```sh
xcode-select --install
```

Then run the app with hot reload:

```sh
wails dev
```

### Windows (PowerShell)

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
   npm test --prefix frontend
   npm exec --prefix frontend -- playwright install chromium
   npm run test:e2e --prefix frontend
   ```

   The Playwright E2E suite starts the frontend and simulates the Wails bridge to test the interface without opening the desktop app. Install Chromium once before running it for the first time.

   On Linux or in the Dev Container, you can also run `go test -count=1 -race -tags ci ./...`. On Windows, the race detector requires a compatible C compiler.

5. Test the desktop GUI with `wails dev` on the system where you intend to use it. **Try example** exercises the app without real credentials.
6. Review the changes with `git diff`. Ensure they contain no secrets or local project data before committing and pushing. Never commit project JSON files or personal credentials.

To disconnect VS Code from the container, run **Dev Containers: Reopen Folder Locally** or **Close Remote Connection** from the Command Palette. This disconnects VS Code but does not stop host applications or terminals. To stop a still-running container, stop it from Docker Desktop or use Docker Engine.

Build native executables on their target operating systems: run `wails build` on macOS or Windows; on Ubuntu 24.04, run `wails build -tags webkit2_41`. Installer generation and automated signing/notarization are not configured yet.

## CI and releases

Pull requests to `main` and pushes to `main` run Go tests, `go vet`, the frontend production build, unit tests, and Playwright E2E tests.

Pushing a stable version tag in `vMAJOR.MINOR.PATCH` format (for example, `v1.2.3`) starts the release workflow. It validates the source, builds native artifacts on Linux, Windows, and macOS runners, then publishes a GitHub Release containing:

- `difftwin-linux-amd64.tar.gz` with the Linux executable.
- `difftwin-windows-amd64.zip` with the Windows executable.
- `difftwin-macos-universal.zip` with a universal Intel/Apple Silicon `.app`.
- `SHA256SUMS` for verifying the downloaded archives.

To publish a release, first ensure the desired commit is on `main`, then create and push a version tag:

```sh
git switch main
git pull --ff-only
git tag -a v1.2.3 -m "DiffTwin v1.2.3"
git push origin v1.2.3
```

Releases are not code-signed or notarized. Operating systems may show security warnings when opening downloaded apps.

## Code layout

- `internal/domain`: models, validation, and collection/folder operations.
- `internal/comparison`: parallel HTTP requests and response comparison.
- `internal/credentials`: operating-system credential-store access.
- `internal/storage`: JSON persistence and migration.
- `app.go`: Wails-exposed methods and application use-case coordination.
- `frontend/src`: Vue interface; `frontend/dist` contains assets embedded in the desktop executable.
- `main.go`: Wails startup and legacy HTTP handlers retained for compatibility/tests.

Internal packages use concrete dependencies where sufficient; interfaces will be added when there is a real need for decoupling.

## License

DiffTwin is distributed under the [MIT License](../LICENSE).

For project context when resuming work in another session or on another machine, see [HANDOFF.md](../HANDOFF.md).
