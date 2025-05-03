// pkg/state/pagination.go
package state

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alwinihza/discord-bot/models"
	"github.com/bwmarrin/discordgo"
)

type PaginationManager struct {
	mu    sync.RWMutex
	state map[string]*models.PaginationState // messageID -> state
}

func NewPaginationManager() *PaginationManager {
	return &PaginationManager{
		state: make(map[string]*models.PaginationState),
	}
}

// Thread-safe methods
func (pm *PaginationManager) Get(messageID string) (*models.PaginationState, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	state, exists := pm.state[messageID]
	return state, exists
}

func (pm *PaginationManager) Set(messageID string, state *models.PaginationState) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.state[messageID] = state
}

func (pm *PaginationManager) Delete(messageID string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.state, messageID)
}

func (pm *PaginationManager) UpdateCurrentPage(messageID string, newPage int64, interaction *discordgo.Interaction) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if state, exists := pm.state[messageID]; exists {
		state.CurrentPage = newPage
		return true
	}
	return false
}

func (pm *PaginationManager) Cleanup(maxAge time.Duration) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	now := time.Now()
	for id, state := range pm.state {
		if now.Sub(state.CreatedAt) > maxAge {
			delete(pm.state, id)
		}
	}
}

func (pm *PaginationManager) List() map[string]*models.PaginationState {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	// Create a new map to avoid exposing the internal state directly
	result := make(map[string]*models.PaginationState, len(pm.state))
	for k, v := range pm.state {
		// Create a copy of each state to prevent external modifications
		copy := *v
		result[k] = &copy
	}
	return result
}

// Enhanced version with formatted output
func (pm *PaginationManager) ListFormatted() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	var sb strings.Builder
	sb.WriteString("Active Pagination Sessions:\n")
	sb.WriteString("--------------------------------\n")

	for msgID, state := range pm.state {
		sb.WriteString(fmt.Sprintf("Message ID: %s\n", msgID))
		sb.WriteString(fmt.Sprintf("• Current Page: %d\n", state.CurrentPage))
		sb.WriteString(fmt.Sprintf("• Total Pages: %d\n", state.TotalPages))
		sb.WriteString(fmt.Sprintf("• Items Per Page: %d\n", state.ItemsPerPage))
		sb.WriteString(fmt.Sprintf("• Created At: %s\n", state.CreatedAt.Format(time.RFC822)))
		sb.WriteString("--------------------------------\n")
	}

	if len(pm.state) == 0 {
		sb.WriteString("No active pagination sessions\n")
	}

	return sb.String()
}
