package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// tokenRefreshTempUnschedDuration token 鍒锋柊閲嶈瘯鑰楀敖鍚庝复鏃朵笉鍙皟搴︾殑鎸佺画鏃堕棿
const tokenRefreshTempUnschedDuration = 10 * time.Minute

// TokenRefreshService OAuth token鑷姩鍒锋柊鏈嶅姟
// 瀹氭湡妫€鏌ュ苟鍒锋柊鍗冲皢杩囨湡鐨則oken
type TokenRefreshService struct {
	accountRepo      AccountRepository
	refreshers       []TokenRefresher
	executors        []OAuthRefreshExecutor // 涓?refreshers 涓€涓€瀵瑰簲鐨?executor锛堝甫 CacheKey锛?
	refreshPolicy    BackgroundRefreshPolicy
	cfg              *config.TokenRefreshConfig
	cacheInvalidator TokenCacheInvalidator
	schedulerCache   SchedulerCache // 鐢ㄤ簬鍚屾鏇存柊璋冨害鍣ㄧ紦瀛橈紝瑙ｅ喅 token 鍒锋柊鍚庣紦瀛樹笉涓€鑷撮棶棰?
	tempUnschedCache TempUnschedCache
	refreshAPI       *OAuthRefreshAPI // 缁熶竴鍒锋柊 API
	runtimeBlocker   AccountRuntimeBlocker

	// OpenAI privacy: 鍒锋柊鎴愬姛鍚庢鏌ュ苟璁剧疆 training opt-out
	privacyClientFactory PrivacyClientFactory
	proxyRepo            ProxyRepository

	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

// NewTokenRefreshService 鍒涘缓token鍒锋柊鏈嶅姟
func NewTokenRefreshService(
	accountRepo AccountRepository,
	oauthService *OAuthService,
	openaiOAuthService *OpenAIOAuthService,
	geminiOAuthService *GeminiOAuthService,
	antigravityOAuthService *AntigravityOAuthService,
	cacheInvalidator TokenCacheInvalidator,
	schedulerCache SchedulerCache,
	cfg *config.Config,
	tempUnschedCache TempUnschedCache,
) *TokenRefreshService {
	s := &TokenRefreshService{
		accountRepo:      accountRepo,
		refreshPolicy:    DefaultBackgroundRefreshPolicy(),
		cfg:              &cfg.TokenRefresh,
		cacheInvalidator: cacheInvalidator,
		schedulerCache:   schedulerCache,
		tempUnschedCache: tempUnschedCache,
		stopCh:           make(chan struct{}),
	}

	openAIRefresher := NewOpenAITokenRefresher(openaiOAuthService, accountRepo)

	claudeRefresher := NewClaudeTokenRefresher(oauthService)
	geminiRefresher := NewGeminiTokenRefresher(geminiOAuthService)
	agRefresher := NewAntigravityTokenRefresher(antigravityOAuthService)

	// 娉ㄥ唽骞冲彴鐗瑰畾鐨勫埛鏂板櫒锛圱okenRefresher 鎺ュ彛锛?
	s.refreshers = []TokenRefresher{
		claudeRefresher,
		openAIRefresher,
		geminiRefresher,
		agRefresher,
	}

	// 娉ㄥ唽瀵瑰簲鐨?OAuthRefreshExecutor锛堝甫 CacheKey 鏂规硶锛?
	s.executors = []OAuthRefreshExecutor{
		claudeRefresher,
		openAIRefresher,
		geminiRefresher,
		agRefresher,
	}

	return s
}

// SetPrivacyDeps 娉ㄥ叆 OpenAI privacy opt-out 鎵€闇€渚濊禆
func (s *TokenRefreshService) SetPrivacyDeps(factory PrivacyClientFactory, proxyRepo ProxyRepository) {
	s.privacyClientFactory = factory
	s.proxyRepo = proxyRepo
}

// SetRefreshAPI 娉ㄥ叆缁熶竴鐨?OAuth 鍒锋柊 API
func (s *TokenRefreshService) SetRefreshAPI(api *OAuthRefreshAPI) {
	s.refreshAPI = api
}

// SetRefreshPolicy 娉ㄥ叆鍚庡彴鍒锋柊璋冪敤渚х瓥鐣ワ紙鐢ㄤ簬鏄惧紡鍖栧钩鍙?鍦烘櫙宸紓琛屼负锛夈€?
func (s *TokenRefreshService) SetRefreshPolicy(policy BackgroundRefreshPolicy) {
	s.refreshPolicy = policy
}

func (s *TokenRefreshService) SetAccountRuntimeBlocker(blocker AccountRuntimeBlocker) {
	s.runtimeBlocker = blocker
}

func (s *TokenRefreshService) notifyAccountSchedulingBlocked(account *Account, until time.Time, reason string) {
	if s == nil || s.runtimeBlocker == nil || account == nil {
		return
	}
	s.runtimeBlocker.BlockAccountScheduling(account, until, reason)
}

func (s *TokenRefreshService) notifyAccountSchedulingBlockCleared(accountID int64) {
	if s == nil || s.runtimeBlocker == nil || accountID <= 0 {
		return
	}
	s.runtimeBlocker.ClearAccountSchedulingBlock(accountID)
}

// Start 鍚姩鍚庡彴鍒锋柊鏈嶅姟
func (s *TokenRefreshService) Start() {
	if !s.cfg.Enabled {
		slog.Info("token_refresh.service_disabled")
		return
	}

	s.wg.Add(1)
	go s.refreshLoop()

	slog.Info("token_refresh.service_started",
		"check_interval_minutes", s.cfg.CheckIntervalMinutes,
		"refresh_before_expiry_hours", s.cfg.RefreshBeforeExpiryHours,
	)
}

// Stop 鍋滄鍒锋柊鏈嶅姟锛堝彲瀹夊叏澶氭璋冪敤锛?
func (s *TokenRefreshService) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
	slog.Info("token_refresh.service_stopped")
}

