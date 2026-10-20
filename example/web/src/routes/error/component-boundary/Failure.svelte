<script lang="ts">
  import { error, redirect } from "@sveltejs/kit";
  // @ts-expect-error Kit's runtime-only internal classes have no public TS module.
  // prettier-ignore
  import { HandledHttpError, SvelteKitError, ValidationError } from "@sveltejs/kit/internal";

  let { kind }: { kind: string } = $props();
  function renderFailure() {
    if (kind === "unknown")
      throw new Error("literal-private-component-cause-353");
    if (kind === "app")
      error(409, "literal-public-component-app-353", {
        original: "literal-public-original-353",
      });
    if (kind === "framework")
      throw new SvelteKitError(
        404,
        "literal-public-framework-353",
        "literal-private-framework-cause-353",
      );
    if (kind === "validation")
      throw new ValidationError([
        { message: "literal-private-validation-cause-353" },
      ]);
    if (kind === "handled")
      throw new HandledHttpError({
        status: 418,
        message: "literal-already-handled-353",
        original: "literal-public-original-353",
      });
    if (kind === "redirect") redirect(307, "/about");
    return "Component rendered normally";
  }
</script>

<p>{renderFailure()}</p>
