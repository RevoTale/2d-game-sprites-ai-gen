# Workflow

This document describes the sprite draft lifecycle and run artifacts.

## Lifecycle

1. `init` creates a minimal sprite pack.
2. `validate` proves `THEME.md`, `sprites.json`, references, target expansion, and path templates are valid.
3. `generate` creates or resumes a run and generates only matched pending targets.
4. `sheet` assembles review sheets from normalized target images.
5. `review` records visual QA decisions.
6. `deploy-plan` previews what accepted targets would replace and what non-accepted targets would leave unchanged.
7. `deploy` copies only accepted target images.
8. `prune --only-raw` removes raw attempt files after metadata is preserved.

Every expanded target is generated as an independent image. Sheets are review artifacts only and are never cropped into frame sources.

## Run Output

Managed output lives under `output/runs/<run-id>/` by default:

```text
output/
  runs/
    2026-07-03-m0847/
      manifest.json
      targets/
        blood-duelist__attack__direction-right__contact/
          prompt.md
          qa.md
          normalized.png
          attempts/
            001/
              raw-candidate.png
      contact-sheets/
        blood-duelist.png
```

`raw-candidate.png` is provider evidence. `normalized.png` is the deployment candidate. Only accepted normalized target images are copied by `deploy`.

## Status Lifecycle

```text
pending -> generated -> accepted -> deployed
                    \-> rejected -> generated
```

Accepted and deployed targets are protected from regeneration unless `--force` is used. When `--force` creates a new attempt, old attempt and review history remains in `manifest.json`.

## Review Rules

- `accepted` can omit `--reason`.
- `rejected` requires `--reason`.
- Bulk accept writes a default audit note when no reason is provided.
- Pending or missing targets are skipped during broad review and make the command fail unless `--allow-partial` is set.

## Deploy Rules

Partial deploy is the default. Accepted targets are copied, and non-accepted targets are left unchanged.

Use `deploy --complete` when every target in the selected scope must be accepted before any file is copied.

Use `deploy-plan` or `deploy --dry-run` before deploying a mixed run. The plan reports the exact files that will be replaced and the exact files that will stay unchanged.
