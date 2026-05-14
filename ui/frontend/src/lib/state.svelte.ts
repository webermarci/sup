import { SvelteMap } from 'svelte/reactivity';
import type { Node, Update } from '$lib/types';

const MAX_UPDATES_PER_NODE = 100;

export function createState() {
	let nodes = $state<Node[]>([]);
	const updates = new SvelteMap<string, Update[]>();

	function addUpdate(update: Update) {
		const current = updates.get(update.name) ?? [];
		const next = [update, ...current];

		if (next.length > MAX_UPDATES_PER_NODE) {
			next.length = MAX_UPDATES_PER_NODE;
		}

		updates.set(update.name, next);
	}

	return {
		get nodes() {
			return nodes;
		},
		set nodes(value) {
			nodes = value;
		},

		get updates() {
			return updates;
		},
		addUpdate
	};
}

export const globalState = createState();
