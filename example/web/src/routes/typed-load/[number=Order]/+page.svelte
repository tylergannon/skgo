<script lang="ts">
  let { data } = $props();
</script>

<h1>Typed Go load</h1>
<p data-testid="typed-load-label">{data.label}</p>
<nav>
  <a href="/typed-load/42">Order 42</a>
  <a href="/typed-load/0">Order zero</a>
</nav>
