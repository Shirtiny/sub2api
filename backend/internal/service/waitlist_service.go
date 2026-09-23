package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
)

var ErrWaitlistEmailInvalid = infraerrors.BadRequest("WAITLIST_EMAIL_INVALID", "Please enter a valid email address")
var ErrWaitlistConfirmationFailed = infraerrors.ServiceUnavailable("WAITLIST_CONFIRMATION_FAILED", "Your application is saved, but the confirmation email is not complete. Please try again shortly")
var ErrWaitlistNotFound = infraerrors.NotFound("WAITLIST_NOT_FOUND", "Waiting list application not found")
var ErrWaitlistAccountConflict = infraerrors.Conflict("WAITLIST_ACCOUNT_CONFLICT", "Multiple accounts match this email. Please resolve the conflict before approving")
var ErrWaitlistApprovalNoticeFailed = infraerrors.ServiceUnavailable("WAITLIST_APPROVAL_NOTICE_FAILED", "Access has been granted, but the notification email is not complete. Please retry the notification")
var ErrWaitlistSignInRequired = infraerrors.Conflict("WAITLIST_SIGN_IN_REQUIRED", "This approval is already linked to an account. Please sign in with your existing account instead of registering again")

const waitlistConfirmationSubject = "Waiting List 申请成功"
const waitlistConfirmationMessage = "欢迎，您已经加入到Waiting List，请耐心等待。关注邮件消息，开放后会即时通知。"
const waitlistApprovalSubject = "访问权限已开通"
const waitlistApprovalMessage = "您已获得访问权限，可以注册或进入控制台了。"
const waitlistExistingAccountApprovalMessage = "您已获得访问权限，请使用原账号登录并进入控制台，无需重新注册。"

// Longer than the bounded SMTP dial / I/O operation; abandoned attempts can be retried.
const WaitlistConfirmationLease = 2 * time.Minute

var waitlistDomainLetter = regexp.MustCompile(`[a-z]`)
var waitlistLocalPart = regexp.MustCompile("^[a-z0-9!#$%&'*+/=?^_`{|}~.-]+$")
var waitlistDomainLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type WaitlistEntry struct {
	ID                   int64      `json:"id"`
	Email                string     `json:"email"`
	CreatedAt            time.Time  `json:"created_at"`
	ApprovedAt           *time.Time `json:"approved_at"`
	ApprovedBy           *int64     `json:"approved_by"`
	GrantedUserID        *int64     `json:"granted_user_id"`
	ApprovalNoticeSentAt *time.Time `json:"approval_notice_sent_at"`
}

type WaitlistRepository interface {
	Join(ctx context.Context, email string) error
	ClaimConfirmation(ctx context.Context, email string, attempt time.Time) (bool, error)
	FinishConfirmation(ctx context.Context, email string, attempt time.Time, sent bool) error
	List(ctx context.Context, params pagination.PaginationParams) ([]WaitlistEntry, int64, error)
	Approve(ctx context.Context, id, adminID int64) (*WaitlistEntry, error)
	ClaimApprovalNotice(ctx context.Context, id int64, attempt time.Time) (bool, error)
	FinishApprovalNotice(ctx context.Context, id int64, attempt time.Time, sent bool) error
	HasRegistrationApproval(ctx context.Context, email string) (bool, error)
	GetApprovedEntryByEmail(ctx context.Context, email string) (*WaitlistEntry, error)
	ConsumeRegistrationApproval(ctx context.Context, email string, userID int64) error
}

type WaitlistEmailSender interface {
	SendEmail(ctx context.Context, to, subject, body string) error
}

type WaitlistBranding interface {
	GetSiteName(ctx context.Context) string
	GetFrontendURL(ctx context.Context) string
}

type WaitlistService struct {
	repo      WaitlistRepository
	mailer    WaitlistEmailSender
	branding  WaitlistBranding
	authCache APIKeyAuthCacheInvalidator
}

func NewWaitlistService(repo WaitlistRepository, mailer WaitlistEmailSender, branding WaitlistBranding, authCache APIKeyAuthCacheInvalidator) *WaitlistService {
	return &WaitlistService{repo: repo, mailer: mailer, branding: branding, authCache: authCache}
}

