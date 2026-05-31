import { SvelteMap } from 'svelte/reactivity';
import type { Event, Signal, SignalUpdatedEvent } from '$lib/types';

const MAX_UPDATES_PER_SIGNAL = 128;

export function createState() {
	let signals = $state<Signal[]>([]);
	const events = new SvelteMap<string, Event[]>();

	function addSignalUpdateEvent(event: SignalUpdatedEvent) {
		const current = events.get(event.source_id) ?? [];
		events.set(event.source_id, [event, ...current].slice(0, MAX_UPDATES_PER_SIGNAL));

		for (const signal of signals) {
			if (signal.id === event.source_id) {
				signal.value = event.payload.value;
				break;
			}
		}
	}

	return {
		get signals() {
			return signals;
		},
		set signals(value) {
			signals = value;
		},

		get events() {
			return events;
		},
		addSignalUpdateEvent
	};
}

export const globalState = createState();
