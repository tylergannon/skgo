decision: Kit page OPTIONS uses GET, HEAD, OPTIONS, POST insertion order, while page 405 uses ENDPOINT_METHODS order (GET, POST, OPTIONS, HEAD); keep separate Allow expectations.
friction: The Actions page deliberately contains a hidden Grace Hopper form, so a whole-document absence check falsely reports fixture mutation -> assert the editable profile input against the literal Ada fixture.
decision: Native remote forms must use the same trusted-origin and development CSRF rule as classic actions and endpoints; a second narrower check rejects requests Kit permits.
