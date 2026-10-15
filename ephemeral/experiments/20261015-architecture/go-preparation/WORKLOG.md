correction: historical roughly 0.8-second candidate timing predates current per-candidate artifact dependency hashing; current stage comparison must include writeArtifact to avoid understating preparation.
decision: use existing goload.View as a bounded comparator because current gen.Project already owns it; the successful zero-fallback result supports composition research, not another cache framework.
correction: legacy Lower fixture does not pin HostView dependency identity; imported-constant HTTP success proves fresh inputs and compilation, not production same-host compatibility.
