# DiffTwin

DiffTwin es una aplicación de escritorio para organizar peticiones HTTP y comparar dos o tres destinos de una API: estable principal y candidata son obligatorios; la estable secundaria es opcional y ayuda a detectar ruido. Está construida con Go y Wails; la interfaz usa Vue 3 y Vite.

Para retomar el proyecto en otra sesión o equipo, consulta [HANDOFF.md](HANDOFF.md).

## Requisitos

- Go 1.22 o posterior.
- Node.js 20 o posterior y npm, necesarios para reconstruir la interfaz.
- Dependencias nativas de Wails:
  - **Linux (Ubuntu/Debian):** `build-essential`, `pkg-config`, `libgtk-3-dev` y WebKitGTK. Para Ubuntu 24.04 y otras distribuciones con WebKitGTK 4.1, instala `libwebkit2gtk-4.1-dev` y añade el tag `webkit2_41` a los comandos Wails. Con WebKitGTK 4.0 no hace falta ese tag.
  - **macOS:** Xcode Command Line Tools.
  - **Windows:** WebView2 Runtime (incluido en la mayoría de instalaciones actuales de Windows).

Consulta la [guía de instalación de Wails](https://wails.io/docs/gettingstarted/installation) para otros sistemas y distribuciones.

## Ejecutar en Linux (Ubuntu/Debian)

Instala las dependencias del sistema:

```sh
sudo apt update
sudo apt install -y golang-go nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

La aplicación usa los recursos Vue ya compilados y embebidos. Con WebKitGTK 4.1, ejecútala así:

```sh
go run -tags 'production webkit2_41' .
```

Para trabajar en la interfaz con recarga automática, instala el CLI de Wails y arranca el modo desarrollo (con WebKitGTK 4.1):

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.2
wails dev -tags webkit2_41
```

También se puede reconstruir la interfaz de forma independiente:

```sh
cd frontend
npm install
npm run build
cd ..
go run .
```

Para compilar el ejecutable de escritorio (con WebKitGTK 4.1):

```sh
wails build -tags webkit2_41
```

Hay que compilar en cada plataforma con sus toolchains y dependencias; la compilación cruzada y los instaladores todavía no están configurados.

## Funcionalidad

El dashboard muestra los proyectos en tarjetas. **Probar ejemplo** crea o abre un proyecto listo para enviar una petición a HTTPBin, un servicio público de eco: la estable principal y la secundaria apuntan a la misma ruta, mientras que la candidata usa otra; la diferencia aparece en `$.url`. Requiere conexión a internet. No envíes secretos ni datos sensibles a este ejemplo público.

Dentro de cada proyecto se pueden gestionar colecciones, carpetas anidadas y peticiones; cada petición se ejecuta en paralelo contra los destinos configurados. La autenticación se configura independientemente para la estable principal, la candidata y la estable secundaria: sin autenticación, Bearer, Basic o API key en header/query. El editor tiene pestañas Params, Headers y Body; los parámetros de consulta se pueden editar en una tabla o directamente en la URL. DiffTwin compara códigos HTTP, headers de respuesta y cuerpos, normalizando el orden de las claves JSON. Los headers de fecha, longitud del contenido y hop-by-hop se excluyen del veredicto porque suelen variar por transporte o formato; `CF-Ray`, identificador de petición de Cloudflare, también se excluye. Otros headers como `NEL` y `Report-To` se comparan, ya que pueden representar configuración relevante del servidor. Los tiempos se muestran como información comparativa, pero no cambian el veredicto, para evitar falsos positivos por variación de red. En las opciones del proyecto se pueden excluir rutas JSON exactas o con comodines (`$.meta.generatedAt`, `$.items[*].id`, `$["campo.con.puntos"]`); se conservan los índices de arrays al excluir valores. Cuando detecta diferencias entre las dos respuestas estables, también muestra sus rutas y valores para ayudar a distinguir ruido de cambios de la candidata. Las variables comunes se pueden sobrescribir por destino desde el panel bajo las tarjetas; las específicas tienen prioridad y las demás usan el valor común. La interpolación con `{{nombre}}` se aplica a rutas, parámetros, headers y cuerpos.

Las colecciones Postman v2.0/v2.1 se importan desde el workspace conservando carpetas anidadas, métodos, rutas/query, headers, cuerpos raw y variables de colección. El host Postman se descarta y se usan los destinos configurados en el proyecto. Los conflictos de variables preservan el valor existente y se notifican los scripts, autenticación de colección y cuerpos no compatibles que se omiten.

## Datos

Cada proyecto se guarda en su propio JSON en el directorio de configuración del usuario (por ejemplo, `~/.config/DiffTwin/projects/` en Linux). `DIFFTWIN_DATA` permite seleccionar otra ruta; se sigue admitiendo `DIFFBENCH_DATA` para compatibilidad. Si ya existe el directorio de datos de DiffBench, DiffTwin lo reutiliza para conservar los proyectos existentes; las credenciales seguras también se mantienen accesibles. Si la ruta seleccionada apunta a un archivo JSON antiguo, los datos se migran a un directorio hermano con sufijo `.d`; el original queda como respaldo y un marcador impide restaurar proyectos eliminados. También se pueden importar archivos de proyecto desde el dashboard. Los secretos de autenticación se guardan en el almacén de credenciales del sistema (Secret Service en Linux, Keychain en macOS o Credential Manager en Windows); no se escriben en el JSON ni se recuperan al importar un proyecto en otro equipo. Si el almacén del sistema no está disponible, guardar o ejecutar con autenticación falla explícitamente. Las variables, headers de petición y demás datos del proyecto sí permanecen en JSON sin cifrar. Las API keys enviadas en la query pueden quedar registradas por servidores o proxies; es preferible usar headers cuando la API lo permita.

La preferencia de tema claro/oscuro se mantiene en el almacenamiento local de la interfaz.

## Organización del código

- `internal/domain`: modelos, validación y operaciones sobre colecciones/carpetas; no depende de la UI, HTTP ni persistencia.
- `internal/comparison`: envío HTTP paralelo y comparación de respuestas.
- `internal/storage`: persistencia JSON y migración.
- `app.go`: métodos expuestos a la interfaz Wails y coordinación de los casos de uso.
- `frontend/src`: interfaz Vue; `frontend/dist` contiene los recursos compilados que se embeben en el ejecutable.
- `main.go`: inicialización Wails y handlers HTTP retenidos para compatibilidad/pruebas.

Los paquetes internos usan dependencias concretas donde basta; las interfaces se añadirán cuando exista una necesidad real de desacoplamiento.

## Validación sin WebKit nativo

En entornos de CI que no tengan WebKit/GTK se pueden ejecutar las pruebas con `-tags ci`; ese tag sustituye solo el arranque gráfico. Por ejemplo:

```sh
go test -count=1 -race -tags ci ./...
go vet -tags ci ./...
```

## Licencia

DiffTwin se distribuye bajo la licencia [MIT](LICENSE).
