//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAuthWaitlistRegistrationAccess(t *testing.T) {
	ctx := context.Background()
	for _, open := range []string{"false", "true"} {
		s := newAuthService(&userRepoStub{}, map[string]string{SettingKeyRegistrationEnabled: open}, nil, nil)
		r := &waitlistApprovalRepoStub{allowedEmail: "approved@example.com"}
		s.waitlistRepo = r
		approved, err := s.checkEmailRegistrationAccess(ctx, " Approved@Example.COM ")
		require.NoError(t, err)
		require.True(t, approved)
		approved, err = s.checkEmailRegistrationAccess(ctx, "other@example.com")
		require.False(t, approved)
		if open == "true" {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, ErrRegDisabled)
		}
		r.lookupErr = errors.New("db unavailable")
		_, _, err = s.Register(ctx, "approved@example.com", "password")
		require.ErrorIs(t, err, ErrServiceUnavailable) // never fail open, even if public signup is on
	}
}

func TestAuthWaitlistAlwaysRequiresMailboxProof(t *testing.T) {
	for _, open := range []string{"false", "true"} {
		s := newAuthService(&userRepoStub{}, map[string]string{
			SettingKeyRegistrationEnabled: open, SettingKeyEmailVerifyEnabled: "false", SettingKeyInvitationCodeEnabled: "true",
		}, &emailCacheStub{data: &VerificationCodeData{Code: "123456", CreatedAt: time.Now()}}, nil)
		s.waitlistRepo = &waitlistApprovalRepoStub{allowedEmail: "approved@example.com"}
		_, _, err := s.Register(context.Background(), "approved@example.com", "password")
		require.ErrorIs(t, err, ErrEmailVerifyRequired) // grant replaces invitation, NOT email proof
		_, _, err = s.RegisterWithVerification(context.Background(), "approved@example.com", "password", "999999", "", "", "")
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrRegDisabled)
		require.NotErrorIs(t, err, ErrInvitationCodeRequired)
		s.emailService = nil
		_, _, err = s.RegisterWithVerification(context.Background(), "approved@example.com", "password", "123456", "", "", "")
		require.ErrorIs(t, err, ErrServiceUnavailable)
	}
}

func TestAuthWaitlistVerifyCodeAdmission(t *testing.T) {
	ctx := context.Background()
	s := newAuthService(&userRepoStub{}, map[string]string{SettingKeyRegistrationEnabled: "false"}, nil, nil)
	s.waitlistRepo = &waitlistApprovalRepoStub{allowedEmail: "approved@example.com"}
	require.ErrorIs(t, s.SendVerifyCode(ctx, "other@example.com"), ErrRegDisabled)
	_, err := s.SendVerifyCodeAsync(ctx, "other@example.com")
	require.ErrorIs(t, err, ErrRegDisabled)
	// Approved email passes admission; unconfigured mail services still fail (no live SMTP).
	err = s.SendVerifyCode(ctx, "Approved@Example.com")
	require.ErrorContains(t, err, "email service not configured")
	_, err = s.SendVerifyCodeAsync(ctx, "Approved@Example.com")
	require.ErrorContains(t, err, "email queue service not configured")
}

func TestAuthWaitlistTurnstileVerificationFlow(t *testing.T) {
	verifier := &turnstileVerifierSpy{}
	s := newAuthServiceForRegisterTurnstileTest(map[string]string{
		SettingKeyEmailVerifyEnabled: "false", SettingKeyTurnstileEnabled: "true", SettingKeyTurnstileSecretKey: "secret",
	}, verifier)
	s.waitlistRepo = &waitlistApprovalRepoStub{allowedEmail: "approved@example.com"}
	require.NoError(t, s.VerifyTurnstileForRegister(context.Background(), "", "127.0.0.1", "123456", "Approved@Example.com"))
	require.Zero(t, verifier.called)
	require.ErrorIs(t, s.VerifyTurnstileForRegister(context.Background(), "", "127.0.0.1", "", "approved@example.com"), ErrTurnstileVerificationFailed)
	require.ErrorIs(t, s.VerifyTurnstileForRegister(context.Background(), "", "127.0.0.1", "123456", "other@example.com"), ErrTurnstileVerificationFailed)
}
