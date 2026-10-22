# Date transport wire declaration repair

correction: Transported time values must be declared as strings, matching the codec's RFC3339 payload. Describing them as empty objects causes TypeScript contract errors.

implementation: The generator uses Polytype's lowered grammar to recognize Time leaves through named references. Ordinary object payload projection remains unchanged. A focused regression covers Date and a named Date alias.

decision: A hook declared with `satisfies Transport` retains its concrete object shape while checking Kit's contract. A broad type annotation erases the keys needed for generated agreement checks.
