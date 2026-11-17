package repository

import (
	"context"

	"github.com/ra1phdd/zev/internal/entities/history/service"
)

// LastConversation returns the most recently updated completed or failed query
// containing a nonblank initial user prompt or refinement. Ties use descending
// conversation ID. It returns service.ErrNotFound when no conversation qualifies;
// callers can classify storage and cancellation errors with errors.Is.
func (s *Store) LastConversation(ctx context.Context) (service.ConversationView, error) {
	// Match strings.TrimSpace, including Unicode spaces SQLite trim omits by default.
	const whitespace = "\t\n\v\f\r \u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"
	var id string
	err := s.db.QueryRowContext(ctx, `
SELECT v.id FROM conversations v
WHERE v.kind='query' AND v.status IN ('completed','failed')
  AND EXISTS (
    SELECT 1 FROM messages m
    WHERE m.conversation_id=v.id AND m.role='user'
      AND m.kind IN ('initial_prompt','refinement') AND trim(m.content,?) <> ''
  )
ORDER BY v.updated_at DESC,v.id DESC LIMIT 1`, whitespace).Scan(&id)
	if err != nil {
		return service.ConversationView{}, missing(err)
	}
	return s.GetConversation(ctx, id)
}
