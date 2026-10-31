package state

import (
	"sync"
	"time"

	"github.com/alwinihza/discord-bot/models"
)

// PaginationManager maps a bot message ID to its pagination state.
// State is in memory, so buttons on messages sent before a restart expire.
type PaginationManager struct {
	mu    sync.Mutex
	state map[string]models.PaginationState
}

func NewPaginationManager() *PaginationManager {
	return &PaginationManager{state: make(map[string]models.PaginationState)}
}

func (pm *PaginationManager) Get(messageID string) (models.PaginationState, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	st, ok := pm.state[messageID]
	return st, ok
}

func (pm *PaginationManager) Set(messageID string, st models.PaginationState) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state[messageID] = st
}

func (pm *PaginationManager) Delete(messageID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.state, messageID)
}

func (pm *PaginationManager) Cleanup(maxAge time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	for id, st := range pm.state {
		if time.Since(st.CreatedAt) > maxAge {
			delete(pm.state, id)
		}
	}
}
