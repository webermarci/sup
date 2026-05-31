<script lang="ts">
	import { Play, Pause, Box, Cog, RadioTower } from '@lucide/svelte';
	import { globalState } from '$lib/state.svelte';
	import type { Event } from '$lib/types';

	let search = $state('');
	let eventsSnapshot = $state<Event[]>([]);

	let filteredEvents = $derived(
		eventsSnapshot.length === 0
			? [...globalState.events.values()]
					.flat()
					.sort((a, b) => b.timestamp - a.timestamp)
					.filter(
						(event) =>
							event.type.toLocaleLowerCase().includes(search.toLocaleLowerCase()) ||
							event.source_id.toLocaleLowerCase().includes(search.toLocaleLowerCase()) ||
							JSON.stringify(event.payload).toLocaleLowerCase().includes(search.toLocaleLowerCase())
					)
			: eventsSnapshot
	);

	function handlePause() {
		if (eventsSnapshot.length === 0) {
			eventsSnapshot = filteredEvents;
		} else {
			eventsSnapshot = [];
		}
	}
</script>

<section class="flex flex-col gap-4">
	<div class="flex gap-2 items-center">
		<div class="relative">
			<input type="text" class="input pl-9 pr-20" placeholder="Search..." bind:value={search} />
			<div
				class="absolute left-3 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground [&>svg]:size-4"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					width="24"
					height="24"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<circle cx="11" cy="11" r="8" />
					<path d="m21 21-4.3-4.3" />
				</svg>
			</div>
			<div
				class="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground text-sm"
			>
				{#if filteredEvents.length > 1}
					{filteredEvents.length} results
				{:else if filteredEvents.length === 1}
					1 result
				{:else}
					No results
				{/if}
			</div>
		</div>

		<div class="flex gap-2 items-center">
			<button type="button" class="btn-outline" onclick={handlePause}>
				{#if eventsSnapshot.length === 0}
					<Pause color="var(--muted-foreground)" />
					Pause
				{:else}
					<Play color="var(--muted-foreground)" />
					Resume
				{/if}
			</button>
		</div>
	</div>

	<div class="overflow-auto border dark:border-(--foreground)/20 rounded-lg shadow-xs">
		<table class="table">
			<thead>
				<tr class="bg-mist-50 dark:bg-mist-950">
					<th class="font-semibold border-r">Timestamp</th>
					<th class="font-semibold border-r">Source ID</th>
					<th class="font-semibold border-r">Type</th>
					<th class="font-semibold w-full">Payload</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredEvents as event (event)}
					<tr>
						<td class="font-mono border-r text-(--muted-foreground)">
							{new Date(event.timestamp).toLocaleTimeString(undefined, {
								hour: '2-digit',
								minute: '2-digit',
								second: '2-digit',
								fractionalSecondDigits: 3
							})}
						</td>
						<td class="font-medium border-r">
							{event.source_id}
						</td>
						<td class="border-r">
							<span class="badge-outline bg-(--secondary)">
								{#if event.type.includes('signal')}
									<RadioTower size={16} color="oklch(69.6% 0.17 162.48)" />
								{:else if event.type.includes('actor')}
									<Box size={16} color="oklch(68.5% 0.169 237.323)" />
								{:else if event.type.includes('supervisor')}
									<Cog size={16} color="oklch(62.7% 0.265 303.9)" />
								{/if}
								{event.type}
							</span>
						</td>
						<td>
							<span class="font-mono font-medium text-(--muted-foreground)">
								{JSON.stringify(event.payload, null, 2)}
							</span>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</section>
