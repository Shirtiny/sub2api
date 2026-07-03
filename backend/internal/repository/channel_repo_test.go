//go:build unit

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// --- marshalModelMapping ---

func TestMarshalModelMapping(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]map[string]string
		wantJSON string // expected JSON output (exact match)
	}{
		{
			name:     "empty map",
			input:    map[string]map[string]string{},
			wantJSON: "{}",
		},
		{
			name:     "nil map",
			input:    nil,
			wantJSON: "{}",
		},
		{
			name: "populated map",
			input: map[string]map[string]string{
				"openai": {"gpt-4": "gpt-4-turbo"},
			},
		},
		{
			name: "nested values",
			input: map[string]map[string]string{
				"openai":    {"*": "gpt-5.4"},
				"anthropic": {"claude-old": "claude-new"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := marshalModelMapping(tt.input)
			require.NoError(t, err)

			if tt.wantJSON != "" {
				require.Equal(t, []byte(tt.wantJSON), result)
			} else {
				// round-trip: unmarshal and compare with input
				var parsed map[string]map[string]string
				require.NoError(t, json.Unmarshal(result, &parsed))
				require.Equal(t, tt.input, parsed)
			}
		})
	}
}

// --- unmarshalModelMapping ---

func TestUnmarshalModelMapping(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		wantNil bool
		want    map[string]map[string]string
	}{
		{
			name:    "nil data",
			input:   nil,
			wantNil: true,
		},
		{
			name:    "empty data",
			input:   []byte{},
			wantNil: true,
		},
		{
			name:    "invalid JSON",
			input:   []byte("not-json"),
			wantNil: true,
		},
		{
			name:    "type error - number",
			input:   []byte("42"),
			wantNil: true,
		},
		{
			name:    "type error - array",
			input:   []byte("[1,2,3]"),
			wantNil: true,
		},
		{
			name:  "valid JSON",
			input: []byte(`{"openai":{"gpt-4":"gpt-4-turbo"},"anthropic":{"old":"new"}}`),
			want: map[string]map[string]string{
				"openai":    {"gpt-4": "gpt-4-turbo"},
				"anthropic": {"old": "new"},
			},
		},
		{
			name:  "empty object",
			input: []byte("{}"),
			want:  map[string]map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unmarshalModelMapping(tt.input)
			if tt.wantNil {
				require.Nil(t, result)
			} else {
				require.NotNil(t, result)
				require.Equal(t, tt.want, result)
			}
		})
	}
}

// --- escapeLike ---

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no special chars",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "backslash",
			input: `a\b`,
			want:  `a\\b`,
		},
		{
			name:  "percent",
			input: "50%",
			want:  `50\%`,
		},
		{
			name:  "underscore",
			input: "a_b",
			want:  `a\_b`,
		},
		{
			name:  "all special chars",
			input: `a\b%c_d`,
			want:  `a\\b\%c\_d`,
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "consecutive special chars",
			input: "%_%",
			want:  `\%\_\%`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, escapeLike(tt.input))
		})
	}
}

// --- isUniqueViolation ---

func TestIsUniqueViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unique violation code 23505",
			err:  &pq.Error{Code: "23505"},
			want: true,
		},
		{
			name: "different pq error code",
			err:  &pq.Error{Code: "23503"},
			want: false,
		},
		{
			name: "non-pq error",
			err:  errors.New("some generic error"),
			want: false,
		},
		{
			name: "typed nil pq.Error",
			err: func() error {
				var pqErr *pq.Error
				return pqErr
			}(),
			want: false,
		},
		{
			name: "bare nil",
			err:  nil,
			want: false,
		},
		{
			name: "wrapped pq error with 23505",
			err:  fmt.Errorf("wrapped: %w", &pq.Error{Code: "23505"}),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isUniqueViolation(tt.err))
		})
	}
}

func TestChannelListOrderBy_AllowsDescendingIDSort(t *testing.T) {
	params := pagination.PaginationParams{
		SortBy:    "id",
		SortOrder: "desc",
	}

	require.Equal(t, "c.id DESC, c.id DESC", channelListOrderBy(params))
}

func TestExpandGroupIDsWithCustomGroups(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{"custom_source_group_id", "id"}).
		AddRow(int64(10), int64(101)).
		AddRow(int64(10), int64(102)).
		AddRow(int64(20), int64(201))
	mock.ExpectQuery("SELECT custom_source_group_id, id").
		WithArgs(sqlmock.AnyArg(), service.StatusActive).
		WillReturnRows(rows)

	got, err := expandGroupIDsWithCustomGroups(context.Background(), db, []int64{10, 20, 10, 0})
	require.NoError(t, err)
	require.Equal(t, []int64{10, 101, 102, 20, 201}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestExpandAccountGroupBindingsWithCustomGroupsKeepsSourcePriority(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	rows := sqlmock.NewRows([]string{"custom_source_group_id", "id"}).
		AddRow(int64(10), int64(101)).
		AddRow(int64(20), int64(201))
	mock.ExpectQuery("SELECT custom_source_group_id, id").
		WithArgs(sqlmock.AnyArg(), service.StatusActive).
		WillReturnRows(rows)

	got, err := expandAccountGroupBindingsWithCustomGroups(context.Background(), db, []int64{10, 20})
	require.NoError(t, err)
	require.Equal(t, []accountGroupBindingTarget{
		{GroupID: 10, Priority: 1},
		{GroupID: 101, Priority: 1},
		{GroupID: 20, Priority: 2},
		{GroupID: 201, Priority: 2},
	}, got)
	require.NoError(t, mock.ExpectationsWereMet())
}
