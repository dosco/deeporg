package serv

import (
	"context"
	"strconv"
	"time"
)

// More than one server can run on one store.
//
// A setup change is written to the store by the server that took the request,
// which then restarts itself to load it. The others have to load it too, so
// every server looks at the stored configuration's version every few seconds
// and restarts itself when it is not the one it started with. The same check
// is what makes `deeporg config import` take effect on a running server.

// storeConfigCheckEvery is how often a running server looks for a newer
// stored configuration.
var storeConfigCheckEvery = 5 * time.Second

// storedConfigNoVersion is the version of a store that holds no configuration
// yet.
const storedConfigNoVersion = "none"

// SetStoreVersion records the version of the stored configuration this
// server was started from, as StoredConfigVersion reported it. Read it before
// the configuration itself: a change that lands in between is then seen as
// newer, and loaded.
func (c *Config) SetStoreVersion(version string) {
	if c != nil {
		c.storeVersion = version
	}
}

// StoredConfigVersion returns a marker that changes every time the stored
// configuration is written.
func StoredConfigVersion(ctx context.Context, store string) (string, error) {
	st, done, err := openStoreSettings(ctx, store)
	if err != nil {
		return "", err
	}
	defer done()
	return storedConfigVersion(ctx, st)
}

func storedConfigVersion(ctx context.Context, st *sqlAuthStore) (string, error) {
	row, err := st.get(ctx, storedConfigKind, storedConfigID)
	if err != nil {
		return "", err
	}
	if row == nil {
		return storedConfigNoVersion, nil
	}
	return strconv.FormatInt(row.version, 10), nil
}

// applyStoredConfig makes the stored configuration take effect if it is not
// the one this server is running. It is called by the copy that just saved a
// change and by every copy's own check, and does the work once however many
// of them notice the same change.
func (s *graphjinService) applyStoredConfig(ctx context.Context) {
	apply := s.configApplier
	if apply == nil || s.accounts == nil {
		return
	}
	s.storeApplyMu.Lock()
	defer s.storeApplyMu.Unlock()
	version, err := storedConfigVersion(ctx, s.accounts.store)
	if err != nil || version == s.storeSeenVersion {
		return
	}
	// Remember it whether or not applying works, so a failure is reported
	// once and not every few seconds.
	s.storeSeenVersion = version
	if err := apply(); err != nil && s.log != nil {
		s.log.Errorf("setup: the saved configuration could not be applied; restart the server: %s", err)
	}
}

// startStoreWatch starts this copy's check for a newer stored configuration.
func (s *graphjinService) startStoreWatch() {
	if !s.storeBacked() || s.configApplier == nil {
		return
	}
	s.storeSeenVersion = s.conf.storeVersion
	if s.storeSeenVersion == "" {
		if version, err := storedConfigVersion(context.Background(), s.accounts.store); err == nil {
			s.storeSeenVersion = version
		}
	}
	stop := make(chan struct{})
	s.storeWatchStop = stop
	s.storeWatchWG.Add(1)
	go func() {
		defer s.storeWatchWG.Done()
		ticker := time.NewTicker(storeConfigCheckEvery)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), storeConfigCheckEvery)
				s.applyStoredConfig(ctx)
				cancel()
			}
		}
	}()
}

func (s *graphjinService) stopStoreWatch() {
	if s.storeWatchStop == nil {
		return
	}
	close(s.storeWatchStop)
	s.storeWatchStop = nil
	s.storeWatchWG.Wait()
}
