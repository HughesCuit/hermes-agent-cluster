package capability

import "strings"

// Match checks if a node's capabilities satisfy task requirements.
// Returns true if the node has all required capabilities.
func Match(nodeCapabilities, taskRequirements []string) bool {
	if len(taskRequirements) == 0 {
		return true // no requirements = any node can handle it
	}
	capSet := make(map[string]bool, len(nodeCapabilities))
	for _, c := range nodeCapabilities {
		capSet[strings.ToLower(c)] = true
	}
	for _, r := range taskRequirements {
		if !capSet[strings.ToLower(r)] {
			return false
		}
	}
	return true
}
