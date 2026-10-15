# Bounded native retirement

scope: Stage1b pairs one synchronous numeric calculation/helper reclamation class with a candidate-backed closure retained in host state. Actual native mappings must survive consumption and disappear after eligible release; compatible retained backing survives later swaps and GC. No browser integration, host recycling, or general value-flow analysis.

decision: Upstream CodeModule.Unload does not establish quiescence. Generated host admission/publication and consumer ownership must select eligible retirement before calling it; source compatibility alone never grants reclamation.

correction: Darwin address-only probes sometimes reported retired native ranges as still mapped. Independent recursive VM-region inspection and vmmap confirmed both Munmap calls succeeded and the old code address had been reused by different rw-only backing with a different full kernel object identity. Basic versus recursive region lookup alone did not solve that observation error. Compare original kernel backing identity at the supplied address, not permanent vacancy; Linux qualification still requires its native run.

trap: Client response arrival can precede the server handler's deferred release. Observe eventual mapping disappearance after release under a bounded deadline; keep the positive mapped observation while the response consumer is explicitly held.

failure: Native Linux d5d335d reclaimed both ranges, then crashed on the next Load in runtime.typelinksinit at type.go458. The fault0x41f04018 was retired data base0x41f04000 plus24: the first descriptor's Hash. Runtime typemap deduplication lets the surviving revision refer into a prior revision; upstream Unload removes registrations but does not repair those cross-revision maps. Native Mac passed while reusing retired addresses for the next allocation, so its passing run did not exclude this defect.

decision: Only the positive numeric class omits runtime type discovery before registration, while retaining physical descriptors and type address ranges for execution. This prevents a later revision from canonicalizing to reclaimable backing. Retained revisions preserve upstream type discovery. Include the pinned Go1.27 loader adaptation in preparation identity and qualify the same full fixtures again on both native platforms.

correction: The user clarified integration targets dev until a new version release is ready, with direct checkpoint integration and no intermediate PR/squash ceremony. Do not release a version with the development server still unavailable. Stage3 issue324 includes design followed by implementation, branching from and merging into dev.