// NormalizeWaitlistEmail validates a common ASCII mailbox without a DNS lookup.
// This validates syntax only, not ownership or deliverability.
func NormalizeWaitlistEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	parts := strings.Split(email, "@")
	if len(email) > 254 || len(parts) != 2 {
		return "", ErrWaitlistEmailInvalid
	}
	local, domain := parts[0], parts[1]
	if len(local) == 0 || len(local) > 64 || !waitlistLocalPart.MatchString(local) || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return "", ErrWaitlistEmailInvalid
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", ErrWaitlistEmailInvalid
	}
	for _, label := range labels {
		if !waitlistDomainLabel.MatchString(label) {
			return "", ErrWaitlistEmailInvalid
		}
	}
	// Reject numeric top-level domains / IP addresses, not ordinary mailbox aliases.
	if !waitlistDomainLetter.MatchString(labels[len(labels)-1]) {
		return "", ErrWaitlistEmailInvalid
	}
	return email, nil
}

func (s *WaitlistService) Join(ctx context.Context, email string) error {
	normalized, err := NormalizeWaitlistEmail(email)
	if err != nil {
		return err
	}
	if err := s.repo.Join(ctx, normalized); err != nil {
		return fmt.Errorf("join waitlist: %w", err)
	}
	// The application is durable before any SMTP traffic. A database-backed claim
	// prevents simultaneous submissions from sending the same confirmation.
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	claimed, err := s.repo.ClaimConfirmation(ctx, normalized, attempt)
	if err != nil {
		return ErrWaitlistConfirmationFailed.WithCause(err)
	}
	if !claimed {
		return nil
	} // Already sent; repeated requests remain idempotent.

	// Finish a persisted opt-in even if the browser disconnects. SMTP itself uses
	// the shared dial/I/O deadlines; do not keep a DB transaction open during SMTP.
	mailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	body := waitlistConfirmationHTML(s.branding.GetSiteName(mailCtx))
	sendErr := s.mailer.SendEmail(mailCtx, normalized, waitlistConfirmationSubject, body)
	cancel()
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	finishErr := s.repo.FinishConfirmation(finishCtx, normalized, attempt, sendErr == nil)
	if sendErr != nil {
		return ErrWaitlistConfirmationFailed.WithCause(sendErr)
	}
	if finishErr != nil {
		return ErrWaitlistConfirmationFailed.WithCause(finishErr)
	}
	return nil
}

func (s *WaitlistService) List(ctx context.Context, params pagination.PaginationParams) ([]WaitlistEntry, int64, error) {
	return s.repo.List(ctx, params)
}

// Approve durably grants access before SMTP. Retrying only retries notification;
// it must never undo a later account suspension.
func (s *WaitlistService) Approve(ctx context.Context, id, adminID int64) error {
	if id <= 0 || adminID <= 0 {
		return infraerrors.BadRequest("WAITLIST_APPROVAL_INVALID", "Invalid approval request")
	}
	entry, err := s.repo.Approve(ctx, id, adminID)
	if err != nil {
		return err
	}
	workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 35*time.Second)
	defer cancel()
	if entry.GrantedUserID != nil && s.authCache != nil {
		s.authCache.InvalidateAuthCacheByUserID(workCtx, *entry.GrantedUserID)
	}
	attempt := time.Now().UTC().Truncate(time.Microsecond)
	claimed, err := s.repo.ClaimApprovalNotice(workCtx, id, attempt)
	if err != nil {
		return ErrWaitlistApprovalNoticeFailed.WithCause(err)
	}
	if !claimed {
		return nil
	}
	body := waitlistApprovalHTML(s.branding.GetSiteName(workCtx), s.branding.GetFrontendURL(workCtx), entry.GrantedUserID != nil)
	sendErr := s.mailer.SendEmail(workCtx, entry.Email, waitlistApprovalSubject, body)
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	finishErr := s.repo.FinishApprovalNotice(finishCtx, id, attempt, sendErr == nil)
	if sendErr != nil {
		return ErrWaitlistApprovalNoticeFailed.WithCause(sendErr)
	}
	if finishErr != nil {
		return ErrWaitlistApprovalNoticeFailed.WithCause(finishErr)
	}
	return nil
}
