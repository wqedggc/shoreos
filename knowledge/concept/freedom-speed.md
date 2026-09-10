# Freedom Speed / 自由速度

## Definition

Freedom Speed is the rate at which the user's expected FIRE date moves under the current asset base, investable cash flow, long-term spending target, and investment-return assumptions.

It answers a different question from a static FIRE Number:

> If nothing changes, when do I gain the option not to work, and how much does a change in recurring cash flow move that date?

## Inputs

- current investable assets
- FIRE target
- monthly investable cash flow
- long-term spending target
- expected asset return
- Ledger rolling spending baseline when available

## Outputs

- current projected FIRE date
- FIRE completion ratio and remaining gap
- monthly investable amount
- target spending pace vs recent actual spending pace
- counterfactual date movement for recurring changes such as `+¥500/month` or `+¥1,000/month` investable cash flow

## Boundary

The FIRE target is driven by long-term minimum spending. Short-term Ledger variance changes the current speed, not the canonical target for the same FIRE scenario.

See `docs/shared-ledger-fire-contract-v2.md`.
