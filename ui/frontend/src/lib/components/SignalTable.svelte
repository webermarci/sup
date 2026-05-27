<script lang="ts">
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
	<h3 class="mb-2!">
		<i class="ri-signal-tower-line font-normal text-(--pico-primary)"></i>
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
									class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap font-mono flex gap-1 items-center"
								>
									{#if signal.spec.kind === 'computed_signal'}
										<i class="ri-formula text-emerald-500"></i>
									{:else if signal.spec.kind === 'pushed_signal'}
										<i class="ri-arrow-right-line text-sky-500"></i>
									{:else if signal.spec.kind === 'periodic_signal'}
										<i class="ri-timer-line text-purple-500"></i>
									{:else if signal.spec.kind === 'throttled_signal'}
										<i class="ri-pulse-fill text-amber-500"></i>
									{:else if signal.spec.kind === 'debounced_signal'}
										<i class="ri-pulse-fill text-rose-500"></i>
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
										<i class="ri-toggle-fill text-green-500 text-2xl mb-0.5"></i>
									{:else}
										<i class="ri-toggle-line text-slate-400 text-2xl mb-0.5"></i>
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
				<h3 class="ml-2 mb-0!">
					<i class="ri-signal-tower-line font-normal text-(--pico-primary)"></i>
					{selectedSignal.id}
				</h3>
				<button class="secondary" onclick={() => dialog.close()}>
					<i class="ri-close-fill"></i>
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
												<i class="ri-toggle-fill text-green-500 text-2xl mb-0.5"></i>
											{:else}
												<i class="ri-toggle-line text-slate-400 text-2xl mb-0.5"></i>
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
