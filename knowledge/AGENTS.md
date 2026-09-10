# Knowledge maintenance contract

When an agent updates ShoreOS Knowledge:

1. Preserve provenance. Every durable claim should be traceable to a source, decision, or observed system fact.
2. Keep raw material in `raw/`; do not treat raw chat logs as distilled knowledge.
3. Choose the narrowest canonical type: `domain`, `concept`, `entity`, `decision`, or `synthesis`.
4. Update an existing canonical page when the subject already exists. Do not create duplicate versioned pages.
5. Separate facts from interpretation. Mark assumptions, estimates, and unresolved conflicts explicitly.
6. When information conflicts, record the contradiction instead of silently replacing history.
7. Update `web/static/knowledge-index.json` when a user-facing knowledge item is added, renamed, or materially changed.
8. Do not copy operational Ledger/FIRE data into Knowledge as a competing source of truth. Link or summarize the model/rule instead.
9. Prefer concise durable statements over transcript-like prose.
10. A synthesis must cite or point to the lower-level knowledge it combines.
