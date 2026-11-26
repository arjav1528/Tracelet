# Tracelet browser interface

React and TypeScript render the local API. Vite produces the checked-in `dist/`
bundle embedded by `web.FS()`; the installed Go binary needs no Node runtime,
development server, web fonts, CDN, or external requests.

From this directory:

```sh
npm ci
npm test
npm run typecheck
npm run lint
npm run build
```

Use the real binary and its printed private `tracelet ui` link for integration.
The standalone development server is for development only and does not proxy
private API traffic. Tests use synthetic fixtures under `test/`.

The private fragment token is removed before requests and retained only in
per-tab session storage. Every API request sends `X-Tracelet-Token`; a stale
token requires a fresh CLI link. Paging keeps a bounded 50 rows in memory,
restarts on revision changes, and aborts superseded requests. Nanoseconds remain
decimal strings; formatting uses `BigInt` before converting whole milliseconds
to `Date`. The summary selection is based on the returned trace-list record.

Exact package versions were checked against the official npm registry on
2026-11-26. Primary documentation: [React versions](https://react.dev/versions),
[TypeScript configuration](https://www.typescriptlang.org/docs/handbook/tsconfig-json.html),
[Vite](https://vite.dev/guide/), and [Vitest](https://vitest.dev/guide/).
