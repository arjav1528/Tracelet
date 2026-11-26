# Continuous integration

The `CI` workflow runs on pushes, pull requests, and explicit manual dispatches.
Its Ubuntu job installs the pinned Go and Node versions, checks and rebuilds the
embedded web assets, verifies that the asset build is reproducible, runs Go
formatting/vet/tests/race checks, builds the standalone binary, and prints its
version. Actions are pinned to commit SHAs; the workflow has read-only repository
contents permission.

Run the existing workflow for a selected branch:

```sh
gh workflow run ci.yml --repo arjav1528/Tracelet --ref feat/tracelet-core
gh run list --repo arjav1528/Tracelet --workflow ci.yml --limit 10
```

Inspect the resulting run before treating CI as verified. A successful local
test or a pushed commit is not evidence of a completed hosted workflow.

## Initial dispatch investigation

On 2026-11-26, GitHub reported the public repository as enabled, non-archived and
not a fork, repository Actions as enabled, and `CI` as active. The workflow file
was present on both `main` and `feat/tracelet-core`. For pushed feature revision
`c46713eb311b1525afe5b5470eb42cfaaf94132a`, the Actions run lists and GitHub check
run list were empty; its commit message contained no CI-skip annotation. Other
installed applications had queued check suites, but no GitHub Actions suite was
reported. These observations do not establish a cause for the missing push run.

Manual dispatch was added to obtain explicit verification evidence without
changing the pinned tools, weakening checks, or rewriting published history.
GitHub requires the workflow file to exist on the default branch for manual
dispatch; this repository already had that file on `main`. See
[GitHub's manual-run documentation](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow).

This initial workflow targets Ubuntu only. Native macOS/Windows runner evidence
and release packaging belong to T22; cross-compilation alone does not establish
runtime support on those systems. No release publication is performed by CI.
