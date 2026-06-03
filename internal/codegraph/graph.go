package codegraph

// NodeKind classifies a symbol in the graph.
type NodeKind int

const (
	KindFunc      NodeKind = iota
	KindType
	KindInterface
)

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
type NodeID = string

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

// Store is the in-memory graph.
type Store struct {
	Nodes map[NodeID]*Node
	// Outgoing adjacency: from → []Edge
	Out map[NodeID][]Edge
	// Incoming adjacency: to → []Edge
	In map[NodeID][]Edge
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{
		Nodes: make(map[NodeID]*Node),
		Out:   make(map[NodeID][]Edge),
		In:    make(map[NodeID][]Edge),
	}
}

// AddNode inserts or replaces a node.
func (s *Store) AddNode(n *Node) {
	s.Nodes[n.ID] = n
}

// AddEdge inserts an edge (deduplication by From+To+Kind).
func (s *Store) AddEdge(e Edge) {
	for _, existing := range s.Out[e.From] {
		if existing == e {
			return
		}
	}
	s.Out[e.From] = append(s.Out[e.From], e)
	s.In[e.To] = append(s.In[e.To], e)
}
