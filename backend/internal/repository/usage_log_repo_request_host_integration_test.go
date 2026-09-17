//go:build integration

package repository

import (
	"fmt"

	"github.com/Wei-Shaw/sub2api/ent/usagelog"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
)

func (s *UsageLogRepoSuite) TestRequestHostAndClientIPRoundTrip() {
	user := mustCreateUser(s.T(), s.client, &service.User{Email: "request-host-" + uuid.NewString() + "@example.com"})
	key := mustCreateApiKey(s.T(), s.client, &service.APIKey{UserID: user.ID, Key: "sk-" + uuid.NewString(), Name: "ingress"})
	account := mustCreateAccount(s.T(), s.client, &service.Account{Name: "ingress"})

	expectedHosts := make(map[string]*string)
	for _, mode := range []string{"single", "fallback", "batch", "best_effort"} {
		for _, host := range []*string{nil, requestHostPointer("www.cafeshop.ai"), requestHostPointer("nl.cafeshop.ai")} {
			log := &service.UsageLog{
				UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
				RequestID: uuid.NewString(), Model: "gpt-5", RequestHost: host,
				IPAddress: requestHostPointer("2001:db8::123"),
			}
			expectedHosts[log.RequestID] = host
			prepared := prepareUsageLogInsert(log)
			switch mode {
			case "single":
				_, err := s.repo.createSingle(s.ctx, s.tx, log)
				s.Require().NoError(err)
			case "fallback":
				s.Require().NoError(execUsageLogInsertNoResult(s.ctx, s.tx, prepared))
			case "batch":
				batchKey := usageLogBatchKey(log.RequestID, log.APIKeyID)
				query, args := buildUsageLogBatchInsertQuery([]string{batchKey}, map[string]usageLogInsertPrepared{batchKey: prepared})
				rows, err := s.tx.QueryContext(s.ctx, query, args...)
				s.Require().NoError(err)
				s.Require().NoError(rows.Close())
			case "best_effort":
				query, args := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
				_, err := s.tx.ExecContext(s.ctx, query, args...)
				s.Require().NoError(err)
			}

			entity, err := s.client.UsageLog.Query().Where(usagelog.RequestIDEQ(log.RequestID)).Only(s.ctx)
			s.Require().NoError(err, fmt.Sprintf("%s ent read", mode))
			s.Equal(host, entity.RequestHost)
			s.Equal(log.IPAddress, entity.IPAddress)
			stored, err := s.repo.GetByID(s.ctx, entity.ID)
			s.Require().NoError(err)
			s.Equal(host, stored.RequestHost)
			s.Equal(log.IPAddress, stored.IPAddress)
		}
	}
	logs, page, err := s.repo.ListByUser(s.ctx, user.ID, pagination.PaginationParams{Page: 1, PageSize: 20})
	s.Require().NoError(err)
	s.Equal(int64(12), page.Total)
	s.Len(logs, 12)
	for _, log := range logs {
		s.Equal(expectedHosts[log.RequestID], log.RequestHost)
		s.Equal(requestHostPointer("2001:db8::123"), log.IPAddress)
	}
}
