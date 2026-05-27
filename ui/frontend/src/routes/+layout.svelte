<script lang="ts">
	import './layout.css';
	import 'remixicon/fonts/remixicon.css';
	import '@fontsource-variable/inter';
	import '@fontsource-variable/jetbrains-mono';

	import { onDestroy, onMount } from 'svelte';
	import { fetchNodes } from '$lib/api';
	import { globalState } from '$lib/state.svelte';
	import type { SignalUpdate } from '$lib/types';

	let { children } = $props();
	let eventSource: EventSource;

	async function fetchAll() {
		const [nodes] = await Promise.all([fetchNodes()]);
		globalState.nodes = nodes;
	}

	onMount(() => {
		eventSource = new EventSource('http://localhost:8080/api/events');

		eventSource.addEventListener('error', (e) => {
			console.error(e);
		});

		eventSource.addEventListener('signal:update', (event) => {
			const update = JSON.parse(event.data) as SignalUpdate;
			for (const signal of globalState.nodes.signals) {
				if (signal.id === update.id) {
					signal.value = update.value;
					break;
				}
			}
			globalState.addSignalUpdate(update);
		});
	});

	onDestroy(() => {
		if (eventSource) {
			eventSource.close();
		}
	});
</script>

<svelte:head>
	<title>Sup Dashboard</title>
</svelte:head>

{#await fetchAll() then}
	{@render children()}
{/await}
