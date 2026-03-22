// V0022: J17 AI-to-AI Communication Protocol
// Adds constraint_code property for T1 encoding + index for fast lookup

// Add constraint_code property index on Observation nodes (used for constraint codes)
CREATE INDEX idx_observation_constraint_code IF NOT EXISTS
FOR (n:Observation)
ON (n.constraint_code);
