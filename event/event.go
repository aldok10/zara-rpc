// Package event defines the SSE event envelope. Handlers that want
// fine-grained control over the SSE framing (event name, id, retry)
// send *event.Event messages on a server stream; the SSE transport
// renders them as structured events instead of plain "data:" lines.
package event

// Event is an SSE event envelope. Data is the payload; the SSE transport
// marshals it with the negotiated codec.
type Event struct {
	// Name is the optional SSE event type (the "event:" field).
	Name string
	// ID is the optional SSE event id (the "id:" field).
	ID string
	// Retry is the optional reconnection time in milliseconds (the
	// "retry:" field). Zero means the field is omitted.
	Retry int
	// Data is the event payload (the "data:" field).
	Data any
}