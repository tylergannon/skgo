# Issue 247 plan review

correction: Concrete fixture planning must distinguish current main from active parameter branches; no single existing stable base currently contains all named issue tests. Implement existing-fixture changes first and parameter-fixture changes after integration, with separate comparable baselines.
decision: Checked-in fixture contracts use an explicit allowlist of the current example build bootstrap and dependency pins; never copy demo consumers or maintain independent per-fixture pins.
correction: Splitting the production artifact assertions exposes the existing global transform-middleware slot to ordering/race failures. Use fixture-local per-handler injection and local transform counters.
decision: Keep the deliberately failing layout fixture limited to /about, sharing bootstrap but not the successful route tree, so its diagnostic stays deterministic.
