import { dev } from '$app/environment';
import type { Event, Signal } from '$lib/types';

export const BASE_URL = dev ? 'http://localhost:8080' : '';

export async function fetchSignals() {
	const res = await fetch(`${BASE_URL}/signals`);
	if (res.status !== 200) {
		throw new Error(`failed to fetch signals: ${res.statusText}`);
	}
	return (await res.json()) as Signal[];
}

export async function fetchEvents() {
	const res = await fetch(`${BASE_URL}/events`);
	if (res.status !== 200) {
		throw new Error(`failed to fetch events: ${res.statusText}`);
	}
	return (await res.json()) as Record<string, Event[]>;
}
