import type { Nodes } from '$lib/types';

export async function fetchNodes() {
	const res = await fetch('http://localhost:8080/api/');
	if (res.status !== 200) {
		throw new Error(`failed to fetch nodes: ${res.statusText}`);
	}
	return (await res.json()) as Nodes;
}
