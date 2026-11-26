# Local sessions and stored history

`tracelet run -- command args...` takes exclusive ownership of the selected project's database, binds the receiver and UI, verifies UI readiness and publishes a private session descriptor before starting the application. Its normal output prints the base UI address. Run `tracelet ui` in another terminal for a private browser-opening URL.

`tracelet ui --no-open` prints that URL without opening a browser. If a verified matching session is already running, the command returns immediately. Otherwise it takes ownership and serves stored history in the foreground, without a child or receiver. Press Ctrl+C to stop history mode before using `run` for the same project. Browser-opening failure prints a manual fallback and does not discard the running history session. `ui.open_browser: false` also disables automatic opening when invoking `ui`.

`tracelet status` outputs JSON without starting services or creating storage. An inactive result can mean no descriptor, stale/crashed instance, mismatched identity or unreachable listener. It does not delete stale state. All commands accept `--config PATH` for the selected project.

The database sidecar `.lock` uses an operating-system lock released when the process exits. Its file persists deliberately; deleting a locked file could allow two owners. The private `.session.json` sidecar contains version, project ID, PID, role, actual endpoints, random public instance ID and a separate random API capability. Files are created with private POSIX permissions. Windows file permissions depend on the containing user's directory ACL and require native validation.

Discovery checks literal loopback endpoints, matching project and random instance identity, a two-second timeout, bounded JSON and no redirects or proxies. A PID alone never establishes ownership, and no stale process is killed. A new owner can atomically replace stale discovery state only while holding the lock. Cleanup removes only its own instance descriptor.

The public health endpoint exposes no telemetry or capability. The private browser URL contains `#token=...`; fragments are not sent as HTTP request targets. Treat that URL as access to this local session. The forthcoming browser UI sends the capability in `X-Tracelet-Token` for data requests. Status output and ordinary run logs omit it. This is local access protection, not a multiuser authentication system.

At T09 the page is a minimal session placeholder. Trace lists and waterfall inspection are implemented in T12–T15. History lifecycle is covered by CLI tests, including read-only inactive status, verified discovery, no-open behavior, browser fallback, run/history ownership conflict and cleanup.
