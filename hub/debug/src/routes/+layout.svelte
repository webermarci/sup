<script lang="ts">
	import './layout.css';
	import 'basecoat-css/all';

	import { onDestroy, onMount } from 'svelte';
	import { Bug, CalendarClock, RadioTower } from '@lucide/svelte';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { BASE_URL, fetchEvents, fetchSignals } from '$lib/api';
	import type { SignalUpdatedEvent } from '$lib/types';
	import { globalState } from '$lib/state.svelte';

	let { children } = $props();
	let eventSource: EventSource | null = null;

	async function fetchAll() {
		const [events, signals] = await Promise.all([fetchEvents(), fetchSignals()]);
		globalState.signals = signals;

		for (const [id, e] of Object.entries(events)) {
			globalState.events.set(
				id,
				e.sort((a, b) => b.timestamp - a.timestamp)
			);
		}
	}

	onMount(() => {
		const media = window.matchMedia('(prefers-color-scheme: dark)');

		const applyTheme = () => {
			document.documentElement.classList.toggle('dark', media.matches);
		};

		applyTheme();
		media.addEventListener('change', applyTheme);

		return () => media.removeEventListener('change', applyTheme);
	});

	onMount(() => {
		eventSource = new EventSource(`${BASE_URL}/events/stream`);

		eventSource.addEventListener('error', (e) => {
			console.error(e);
		});

		eventSource.addEventListener('signal:updated', (e) => {
			const event = JSON.parse(e.data) as SignalUpdatedEvent;
			globalState.addSignalUpdateEvent(event);
		});
	});

	onDestroy(() => {
		eventSource?.close();
		eventSource = null;
	});
</script>

<svelte:head>
	<title>sup/debug</title>
</svelte:head>

<main>
	<nav class="flex gap-8 w-full border-b p-4 overflow-x-auto">
		<h1 class="font-bold tracking-wide flex items-center gap-1">
			<Bug size={18} class="mt-0.5 shrink-0" />
			sup/debug
		</h1>
		<div class="flex gap-2">
			<a
				class="font-medium text-sm flex items-center gap-1 border px-3 py-2 rounded-lg"
				class:text-(--foreground)={page.url.toString().includes('/signals')}
				class:bg-(--secondary)={page.url.toString().includes('/signals')}
				class:text-(--muted-foreground)={!page.url.toString().includes('/signals')}
				class:border-transparent={!page.url.toString().includes('/signals')}
				href={resolve('/signals')}
			>
				<RadioTower size={16} color="var(--muted-foreground)" class="shrink-0" />
				Signals
			</a>
			<a
				class="font-medium text-sm flex items-center gap-1 border px-3 py-2 rounded-lg"
				class:text-(--foreground)={page.url.toString().includes('/events')}
				class:bg-(--secondary)={page.url.toString().includes('/events')}
				class:text-(--muted-foreground)={!page.url.toString().includes('/events')}
				class:border-transparent={!page.url.toString().includes('/events')}
				href={resolve('/events')}
			>
				<CalendarClock size={16} color="var(--muted-foreground)" class="shrink-0" />
				Events
			</a>
		</div>
	</nav>

	<section class="p-4">
		{#await fetchAll()}
			<p>Loading...</p>
		{:then}
			{@render children()}
		{:catch}
			<p>Error loading data.</p>
		{/await}
	</section>
</main>
