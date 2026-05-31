export type Spec = {
	kind: string;
	dependencies: string[];
	metadata: Record<string, string>;
};

export type Signal<T = unknown> = {
	id: string;
	spec: Spec;
	type: string;
	value: T;
};

export type ActorRegisteredEvent = {
	timestamp: number;
	type: 'actor:registered';
	source_id: string;
	payload: {
		supervisor_id: string;
	};
};

export type ActorStartedEvent = {
	timestamp: number;
	type: 'actor:started';
	source_id: string;
	payload: {
		supervisor_id: string;
	};
};

export type ActorStoppedEvent = {
	timestamp: number;
	type: 'actor:stopped';
	source_id: string;
	payload: {
		supervisor_id: string;
		error?: string;
	};
};

export type ActorRestartingEvent = {
	timestamp: number;
	type: 'actor:restarting';
	source_id: string;
	payload: {
		supervisor_id: string;
		restart_count: number;
		last_error: string;
	};
};

export type SupervisorTerminalEvent = {
	timestamp: number;
	type: 'supervisor:terminal';
	source_id: string;
	payload: {
		error: string;
	};
};

export type SignalUpdatedEvent<T = unknown> = {
	timestamp: number;
	type: 'signal:updated';
	source_id: string;
	payload: {
		value: T;
	};
};

export type Event =
	| ActorRegisteredEvent
	| ActorStartedEvent
	| ActorStoppedEvent
	| ActorRestartingEvent
	| SupervisorTerminalEvent
	| SignalUpdatedEvent;
