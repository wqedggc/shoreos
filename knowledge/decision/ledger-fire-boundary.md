# Decision: Ledger / FIRE boundary

## Decision

Ledger owns real spending, budgets, sinking funds and rolling spending baselines. FIRE owns manually maintained income/assets/target assumptions and projection runs.

FIRE consumes Ledger capabilities through APIs and does not directly rewrite Ledger facts or query Ledger implementation tables.

## Why

- prevents duplicate sources of truth
- keeps refund/internal-transfer semantics in the accounting layer
- allows FIRE projections to snapshot stable inputs
- makes historical projection runs reproducible even after future ledger changes

## Source

`docs/shared-ledger-fire-contract-v2.md`
