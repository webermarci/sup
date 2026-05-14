<script lang="ts">
	import './layout.css';
	import { onDestroy, onMount } from 'svelte';
	import { fetchNodes } from '$lib/api';
	import { globalState } from '$lib/state.svelte';
	import type { Update } from '$lib/types';

	let { children } = $props();
	let eventSource: EventSource;

	async function fetchAll() {
		const [nodes] = await Promise.all([fetchNodes()]);

		globalState.nodes = nodes;
	}

	onMount(() => {
		eventSource = new EventSource('/api/events');

		eventSource.addEventListener('error', (e) => {
			console.error(e);
		});

		eventSource.addEventListener('update', (event) => {
			const data = JSON.parse(event.data) as Update;
			for (const node of globalState.nodes) {
				if (node.name === data.name) {
					node.value = data.value;
					break;
				}
			}
			globalState.addUpdate(data);
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
