# Continuar DiffTwin

## Estado actual

DiffTwin es una aplicación de escritorio Go/Wails para comparar peticiones HTTP con estable principal y candidata, más una estable secundaria opcional para detectar ruido. La interfaz está en Vue 3/Vite; el puente de Wails expone operaciones de proyectos, colecciones, importación Postman y comparación. La marca y el módulo Go ya se renombraron; las ubicaciones de datos y credenciales existentes conservan compatibilidad con DiffBench.

La UI nueva incluye dashboard con tarjetas, selector de proyecto, navegación de colecciones y carpetas anidadas, editor con pestañas Params/Headers/Body y barra de método/URL/Enviar fija al desplazarse, comparación en tres destinos (incluido el detalle por rutas de JSON que difiere entre las dos estables) y tema claro/oscuro. La comparación considera status, headers de respuesta (excepto campos variables/de transporte como `Date`, `Content-Length` y `CF-Ray`; `NEL` y `Report-To` sí se comparan) y cuerpo; las latencias se muestran pero no afectan el veredicto. Las rutas JSON variables se pueden excluir con patrones exactos o comodines en las opciones del proyecto. Las variables comunes admiten sobrescrituras por destino que se aplican a rutas, query, headers y body, con fallback al valor compartido. La autenticación es independiente por destino (sin autenticación, Bearer, Basic o API key en header/query); los secretos van al almacén de credenciales del sistema, no a los JSON. La importación Postman v2.0/v2.1 conserva la jerarquía; descarta hosts, mantiene rutas/query, headers, cuerpos raw y variables de colección, y notifica las limitaciones conocidas.

El dashboard incluye **Probar ejemplo**, que crea o abre idempotentemente el proyecto `Demo: comparación de APIs`, selecciona su petición y usa HTTPBin para mostrar un JSON donde la candidata difiere en `$.url`; requiere internet y no debe recibir datos sensibles.

La estructura interna es:

- `internal/domain`: entidades, reglas y operaciones recursivas en colecciones.
- `internal/comparison`: cliente HTTP y comparación de resultados.
- `internal/credentials`: acceso al almacén de credenciales del sistema.
- `internal/storage`: archivos JSON y migración.
- `app.go`: casos de uso expuestos a Wails.
- `frontend/src`: Vue y estilos; `frontend/dist`: recursos compilados y embebidos.
- `main.go`: arranque y handlers HTTP antiguos conservados para pruebas/compatibilidad.

## Mejoras pendientes

Avanzar de una en una; todavía no están implementadas:

1. **Ejecución de colecciones:** permitir lanzar todas las peticiones de una colección y mostrar el progreso y los resultados de cada una.
2. **Historial de comparaciones:** permitir consultar ejecuciones anteriores y revisar sus respuestas y diferencias.

La siguiente mejora acordada es la ejecución de colecciones. Definir su interacción y persistencia al empezar esa tarea, conservando el flujo actual de ejecución individual.

## Ejecutar

Linux requiere Go 1.22+, Node.js 20+ para compilar la UI, GTK 3 y WebKitGTK 4.1. Instala en Ubuntu/Debian:

```sh
sudo apt install -y golang-go nodejs npm build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
```

Con WebKitGTK 4.1, ejecuta `go run -tags 'production webkit2_41' .`, usa `wails dev -tags webkit2_41` para desarrollo y `wails build -tags webkit2_41` para compilar. Con WebKitGTK 4.0 se omite `webkit2_41`. Instrucciones completas para las tres plataformas en `README.md`.

## Validación y limitaciones conocidas

- Pasan `go test -count=1 -race -tags ci ./...` y `go vet -tags ci ./...`.
- `npm run build` en `frontend/` genera `frontend/dist`.
- Se comprobó compilación cruzada Windows/amd64 con `CGO_ENABLED=0`.
- El arranque nativo Linux se verificó con WebKitGTK 4.1 y los tags `production webkit2_41`; `timeout` cerró la ventana tras la prueba.

## Continuar en otro equipo

La carpeta local aún no está configurada como repositorio Git ni tiene remoto de GitHub. Para publicar el código, inicializa Git, excluye configuración local y artefactos, y crea el repositorio público acordado con el nombre DiffTwin en la cuenta `jrcasanya`. Para trasladar proyectos, copia sus JSON desde `~/.config/DiffTwin/projects/` (o desde la carpeta DiffBench anterior, que se reutiliza si existe) o impórtalos desde la aplicación y vuelve a configurar sus credenciales en el otro equipo. Los secretos no se exportan; las variables, headers y otros datos del proyecto sí se guardan sin cifrar.
