package simple

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/grafana-app-sdk/operator"
)

// sharedWatches records the entries registered under each informer controller key. The controller sends every
// informer's events to every handler under the same key, so each entry must also be checked against the others.
type sharedWatches struct {
	mu    sync.Mutex
	byKey map[string][]sharedWatch
}

// sharedWatch is one registered entry: what its handlers need, and what its informer provides (if it says).
type sharedWatch struct {
	needs    operator.WatchOptions
	provided operator.WatchOptions
	stated   bool
}

// register builds the entry's informer, checks it against the entries already under key, and then adds it with add.
// If any step fails, nothing is recorded.
//
// The check uses operator.WatchOptions.RefusedDeliveries. Each entry has its own informer for what it asked for, so
// a missing or split channel in another entry's informer is fine. Less detail, or no sharding when sharding is
// required, is not.
func (s *sharedWatches) register(
	key string, req operator.InformerRequest, add func(operator.Informer, string) error,
) error {
	informer, err := operator.BuildInformer(req)
	if err != nil {
		return err
	}
	// Copy both sides: callers may reuse and change the channel pointers later.
	entry := sharedWatch{needs: req.Options.WatchNeeds.WithDefaults()}
	entry.provided, entry.stated = operator.InformerWatchOptions(informer)
	entry.provided = entry.provided.WithDefaults()

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.byKey[key] {
		if refused := existing.refuses(entry); len(refused) > 0 {
			return sharedWatchError(key, refused)
		}
		if refused := entry.refuses(existing); len(refused) > 0 {
			return sharedWatchError(key, refused)
		}
	}
	if err := add(informer, key); err != nil {
		return err
	}
	if s.byKey == nil {
		s.byKey = make(map[string][]sharedWatch)
	}
	s.byKey[key] = append(s.byKey[key], entry)
	return nil
}

// refuses describes the deliveries from other's informer that this entry's handlers cannot accept. An informer that
// does not say what it provides is not checked.
func (w sharedWatch) refuses(other sharedWatch) []string {
	if !other.stated {
		return nil
	}
	return w.needs.RefusedDeliveries(other.provided)
}

func sharedWatchError(key string, refused []string) error {
	return fmt.Errorf("kind '%s' is watched by more than one entry, and every informer for it delivers to every one"+
		" of its reconcilers and watchers: one of them makes %s, which another entry's handlers cannot take. Give"+
		" the entries the same declaration, or watch the kind once", key, strings.Join(refused, " and "))
}
