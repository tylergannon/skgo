decision: User requires current stable Go, starting with 1.27.3. setup-go selects stable rather than installing the go.mod minimum; protected runner tool storage preserves that installation between jobs and cleanup.
correction: Native development explicitly rejects compilers other than toolchain.Go; upgrading workflow selection also requires moving the qualified pin and its enforced module/scaffold declarations to 1.27.3, followed by real Linux and Darwin native qualification.

correction: Runner delivery does not authorize application fixes. The user subsequently explicitly approved merging this Go 1.27.3 alignment; further application work remains separate.
