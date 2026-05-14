<script lang="ts">
	import { globalState } from '$lib/state.svelte';
	import type { Node } from '$lib/types';

	let dialog: HTMLDialogElement;
	let selectedNode = $state<Node | null>(null);

	function displayValue(value: unknown) {
		if (typeof value === 'object' && value !== null) {
			return JSON.stringify(value, null, 2);
		}
		if (typeof value === 'boolean') return value ? 'true' : 'false';
		return String(value ?? '');
	}

	function openNode(node: Node) {
		selectedNode = node;
		dialog.showModal();
	}

	function closeDialog() {
		dialog.close();
	}

	function onBackdropClick(event: MouseEvent) {
		const rect = dialog.getBoundingClientRect();
		const inDialog =
			event.clientX >= rect.left &&
			event.clientX <= rect.right &&
			event.clientY >= rect.top &&
			event.clientY <= rect.bottom;

		if (!inDialog) {
			closeDialog();
		}
	}
</script>

<main class="p-3">
	<div class="overflow-x-auto rounded-lg border border-zinc-300">
		<table class="w-full table-auto border-collapse">
			<thead>
				<tr class="border-b border-zinc-300 bg-zinc-100 text-zinc-600">
					<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
						Name
					</th>
					<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
						Kind
					</th>
					<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
						Metadata
					</th>
					<th scope="col" class="w-full p-2 text-left text-sm font-medium">Value</th>
				</tr>
			</thead>
			<tbody>
				{#each globalState.nodes as node (node.name)}
					<tr
						class="cursor-pointer border-b border-zinc-300 last:border-0 even:bg-zinc-50 hover:bg-slate-100"
						onclick={() => openNode(node)}
					>
						<td class="border-r border-zinc-300 p-2 text-sm font-medium whitespace-nowrap">
							{node.name}
						</td>
						<td
							class="border-r border-zinc-300 p-2 text-left text-sm font-medium whitespace-nowrap"
						>
							<span
								class="rounded border px-2 py-1 text-xs font-normal"
								class:bg-sky-50={node.spec.kind === 'periodic_signal'}
								class:border-sky-300={node.spec.kind === 'periodic_signal'}
								class:text-sky-600={node.spec.kind === 'periodic_signal'}
								class:bg-purple-50={node.spec.kind === 'pushed_signal'}
								class:border-purple-300={node.spec.kind === 'pushed_signal'}
								class:text-purple-600={node.spec.kind === 'pushed_signal'}
								class:bg-emerald-50={node.spec.kind === 'computed_signal'}
								class:border-emerald-300={node.spec.kind === 'computed_signal'}
								class:text-emerald-600={node.spec.kind === 'computed_signal'}
								class:bg-rose-50={node.spec.kind === 'throttled_signal'}
								class:border-rose-300={node.spec.kind === 'throttled_signal'}
								class:text-rose-600={node.spec.kind === 'throttled_signal'}
								class:bg-indigo-50={node.spec.kind === 'debounced_signal'}
								class:border-indigo-300={node.spec.kind === 'debounced_signal'}
								class:text-indigo-600={node.spec.kind === 'debounced_signal'}
							>
								{node.spec.kind}
							</span>
						</td>
						<td class="border-r border-zinc-300 p-2">
							<div class="flex gap-1">
								{#each Object.entries(node.spec.metadata) as [key, value] (key)}
									<span
										class="rounded border border-slate-300 bg-slate-50 px-2 py-1 text-xs whitespace-nowrap text-slate-600"
									>
										{key}: {value}
									</span>
								{/each}
							</div>
						</td>
						<td class="p-2">
							<p class="font-mono text-sm whitespace-nowrap">
								{#if node.type === 'boolean'}
									<span class="flex min-w-0 items-center gap-1">
										<span
											class="h-2 w-2 rounded-full"
											class:bg-green-400={node.value}
											class:bg-red-400={!node.value}
										></span>
										{displayValue(node.value)}
									</span>
								{:else}
									{displayValue(node.value)}
								{/if}
							</p>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>

	<dialog
		bind:this={dialog}
		onclick={onBackdropClick}
		class="mx-auto my-auto rounded-xl p-3 outline-0 backdrop:bg-black/40 lg:max-w-2/3"
	>
		{#if selectedNode}
			<div class="mb-3 overflow-x-auto rounded-lg border border-zinc-300">
				<table class="w-full table-auto border-collapse">
					<thead>
						<tr class="border-b border-zinc-300 bg-zinc-100 text-zinc-600">
							<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
								Name
							</th>
							<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
								Kind
							</th>
							<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
								Metadata
							</th>
							<th scope="col" class="w-full p-2 text-left text-sm font-medium">Dependencies</th>
						</tr>
					</thead>
					<tbody>
						<tr class="border-b border-zinc-300 last:border-0 even:bg-zinc-50">
							<td class="border-r border-zinc-300 p-2 text-sm font-medium whitespace-nowrap">
								{selectedNode.name}
							</td>
							<td
								class="border-r border-zinc-300 p-2 text-left text-sm font-medium whitespace-nowrap"
							>
								<span
									class="rounded border px-2 py-1 text-xs font-normal"
									class:bg-sky-50={selectedNode.spec.kind === 'periodic_signal'}
									class:border-sky-300={selectedNode.spec.kind === 'periodic_signal'}
									class:text-sky-600={selectedNode.spec.kind === 'periodic_signal'}
									class:bg-purple-50={selectedNode.spec.kind === 'pushed_signal'}
									class:border-purple-300={selectedNode.spec.kind === 'pushed_signal'}
									class:text-purple-600={selectedNode.spec.kind === 'pushed_signal'}
									class:bg-emerald-50={selectedNode.spec.kind === 'computed_signal'}
									class:border-emerald-300={selectedNode.spec.kind === 'computed_signal'}
									class:text-emerald-600={selectedNode.spec.kind === 'computed_signal'}
									class:bg-rose-50={selectedNode.spec.kind === 'throttled_signal'}
									class:border-rose-300={selectedNode.spec.kind === 'throttled_signal'}
									class:text-rose-600={selectedNode.spec.kind === 'throttled_signal'}
									class:bg-indigo-50={selectedNode.spec.kind === 'debounced_signal'}
									class:border-indigo-300={selectedNode.spec.kind === 'debounced_signal'}
									class:text-indigo-600={selectedNode.spec.kind === 'debounced_signal'}
								>
									{selectedNode.spec.kind}
								</span>
							</td>
							<td class="border-r border-zinc-300 p-2">
								<div class="flex gap-1">
									{#each Object.entries(selectedNode.spec.metadata) as [key, value] (key)}
										<span
											class="rounded border border-slate-300 bg-slate-50 px-2 py-1 text-xs whitespace-nowrap text-slate-600"
										>
											{key}: {value}
										</span>
									{/each}
								</div>
							</td>
							<td class="p-2">
								{#if selectedNode.spec.dependencies.length > 0}
									<div class="flex flex-wrap gap-1">
										{#each selectedNode.spec.dependencies as dep (dep)}
											<span
												class="rounded border border-slate-300 bg-slate-50 px-2 py-1 text-xs whitespace-nowrap text-slate-600"
											>
												{dep}
											</span>
										{/each}
									</div>
								{:else}
									<span class="text-sm text-slate-500 italic">No dependencies</span>
								{/if}
							</td>
						</tr>
					</tbody>
				</table>
			</div>

			<div class="overflow-x-auto rounded-lg border border-zinc-300">
				<table class="w-full table-auto border-collapse">
					<thead>
						<tr class="border-b border-zinc-300 bg-zinc-100 text-zinc-600">
							<th scope="col" class="border-r border-zinc-300 p-2 text-left text-sm font-medium">
								Time
							</th>
							<th scope="col" class="w-full p-2 text-left text-sm font-medium">Value</th>
						</tr>
					</thead>
					<tbody>
						{#each globalState.updates.get(selectedNode.name) as update (update)}
							<tr class="border-b border-zinc-300 last:border-0 even:bg-zinc-50">
								<td
									class="border-r border-zinc-300 p-2 font-mono text-sm whitespace-nowrap text-slate-600"
								>
									{new Date(update.timestamp).toLocaleString(undefined, {
										year: 'numeric',
										month: '2-digit',
										day: '2-digit',
										hour: '2-digit',
										minute: '2-digit',
										second: '2-digit',
										fractionalSecondDigits: 3
									})}
								</td>
								<td class="p-2 font-mono text-sm">
									{#if selectedNode.type === 'json'}
										<pre>{displayValue(update.value)}</pre>
									{:else}
										{displayValue(update.value)}
									{/if}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</dialog>
</main>
