skgo now uses the released standalone runtime: polytype v1.5.0 generates codecs that import devalue/v5 v5.0.0, at parity with the example's resolved JavaScript devalue 5.9.4. Migrate root/example imports and no-argument prerender UndefinedValue recognition, and regenerate application and Go client codecs.

Remote functions, forms, SSR, streamed promises, transport hooks and prerendering retain their behavior. Each browser mode passed all 186 scenarios with zero skips, failures or retries; pricing and streaming were also inspected in Chrome. The independent reviewer verified generated-output reproducibility, passed all 24 test packages, and demonstrated that wire/transport/streaming and prerender tests fail under targeted mutations. No material findings remain.

Validation: fresh frontend builds, just vet, just test in both modules, tidy drift checks, native Playwright reports. Browser runs exercised the runtime source committed as 9a2f25b; subsequent changes are comments, documentation and retained artifacts. Initial failures and final native reports are retained in ephemeral/skgo-runtime-validation.tar.gz.

The baseline exposed an existing relay timeout test that guessed ordering with timers. Replace the guess with a failure/late-reply handshake while preserving the terminal-failure assertions; ten repetitions pass. The review caught a stale Kit source citation, which is corrected.

Applications that directly imported github.com/tylergannon/polytype/devalue must switch to github.com/tylergannon/devalue/v5 and regenerate bindings. Polytype's devalue/codegen package keeps its import path.
