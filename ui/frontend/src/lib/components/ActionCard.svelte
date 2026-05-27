<script lang="ts">
	import type { JSONSchema } from '$lib/types';

	type ActionKind = 'cast' | 'call';
	type ActionResult = {
		actorID: string;
		kind: ActionKind;
		name: string;
		ok: boolean;
		body: string;
	};

	let {
		actorID,
		kind,
		name,
		schema,
		onResult
	}: {
		actorID: string;
		kind: ActionKind;
		name: string;
		schema: JSONSchema;
		onResult: (result: ActionResult) => void;
	} = $props();

	let values = $state<Record<string, unknown>>({});
	let jsonText = $state<Record<string, string>>({});
	let sending = $state(false);
	let initializedKey = '';

	let properties = $derived(schema.properties ?? {});
	let fields = $derived(Object.entries(properties));
	let required = $derived(new Set(schema.required ?? []));
	let hasFields = $derived(fields.length > 0);

	function initialValue(fieldSchema: JSONSchema): unknown {
		switch (fieldSchema.type) {
			case 'string':
				return '';
			case 'integer':
			case 'number':
				return 0;
			case 'boolean':
				return false;
			case 'array':
				return [];
			case 'object':
				return {};
			default:
				return null;
		}
	}

	$effect(() => {
		const key = `${actorID}:${kind}:${name}`;

		if (initializedKey === key) return;
		initializedKey = key;

		const nextValues: Record<string, unknown> = {};
		const nextJSONText: Record<string, string> = {};

		for (const [field, fieldSchema] of Object.entries(schema.properties ?? {})) {
			const value = initialValue(fieldSchema);

			nextValues[field] = value;

			if (fieldSchema.type === 'array' || fieldSchema.type === 'object' || !fieldSchema.type) {
				nextJSONText[field] = JSON.stringify(value, null, 2);
			}
		}

		values = nextValues;
		jsonText = nextJSONText;
	});

	function setValue(field: string, value: unknown) {
		values = {
			...values,
			[field]: value
		};
	}

	function parseNumber(value: string) {
		if (value.trim() === '') return 0;

		const parsed = Number(value);
		return Number.isFinite(parsed) ? parsed : 0;
	}

	function payload() {
		if (schema.type !== 'object') {
			return values;
		}

		const out: Record<string, unknown> = {};

		for (const field of Object.keys(properties)) {
			out[field] = values[field];
		}

		return out;
	}

	async function send() {
		sending = true;

		try {
			const res = await fetch(`http://localhost:8080/api/actors/${actorID}/${kind}s/${name}`, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				},
				body: JSON.stringify(payload())
			});

			const body = await res.text();

			onResult({
				actorID,
				kind,
				name,
				ok: res.ok,
				body
			});
		} catch (err) {
			onResult({
				actorID,
				kind,
				name,
				ok: false,
				body: err instanceof Error ? err.message : String(err)
			});
		} finally {
			sending = false;
		}
	}
</script>

{#if hasFields}
	<article class="mb-0! border border-(--pico-table-border-color) shadow-none!">
		<header class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-2">
				<h4 class="mb-0! font-mono">{name}</h4>
				<span class="rounded bg-(--pico-table-border-color) px-2 py-1 font-mono whitespace-nowrap">
					{kind}
				</span>
			</div>

			<button class="mb-0!" disabled={sending} onclick={send}>
				<i class="ri-send-ins-line"></i>
				Send
			</button>
		</header>

		<div class="flex flex-col gap-4">
			<div class="grid gap-3 md:grid-cols-2">
				{#each fields as [field, fieldSchema] (field)}
					<label class="mb-0!">
						<span class="mb-1 font-mono">
							{field}{#if required.has(field)}<span class="text-(--pico-primary)">*</span>{/if}
						</span>

						{#if fieldSchema.type === 'boolean'}
							<input
								class="mb-0!"
								type="checkbox"
								role="switch"
								checked={Boolean(values[field])}
								onchange={(event) => setValue(field, event.currentTarget.checked)}
							/>
						{:else if fieldSchema.type === 'integer'}
							<input
								class="mb-0!"
								type="number"
								step="1"
								value={Number(values[field])}
								oninput={(event) => setValue(field, parseNumber(event.currentTarget.value))}
							/>
						{:else if fieldSchema.type === 'number'}
							<input
								class="mb-0!"
								type="number"
								step="any"
								value={Number(values[field])}
								oninput={(event) => setValue(field, parseNumber(event.currentTarget.value))}
							/>
						{:else if fieldSchema.type === 'string'}
							<input
								class="mb-0!"
								type="text"
								value={String(values[field] ?? '')}
								oninput={(event) => setValue(field, event.currentTarget.value)}
							/>
						{:else}
							<textarea
								class="mb-0! min-h-28 font-mono text-sm"
								rows="4"
								value={jsonText[field] ?? ''}
								oninput={(event) => {
									const text = event.currentTarget.value;

									jsonText = {
										...jsonText,
										[field]: text
									};

									setValue(field, JSON.parse(text));
								}}
							></textarea>
						{/if}
					</label>
				{/each}
			</div>
		</div>
	</article>
{:else}
	<div class="article-header-only border border-(--pico-table-border-color)">
		<header class="flex items-center justify-between gap-4">
			<div class="flex items-center gap-2">
				<h4 class="mb-0! font-mono">{name}</h4>
				<span class="rounded bg-(--pico-table-border-color) px-2 py-1 font-mono whitespace-nowrap">
					{kind}
				</span>
			</div>

			<button class="mb-0!" disabled={sending} onclick={send}>
				<i class="ri-send-ins-line"></i>
				Send
			</button>
		</header>
	</div>
{/if}

<style>
	.article-header-only {
		border-radius: var(--pico-border-radius);
		padding: var(--pico-block-spacing-vertical) var(--pico-block-spacing-horizontal);
		padding-bottom: 0;
	}

	.article-header-only > header {
		margin-top: calc(var(--pico-block-spacing-vertical) * -1);
		border-radius: var(--pico-border-radius);
		margin-right: calc(var(--pico-block-spacing-horizontal) * -1);
		margin-left: calc(var(--pico-block-spacing-horizontal) * -1);
		padding: calc(var(--pico-block-spacing-vertical) * 0.66) var(--pico-block-spacing-horizontal);
		background-color: var(--pico-card-sectioning-background-color);
	}
</style>
