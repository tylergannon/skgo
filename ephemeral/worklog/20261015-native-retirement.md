# Bounded native retirement

scope: Stage1b pairs one synchronous numeric calculation/helper reclamation class with a candidate-backed closure retained in host state. Actual native mappings must survive consumption and disappear after eligible release; compatible retained backing survives later swaps and GC. No browser integration, host recycling, or general value-flow analysis.

decision: Upstream CodeModule.Unload does not establish quiescence. Generated host admission/publication and consumer ownership must select eligible retirement before calling it; source compatibility alone never grants reclamation.

correction: Darwin mincore returned success for the retired address ranges after Unload completed. Replacing that observation with mach_vm_region established the mappings actually disappeared. Use VM-region address coverage on Darwin, not mincore's return code as a mapping oracle. Linux qualification still requires its native run.

trap: Client response arrival can precede the server handler's deferred release. Observe eventual mapping disappearance after release under a bounded deadline; keep the positive mapped observation while the response consumer is explicitly held.
