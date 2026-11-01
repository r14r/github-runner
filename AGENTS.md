# Agent guidelines

- Canonical repository: `r14r/github-runner` (the runner manager itself).
- The Go module is `github.com/r14r/github-runner`.
- Every target repository must be supplied explicitly with `--repo OWNER/REPO`
  or `--repository OWNER/REPO`. Never add a default target repository.
- Do **not** introduce a named application or target repository into source,
  manifests, tests, workflow jobs or examples. Use synthetic
  `example-org/demo-app` fixtures only.
- Provision tooling using recognized workflow actions and auditable allowlists;
  never execute third-party workflow code during analysis.
- The runner's registration and workspace volume must survive rebuilds.
  No automatic removal or re-registration; uninstall checks busy state.
- No arbitrary package names or untrusted YAML content in Docker build args.
- `just ci` must not mutate Go source; `just fmt` is explicit.
- Run `just bootstrap`, `just ci`, `just build` and verify generic image CI.
- GitHub public self-hosted runners and Docker socket mounts are security-sensitive;
  both require explicit operator opt-in.
