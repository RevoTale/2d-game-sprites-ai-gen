# Workflow

1. Validate strict V5 JSON.
2. Generate, review, dry-run, and deploy the original style guide.
3. Generate one complete unit or one static target.
4. Inspect every emitted review artifact.
5. Record a manual accepted/rejected decision.
6. Dry-run deployment.
7. Deploy the complete unit or static target atomically.

Manifest V11 binds the configuration hash, style-guide hash, provider metadata,
evidence hashes, prompts, raw/recovered/normalized artifacts, metrics, lineage,
reviews, production-start hashes, failures, and deployment evidence.

Interrupted non-rejected runs resume completed calls. Rejected assets start
fresh runs. `--all` is explicit, prints the planned call count, supports
repeatable exclusions, and isolates object failures.

No generation is automatically accepted or deployed. Automated tests use only
the injected fake provider.
