# Workflow

1. Validate strict V6 JSON, including explicit causal `magicSources`.
2. Generate, review, dry-run, and deploy the original style guide.
3. Generate one complete unit, one static target, or one complete static set.
4. Inspect every emitted review artifact.
5. Record a manual accepted/rejected decision.
6. Dry-run deployment.
7. Deploy the complete unit, static target, or complete static set atomically.

Manifest V12 binds the configuration hash, style-guide hash, provider metadata,
evidence hashes, prompts, raw/recovered/normalized artifacts, metrics, lineage,
reviews, production-start hashes, failures, and deployment evidence.

Interrupted non-rejected runs resume completed calls. Rejected assets start
fresh runs. `--all` is explicit, prints the planned call count, supports
repeatable exclusions, and isolates object failures.

No generation is automatically accepted or deployed. Automated tests use only
the injected fake provider.

A `static-set` expands into deployable semantic parts but remains one public
scope. It makes one provider request, derives one shared palette and lineage,
measures the complete recovered set, and applies one shared non-upscaling scale
to every part on its exact `2x` intrinsic canvas. The manifest records the
scale and limiting part/axis. Algorithm-version changes reprocess the saved raw
candidate locally without another provider request. The complete review or
deployment is blocked if any part is missing, stale, malformed, or rejected.
For assembled map review, the generator also emits a flat ignored
`review/runtime-overrides` directory keyed by canonical part ID. A map runtime
may consume it explicitly for disposable previews; it is not a deployment and
must never be copied into tracked production paths.
