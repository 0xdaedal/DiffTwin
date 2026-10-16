# DiffTwin User Manual

DiffTwin is a desktop application for comparing HTTP API responses between a stable environment and a candidate environment. An optional second stable environment helps identify differences that already exist between stable instances.

DiffTwin's interface is available in Spanish and English. It follows the operating system language on first launch (English when the system language is English; otherwise Spanish). Use the language selector in the bottom-right corner, next to the theme control, to change it; your choice is remembered on this computer. Where a control is mentioned below, its Spanish label is included in parentheses when useful.

## 1. Install and open DiffTwin

1. Open the [DiffTwin releases page](https://github.com/0xdaedal/DiffTwin/releases/latest).
2. Download the archive for your operating system:
   - **Windows:** `difftwin-windows-amd64.zip`
   - **Linux:** `difftwin-linux-amd64.tar.gz`
   - **macOS:** `difftwin-macos-universal.zip`
3. Extract the archive and launch the application inside.

These downloads are not installers and are not currently code-signed or notarized. Your operating system may show a security warning when you first open the app.

## 2. Create a project

A project holds the API environments, authentication settings, comparison options, collections, and requests for one API or testing effort.

1. On the dashboard, select **New project** (*Nuevo proyecto*).
2. Enter a project name.
3. Enter the base URL for the **Stable primary** environment and the **Candidate** environment. For example:
   - `https://api.example.com`
   - `https://api-staging.example.com`
4. Optionally enter a **Stable secondary** URL. This third environment is used to help distinguish candidate changes from differences between stable instances.
5. Optionally configure [JSON paths to ignore](#ignore-known-json-differences) and shared [variables](#use-variables).
6. Select **Create project** (*Crear proyecto*).

The request path is appended to each project's base URL. Keep environment-specific hostnames in the project URLs and use the same request path to compare environments.

To edit project details later, select **Project settings** (*Configuración del proyecto*) in the top bar.

## 3. Organize requests

Projects organize requests into collections and folders.

1. Select **New collection** (*Nueva colección*) in the workspace sidebar.
2. Open a collection's **···** options menu to add a request or folder, rename it, run the collection, or delete it.
3. To rename a collection or folder, choose **Rename** (*Renombrar*) from its options menu. Edit the name in the list, then press **Enter** to save. Press **Escape** to cancel. Clicking outside the name field also saves the change.
4. To rename a project, open **Project settings** (*Configuración del proyecto*) from the gear control in the top bar, change its name, and save the changes.
5. Select a request to edit it. You can also drag requests or folders onto another folder or collection to move them within the project.
6. Select **Save** (*Guardar*) in the request editor to keep your changes.

### Configure a request

- Choose an HTTP method: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, `HEAD`, or `OPTIONS`.
- Enter the path in the URL field, such as `/v1/users`.
- In **Params**, add query parameters or edit them in the URL.
- In **Headers**, enter a JSON object, for example:

  ```json
  {
    "Accept": "application/json"
  }
  ```

- In **Body**, enter the request body, if needed.
- Select **Save** (*Guardar*) before leaving if you want to keep the request.
- Select **Send** (*Enviar*) to run the request against the configured environments.

DiffTwin runs the configured destinations in parallel. If a request contains variables, they are substituted in its path, query parameters, headers, and body.

## 4. Configure authentication

Authentication is configured separately for each destination in **Configuration of destinations** (*Configuración de destinos*) in the workspace. Select the destination tab, choose an authentication method, enter its details, and select **Save options** (*Guardar opciones*).

Supported methods are:

- **No authentication**
- **Bearer** token
- **Basic** username and password
- **API key** in a header or query parameter

Secrets are stored in the operating system's secure credential store, not in the project JSON. They are not included in exported project files. When you import a project on another computer, configure its credentials again.

If DiffTwin reports that credentials are missing, re-enter them for the selected destination and save the options. The operating system's credential store must be available to save or use authenticated requests.

## 5. Compare responses

After selecting **Send** (*Enviar*), DiffTwin displays a verdict, response details, and differences between the stable primary and candidate destinations.

The comparison includes HTTP status, response headers, and response bodies. JSON object key order is normalized. Some transport-specific or volatile headers are excluded. Latency is shown for context but does not affect the verdict.

When a stable secondary destination is configured, DiffTwin also compares the two stable responses. A difference between them may indicate existing environment noise rather than a candidate-only change.

For JSON responses, differences are shown as paths and values. The interface displays up to 100 differences in a panel. Response bodies are limited to 2 MiB for display.

### Ignore known JSON differences

When creating or editing a project, enter JSON paths in **Ignore JSON paths**, one per line. For example:

```text
$.meta.generatedAt
$.items[*].id
$["field.with.dots"]
```

Exact paths and `*` wildcards are supported. Ignored paths are excluded from the JSON body comparison.

### Use variables

Shared variables are entered as a JSON object in the project settings:

```json
{
  "userId": "123"
}
```

Use a variable with double braces in a request path, query parameter, header, or body, for example `/v1/users/{{userId}}`. Destination-specific variables can override shared values; unspecified variables continue to use their shared values.

**Do not put secrets in project variables.** Project data, including variables, is stored in a JSON file and is not encrypted. Use the authentication settings for credentials.

## 6. Run a collection

Open a collection's options menu and select **Run collection** (*Ejecutar colección*). DiffTwin executes its requests sequentially in the displayed order, shows progress and each request's outcome, and continues if an individual request fails. Select a completed result to open its comparison.

You can still run an individual request at any time with **Send**.

## 7. Import a Postman collection

1. Open a project.
2. Open the options menu at the top of the collections sidebar.
3. Select **Import Postman collection** (*Importar colección de Postman*) and choose a Postman v2.0 or v2.1 JSON file.

DiffTwin preserves supported folders, methods, paths and query parameters, headers, raw bodies, and collection variables. Postman host URLs are not used; the imported requests use the destinations configured in the DiffTwin project. DiffTwin reports unsupported items, such as scripts, collection authentication, and unsupported body formats.

## 8. Import, export, or delete a project

### Export

On the dashboard, select **Export project** (*Exportar proyecto*) on a project card. Choose the file name and location in the save dialog. DiffTwin confirms where the JSON file was saved.

The export contains project configuration and requests, but **does not contain credentials**. Treat exported files as unencrypted project data.

### Import

On the dashboard, select **Import project** (*Importar proyecto*) and choose a DiffTwin project JSON file. If a project with the same identifier already exists, choose one of the displayed options:

- **Replace project** (*Reemplazar proyecto*): replaces its collections and requests with the imported data. Its saved credentials are removed; configure authentication again.
- **Import as copy** (*Importar como copia*): creates a separate project copy.
- **Cancel** (*Cancelar*): leaves the existing project unchanged.

### Delete

Select **Delete** (*Eliminar*) on a project card and confirm the deletion. This permanently removes the project, its collections and requests, and its saved credentials.

## 9. Try the example

Select **Try example** (*Probar ejemplo*) on the dashboard to create or open the `Demo: comparación de APIs` (Demo: API comparison) project. It sends requests to HTTPBin, a public internet service, to demonstrate a difference in the response URL.

The example requires an internet connection. Do not send private data, credentials, or other sensitive information to the public service.

## 10. Troubleshooting

| Issue | What to try |
| --- | --- |
| A request cannot connect | Check the base URL, request path, network connection, and whether the API is reachable from this computer. |
| An authenticated request reports missing credentials | Open **Configuration of destinations** (*Configuración de destinos*), select the affected destination, re-enter its credentials, and save the options. |
| Credentials cannot be saved | Check that the operating system's secure credential store is available. On Windows, update to a recent DiffTwin release if saving a large set of credentials fails. |
| Headers cannot be saved or used | Make sure the **Headers** field contains valid JSON with quoted property names and string values. |
| The candidate differs, but the cause is unclear | Configure a stable secondary destination. Differences between the two stable responses may indicate existing noise. |
| Imported Postman requests target the wrong server | Set the correct stable and candidate base URLs in the DiffTwin project; Postman hosts are intentionally not imported. |
| The example does not run | Check your internet connection. The example uses a public service. |

## 11. Project data and privacy

Project configuration and requests are stored as JSON in the user's configuration directory. This data is not encrypted and may include URLs, variables, request headers, and bodies. Do not store secrets in those fields or share project files without checking their contents.

Authentication secrets are stored separately in the operating system's secure credential store. They are not included in project exports. API keys sent as query parameters may be recorded by servers, proxies, or other infrastructure; use a header when the API supports it.
