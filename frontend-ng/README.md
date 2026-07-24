# Lethean Desktop Angular frontend

This standalone Angular application is the production frontend embedded by
`lthn`. It is client-side rendered, uses hash routing inside the Wails WebView,
and keeps the Wails bridge, WebMCP registration, and Angular localisation in
the browser bundle.

## Development server

From this directory:

```bash
npm install
npm start -- --host 127.0.0.1 --port 9245 --hmr --poll 1000
```

For the native hot-reload loop, run `wails3 task dev` at the repository root.
Wails proxies the same Angular development server on port 9245.

## Code scaffolding

Angular CLI includes powerful code scaffolding tools. To generate a new component, run:

```bash
ng generate component component-name
```

For a complete list of available schematics (such as `components`, `directives`, or `pipes`), run:

```bash
ng generate --help
```

## Building

To build the project run:

```bash
ng build
```

The production build writes directly to `../go/cmd/lthn/dist/`, including
`index.html` at that directory's root. This is the directory consumed by
`go/cmd/lthn/embed.go`.

## Running unit tests

Run the CI configuration, including v8 coverage, with:

```bash
npm run test:ci
```

## Additional Resources

For more information on using the Angular CLI, including detailed command references, visit the [Angular CLI Overview and Command Reference](https://angular.dev/tools/cli) page.
