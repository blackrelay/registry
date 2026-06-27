package snapshots

import (
	"context"
	"sort"
	"sync"

	"github.com/blackrelay/registry/internal/model"
)

type MemoryStore struct {
	mu        sync.RWMutex
	Sources   map[string]model.Source
	Artefacts map[string]model.SourceArtefact
	Rows      map[string][]NormalizedEnemyRow
	Snapshots map[string]SnapshotSet
	Current   map[string]string
	Noops     []NoopRecord
	Jobs      []OutboxJob
	Diffs     []DiffSummary
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		Sources:   make(map[string]model.Source),
		Artefacts: make(map[string]model.SourceArtefact),
		Rows:      make(map[string][]NormalizedEnemyRow),
		Snapshots: make(map[string]SnapshotSet),
		Current:   make(map[string]string),
	}
}

func (s *MemoryStore) FindArtefactByHash(ctx context.Context, sha256 string) (model.SourceArtefact, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, artefact := range s.Artefacts {
		if artefact.SHA256 == sha256 {
			return artefact, true, nil
		}
	}
	return model.SourceArtefact{}, false, nil
}

func (s *MemoryStore) CurrentSnapshot(ctx context.Context, environment model.Environment, kind string) (SnapshotSet, bool, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	id, ok := s.Current[currentKey(environment, kind)]
	if !ok {
		return SnapshotSet{}, false, nil
	}
	snapshot := s.Snapshots[id]
	snapshot.Rows = append([]NormalizedEnemyRow(nil), snapshot.Rows...)
	return snapshot, true, nil
}

func (s *MemoryStore) RecordSnapshotNoop(ctx context.Context, record NoopRecord) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[record.Source.ID] = record.Source
	s.Noops = append(s.Noops, record)
	return nil
}

func (s *MemoryStore) RegisterSnapshotCandidate(ctx context.Context, source model.Source, artefact model.SourceArtefact, rows []NormalizedEnemyRow) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[source.ID] = source
	s.Artefacts[artefact.ID] = artefact
	s.Rows[artefact.ID] = append([]NormalizedEnemyRow(nil), rows...)
	return nil
}

func (s *MemoryStore) PromoteSnapshot(ctx context.Context, promotion Promotion) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Sources[promotion.Source.ID] = promotion.Source
	s.Artefacts[promotion.Artefact.ID] = promotion.Artefact
	s.Rows[promotion.Artefact.ID] = append([]NormalizedEnemyRow(nil), promotion.Rows...)
	if promotion.PreviousSnapshot != nil {
		prev := s.Snapshots[promotion.PreviousSnapshot.ID]
		prev.SupersededBySnapshotSetID = promotion.NewSnapshot.ID
		s.Snapshots[prev.ID] = prev
		if prev.Artefact.ID != "" {
			artefact := s.Artefacts[prev.Artefact.ID]
			artefact.SupersededByArtefactID = promotion.Artefact.ID
			s.Artefacts[artefact.ID] = artefact
		}
	}
	snapshot := promotion.NewSnapshot
	snapshot.Artefact = promotion.Artefact
	snapshot.Rows = append([]NormalizedEnemyRow(nil), promotion.Rows...)
	s.Snapshots[snapshot.ID] = snapshot
	s.Current[currentKey(snapshot.Environment, snapshot.Kind)] = snapshot.ID
	s.Diffs = append(s.Diffs, promotion.Diff)
	s.Jobs = append(s.Jobs, promotion.OutboxJobs...)
	return nil
}

func (s *MemoryStore) JobKinds() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.Jobs))
	for _, job := range s.Jobs {
		out = append(out, job.Kind)
	}
	return out
}

func (s *MemoryStore) ArtefactIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.Artefacts))
	for id := range s.Artefacts {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func currentKey(environment model.Environment, kind string) string {
	return string(environment) + ":" + kind
}
