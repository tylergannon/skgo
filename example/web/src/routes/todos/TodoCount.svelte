<script lang="ts">
	import { watchCount } from './todos.remote';
</script>

<p>Todos: <strong data-testid="count">{await watchCount()}</strong></p>
