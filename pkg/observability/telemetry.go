package observability

import "log"

// Streamer emits structured events to stdout until real exporters exist.
type Streamer struct{}

// Publish writes the event into the developer logs so hooks can be traced during early prototyping.
func (Streamer) Publish(event string) {
	log.Printf("observability placeholder event: %s", event)
}
