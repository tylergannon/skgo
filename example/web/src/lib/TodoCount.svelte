<script lang="ts">
	import { watchCount } from '#lib/todos.remote';
</script>

<p>Todos: <strong data-testid="count">{await watchCount()}</strong></p>
