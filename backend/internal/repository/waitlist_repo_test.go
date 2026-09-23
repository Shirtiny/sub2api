package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	_ "github.com/Wei-Shaw/sub2api/ent/runtime"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestWaitlistRepositoryJoin(t *testing.T) {
	for _, result := range []error{nil, sql.ErrNoRows, errors.New("unavailable")} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
		t.Cleanup(func() { _ = client.Close() })
		q := mock.ExpectQuery(`INSERT INTO "waitlist_entries" .*ON CONFLICT \("email"\) DO NOTHING RETURNING "id"`).WithArgs("a@example.com", sqlmock.AnyArg())
		if result == nil {
			q.WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))
		} else {
			q.WillReturnError(result)
		}
		err = NewWaitlistRepository(client).Join(context.Background(), "a@example.com")
		if result == nil || errors.Is(result, sql.ErrNoRows) {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, result)
		}
		require.NoError(t, mock.ExpectationsWereMet())
	}
}
func TestWaitlistRepositoryList(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	mock.ExpectQuery(`SELECT COUNT.*FROM "waitlist_entries"`).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(21))
	mock.ExpectQuery(`SELECT .*FROM "waitlist_entries" ORDER BY "waitlist_entries"\."id" DESC LIMIT 20 OFFSET 20`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "email", "created_at"}).AddRow(1, "a@example.com", time.Now()))
	items, total, err := NewWaitlistRepository(client).List(context.Background(), pagination.PaginationParams{Page: 2, PageSize: 20})
	require.NoError(t, err)
	require.EqualValues(t, 21, total)
	require.Len(t, items, 1)
	require.Equal(t, "a@example.com", items[0].Email)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestWaitlistRepositoryClaimConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		affected int64
		sent     bool
		wantErr  bool
	}{
		{"first or expired attempt", 1, false, false},
		{"already sent", 0, true, false},
		{"another request is sending", 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			attempt := time.Now().UTC().Truncate(time.Microsecond)
			mock.ExpectExec(regexp.QuoteMeta(`UPDATE "waitlist_entries" SET "confirmation_attempted_at" = $1 WHERE ("waitlist_entries"."email" = $2 AND "waitlist_entries"."confirmation_sent_at" IS NULL) AND ("waitlist_entries"."confirmation_attempted_at" IS NULL OR "waitlist_entries"."confirmation_attempted_at" < $3)`)).
				WithArgs(attempt, "a@example.com", attempt.Add(-service.WaitlistConfirmationLease)).WillReturnResult(sqlmock.NewResult(0, tc.affected))
			if tc.affected == 0 {
				rows := sqlmock.NewRows([]string{"id"})
				if tc.sent {
					rows.AddRow(1)
				}
				mock.ExpectQuery(`SELECT .*"id" FROM "waitlist_entries" WHERE .*"email" = \$1 AND .*"confirmation_sent_at" IS NOT NULL LIMIT 1`).WithArgs("a@example.com").WillReturnRows(rows)
			}
			claimed, err := NewWaitlistRepository(client).ClaimConfirmation(context.Background(), "a@example.com", attempt)
			if tc.wantErr {
				require.ErrorIs(t, err, service.ErrWaitlistConfirmationFailed)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.affected == 1, claimed)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWaitlistRepositoryFinishConfirmation(t *testing.T) {
	for _, sent := range []bool{true, false} {
		for _, affected := range []int64{1, 0} {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
			t.Cleanup(func() { _ = client.Close() })
			attempt := time.Now().UTC().Truncate(time.Microsecond)
			if sent {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "waitlist_entries" SET "confirmation_sent_at" = $1 WHERE ("waitlist_entries"."email" = $2 AND "waitlist_entries"."confirmation_attempted_at" = $3) AND "waitlist_entries"."confirmation_sent_at" IS NULL`)).WithArgs(sqlmock.AnyArg(), "a@example.com", attempt).WillReturnResult(sqlmock.NewResult(0, affected))
			} else {
				mock.ExpectExec(regexp.QuoteMeta(`UPDATE "waitlist_entries" SET "confirmation_attempted_at" = NULL WHERE ("waitlist_entries"."email" = $1 AND "waitlist_entries"."confirmation_attempted_at" = $2) AND "waitlist_entries"."confirmation_sent_at" IS NULL`)).WithArgs("a@example.com", attempt).WillReturnResult(sqlmock.NewResult(0, affected))
			}
			err = NewWaitlistRepository(client).FinishConfirmation(context.Background(), "a@example.com", attempt, sent)
			if affected == 0 {
				require.ErrorContains(t, err, "claim lost")
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		}
	}
}
