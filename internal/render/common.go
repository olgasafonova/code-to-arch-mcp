package render

import (
	"strings"

	"github.com/olgasafonova/code-to-arch-mcp/internal/model"
)

// FilterNodesByViewLevel filters nodes based on the requested view level.
func FilterNodesByViewLevel(nodes []*model.Node, level ViewLevel) []*model.Node {
	var result []*model.Node
	for _, n := range nodes {
		switch level {
		case ViewSystem:
			// Only services and external APIs
			if n.Type == model.NodeService || n.Type == model.NodeExternalAPI {
				result = append(result, n)
			}
		case ViewContainer:
			// Services, databases, queues, caches, external APIs
			if n.Type != model.NodePackage && n.Type != model.NodeEndpoint {
				result = append(result, n)
			}
		case ViewComponent:
			// Everything
			result = append(result, n)
		}
	}
	return result
}

// SanitizeID replaces characters that are invalid in diagram node IDs.
func SanitizeID(id string) string {
	r := strings.NewReplacer(
		"/", "_",
		":", "_",
		".", "_",
		" ", "_",
		"-", "_",
	)
	return r.Replace(id)
}
