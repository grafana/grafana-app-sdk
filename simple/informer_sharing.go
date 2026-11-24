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

// sharedWatch is one registered entry: what its handlers need, and what its informer carries where the informer
// says.
type sharedWatch struct {
	needs    operator.WatchOptions
	provided operator.WatchOptions
	stated   bool
}

// register builds this entry's informer, checks the deliveries it and the entries already under key would exchange,
// and adds it with add before recording it, so an informer which fails to register leaves nothing behind.
//
// Deliveries are compared with operator.WatchOptions.RefusedDeliveries: each entry has its own informer for what it
// asked, so a channel another entry's informer lacks or divides takes nothing away from it, but one carrying less
// detail, or breaking the sharding it requires, does.
func (s *sharedWatches) register(
	key string, req operator.InformerRequest, add func(operator.Informer, string) error,
) error {
	informer, err := operator.BuildInformer(req)
	if err != nil {
		return err
	}
	entry := sharedWatch{needs: req.Options.WatchNeeds}
	entry.provided, entry.stated = operator.InformerWatchOptions(informer)

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

// refuses describes the deliveries other's informer makes which this entry's handlers cannot take. An informer
// which says nothing about what it provides has nothing to check.
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
