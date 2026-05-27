import { SvelteMap } from 'svelte/reactivity';
import type { Nodes, SignalUpdate } from '$lib/types';

const MAX_UPDATES_PER_NODE = 100;

export function createState() {
	let nodes = $state<Nodes>({ actors: [], signals: [] });
	const updates = new SvelteMap<string, SignalUpdate[]>();

	function addSignalUpdate(signalUpdate: SignalUpdate) {
		const current = updates.get(signalUpdate.id) ?? [];
		const next = [signalUpdate, ...current];

		if (next.length > MAX_UPDATES_PER_NODE) {
			next.length = MAX_UPDATES_PER_NODE;
		}

		updates.set(signalUpdate.id, next);
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
		addSignalUpdate
	};
}

export const globalState = createState();
