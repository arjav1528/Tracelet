# Engineering decisions

## 2026-11-26 — T00/T01 foundation

- Local author and committer: Arjav Patel <arjav1528@gmail.com>. The initial documentation commit is `695c49a`.
- Work on `feat/tracelet-core` in the new checkout. `main` retains the initial planning baseline. There was no existing application work requiring a second checkout.
- GitHub owner, visibility and license remain open. Local work uses `tracelet.local/tracelet`; replace it and all imports with the verified GitHub module path before publication.
- Pin Go 1.27.4 as the build toolchain (module language floor 1.27.0) and Cobra v1.10.2. Cobra is Apache-2.0 licensed; Go uses its BSD-style license. The installed system Go was 1.27.0, so Go's toolchain management supplies the selected patch version without changing the system installation.
- CI actions are pinned to reviewed release commits: checkout v7.0.1 and setup-go v7.1.0. CI is read-only; no release credentials or publication workflow are introduced at this stage.
- No frontend or ingestion capability is claimed by the version command. Those arrive in their owning tasks.

References: [Go releases](https://go.dev/dl/), [Cobra release](https://github.com/spf13/cobra/releases/tag/v1.10.2), [checkout](https://github.com/actions/checkout/releases/tag/v7.0.1), [setup-go](https://github.com/actions/setup-go/releases/tag/v7.1.0).
