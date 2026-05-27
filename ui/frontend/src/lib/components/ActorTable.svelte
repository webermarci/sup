<script lang="ts">
	import { globalState } from '$lib/state.svelte';
	import type { ExposedActor } from '$lib/types';
	import ActionCard from '$lib/components/ActionCard.svelte';

	type ActionKind = 'cast' | 'call';
	type ActionResult = {
		actorID: string;
		kind: ActionKind;
		name: string;
		ok: boolean;
		body: string;
	};

	let dialog: HTMLDialogElement;
	let resultDialog: HTMLDialogElement;
	let selectedActor = $state<ExposedActor | null>(null);
	let actionResult = $state<ActionResult | null>(null);

	function openActor(actor: ExposedActor) {
		selectedActor = actor;
		dialog.showModal();
	}

	function showActionResult(result: ActionResult) {
		actionResult = result;
		resultDialog.showModal();
	}

	function displayBody(body: string) {
		if (!body) return '';

		try {
			return JSON.stringify(JSON.parse(body), null, 2);
		} catch {
			return body;
		}
	}
</script>

<div>
	<h3 class="mb-2!">
		<i class="ri-instance-line font-normal text-(--pico-primary)"></i>
		Actors
	</h3>

	<div class="overflow-x-auto border rounded-lg border-(--pico-table-border-color)">
		<table class="striped mb-0!">
			<thead>
				<tr>
					<th class="border-r border-(--pico-table-border-color)">ID</th>
					<th class="border-r border-(--pico-table-border-color)">Kind</th>
					<th class="border-r border-(--pico-table-border-color)">Metadata</th>
					<th class="w-full">Dependencies</th>
				</tr>
			</thead>

			<tbody>
				{#each globalState.nodes.actors as actor (actor.id)}
					<tr
						class="cursor-pointer border-b border-(--pico-table-border-color) last:border-b-0"
						onclick={() => openActor(actor)}
					>
						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<span class="font-medium whitespace-nowrap">{actor.id}</span>
						</td>

						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<div class="flex items-center">
								<span
									class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap font-mono"
								>
									{actor.spec.kind}
								</span>
							</div>
						</td>

						<td class="border-r border-(--pico-table-border-color) border-b-0!">
							<div class="flex items-center gap-2">
								{#each Object.entries(actor.spec.metadata) as [key, value] (key)}
									<span class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap">
										{key}: {value}
									</span>
								{/each}
							</div>
						</td>

						<td class="border-b-0!">
							<div class="flex items-center gap-2">
								{#each actor.spec.dependencies as dependency (dependency)}
									<span class="rounded bg-(--pico-table-border-color) px-2 py-1 whitespace-nowrap">
										{dependency}
									</span>
								{/each}
							</div>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
</div>

<dialog bind:this={dialog}>
	{#if selectedActor}
		<article>
			<header class="flex items-center justify-between">
				<h3 class="ml-2 mb-0!">
					<i class="ri-instance-line font-normal text-(--pico-primary)"></i>
					{selectedActor.id}
				</h3>

				<button class="secondary" onclick={() => dialog.close()}>
					<i class="ri-close-fill"></i>
					Close
				</button>
			</header>

			{#if selectedActor.control}
				<div class="flex flex-col gap-3">
					{#if selectedActor.control.calls.length > 0}
						<div class="flex flex-col gap-3">
							{#each selectedActor.control.calls as call (call.name)}
								<ActionCard
									actorID={selectedActor.id}
									kind="call"
									name={call.name}
									schema={call.input_schema}
									onResult={showActionResult}
								/>
							{/each}
						</div>
					{/if}

					{#if selectedActor.control.casts.length > 0}
						<div class="flex flex-col gap-3">
							{#each selectedActor.control.casts as cast (cast.name)}
								<ActionCard
									actorID={selectedActor.id}
									kind="cast"
									name={cast.name}
									schema={cast.input_schema}
									onResult={showActionResult}
								/>
							{/each}
						</div>
					{/if}

					{#if selectedActor.control.calls.length === 0 && selectedActor.control.casts.length === 0}
						<p class="mb-0!">No controls exposed.</p>
					{/if}
				</div>
			{:else}
				<p class="mb-0!">No controls exposed.</p>
			{/if}
		</article>
	{/if}
</dialog>

<dialog bind:this={resultDialog}>
	{#if actionResult}
		<article>
			<header class="flex items-center justify-between">
				<div class="flex items-center gap-2">
					<h4 class="mb-0! font-mono">{actionResult.name}</h4>
					<span
						class="rounded bg-(--pico-table-border-color) px-2 py-1 font-mono whitespace-nowrap"
					>
						{actionResult.kind}
					</span>
				</div>

				<button class="secondary" onclick={() => resultDialog.close()}>
					<i class="ri-close-fill"></i>
					Close
				</button>
			</header>

			{#if actionResult.body}
				<div
					class="max-h-96 overflow-auto whitespace-pre rounded bg-(--pico-table-border-color) p-3 font-mono"
				>
					{displayBody(actionResult.body)}
				</div>
			{/if}
		</article>
	{/if}
</dialog>
