<script lang="ts">
	import {
		ArrowRight,
		SquareActivity,
		SquareFunction,
		Timer,
		ToggleLeft,
		ToggleRight,
		X
	} from '@lucide/svelte';
	import { globalState } from '$lib/state.svelte';
	import type { Signal } from '$lib/types';

	let search = $state('');
	let dialog: HTMLDialogElement;
	let selectedSignal = $state<Signal | null>(null);

	let filteredSignals = $derived(
		globalState.signals.filter(
			(e) =>
				e.id.toLocaleLowerCase().includes(search.toLocaleLowerCase()) ||
				e.spec.kind.toLocaleLowerCase().includes(search.toLocaleLowerCase())
		)
	);

	function handleSignalClick(signal: Signal) {
		selectedSignal = signal;
		dialog.showModal();
	}
</script>

<section class="flex flex-col gap-4">
	<div class="flex items-center gap-2">
		<div class="relative">
			<input type="text" class="input pr-20 pl-9" placeholder="Search..." bind:value={search} />
			<div
				class="pointer-events-none absolute top-1/2 left-3 -translate-y-1/2 text-muted-foreground [&>svg]:size-4"
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
				class="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-sm text-muted-foreground"
			>
				{#if filteredSignals.length > 1}
					{filteredSignals.length} results
				{:else if filteredSignals.length === 1}
					1 result
				{:else}
					No results
				{/if}
			</div>
		</div>
	</div>

	<div class="overflow-auto rounded-lg border shadow-xs dark:border-(--foreground)/20">
		<table class="table">
			<thead>
				<tr class="bg-mist-50 dark:bg-mist-950">
					<th class="border-r font-semibold">ID</th>
					<th class="border-r font-semibold">Kind</th>
					<th class="border-r font-semibold">Metadata</th>
					<th class="border-r font-semibold">Dependencies</th>
					<th class="w-full font-semibold">Value</th>
				</tr>
			</thead>
			<tbody>
				{#each filteredSignals as signal (signal.id)}
					<tr class="cursor-pointer" onclick={() => handleSignalClick(signal)}>
						<td class="border-r font-medium">
							{signal.id}
						</td>
						<td class="border-r">
							<span class="badge-outline bg-(--secondary)">
								{#if signal.spec.kind === 'computed_signal'}
									<SquareFunction size={16} color="oklch(69.6% 0.17 162.48)" />
								{:else if signal.spec.kind === 'pushed_signal'}
									<ArrowRight size={16} color="oklch(68.5% 0.169 237.323)" />
								{:else if signal.spec.kind === 'periodic_signal'}
									<Timer size={16} color="oklch(62.7% 0.265 303.9)" />
								{:else if signal.spec.kind === 'throttled_signal'}
									<SquareActivity size={16} color="oklch(76.9% 0.188 70.08)" />
								{:else if signal.spec.kind === 'debounced_signal'}
									<SquareActivity size={16} color="oklch(64.5% 0.246 16.439)" />
								{/if}
								{signal.spec.kind}
							</span>
						</td>
						<td class="border-r">
							<div class="flex gap-1">
								{#each Object.entries(signal.spec.metadata) as [key, value] (key)}
									<span class="badge-outline bg-(--secondary)">{key}: {value}</span>
								{/each}
							</div>
						</td>
						<td class="border-r">
							<div class="flex gap-1">
								{#each signal.spec.dependencies as dependency (dependency)}
									<span class="badge-outline bg-(--secondary)">{dependency}</span>
								{/each}
							</div>
						</td>
						<td class="font-mono font-medium text-(--muted-foreground)">
							{#if signal.type === 'boolean'}
								<span class="flex items-center gap-2 capitalize">
									{#if signal.value}
										<ToggleRight size={16} color="oklch(72.3% 0.219 149.579)" />
									{:else}
										<ToggleLeft size={16} color="oklch(70.4% 0.04 256.788)" />
									{/if}
									{signal.value}
								</span>
							{:else if signal.type === 'json'}
								<span>{JSON.stringify(signal.value)}</span>
							{:else}
								{signal.value}
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</section>

<dialog
	class="dialog"
	onclick={(e) => {
		if (e.target === dialog) {
			dialog.close();
		}
	}}
	bind:this={dialog}
>
	<div class="min-h-0 overflow-hidden">
		<header class="shrink-0">
			<h2>{selectedSignal?.id}</h2>
		</header>

		<section class="flex min-h-0 flex-1 flex-col">
			<div
				class="min-h-0 flex-1 overflow-auto rounded-lg border shadow-xs dark:border-(--foreground)/20"
			>
				<table class="table">
					<thead>
						<tr class="bg-mist-50 dark:bg-mist-950">
							<th class="border-r font-semibold">Timestamp</th>
							<th class="w-full font-semibold">Value</th>
						</tr>
					</thead>
					<tbody>
						{#each globalState.events
							.get(selectedSignal?.id || '')
							?.filter((e) => e.type === 'signal:updated') || [] as event (event)}
							<tr>
								<td class="border-r font-mono text-(--muted-foreground)">
									{new Date(event.timestamp).toLocaleTimeString(undefined, {
										hour: '2-digit',
										minute: '2-digit',
										second: '2-digit',
										fractionalSecondDigits: 3
									})}
								</td>
								<td class="font-mono font-medium">
									{#if selectedSignal?.type === 'boolean'}
										<span class="flex items-center gap-2 capitalize">
											{#if event.payload.value}
												<ToggleRight size={16} color="oklch(72.3% 0.219 149.579)" />
											{:else}
												<ToggleLeft size={16} color="oklch(70.4% 0.04 256.788)" />
											{/if}
											{event.payload.value}
										</span>
									{:else if selectedSignal?.type === 'json'}
										<pre>{JSON.stringify(event.payload.value, null, 2)}</pre>
									{:else}
										{event.payload.value}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</section>

		<button
			class="cursor-pointer"
			type="button"
			aria-label="Close dialog"
			onclick={() => dialog.close()}
		>
			<X />
		</button>
	</div>
</dialog>
