package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCafecodeIdentityHeaderUname(t *testing.T) {
	tests := []struct {
		name string
		user *User
		want string
	}{
		{
			name: "uses ascii username",
			user: &User{Username: "cafe-user", Email: "fallback@example.test"},
			want: "cafe-user",
		},
		{
			name: "falls back to email for non ascii username",
			user: &User{Username: "幸运狗", Email: "1115384808@qq.com"},
			want: "1115384808@qq.com",
		},
		{
			name: "falls back to email for control characters",
			user: &User{Username: "bad\nname", Email: "fallback@example.test"},
			want: "fallback@example.test",
		},
		{
			name: "empty when no ascii safe identity exists",
			user: &User{Username: "幸运狗", Email: "用户@example.test"},
			want: "",
		},
		{
			name: "trims chosen value",
			user: &User{Username: "  cafe-user  ", Email: "fallback@example.test"},
			want: "cafe-user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, cafecodeIdentityHeaderUname(tt.user))
		})
	}
}
