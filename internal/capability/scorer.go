package capability

// NodeInfo holds the scoring inputs for a node.
type NodeInfo struct {
	ID            string
	Capabilities  []string
	Load          float64 // 0.0 - 1.0, lower is better
	HeartbeatAge  float64 // seconds since last heartbeat, lower is better
}

// Score computes a composite score for a node relative to task requirements.
// Weights: capability 0.5, load 0.3, heartbeat 0.2.
// Returns -1 if the node doesn't match capabilities; otherwise 0.0 - 1.0 (higher is better).
func Score(node NodeInfo, requirements []string) float64 {
	if !Match(node.Capabilities, requirements) {
		return -1
	}

	// Capability score: fraction of requirements matched (1.0 if all match)
	capScore := 1.0
	if len(requirements) > 0 {
		matchCount := 0
		capSet := make(map[string]bool)
		for _, c := range node.Capabilities {
			capSet[c] = true
		}
		for _, r := range requirements {
			if capSet[r] {
				matchCount++
			}
		}
		capScore = float64(matchCount) / float64(len(requirements))
	}

	// Load score: lower load = higher score
	loadScore := 1.0 - node.Load

	// Heartbeat score: more recent = higher score (cap at 60s)
	hbScore := 1.0
	if node.HeartbeatAge > 60 {
		hbScore = 0.0
	} else {
		hbScore = 1.0 - (node.HeartbeatAge / 60.0)
	}

	return capScore*0.5 + loadScore*0.3 + hbScore*0.2
}

// RankNodes ranks nodes by score (descending). Returns only nodes with score >= 0.
func RankNodes(nodes []NodeInfo, requirements []string) []NodeInfo {
	type scored struct {
		node  NodeInfo
		score float64
	}
	var candidates []scored
	for _, n := range nodes {
		s := Score(n, requirements)
		if s >= 0 {
			candidates = append(candidates, scored{node: n, score: s})
		}
	}
	// Simple selection sort (small N)
	for i := 0; i < len(candidates); i++ {
		maxIdx := i
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].score > candidates[maxIdx].score {
				maxIdx = j
			}
		}
		candidates[i], candidates[maxIdx] = candidates[maxIdx], candidates[i]
	}
	result := make([]NodeInfo, len(candidates))
	for i, c := range candidates {
		result[i] = c.node
	}
	return result
}
