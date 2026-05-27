<script lang="ts">
	import {
		ArrowRight,
		CircleX,
		RadioTower,
		SquareActivity,
		SquareFunction,
		Timer,
		ToggleLeft,
		ToggleRight
	} from '@lucide/svelte';
	import { globalState } from '$lib/state.svelte';
	import type { ExposedSignal } from '$lib/types';

	let dialog: HTMLDialogElement;
	let selectedSignal = $state<ExposedSignal | null>(null);

	function displayValue(value: unknown) {
		if (typeof value === 'object' && value !== null) {
			return JSON.stringify(value, null, 2);
		}
		if (typeof value === 'boolean') return value ? 'true' : 'false';
		return String(value ?? '');
	}

	function openSignal(signal: ExposedSignal) {
		selectedSignal = signal;
		dialog.showModal();
	}
</script>

<div>
	<h3 class="mb-2! flex gap-2 items-center">
		<RadioTower size={22} color="var(--pico-primary)" />
		Signals
	</h3>
	<div class="overflow-x-auto border rounded-lg border-(--pico-table-border-color)">
		<table class="striped mb-0!">
			<thead>
				<tr>
					<th class="border-r border-(--pico-table-border-color)">ID</th>
					<th class="border-r border-(--pico-table-border-color)">Kind</th>
					<th class="border-r border-(--pico-table-border-color)">Metadata</th>
					<th class="border-r border-(--pico-table-border-color)">Dependencies</th>
					<th class="w-full">Value</th>
				</tr>
			</thead>
			<tbody>
				{#each globalState.nodes.signals as signal (signal.id)}
					<tr
						class="cursor-pointer border-b border-(--pico-table-border-color) last:border-b-0"
						onclick={() => openSignal(signal)}
					>
						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<span class="font-medium whitespace-nowrap">{signal.id}</span>
						</td>
						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<div class="flex items-center">
								<span
									class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap font-mono flex gap-1.5 items-center"
								>
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
							</div>
						</td>
						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<div class="flex items-center gap-2">
								{#each Object.entries(signal.spec.metadata) as [key, value] (key)}
									<span
										class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap font-mono"
									>
										{key}: {value}
									</span>
								{/each}
							</div>
						</td>
						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<div class="flex items-center gap-2">
								{#each signal.spec.dependencies as dependency (dependency)}
									<span
										class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap font-mono"
									>
										{dependency}
									</span>
								{/each}
							</div>
						</td>
						<td class="border-b-0!">
							{#if signal.type === 'boolean'}
								<span class="flex items-center gap-2">
									{#if signal.value}
										<ToggleRight size={18} color="oklch(72.3% 0.219 149.579)" />
									{:else}
										<ToggleLeft size={18} color="oklch(70.4% 0.04 256.788)" />
									{/if}
									<span class="capitalize font-mono">{displayValue(signal.value)}</span>
								</span>
							{:else}
								<span class="font-mono">{displayValue(signal.value)}</span>
							{/if}
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>

<dialog bind:this={dialog}>
	{#if selectedSignal}
		<article>
			<header class="flex items-center justify-between">
				<h3 class="ml-2 mb-0! flex gap-2 items-center">
					<RadioTower size={22} color="var(--pico-primary)" />
					{selectedSignal.id}
				</h3>
				<button class="secondary flex gap-2 items-center shrink-0" onclick={() => dialog.close()}>
					<CircleX size={16} />
					Close
				</button>
			</header>

			<div class="overflow-x-auto border rounded-lg border-(--pico-table-border-color)">
				<table class="striped mb-0!">
					<thead>
						<tr>
							<th class="border-r border-(--pico-table-border-color)">Time</th>
							<th class="w-full">Value</th>
						</tr>
					</thead>
					<tbody>
						{#each globalState.updates.get(selectedSignal.id) ?? [] as update (update)}
							<tr class="border-b border-(--pico-table-border-color) last:border-b-0">
								<td class="border-r border-(--pico-table-border-color) border-b-0!">
									<span class="font-mono whitespace-nowrap">
										{new Date(update.timestamp).toLocaleString(undefined, {
											year: 'numeric',
											month: '2-digit',
											day: '2-digit',
											hour: '2-digit',
											minute: '2-digit',
											second: '2-digit',
											fractionalSecondDigits: 3
										})}
									</span>
								</td>
								<td class="h-12 border-b-0!">
									{#if selectedSignal.type === 'json'}
										<div class="font-mono whitespace-pre">
											{displayValue(update.value)}
										</div>
									{:else if selectedSignal.type === 'boolean'}
										<span class="flex items-center gap-2">
											{#if update.value}
												<ToggleRight size={18} color="oklch(72.3% 0.219 149.579)" />
											{:else}
												<ToggleLeft size={18} color="oklch(70.4% 0.04 256.788)" />
											{/if}
											<span class="capitalize font-mono">{displayValue(update.value)}</span>
										</span>
									{:else}
										<span class="font-mono">{displayValue(update.value)}</span>
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</article>
	{/if}
</dialog>
