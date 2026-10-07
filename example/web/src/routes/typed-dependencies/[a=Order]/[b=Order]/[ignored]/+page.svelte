<script lang="ts">
  let { data } = $props();
</script>

<h1>Typed load dependencies</h1>
<p data-testid="typed-dependency-label">{data.label}</p>
<p>
  Load serial: <span data-testid="typed-dependency-serial">{data.serial}</span>
</p>
<p>
  Untracked value: <span data-testid="typed-dependency-ignored"
    >{data.ignored}</span
  >
</p>
<nav>
  <a href="/typed-dependencies/0/7/first">Unread B</a>
  <a href="/typed-dependencies/42/7/first">Read A</a>
  <a href="/typed-dependencies/42/7/second">Untracked ignored</a>
  <a href="/typed-dependencies/42/7/second?read=b">Read B instead</a>
  <a href="/typed-dependencies/0/7/second?read=b">Now unread A</a>
  <a href="/typed-dependencies/0/42/second?read=b">Now read B</a>
</nav>
