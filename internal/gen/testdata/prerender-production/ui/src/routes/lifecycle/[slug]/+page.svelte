<script lang="ts">let { data } = $props();</script>
<h1>Lifecycle</h1><p>{data.layout}</p><p>{data.page}</p><p>{data.remote}</p><p>{data.fetched}</p><p>{data.universalFetched}</p><p>{data.address}</p>{#await data.later then value}<p>{value}</p>{/await}
