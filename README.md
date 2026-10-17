# DiffTwin

**Compare API responses across environments from a desktop workspace.**

DiffTwin helps teams check whether a candidate API behaves like a stable environment. Organize requests into projects, collections, and folders; run them against two or three destinations; and inspect response differences side by side.

![DiffTwin workspace comparing responses across three API destinations](docs/images/user-manual/response-comparison.png)

[Download the latest release](https://github.com/0xdaedal/DiffTwin/releases/latest) · [User manual (English)](docs/user-manual.md) · [Manual de usuario (español)](docs/user-manual.es.md)

## Why DiffTwin?

- **Compare two or three destinations.** A primary stable and candidate destination are required. Add a secondary stable destination to identify differences that already exist between stable instances.
- **Inspect what changed.** Compare HTTP status, headers, and response bodies. JSON object key order is normalized, and differences are reported by JSON path.
- **Reduce noisy diffs.** Ignore exact or wildcard JSON paths, and compare the two stable responses to distinguish existing environment noise from candidate changes.
- **Keep API work organized.** Group requests into collections and nested folders, search the workspace, and run a collection sequentially.
- **Reuse existing collections.** Import Postman v2.0 and v2.1 collections, including supported folders, requests, and collection variables.
- **Configure authentication per destination.** Use Bearer, Basic, or API key authentication. Secrets are stored in the operating system's credential store, not in project files.
- **Use variables safely.** Share project variables and override them for individual destinations. Project variables are plain JSON; do not put secrets in them.

## Get started

1. Download the archive for your operating system from the [latest release](https://github.com/0xdaedal/DiffTwin/releases/latest).
2. Extract the archive and launch DiffTwin.
3. Select **New project**, enter the base URLs for your primary stable and candidate environments, and optionally add a secondary stable URL.
4. Create a collection and request, then select **Send** to compare the configured destinations.

For screenshots and step-by-step instructions—including authentication, Postman import, comparison rules, export, and project deletion—see the [English manual](docs/user-manual.md) or [Spanish manual](docs/user-manual.es.md).

> [!NOTE]
> Downloads are not code-signed or notarized. Your operating system may show a security warning when opening the app.

## Privacy and project data

Authentication secrets are stored in the operating system's secure credential store (Secret Service on Linux, Keychain on macOS, or Credential Manager on Windows). They are not written to project JSON and are not included when importing or exporting a project. Configure credentials again on each machine.

Project settings, variables, request headers, and bodies are stored as unencrypted JSON in the user's configuration directory. Do not store secrets in these fields. API keys sent as query parameters may be recorded by servers or proxies; prefer headers when the API allows them.

The **Try example** action sends requests to HTTPBin, a public echo service, and requires an internet connection. Do not send private data or credentials to it.

## For developers

DiffTwin is built with Go, Wails, Vue 3, and Vite. The [development guide](docs/development.md) covers prerequisites, native and Dev Container setup, tests, builds, CI, and releases.

## License

DiffTwin is distributed under the [MIT License](LICENSE).
