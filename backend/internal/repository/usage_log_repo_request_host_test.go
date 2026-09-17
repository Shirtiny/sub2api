package repository

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogInsertPathsPreserveRequestHostAndClientIP(t *testing.T) {
	for _, host := range []*string{nil, requestHostPointer("www.cafeshop.ai"), requestHostPointer("nl.cafeshop.ai")} {
		name := "historical_null"
		if host != nil {
			name = *host
		}
		t.Run(name, func(t *testing.T) {
			log := &service.UsageLog{
				UserID: 1, APIKeyID: 2, AccountID: 3, RequestID: "request-host-test", Model: "gpt-5",
				RequestHost: host, IPAddress: requestHostPointer("2001:db8::123"), CreatedAt: time.Now().UTC(),
			}
			prepared := prepareUsageLogInsert(log)
			columns := strings.Split(usageLogSelectColumns, ", ")[1:]
			require.Len(t, prepared.args, len(columns))
			for i, column := range columns {
				switch column {
				case "request_host":
					require.Equal(t, nullString(host), prepared.args[i])
					require.Equal(t, "text", usageLogInsertArgTypes[i])
				case "ip_address":
					require.Equal(t, nullString(log.IPAddress), prepared.args[i])
				}
			}

			// Matching the complete column list catches a shifted parameter in either
			// synchronous/fallback insert, not just the presence of a hostname argument.
			insertPattern := `INSERT INTO usage_logs \(\s*` + strings.Join(columns, `,\s*`) + `\s*\) VALUES \(`
			db, mock := newSQLMock(t)
			mock.ExpectQuery(insertPattern).WithArgs(anySliceToDriverValues(prepared.args)...).
				WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), log.CreatedAt))
			inserted, err := (&usageLogRepository{sql: db}).createSingle(context.Background(), db, log)
			require.NoError(t, err)
			require.True(t, inserted)
			mock.ExpectExec(insertPattern).WithArgs(anySliceToDriverValues(prepared.args)...).
				WillReturnResult(sqlmock.NewResult(0, 1))
			require.NoError(t, execUsageLogInsertNoResult(context.Background(), db, prepared))
			require.NoError(t, mock.ExpectationsWereMet())

			key := usageLogBatchKey(log.RequestID, log.APIKeyID)
			batchQuery, batchArgs := buildUsageLogBatchInsertQuery([]string{key}, map[string]usageLogInsertPrepared{key: prepared})
			require.Equal(t, prepared.args, batchArgs[1:])
			bestEffortQuery, bestEffortArgs := buildUsageLogBestEffortInsertQuery([]usageLogInsertPrepared{prepared})
			require.Equal(t, prepared.args, bestEffortArgs)
			columnPattern := strings.Join(columns, `,\s*`)
			for _, query := range []string{batchQuery, bestEffortQuery} {
				// CTE input, INSERT target, and SELECT projection must agree.
				require.Len(t, regexp.MustCompile(columnPattern).FindAllString(query, -1), 3)
			}
		})
	}
}

func requestHostPointer(value string) *string { return &value }

type usageLogIngressScanner struct {
	host *string
	ip   *string
}

func (s usageLogIngressScanner) Scan(dest ...any) error {
	columns := strings.Split(usageLogSelectColumns, ", ")
	for i, target := range dest {
		switch columns[i] {
		case "request_host":
			value, ok := target.(*sql.NullString)
			if !ok {
				return fmt.Errorf("request_host scan target: %T", target)
			}
			*value = nullString(s.host)
		case "ip_address":
			value, ok := target.(*sql.NullString)
			if !ok {
				return fmt.Errorf("ip_address scan target: %T", target)
			}
			*value = nullString(s.ip)
		default:
			value := reflect.ValueOf(target).Elem()
			value.Set(reflect.Zero(value.Type()))
		}
	}
	return nil
}

func TestScanUsageLogPreservesRequestHostAndClientIP(t *testing.T) {
	ip := "203.0.113.24"
	for _, host := range []*string{nil, requestHostPointer("www.cafeshop.ai"), requestHostPointer("nl.cafeshop.ai")} {
		log, err := scanUsageLog(usageLogIngressScanner{host: host, ip: &ip})
		require.NoError(t, err)
		require.Equal(t, host, log.RequestHost)
		require.Equal(t, &ip, log.IPAddress)
	}
}
