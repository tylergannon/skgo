correction: Normal builds must always reuse valid outputs automatically. Remove the opt-in reuse mode; explicit clean is the only cache-discard gesture requested by Tyler.
friction: Native preparation removed the whole SDK directory, including Zig caches, and rewrote identical Swift source. Synchronize source while preserving compiler outputs and unchanged mtimes.
decision: Frontend preparation uses one shared cache for skgo build and skgo native build. It includes generation, dependencies, authored inputs, environment and output bytes; failures and missing outputs cannot satisfy it.
