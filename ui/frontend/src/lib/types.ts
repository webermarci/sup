export type Node = {
	name: string;
	spec: Spec;
	type: string;
	value: unknown;
};

export type Spec = {
	kind: string;
	dependencies: string[];
	metadata: Record<string, string>;
};

export type Update = {
	timestamp: number;
	name: string;
	value: unknown;
};
