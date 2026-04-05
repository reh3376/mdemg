// V0024: SignalState node for persisting signal learner Hebbian strengths.
//
// Each signal code gets one SignalState node, keyed by node_id = "signal-<code>".
// Idempotent: IF NOT EXISTS.

CREATE CONSTRAINT signal_state_node_id IF NOT EXISTS
FOR (s:SignalState) REQUIRE s.node_id IS UNIQUE;
