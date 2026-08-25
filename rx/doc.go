// Package rx provides shared reactive state and composable channel helpers.
//
// # Getting started
//
// Create a passive Signal for shared state and a Derived value for computed
// state:
//
//	status := rx.NewSignal("status", "starting")
//	summary := rx.NewDerived("summary", func() string {
//		return "service: " + status.Value()
//	}, status)
//
// A Derived value is an actor, so add it to a supervisor when it should keep
// recomputing:
//
//	supervisor := sup.NewSupervisor("root", sup.Transient).AddActors(summary)
//	go supervisor.Run(ctx)
//
// Subscribe to values or change notifications from any goroutine:
//
//	values := summary.Subscribe(ctx)
//	status.Set("ready")
//	value := <-values
//
// WaitFor blocks until a readable's current or future value matches a condition.
// Its context can provide cancellation or a timeout:
//
//	ready, err := rx.WaitFor(ctx, status, func(value string) bool {
//		return value == "ready"
//	})
//
// Update performs an atomic read-modify-write and publishes the result:
//
//	count.Update(func(current int) int {
//		return current + 1
//	})
//
// Map, Filter, Distinct, Transitions, Edges, Debounce, and throttle helpers
// transform ordinary Go channels. They can be nested into pipelines and do not
// require a separate pipeline supervisor.
//
// Map transforms each value and may change its type:
//
//	labels := rx.Map(count.Subscribe(ctx), func(value int) string {
//		return strconv.Itoa(value)
//	})
//
// Filter retains only accepted values:
//
//	even := rx.Filter(count.Subscribe(ctx), func(value int) bool {
//		return value%2 == 0
//	})
//
// Distinct removes consecutive equal values. DistinctFunc uses custom equality,
// which is useful for non-comparable values or measurements with a tolerance:
//
//	unique := rx.Distinct(status.Subscribe(ctx))
//	stable := rx.DistinctFunc(temperature.Subscribe(ctx),
//		func(left, right float64) bool {
//			return math.Abs(left-right) < 0.1
//		},
//	)
//
// Transitions emits distinct changes with their previous and current values.
// TransitionsFunc detects them with custom equality:
//
//	changes := rx.Transitions(status.Subscribe(ctx))
//	for change := range changes {
//		log.Printf("status: %s -> %s", change.From, change.To)
//	}
//
//	temperatureChanges := rx.TransitionsFunc(temperature.Subscribe(ctx),
//		func(left, right float64) bool {
//			return math.Abs(left-right) < 0.1
//		},
//	)
//
// Edges is the boolean specialization of Transitions:
//
//	for edge := range rx.Edges(button.Subscribe(ctx)) {
//		switch edge {
//		case rx.EdgeRising:
//			start()
//		case rx.EdgeFalling:
//			stop()
//		}
//	}
//
// Debounce emits the latest value after the input remains quiet:
//
//	settled := rx.Debounce(query.Subscribe(ctx), 250*time.Millisecond)
//
// ThrottleFirst emits the first value immediately and ignores input until the
// interval ends. ThrottleLatest instead emits the latest value at the end of
// each interval:
//
//	first := rx.ThrottleFirst(temperature.Subscribe(ctx), time.Second)
//	latest := rx.ThrottleLatest(temperature.Subscribe(ctx), time.Second)
//
// # Semantics
//
// Every Signal subscription starts with the current value. Subscriptions and
// helper outputs have capacity one and coalesce pending values to the latest
// value, so a slow subscriber does not block Set. Signal.Set is still a
// publication when the new value equals the old value.
//
// Subscribe and Watch channels close when their context is canceled. Both
// provide an initial notification: Subscribe sends the current value, while
// Watch sends one change notification. Helpers close their output when their
// input closes, so canceling the context used to create the first subscription
// cancels an entire nested helper pipeline. Debounce and throttle intervals
// must be positive.
//
// WaitFor owns and cancels its subscription. It returns ErrReadableClosed if a
// custom Readable closes the subscription before a value matches while the
// context remains active.
//
// Transitions and Edges use the first input value as their baseline without
// emitting it. Repeated equal values do not produce transitions or edges.
//
// Derived recomputation is not transactional across independent dependencies.
// It may publish an intermediate result before converging on the latest state;
// values that must change atomically should be kept together in one Signal.
// Debounce and ThrottleLatest do not flush a pending value when their input
// closes.
//
// It is a signal-based reactive layer, not an implementation of ReactiveX.
//
// See the README for application examples and the package reference at
// https://pkg.go.dev/github.com/webermarci/sup/rx for the complete API.
package rx
