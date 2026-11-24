package simple

import (
	"fmt"
	"strings"
	"sync"

	"github.com/grafana/grafana-app-sdk/operator"
)

// sharedWatches records what has been registered under each of an informer controller's keys. The controller fans
// every informer's events out to every reconciler and watcher under the same key, so an entry's deliveries reach
// the handlers of every other entry sharing that key, and each entry's own check is not enough.
type sharedWatches struct {
	mu    sync.Mutex
	byKey map[string][]sharedWatch
}

// sharedWatch is one registered entry: the least its handlers can take on each channel, and what its informer
// carries where the informer says.
type sharedWatch struct {
	live     operator.WatchDetail
	resync   operator.WatchDetail
	provided operator.WatchOptions
	stated   bool
}

// register builds this entry's informer, checks the deliveries it and the entries already under key would exchange,
// and adds it with add before recording it, so an informer which fails to register leaves nothing behind.
//
// Only the detail is compared across entries. An extra copy of a delivery takes nothing away from a handler, so a
// channel another entry's informer divides, or does not have at all, serves a handler with its own informer for it;
// affinity would be the exception, and nothing provides it yet.
func (s *sharedWatches) register(
	key string, req operator.InformerRequest, add func(operator.Informer, string) error,
) error {
	informer, err := operator.BuildInformer(req)
	if err != nil {
		return err
	}
	entry := newSharedWatch(req, informer)

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

// newSharedWatch reads the floors req's handlers impose on each channel: what the kind declared for a channel it
// asked for, and for one it did not, the object's metadata where its handler is opinionated — those deliveries
// still reach the wrapper — or anything at all where it is not.
func newSharedWatch(req operator.InformerRequest, informer operator.Informer) sharedWatch {
	needs := req.Options.WatchNeeds.WithDefaults()
	unasked := operator.WatchDetailIdentity
	if req.Opinionated {
		unasked = operator.WatchDetailMetadata
	}
	entry := sharedWatch{live: unasked, resync: unasked}
	if needs.Live != nil {
		entry.live = needs.Live.Detail
	}
	if needs.Resync != nil {
		entry.resync = needs.Resync.Detail
	}
	entry.provided, entry.stated = operator.InformerWatchOptions(informer)
	return entry
}

// refuses describes the deliveries other's informer makes which carry less than this entry's handlers can take. An
// informer which says nothing about what it provides has nothing to check.
func (w sharedWatch) refuses(other sharedWatch) []string {
	if !other.stated {
		return nil
	}
	var refused []string
	if other.provided.Live != nil && !w.live.SatisfiedBy(other.provided.Live.Detail) {
		refused = append(refused, fmt.Sprintf("Live deliveries carrying less than %q", w.live))
	}
	if other.provided.Resync != nil && !w.resync.SatisfiedBy(other.provided.Resync.Detail) {
		refused = append(refused, fmt.Sprintf("Resync deliveries carrying less than %q", w.resync))
	}
	return refused
}

func sharedWatchError(key string, refused []string) error {
	return fmt.Errorf("kind '%s' is watched by more than one entry, and every informer for it delivers to every one"+
		" of its reconcilers and watchers: one of them makes %s, which another entry's handlers need. Give the"+
		" entries the same declaration, or watch the kind once", key, strings.Join(refused, " and "))
}