// refreshLoop 鍒锋柊寰幆
func (s *TokenRefreshService) refreshLoop() {
	defer s.wg.Done()

	// 璁＄畻妫€鏌ラ棿闅?
	checkInterval := time.Duration(s.cfg.CheckIntervalMinutes) * time.Minute
	if checkInterval < time.Minute {
		checkInterval = 5 * time.Minute
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	// 鍚姩鏃剁珛鍗虫墽琛屼竴娆℃鏌?
	s.processRefresh()

	for {
		select {
		case <-ticker.C:
			s.processRefresh()
		case <-s.stopCh:
			return
		}
	}
}

// processRefresh 鎵ц涓€娆″埛鏂版鏌?
func (s *TokenRefreshService) processRefresh() {
	ctx := context.Background()

	// 璁＄畻鍒锋柊绐楀彛
	refreshWindow := time.Duration(s.cfg.RefreshBeforeExpiryHours * float64(time.Hour))

	// 鑾峰彇鎵€鏈塧ctive鐘舵€佺殑璐﹀彿
	accounts, err := s.listActiveAccounts(ctx)
	if err != nil {
		slog.Error("token_refresh.list_accounts_failed", "error", err)
		return
	}

	totalAccounts := len(accounts)
	oauthAccounts := 0 // 鍙埛鏂扮殑OAuth璐﹀彿鏁?
	needsRefresh := 0
	refreshed, failed, skipped := 0, 0, 0

	for i := range accounts {
		account := &accounts[i]

		// 閬嶅巻鎵€鏈夊埛鏂板櫒锛屾壘鍒拌兘澶勭悊姝よ处鍙风殑
		for idx, refresher := range s.refreshers {
			if !refresher.CanRefresh(account) {
				continue
			}

			oauthAccounts++

			// 妫€鏌ユ槸鍚﹂渶瑕佸埛鏂?
			if !refresher.NeedsRefresh(account, refreshWindow) {
				break // 涓嶉渶瑕佸埛鏂帮紝璺宠繃
			}

			needsRefresh++

			// 鑾峰彇瀵瑰簲鐨?executor
			var executor OAuthRefreshExecutor
			if idx < len(s.executors) {
				executor = s.executors[idx]
			}

			// 鎵ц鍒锋柊
			if err := s.refreshWithRetry(ctx, account, refresher, executor, refreshWindow); err != nil {
				if errors.Is(err, errRefreshSkipped) {
					skipped++
				} else {
					slog.Warn("token_refresh.account_refresh_failed",
						"account_id", account.ID,
						"account_name", account.Name,
						"error", err,
					)
					failed++
				}
			} else {
				slog.Info("token_refresh.account_refreshed",
					"account_id", account.ID,
					"account_name", account.Name,
				)
				refreshed++
			}

			// 姣忎釜璐﹀彿鍙敱涓€涓猺efresher澶勭悊
			break
		}
	}

	// 鏃犲埛鏂版椿鍔ㄦ椂闄嶇骇涓?Debug锛屾湁瀹為檯鍒锋柊娲诲姩鏃朵繚鎸?Info
	if needsRefresh == 0 && failed == 0 {
		slog.Debug("token_refresh.cycle_completed",
			"total", totalAccounts, "oauth", oauthAccounts,
			"needs_refresh", needsRefresh, "refreshed", refreshed, "skipped", skipped, "failed", failed)
	} else {
		slog.Info("token_refresh.cycle_completed",
			"total", totalAccounts,
			"oauth", oauthAccounts,
			"needs_refresh", needsRefresh,
			"refreshed", refreshed,
			"skipped", skipped,
			"failed", failed,
		)
	}
}

// listActiveAccounts 鑾峰彇鎵€鏈塧ctive鐘舵€佺殑璐﹀彿
// 浣跨敤ListActive纭繚鍒锋柊鎵€鏈夋椿璺冭处鍙风殑token锛堝寘鎷复鏃剁鐢ㄧ殑锛?
func (s *TokenRefreshService) listActiveAccounts(ctx context.Context) ([]Account, error) {
	return s.accountRepo.ListActive(ctx)
}

// refreshWithRetry 甯﹂噸璇曠殑鍒锋柊
func (s *TokenRefreshService) refreshWithRetry(ctx context.Context, account *Account, refresher TokenRefresher, executor OAuthRefreshExecutor, refreshWindow time.Duration) error {
	var lastErr error

	for attempt := 1; attempt <= s.cfg.MaxRetries; attempt++ {
		var newCredentials map[string]any
		var err error

		// 浼樺厛浣跨敤缁熶竴 API锛堝甫鍒嗗竷寮忛攣 + DB 閲嶈淇濇姢锛?
		if s.refreshAPI != nil && executor != nil {
			result, refreshErr := s.refreshAPI.RefreshIfNeeded(ctx, account, executor, refreshWindow)
			if refreshErr != nil {
				err = refreshErr
			} else if result.LockHeld {
				// 閿佽鍏朵粬 worker 鎸佹湁锛岀敱璋冪敤渚х瓥鐣ュ喅瀹氬浣曡鏁?
				return s.refreshPolicy.handleLockHeld()
			} else if !result.Refreshed {
				// 宸茶鍏朵粬璺緞鍒锋柊锛岀敱璋冪敤渚х瓥鐣ュ喅瀹氬浣曡鏁?
				return s.refreshPolicy.handleAlreadyRefreshed()
			} else {
				account = result.Account
				_ = result.NewCredentials // 缁熶竴 API 宸茶缃?_token_version 骞舵洿鏂?DB锛屾棤闇€閲嶅鎿嶄綔
			}
		} else {
			// 闄嶇骇锛氱洿鎺ヨ皟鐢?refresher锛堝吋瀹规棫璺緞锛?
			newCredentials, err = refresher.Refresh(ctx, account)
			if newCredentials != nil {
				newCredentials["_token_version"] = time.Now().UnixMilli()
				if saveErr := persistAccountCredentials(ctx, s.accountRepo, account, newCredentials); saveErr != nil {
					return fmt.Errorf("failed to save credentials: %w", saveErr)
				}
			}
		}

		if err == nil {
			s.postRefreshActions(ctx, account)
			return nil
		}

		// 涓嶅彲閲嶈瘯閿欒锛坕nvalid_grant/invalid_client 绛夛級鐩存帴鏍囪 error 鐘舵€佸苟杩斿洖
		if isNonRetryableRefreshError(err) {
			errorMsg := fmt.Sprintf("Token refresh failed (non-retryable): %v", err)
			if !account.IsPoolMode() {
				if setErr := s.accountRepo.SetError(ctx, account.ID, errorMsg); setErr != nil {
					slog.Error("token_refresh.set_error_status_failed",
						"account_id", account.ID,
						"error", setErr,
					)
				}
			}
			s.notifyAccountSchedulingBlocked(account, time.Time{}, "token_refresh_non_retryable")
			// 鍒锋柊澶辫触浣?access_token 鍙兘浠嶆湁鏁堬紝灏濊瘯璁剧疆闅愮
			s.ensureOpenAIPrivacy(ctx, account)
			s.ensureAntigravityPrivacy(ctx, account)
			return err
		}

		lastErr = err
		slog.Warn("token_refresh.retry_attempt_failed",
			"account_id", account.ID,
			"attempt", attempt,
			"max_retries", s.cfg.MaxRetries,
			"error", err,
		)

		// 濡傛灉杩樻湁閲嶈瘯鏈轰細锛岀瓑寰呭悗閲嶈瘯
		if attempt < s.cfg.MaxRetries {
			// 鎸囨暟閫€閬匡細2^(attempt-1) * baseSeconds
			backoff := time.Duration(s.cfg.RetryBackoffSeconds) * time.Second * time.Duration(1<<(attempt-1))
			time.Sleep(backoff)
		}
	}

	// 鍙噸璇曢敊璇€楀敖锛氫复鏃舵爣璁拌处鍙蜂笉鍙皟搴︼紝閬垮厤璇锋眰璺緞鍙嶅鍛戒腑宸茬煡澶辫触鐨勮处鍙?
	slog.Warn("token_refresh.retry_exhausted",
		"account_id", account.ID,
		"platform", account.Platform,
		"max_retries", s.cfg.MaxRetries,
		"error", lastErr,
	)

	// 鍒锋柊澶辫触浣?access_token 鍙兘浠嶆湁鏁堬紝灏濊瘯璁剧疆闅愮
	s.ensureOpenAIPrivacy(ctx, account)
	s.ensureAntigravityPrivacy(ctx, account)

	// 璁剧疆涓存椂涓嶅彲璋冨害 10 鍒嗛挓锛堜笉鏍囪 error锛屼繚鎸?status=active 璁╀笅涓埛鏂板懆鏈熻兘缁х画灏濊瘯锛?
	until := time.Now().Add(tokenRefreshTempUnschedDuration)
	reason := fmt.Sprintf("token refresh retry exhausted: %v", lastErr)
	s.notifyAccountSchedulingBlocked(account, until, "token_refresh_retry_exhausted")
	if setErr := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); setErr != nil {
		slog.Warn("token_refresh.set_temp_unschedulable_failed",
			"account_id", account.ID,
			"error", setErr,
		)
	} else {
		slog.Info("token_refresh.temp_unschedulable_set",
			"account_id", account.ID,
			"until", until.Format(time.RFC3339),
		)
	}

	return lastErr
}

