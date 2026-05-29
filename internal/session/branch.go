package session

import (
	"context"
	"fmt"
)

// Replay returns the full effective message history for a session,
// walking parents and concatenating up to each branch_point.
//
// Concretely, for a chain:
//
//	root          (messages 0..N)
//	 └── child A  (branch_point=5, own messages → effective: root[0..4] + A[0..M])
//	     └── child B  (branch_point=3, own → effective: root[0..4] + A[0..2] + B[...])
//
// We never copy messages, so storage stays O(unique) even for deeply
// branched experiment trees.
func (s *Store) Replay(ctx context.Context, sessionID string) ([]Message, error) {
	chain, err := s.walkParents(ctx, sessionID)
	if err != nil {
		return nil, err
	}

	var combined []Message
	for i, link := range chain {
		msgs, err := s.LoadMessages(ctx, link.session.ID)
		if err != nil {
			return nil, fmt.Errorf("loading %s: %w", link.session.ID, err)
		}
		// For non-leaf links in the chain, truncate at branch_point of
		// the NEXT link (the child whose branch_point references this
		// session).
		if i < len(chain)-1 {
			cutoff := chain[i+1].session.BranchPoint
			if cutoff < len(msgs) {
				msgs = msgs[:cutoff]
			}
		}
		combined = append(combined, msgs...)
	}
	// A session interrupted between persisting an assistant tool_call turn and
	// its results would replay with a dangling tool_call that DeepSeek rejects;
	// repair pairing before handing the history back. No-op when already paired.
	return repairDanglingToolCalls(combined), nil
}

// link is one node in a parent → ... → leaf walk.
type link struct {
	session Session
}

// walkParents returns the root-to-leaf chain ending at sessionID.
func (s *Store) walkParents(ctx context.Context, sessionID string) ([]link, error) {
	var chain []link
	id := sessionID
	for id != "" {
		sess, err := s.GetSession(ctx, id)
		if err != nil {
			return nil, err
		}
		chain = append([]link{{session: sess}}, chain...) // prepend
		id = sess.ParentID
		// Cycle guard — schemas can't currently form cycles, but defend
		// against accidental future bugs.
		if len(chain) > 256 {
			return nil, fmt.Errorf("session parent chain too deep (>256), possible cycle starting at %s", sessionID)
		}
	}
	return chain, nil
}
