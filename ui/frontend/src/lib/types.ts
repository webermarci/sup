export type ExposedActor = {
	id: string;
	spec: Spec;
	control?: Control;
};

export type ExposedSignal = {
	id: string;
	spec: Spec;
	type: string;
	value: unknown;
};

export type Spec = {
	kind: string;
	dependencies: string[];
	metadata: Record<string, string>;
};

export type Control = {
	casts: CastAction[];
	calls: CallAction[];
};

export type CastAction = {
	name: string;
	input_schema: JSONSchema;
};

export type CallAction = {
	name: string;
	input_schema: JSONSchema;
	output_schema: JSONSchema;
};

export type JSONSchema = {
	type?: 'object' | 'string' | 'integer' | 'number' | 'boolean' | 'array' | 'null' | string;
	properties?: Record<string, JSONSchema>;
	required?: string[];
	items?: JSONSchema;
	additionalProperties?: JSONSchema;
};

export type Nodes = {
	actors: ExposedActor[];
	signals: ExposedSignal[];
};

export type SignalUpdate = {
	timestamp: number;
	id: string;
	value: unknown;
};
