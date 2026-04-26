package detector

import (
	"strings"

	"github.com/olgasafonova/ridge/internal/model"
)

const maxTraceDepth = 20

// terminalNodeTypes are node types where a trace naturally terminates.
var terminalNodeTypes = map[model.NodeType]bool{
	model.NodeDatabase:    true,
	model.NodeQueue:       true,
	model.NodeCache:       true,
	model.NodeExternalAPI: true,
}

// ComputeTraces builds process traces from endpoint nodes through the resolved
// edge graph to terminal nodes (databases, queues, caches, external APIs) or
// leaf nodes with no outgoing edges. Traces with identical chains are deduplicated.
func ComputeTraces(graph *model.ArchGraph) []model.ProcessTrace {
	// Build adjacency from resolved edges.
	edges := graph.ResolvedEdges()
	adj := make(map[string][]*model.Edge)
	for _, e := range edges {
		adj[e.Source] = append(adj[e.Source], e)
	}

	// Find entry points: endpoint nodes, or nodes that serve endpoints.
	entryPoints := graph.NodesByType(model.NodeEndpoint)

	// Also consider nodes that have endpoint edges (source side of "serves" edges)
	// as potential starting points if they have endpoints as targets.
	entryIDs := make(map[string]bool, len(entryPoints))
	for _, ep := range entryPoints {
		entryIDs[ep.ID] = true
	}

	var traces []model.ProcessTrace
	seen := make(map[string]bool) // dedup by chain key

	for _, ep := range entryPoints {
		// DFS from each endpoint
		var dfs func(nodeID string, chain []string, edgeTypes []string, minConf float64, visited map[string]bool)
		dfs = func(nodeID string, chain []string, edgeTypes []string, minConf float64, visited map[string]bool) {
			if len(chain) > maxTraceDepth {
				return
			}

			outgoing := adj[nodeID]
			if len(outgoing) == 0 {
				// Leaf node: record trace if chain has at least 2 nodes.
				if len(chain) >= 2 {
					key := strings.Join(chain, "|")
					if !seen[key] {
						seen[key] = true
						traces = append(traces, model.ProcessTrace{
							EntryPoint: chain[0],
							Chain:      copySlice(chain),
							EdgeTypes:  copySlice(edgeTypes),
							Terminal:   chain[len(chain)-1],
							Confidence: minConf,
						})
					}
				}
				return
			}

			for _, e := range outgoing {
				if visited[e.Target] {
					continue
				}

				conf := minConf
				if e.Confidence > 0 && e.Confidence < conf {
					conf = e.Confidence
				}

				targetNode := graph.GetNode(e.Target)
				nextChain := append(chain, e.Target)
				nextEdgeTypes := append(edgeTypes, string(e.Type))

				// Terminal node: record trace and stop.
				if targetNode != nil && terminalNodeTypes[targetNode.Type] {
					key := strings.Join(nextChain, "|")
					if !seen[key] {
						seen[key] = true
						traces = append(traces, model.ProcessTrace{
							EntryPoint: nextChain[0],
							Chain:      copySlice(nextChain),
							EdgeTypes:  copySlice(nextEdgeTypes),
							Terminal:   e.Target,
							Confidence: conf,
						})
					}
					continue
				}

				visited[e.Target] = true
				dfs(e.Target, nextChain, nextEdgeTypes, conf, visited)
				delete(visited, e.Target)
			}
		}

		visited := map[string]bool{ep.ID: true}
		dfs(ep.ID, []string{ep.ID}, nil, 1.0, visited)
	}

	if traces == nil {
		traces = []model.ProcessTrace{}
	}
	return traces
}

func copySlice(s []string) []string {
	c := make([]string, len(s))
	copy(c, s)
	return c
}
