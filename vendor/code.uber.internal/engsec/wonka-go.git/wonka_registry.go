package wonka

import (
	"container/list"
	"context"
	"crypto/ecdsa"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type certificateRegistry interface {
	Register(e certificateRegistrationRequest) (*certificateRegistrationHandle, error)
	Unregister(e *certificateRegistrationHandle) error
}

type certificateRegistrationRequest struct {
	cert      *Certificate
	key       *ecdsa.PrivateKey
	requester *httpRequester
	log       *zap.Logger
}

func (e *certificateRegistrationRequest) toKey() repositoryMapKey {
	k := repositoryMapKey{
		entityName: e.cert.EntityName,
		entityType: e.cert.Type,
		host:       e.cert.Host,
		baseURL:    e.requester.URL(),
	}
	return k
}

type certficateKeyTuple struct {
	cert *Certificate
	key  *ecdsa.PrivateKey
}

// repository is a collection of wonka certificates that are periodically
// refreshed by the registry.
type certificateRepository struct {
	sync.RWMutex
	m map[repositoryMapKey]*repositoryEntryInternal
}

type repositoryEntryInternal struct {
	handles   *list.List
	current   certficateKeyTuple
	requester *httpRequester
	log       *zap.Logger
}

type repositoryMapKey struct {
	entityName string
	entityType EntityType
	host       string
	baseURL    string
}

func (k *repositoryMapKey) MarshalLogObject(e zapcore.ObjectEncoder) error {
	if k == nil {
		return nil
	}

	e.AddString("name", k.entityName)
	e.AddString("type", k.entityType.String())
	e.AddString("host", k.host)
	e.AddString("url", k.baseURL)
	return nil
}

type certificateRegistrationHandle struct {
	current     certficateKeyTuple
	currentLock sync.RWMutex

	mapKey repositoryMapKey
	elem   *list.Element
}

func (h *certificateRegistrationHandle) GetCertificateAndPrivateKey() (*Certificate, *ecdsa.PrivateKey) {
	h.currentLock.RLock()
	cur := h.current
	h.currentLock.RUnlock()
	return cur.cert, cur.key
}

func newCertificateRegistry() certificateRegistry {
	return &certificateRepository{
		m: make(map[repositoryMapKey]*repositoryEntryInternal),
	}
}

// Register registers provided key material with this registry. The
// registry prevents associating a new refreshing background goroutine with every Wonka
// instance.
//
// The following things can vary between Wonka instances will necessiate a seperate
// entry into the registry map:
//  1. From the certificate:
//     a. The entity name
//     b. The entity type
//     c. The host
//  2. The http requester (different instances can communicate with wonkmaster differently)
//  3. The logger (settings depend on configuration)
func (r *certificateRepository) Register(e certificateRegistrationRequest) (*certificateRegistrationHandle, error) {
	k := e.toKey()
	handle := &certificateRegistrationHandle{
		mapKey:  k,
		current: certficateKeyTuple{cert: e.cert, key: e.key},
	}

	r.Lock()
	defer r.Unlock()

	m := r.m

	// If the entry already exists, we only need to add the new handle.
	if v, loaded := m[k]; loaded {
		handle.elem = v.handles.PushBack(handle)
		e.log.Warn("Skipping register since this entry has already been added." +
			" Generally, only a single wonka instance should be created by consumers.")
		return handle, nil
	}

	entry := &repositoryEntryInternal{
		current:   handle.current,
		handles:   list.New(),
		log:       e.log,
		requester: e.requester,
	}
	handle.elem = entry.handles.PushBack(handle)
	m[k] = entry

	// Since we added an entry, kick off a new goroutine
	go _certRefresher(r, k, certRefreshPeriod, e.log)
	return handle, nil
}

// Unregister decrements the reference count for a registered entry.
//
// This can potentially cause a background goroutine to be terminiated
// if this results in the reference count reaching zero.
func (r *certificateRepository) Unregister(e *certificateRegistrationHandle) error {
	k := e.mapKey

	r.Lock()
	defer r.Unlock()

	// Remove the handle from the entry
	m := r.m
	if v, ok := m[k]; ok {
		log := v.log
		v.handles.Remove(e.elem)
		if v.handles.Len() == 0 {
			log.Debug("Removing entry because the ref count reached zero")
			delete(m, k)
		}

		log.Debug("Unregistered wonka instance")
		return nil
	}

	return errors.New("failed to unregister instance because it was not in the registry")
}

// loadForRefresh returns the entryInternal corresponding to the key from the map.
// Returns nil if there is no entry for the key.
//
// This should only be called from within the context of a read lock.
func (r *certificateRepository) loadForRefresh(k repositoryMapKey) *repositoryEntryInternal {
	if e, ok := r.m[k]; ok {
		return e
	}
	return nil
}

// updateEntry updates the entry at the given key, along with all associated handles, to
// use the certficateKeyTuple.
func (r *certificateRepository) updateEntry(k repositoryMapKey, ct certficateKeyTuple) {
	r.Lock()
	defer r.Unlock()

	e, ok := r.m[k]
	if !ok {
		return
	}

	e.current = ct
	handles := getHandles(e.handles)
	go updateHandles(handles, ct)
}

// _certRefresher exists to allow stubbing out calls to periodicWonkaCertRefresh in unit
// testing.
var _certRefresher = periodicWonkaCertRefresh

func periodicWonkaCertRefresh(r *certificateRepository, k repositoryMapKey, period time.Duration, log *zap.Logger) {
	ticker := time.NewTicker(period)
	defer ticker.Stop()

	log = log.With(zap.Object("registry_key", &k))
	log.Info("Starting periodic certificate refresh")

	for range ticker.C {
		// Grab the latest entry
		r.RLock()
		e := r.loadForRefresh(k)
		if e == nil {
			r.RUnlock()
			log.Info("Terminating periodic certificate refresh")
			return
		}

		// All handles share the same key and cert
		cert := e.current.cert
		pk := e.current.key
		h := e.requester
		r.RUnlock()

		var nc *Certificate
		var nk *ecdsa.PrivateKey
		var err error

		nc, nk, err = refreshCertFromWonkamaster(context.Background(), cert, pk, h)
		if err != nil {
			log.Warn("error refreshing from wonkamaster", zap.Error(err))
			nc, nk, err = refreshCertFromWonkad(cert, pk)
		}

		if err != nil {
			log.Warn("error refreshing from wonkad", zap.Error(err))
			continue
		}

		// update the certificate and key on the entry
		newCurrent := certficateKeyTuple{cert: nc, key: nk}
		r.updateEntry(k, newCurrent)
		log.Debug("certificate successfully updated", zap.Object("registry_key", &k))
	}
}

func getHandles(l *list.List) []*certificateRegistrationHandle {
	s := make([]*certificateRegistrationHandle, 0, l.Len())
	for e := l.Front(); e != nil; e = e.Next() {
		s = append(s, e.Value.(*certificateRegistrationHandle))
	}
	return s
}

func updateHandles(handles []*certificateRegistrationHandle, newCurrent certficateKeyTuple) {
	for _, h := range handles {
		h.currentLock.Lock()
		h.current = newCurrent
		h.currentLock.Unlock()
	}
}
