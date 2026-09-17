package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBuildRecordUsageLogRequestOrigin(t *testing.T) {
	for _, tc := range []struct{ host, ip string }{
		{"nl.cafeshop.ai", "203.0.113.9"},
		{"www.cafeshop.ai", "2001:db8::9"},
		{"", ""},
	} {
		t.Run(tc.host, func(t *testing.T) {
			s := &GatewayService{}
			log := s.buildRecordUsageLog(context.Background(), &recordUsageCoreInput{RequestHost: tc.host, IPAddress: tc.ip},
				&ForwardResult{Model: "test-model"}, &APIKey{ID: 1}, &User{ID: 2}, &Account{ID: 3}, nil,
				"test-model", 1, 1, 1, 0, false, nil, &recordUsageOpts{})
			if tc.host == "" {
				require.Nil(t, log.RequestHost)
				require.Nil(t, log.IPAddress)
			} else {
				require.NotNil(t, log.RequestHost)
				require.Equal(t, tc.host, *log.RequestHost)
				require.NotNil(t, log.IPAddress)
				require.Equal(t, tc.ip, *log.IPAddress)
			}
		})
	}
}
