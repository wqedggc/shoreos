# ShoreOS Knowledge

ShoreOS Knowledge is the canonical long-term memory layer for ShoreOS.

It is **not** a chat archive and **not** a second copy of operational databases. The goal is to preserve reusable facts, concepts, decisions and synthesized models with explicit provenance and conflicts.

## Canonical types

- `raw` — source material or an intake record that has not been distilled yet.
- `domain` — durable knowledge about a bounded subject area.
- `concept` — reusable models, definitions, mechanisms and rules.
- `entity` — durable knowledge about a concrete thing such as ShoreOS, a project, service or account class.
- `decision` — an explicit decision, its context, alternatives and consequences.
- `synthesis` — conclusions that integrate multiple sources or lower-level notes.

## Ingestion workflow

```text
source / conversation / file
        ↓
raw capture
        ↓
source summary
        ↓
impact analysis
        ↓
update relevant domain / concept / entity / decision pages
        ↓
update indexes and change log
        ↓
optional synthesis
```

If a new source conflicts with existing knowledge, do not silently overwrite the old claim. Record the contradiction, provenance, and the current resolution status.

## Source of truth

Git + Markdown in this directory is canonical. The ShoreOS UI may provide search, browsing and Inbox capture, but UI/localStorage data is only a staging layer until it is distilled and committed here.

Operational facts remain owned by their operational modules. For example, Ledger owns transaction facts; FIRE owns projection inputs and runs. Knowledge may explain or link those facts but must not duplicate them as an alternative database.

## Naming

Prefer stable, human-readable kebab-case filenames. One durable subject should have one canonical page; update it instead of creating `v2`, `final`, or duplicate pages.