// postRefreshActions 鍒锋柊鎴愬姛鍚庣殑鍚庣画鍔ㄤ綔锛堟竻闄ら敊璇姸鎬併€佺紦瀛樺け鏁堛€佽皟搴﹀櫒鍚屾绛夛級
func (s *TokenRefreshService) postRefreshActions(ctx context.Context, account *Account) {
	// Antigravity 璐︽埛锛氬鏋滀箣鍓嶆槸鍥犱负缂哄皯 project_id 鑰屾爣璁颁负 error锛岀幇鍦ㄦ垚鍔熻幏鍙栧埌浜嗭紝娓呴櫎閿欒鐘舵€?
	if account.Platform == PlatformAntigravity &&
		account.Status == StatusError &&
		strings.Contains(account.ErrorMessage, "missing_project_id:") {
		if clearErr := s.accountRepo.ClearError(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_account_error_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
		} else {
			slog.Info("token_refresh.cleared_missing_project_id_error", "account_id", account.ID)
			s.notifyAccountSchedulingBlockCleared(account.ID)
		}
	}
	// 鍒锋柊鎴愬姛鍚庢竻闄や复鏃朵笉鍙皟搴︾姸鎬侊紙澶勭悊 OAuth 401 鎭㈠鍦烘櫙锛?
	if account.TempUnschedulableUntil != nil && time.Now().Before(*account.TempUnschedulableUntil) {
		if clearErr := s.accountRepo.ClearTempUnschedulable(ctx, account.ID); clearErr != nil {
			slog.Warn("token_refresh.clear_temp_unschedulable_failed",
				"account_id", account.ID,
				"error", clearErr,
			)
		} else {
			slog.Info("token_refresh.cleared_temp_unschedulable", "account_id", account.ID)
			s.notifyAccountSchedulingBlockCleared(account.ID)
		}
		// 鍚屾娓呴櫎 Redis 缂撳瓨锛岄伩鍏嶈皟搴﹀櫒璇诲埌杩囨湡鐨勪复鏃朵笉鍙皟搴︾姸鎬?
		if s.tempUnschedCache != nil {
			if clearErr := s.tempUnschedCache.DeleteTempUnsched(ctx, account.ID); clearErr != nil {
				slog.Warn("token_refresh.clear_temp_unsched_cache_failed",
					"account_id", account.ID,
					"error", clearErr,
				)
			}
		}
	}
	// 瀵规墍鏈?OAuth 璐﹀彿璋冪敤缂撳瓨澶辨晥锛圛nvalidateToken 鍐呴儴鏍规嵁骞冲彴鍒ゆ柇鏄惁闇€瑕佸鐞嗭級
	if s.cacheInvalidator != nil && account.Type == AccountTypeOAuth {
		if err := s.cacheInvalidator.InvalidateToken(ctx, account); err != nil {
			slog.Warn("token_refresh.invalidate_token_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
		} else {
			slog.Debug("token_refresh.token_cache_invalidated", "account_id", account.ID)
		}
	}
	// 鍚屾鏇存柊璋冨害鍣ㄧ紦瀛橈紝纭繚璋冨害鑾峰彇鐨?Account 瀵硅薄鍖呭惈鏈€鏂扮殑 credentials
	if s.schedulerCache != nil {
		if err := s.schedulerCache.SetAccount(ctx, account); err != nil {
			slog.Warn("token_refresh.sync_scheduler_cache_failed",
				"account_id", account.ID,
				"error", err,
			)
		} else {
			slog.Debug("token_refresh.scheduler_cache_synced", "account_id", account.ID)
		}
	}
	// OpenAI OAuth: 鍒锋柊鎴愬姛鍚庯紝妫€鏌ユ槸鍚﹀凡璁剧疆 privacy_mode锛屾湭璁剧疆鍒欏皾璇曞叧闂缁冩暟鎹叡浜?
	s.ensureOpenAIPrivacy(ctx, account)
	// Antigravity OAuth: 鍒锋柊鎴愬姛鍚庯紝妫€鏌ユ槸鍚﹀凡璁剧疆 privacy_mode锛屾湭璁剧疆鍒欒皟鐢?setUserSettings
	s.ensureAntigravityPrivacy(ctx, account)
}

// errRefreshSkipped 琛ㄧず鍒锋柊琚烦杩囷紙閿佺珵浜夋垨宸茶鍏朵粬璺緞鍒锋柊锛夛紝涓嶈鍏?failed 鎴?refreshed
var errRefreshSkipped = fmt.Errorf("refresh skipped")

// isNonRetryableRefreshError 鍒ゆ柇鏄惁涓轰笉鍙噸璇曠殑鍒锋柊閿欒
// 杩欎簺閿欒閫氬父琛ㄧず鍑瘉宸插け鏁堟垨閰嶇疆纭疄缂哄け锛岄渶瑕佺敤鎴烽噸鏂版巿鏉?// 娉ㄦ剰锛歮issing_project_id 閿欒鍙湪鐪熸缂哄け锛堜粠鏈幏鍙栬繃锛夋椂杩斿洖锛屼复鏃惰幏鍙栧け璐ヤ笉浼氳繑鍥炴閿欒
func isNonRetryableRefreshError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	nonRetryable := []string{
		"invalid_grant",
		"refresh_token_reused",
		"invalid_client",
		"unauthorized_client",
		"access_denied",
		"missing_project_id",
		"no refresh token available",
	}
	for _, needle := range nonRetryable {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// ensureOpenAIPrivacy 妫€鏌?OpenAI OAuth 璐﹀彿鏄惁宸茶缃?privacy_mode锛?// 鏈缃垯璋冪敤 disableOpenAITraining 骞舵寔涔呭寲缁撴灉鍒?Extra銆?
func (s *TokenRefreshService) ensureOpenAIPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth {
		return
	}
	if s.privacyClientFactory == nil {
		return
	}
	if shouldSkipOpenAIPrivacyEnsure(account.Extra) {
		return
	}

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
	}

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}

	mode := disableOpenAITraining(ctx, s.privacyClientFactory, token, proxyURL)
	if mode == "" {
		return
	}

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("token_refresh.update_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
	} else {
		slog.Info("token_refresh.privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
	}
}

