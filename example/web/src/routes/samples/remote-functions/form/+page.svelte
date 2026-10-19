<script lang="ts">
  import { getMessages, sendMessage } from "./message.remote";

  const fields = sendMessage.fields;
</script>

<form
  enctype="multipart/form-data"
  {...sendMessage.enhance(async (form) => {
    if (await form.submit().updates(getMessages())) {
      form.element.reset();
    }
  })}
>
  <label>
    Your name
    <input {...fields.from.as("text")} />
  </label>
  {#each fields.from.issues() ?? [] as issue (issue.message)}
    <p class="issue">{issue.message}</p>
  {/each}

  <label>
    Email
    <input {...fields.email.as("text")} />
  </label>
  {#each fields.email.issues() ?? [] as issue (issue.message)}
    <p class="issue">{issue.message}</p>
  {/each}

  <label>
    Message
    <textarea {...fields.body.as("text")}></textarea>
  </label>
  {#each fields.body.issues() ?? [] as issue (issue.message)}
    <p class="issue">{issue.message}</p>
  {/each}

  <label>
    Attachment
    <input {...fields.attachment.as("file")} />
  </label>

  <button>Send</button>
</form>

{#if sendMessage.result}
  <p>Thanks, {sendMessage.result.from}.</p>
{/if}

<ul>
  {#each await getMessages() as message, i (i)}
    <li>{message.from}: {message.body}</li>
  {/each}
</ul>
