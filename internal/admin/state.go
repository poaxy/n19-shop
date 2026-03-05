package admin

import "sync"

type State string

const (
	StateIdle                 State = "idle"
	StateAwaitingBroadcastText      = "awaiting_broadcast_text"
	StateAwaitingDirectTargetID     = "awaiting_direct_target_id"
	StateAwaitingDirectMessage      = "awaiting_direct_message"
)

// Manager holds the admin conversation state.
// The bot currently supports a single admin, so this is a simple singleton state.
type Manager struct {
	mu          sync.RWMutex
	state       State
	directTarget int64
}

func NewManager() *Manager {
	return &Manager{
		state: StateIdle,
	}
}

func (m *Manager) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

func (m *Manager) SetState(s State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = s
	if s == StateIdle {
		m.directTarget = 0
	}
}

func (m *Manager) SetDirectTarget(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.directTarget = id
}

func (m *Manager) GetDirectTarget() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.directTarget
}