// ensureAntigravityPrivacy 鍚庡彴鍒锋柊涓鏌?Antigravity OAuth 璐﹀彿闅愮鐘舵€併€?// 浠呭綋 privacy_mode 宸叉垚鍔熻缃紙"privacy_set"锛夋椂璺宠繃锛?// 鏈缃垨涔嬪墠澶辫触锛?privacy_set_failed"锛夊潎浼氶噸璇曘€?
func (s *TokenRefreshService) ensureAntigravityPrivacy(ctx context.Context, account *Account) {
	if account.Platform != PlatformAntigravity || account.Type != AccountTypeOAuth {
		return
	}
	if account.Extra != nil {
		if mode, ok := account.Extra["privacy_mode"].(string); ok && mode == AntigravityPrivacySet {
			return
		}
	}

	token, _ := account.Credentials["access_token"].(string)
	if token == "" {
		return
	}

	projectID, _ := account.Credentials["project_id"].(string)

	var proxyURL string
	if account.ProxyID != nil && s.proxyRepo != nil {
		if p, err := s.proxyRepo.GetByID(ctx, *account.ProxyID); err == nil && p != nil {
			proxyURL = p.URL()
		}
	}

	mode := setAntigravityPrivacy(ctx, token, projectID, proxyURL)
	if mode == "" {
		return
	}

	if err := s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{"privacy_mode": mode}); err != nil {
		slog.Warn("token_refresh.update_antigravity_privacy_mode_failed",
			"account_id", account.ID,
			"error", err,
		)
	} else {
		applyAntigravityPrivacyMode(account, mode)
		slog.Info("token_refresh.antigravity_privacy_mode_set",
			"account_id", account.ID,
			"privacy_mode", mode,
		)
	}
}
