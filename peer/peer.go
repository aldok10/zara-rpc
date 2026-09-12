package peer

import "context"

// Peer describes the client that made the request.
type Peer struct {
	// Addr is the remote address.
	Addr string
	// Protocol is the HTTP protocol version.
	Protocol string
}

type peerKey struct{}

// WithPeer stores peer info in the context.
func WithPeer(ctx context.Context, p Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, p)
}

// PeerFromContext returns peer info from the context.
func PeerFromContext(ctx context.Context) (Peer, bool) {
	p, ok := ctx.Value(peerKey{}).(Peer)
	return p, ok
}