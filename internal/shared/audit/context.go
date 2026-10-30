package audit

import "context"

// Actor describes who triggered an audited event. Handlers build one from
// the authenticated request (JWT claims + gin.Context fingerprints) and
// stash it in the request context before calling service methods. The
// audit service then snapshots these fields into each audit row.
//
// All fields are optional. A zero Actor produces an audit row with no
// actor attribution, which is valid for system-triggered events.
type Actor struct {
	UserID    string
	Name      string
	Role      string
	IP        string
	UserAgent string
	RequestID string
}

type actorCtxKey struct{}

// WithActor returns a new context carrying the provided actor. If an actor
// is already attached it is replaced.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorCtxKey{}, a)
}

// ActorFromContext extracts the actor, returning a zero Actor if none was
// attached. Callers can treat the result as always non-nil.
func ActorFromContext(ctx context.Context) Actor {
	v, _ := ctx.Value(actorCtxKey{}).(Actor)
	return v
}
