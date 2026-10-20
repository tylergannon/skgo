<script lang="ts">
  import { validateCatalog } from "./catalog.remote";
  let receipt = $state("Ready to validate a catalog name");
  async function check(invalid: boolean) {
    try {
      receipt = await validateCatalog(
        invalid ? ({ name: 42 } as never) : { name: "atlas" },
      );
    } catch (error) {
      const caught = error as {
        body?: { message?: string; reference?: string };
      };
      receipt = `${caught.body?.message}|${caught.body?.reference}`;
    }
  }
</script>

<h1>Remote argument validation</h1>
<button onclick={() => check(true)}>Reject invalid catalog name</button>
<button onclick={() => check(false)}>Accept atlas</button>
<p data-testid="validation-receipt">{receipt}</p>
