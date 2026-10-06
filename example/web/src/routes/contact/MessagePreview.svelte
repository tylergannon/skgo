<script lang="ts">
  import { previewMessage } from "./contact.remote";

  let body = $state("");
  let receipt = $state<Awaited<ReturnType<typeof previewMessage>>>();
  let failed = $state(false);
</script>

<section aria-label="Message preview">
  <label>
    Preview message
    <input data-testid="preview-body" bind:value={body} />
  </label>
  <button
    type="button"
    data-testid="preview-message"
    disabled={previewMessage.pending > 0}
    onclick={async () => {
      failed = false;
      receipt = undefined;
      try {
        receipt = await previewMessage(body);
      } catch {
        failed = true;
      }
    }}>Preview</button
  >
  {#if receipt}
    <p data-testid="preview-summary">{receipt.summary}</p>
    <p data-testid="preview-caller">{receipt.caller}</p>
    <p data-testid="preview-result-body">{receipt.body}</p>
  {/if}
  {#if failed}
    <p role="alert">The preview could not be loaded.</p>
  {/if}
</section>
