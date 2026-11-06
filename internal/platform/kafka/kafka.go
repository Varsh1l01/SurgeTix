// Package kafka creates the franz-go client shared by producers and consumers.
//
// Conventions from the architecture (PDF p.8, p.24), enforced in later phases:
//   - Events are published only from a service's transactional outbox, never
//     directly from request handlers (ADR-009).
//   - Producers wait for all in-sync replicas (acks=all) (ADR-005).
//   - Each topic is keyed by the entity whose event order matters, and every
//     event carries a unique event_id and the aggregate version so consumers
//     can drop duplicates and stale versions.
//
// No topics are defined here: topic names are open (AMB-03), and topic
// creation belongs to the outbox relay in phase 4. The broker runs with
// auto-creation disabled so a mistyped topic fails loudly.
package kafka

import (
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/Varsh1l01/SurgeTix/internal/platform/service"
)

// NewClient returns a client for the seed brokers with acks=all.
// Extra options (consumer group, topics) are added by each service later.
func NewClient(brokers []string, opts ...kgo.Opt) (*kgo.Client, error) {
	base := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.RequiredAcks(kgo.AllISRAcks()),
	}
	return kgo.NewClient(append(base, opts...)...)
}

// Check returns a readiness probe that requires a broker to answer.
func Check(c *kgo.Client) service.Check {
	return service.Check{Name: "kafka", Fn: c.Ping}
}
