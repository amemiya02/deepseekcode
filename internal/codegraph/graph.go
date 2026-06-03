package codegraph

// NodeKind classifies a symbol in the graph.
type NodeKind int

const (
	KindFunc      NodeKind = iota
	KindType
	KindInterface
)

// String returns the lowercase name of the kind (e.g. "func", "type").
// NodeKind uses lowercase to match conventional Go source spelling.
func (k NodeKind) String() string {
	switch k {
	case KindFunc:
		return "func"
	case KindType:
		return "type"
	case KindInterface:
		return "interface"
	default:
		return "unknown"
	}
}

// EdgeKind classifies a directed relationship between two nodes.
type EdgeKind int

const (
	EdgeCalls      EdgeKind = iota
	EdgeDefines
	EdgeImplements
	EdgeImports
)

// String returns the uppercase name of the kind (e.g. "CALLS", "DEFINES").
// EdgeKind uses uppercase to distinguish it visually from node/kind names in
// serialized output and log messages; consumers must compare case-sensitively.
func (k EdgeKind) String() string {
	switch k {
	case EdgeCalls:
		return "CALLS"
	case EdgeDefines:
		return "DEFINES"
	case EdgeImplements:
		return "IMPLEMENTS"
	case EdgeImports:
		return "IMPORTS"
	default:
		return "UNKNOWN"
	}
}

// NodeID is a fully-qualified symbol identifier: "<pkg-path>.<Name>".
// Defined as a distinct type (not an alias) to prevent raw strings from
// being passed where a NodeID is expected.
type NodeID string

// Node represents a single named symbol.
type Node struct {
	ID      NodeID
	Kind    NodeKind
	Name    string
	File    string
	Line    int
	Snippet string // first line of declaration
}

// Edge represents a directed relationship.
type Edge struct {
	From NodeID
	To   NodeID
	Kind EdgeKind
}

// Store is the in-memory graph. The adjacency maps are unexported to ensure
// all mutations go through AddEdge, which maintains Out/In consistency and
// deduplication.
type Store struct {
	nodes map[NodeID]*Node
	// Outgoing adjacency: from → []Edge
	out map[NodeID][]Edge
	// Incoming adjacency: to → []Edge
	in map[NodeID][]Edge
}

// NewStore returns an empty Store with all internal maps initialised.
func NewStore() *Store {
	return &Store{
		nodes: make(map[NodeID]*Node),
		out:   make(map[NodeID][]Edge),
		in:    make(map[NodeID][]Edge),
	}
}

// AddNode inserts or replaces a node.
func (s *Store) AddNode(n *Node) {
	s.nodes[n.ID] = n
}

// AddEdge inserts an edge (deduplication by From+To+Kind).
func (s *Store) AddEdge(e Edge) {
	for _, existing := range s.out[e.From] {
		if existing == e {
			return
		}
	}
	s.out[e.From] = append(s.out[e.From], e)
	s.in[e.To] = append(s.in[e.To], e)
}

// Node returns the node with the given ID, or nil if not present.
func (s *Store) Node(id NodeID) *Node {
	return s.nodes[id]
}

// AllNodes returns a slice of all nodes in the store (order unspecified).
func (s *Store) AllNodes() []*Node {
	out := make([]*Node, 0, len(s.nodes))
	for _, n := range s.nodes {
		out = append(out, n)
	}
	return out
}

// OutEdges returns the outgoing edges from the given node (read-only view).
func (s *Store) OutEdges(from NodeID) []Edge {
	return s.out[from]
}

// InEdges returns the incoming edges to the given node (read-only view).
func (s *Store) InEdges(to NodeID) []Edge {
	return s.in[to]
}
