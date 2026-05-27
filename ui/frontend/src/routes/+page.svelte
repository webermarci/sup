<script lang="ts">
	import SignalTable from '$lib/components/SignalTable.svelte';
	import ActorTable from '$lib/components/ActorTable.svelte';
</script>

<main class="p-5">
	<div class="flex flex-col gap-5">
		<SignalTable />
		<ActorTable />
	</div>
</main>
