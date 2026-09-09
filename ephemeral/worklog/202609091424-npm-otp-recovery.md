correction: A user-supplied npm OTP is a valid one-time bootstrap credential; the failure was the workflow's lack of a safe input channel, not a reason to reject it.
decision: Add a temporary manually dispatched recovery route restricted to immutable tag v0.2.6. It consumes a temporary GitHub NPM_OTP secret, then is removed after registry verification.
