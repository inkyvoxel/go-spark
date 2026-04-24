package server

import "context"

type requestAuthStateContextKey struct{}

type requestAuthState struct {
	authenticated bool
}

func contextWithRequestAuthState(ctx context.Context, state *requestAuthState) context.Context {
	return context.WithValue(ctx, requestAuthStateContextKey{}, state)
}

func requestAuthStateFromContext(ctx context.Context) (*requestAuthState, bool) {
	state, ok := ctx.Value(requestAuthStateContextKey{}).(*requestAuthState)
	return state, ok
}

func markRequestAuthenticated(ctx context.Context) {
	if state, ok := requestAuthStateFromContext(ctx); ok && state != nil {
		state.authenticated = true
	}
}
