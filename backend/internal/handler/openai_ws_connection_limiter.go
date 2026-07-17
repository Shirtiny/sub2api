package handler

import (
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type openAIWSConnectionLease struct {
	limiter  *openAIWSConnectionLimiter
	userID   int64
	apiKeyID int64
	released bool
}

func (l *openAIWSConnectionLease) Release() {
	if l == nil || l.released || l.limiter == nil {
		return
	}
	l.released = true
	l.limiter.release(l.userID, l.apiKeyID)
}

type openAIWSConnectionLimiter struct {
	mu sync.Mutex

	maxTotal  int
	maxUser   int
	maxAPIKey int
	total     int
	users     map[int64]int
	apiKeys   map[int64]int
}

func newOpenAIWSConnectionLimiter(cfg *config.Config) *openAIWSConnectionLimiter {
	if cfg == nil {
		return nil
	}
	ws := cfg.Gateway.OpenAIWS
	if ws.MaxIngressConnections == 0 && ws.MaxIngressConnectionsPerUser == 0 && ws.MaxIngressConnectionsPerAPIKey == 0 {
		return nil
	}
	return &openAIWSConnectionLimiter{
		maxTotal:  ws.MaxIngressConnections,
		maxUser:   ws.MaxIngressConnectionsPerUser,
		maxAPIKey: ws.MaxIngressConnectionsPerAPIKey,
		users:     make(map[int64]int),
		apiKeys:   make(map[int64]int),
	}
}

func (l *openAIWSConnectionLimiter) acquire(userID, apiKeyID int64) (openAIWSConnectionLease, bool, string) {
	if l == nil {
		return openAIWSConnectionLease{}, true, ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxTotal > 0 && l.total >= l.maxTotal {
		return openAIWSConnectionLease{}, false, "global"
	}
	if l.maxUser > 0 && l.users[userID] >= l.maxUser {
		return openAIWSConnectionLease{}, false, "user"
	}
	if l.maxAPIKey > 0 && l.apiKeys[apiKeyID] >= l.maxAPIKey {
		return openAIWSConnectionLease{}, false, "api_key"
	}
	l.total++
	l.users[userID]++
	l.apiKeys[apiKeyID]++
	return openAIWSConnectionLease{limiter: l, userID: userID, apiKeyID: apiKeyID}, true, ""
}

func (l *openAIWSConnectionLimiter) release(userID, apiKeyID int64) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.total--
	l.users[userID]--
	if l.users[userID] == 0 {
		delete(l.users, userID)
	}
	l.apiKeys[apiKeyID]--
	if l.apiKeys[apiKeyID] == 0 {
		delete(l.apiKeys, apiKeyID)
	}
}
