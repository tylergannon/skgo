<script lang="ts">
  let { data, children } = $props();
</script>

<h1>Typed layout request event</h1>
<p data-testid="event-layout-values">{data.eventSection}|{data.eventItem}|{data.eventRoute}|{data.eventUser}|{data.eventSerial}</p>
<nav>
  <a href="/event-layout/red/first/two?route=untracked">Unread sibling</a>
  <a href="/event-layout/red/second/two?route=untracked">Unread item</a>
  <a href="/event-layout/blue/second/two?route=untracked">Tracked section</a>
  <a href="/event-layout/blue/second/two?route=tracked">Track route</a>
  <a href="/event-layout/blue/second/one?route=tracked">Tracked sibling</a>
  <a href="/event-layout/blue/third/one?route=tracked">Tracked item</a>
</nav>
{@render children()}

<style>
  nav { display: flex; flex-wrap: wrap; gap: 0.75rem; }
</style>
