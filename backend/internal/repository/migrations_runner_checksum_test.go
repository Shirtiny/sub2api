package repository

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestIsMigrationChecksumCompatible(t *testing.T) {
	t.Run("054历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.True(t, ok)
	})

	t.Run("054在未知文件checksum下不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"054_drop_legacy_cache_columns.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"0000000000000000000000000000000000000000000000000000000000000000",
		)
		require.False(t, ok)
	})

	t.Run("061历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"08a248652cbab7cfde147fc6ef8cda464f2477674e20b718312faa252e0481c0",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("061第二个历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"061_add_usage_log_request_type.sql",
			"222b4a09c797c22e5922b6b172327c824f5463aaa8760e4f621bc5c22e2be0f3",
			"66207e7aa5dd0429c2e2c0fabdaf79783ff157fa0af2e81adff2ee03790ec65c",
		)
		require.True(t, ok)
	})

	t.Run("非白名单迁移不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"001_init.sql",
			"182c193f3359946cf094090cd9e57d5c3fd9abaffbc1e8fc378646b8a6fa12b4",
			"82de761156e03876653e7a6a4eee883cd927847036f779b0b9f34c42a8af7a7d",
		)
		require.False(t, ok)
	})

	t.Run("109历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109当前checksum可兼容历史checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
		)
		require.True(t, ok)
	})

	t.Run("109回滚到历史文件后仍兼容已应用的新checksum", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"109_auth_identity_compat_backfill.sql",
			"0580b4602d85435edf9aca1633db580bb3932f26517f75134106f80275ec2ace",
			"551e498aa5616d2d91096e9d72cf9fb36e418ee22eacc557f8811cadbc9e20ee",
		)
		require.True(t, ok)
	})

	t.Run("110历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"110_pending_auth_and_provider_default_grants.sql",
			"e3d1f433be2b564cfbdc549adf98fce13c5c7b363ebc20fd05b765d0563b0925",
			"32cf87ee787b1bb36b5c691367c96eee37518fa3eed6f3322cf68795e3745279",
		)
		require.True(t, ok)
	})

	t.Run("112历史checksum可兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"112_add_payment_order_provider_key_snapshot.sql",
			"ffd3e8a2c9295fa9cbefefd629a78268877e5b51bc970a82d9b3f46ec4ebd15e",
			"b75f8f56d39455682787696a3d92ad25b055444ca328fb7fca9a460a15d68d99",
		)
		require.True(t, ok)
	})

	t.Run("115历史checksum可兼容修复后的legacy external backfill", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"115_auth_identity_legacy_external_backfill.sql",
			"4cf39e508be9fd1a5aa41610cbbebeb80385c9adda45bf78a706de9db4f1385f",
			"022aadd97bb53e755f0cf7a3a957e0cb1a1353b0c39ec4de3234acd2871fd04f",
		)
		require.True(t, ok)
	})

	t.Run("116历史checksum可兼容修复后的legacy external safety reports", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"116_auth_identity_legacy_external_safety_reports.sql",
			"f7757bd929ac67ffb08ce69fa4cf20fad39dbff9d5a5085fb2adabb7607e5877",
			"07edb09fa8d04ffb172b0621e3c22f4d1757d20a24ae267b3b36b087ab72d488",
		)
		require.True(t, ok)
	})

	t.Run("161 historical checksum is compatible with embedded virtual custom entitlement migration", func(t *testing.T) {
		content, err := migrations.FS.ReadFile("161_user_subscription_virtual_custom_entitlement.sql")
		require.NoError(t, err)
		sum := sha256.Sum256([]byte(strings.TrimSpace(string(content))))

		ok := isMigrationChecksumCompatible(
			"161_user_subscription_virtual_custom_entitlement.sql",
			"c8bb22ddaecc46efbee7bc60ac3cfeabef4c96c9662df76d1dc10829676b9c26",
			hex.EncodeToString(sum[:]),
		)
		require.True(t, ok)
	})

	t.Run("161 historical checksum is compatible with LF checkout", func(t *testing.T) {
		content, err := migrations.FS.ReadFile("161_user_subscription_virtual_custom_entitlement.sql")
		require.NoError(t, err)
		lfContent := strings.ReplaceAll(string(content), "\r\n", "\n")
		sum := sha256.Sum256([]byte(strings.TrimSpace(lfContent)))

		ok := isMigrationChecksumCompatible(
			"161_user_subscription_virtual_custom_entitlement.sql",
			"c8bb22ddaecc46efbee7bc60ac3cfeabef4c96c9662df76d1dc10829676b9c26",
			hex.EncodeToString(sum[:]),
		)
		require.True(t, ok)
	})

	t.Run("119历史checksum可兼容占位文件", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"0bbe809ae48a9d811dabda1ba1c74955bd71c4a9cc610f9128816818dfa6c11e",
		)
		require.True(t, ok)
	})

	t.Run("118多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"a38243ca0a72c3a01c0a92b7986423054d6133c0399441f853b99802852720fb",
			"e0cdf835d6c688d64100f483d31bc02ac9ebad414bf1837af239a84bf75b8227",
		} {
			ok := isMigrationChecksumCompatible(
				"118_wechat_dual_mode_and_auth_source_defaults.sql",
				dbChecksum,
				"b54194d7a3e4fbf710e0a3590d22a2fe7966804c487052a356e0b55f53ef96b0",
			)
			require.True(t, ok)
		}
	})

	t.Run("120多个历史checksum都可兼容新的notx修复版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"e77921f79d539bc24575cb9c16cbe566d2b23ce816190343d0a7568f6a3fcf61",
			"707431450603e70a43ce9fbd61e0c12fa67da4875158ccefabacea069587ab22",
			"04b082b5a239c525154fe9185d324ee2b05ff90da9297e10dba19f9be79aa59a",
		} {
			ok := isMigrationChecksumCompatible(
				"120_enforce_payment_orders_out_trade_no_unique_notx.sql",
				dbChecksum,
				"34aadc0db59a4e390f92a12b73bd74642d9724f33124f73638ae00089ea5e074",
			)
			require.True(t, ok)
		}
	})

	t.Run("155多个历史checksum都可兼容当前版本", func(t *testing.T) {
		for _, dbChecksum := range []string{
			"33d7246686c64a786bbfd18ff141645098bcde953d81bf17e8dcbe40b994129d",
			"a490c4f55170254c174d3a1a96ea65b88747cd9cc99bed48e18c6c5d0681eec1",
		} {
			ok := isMigrationChecksumCompatible(
				"155_hash_api_keys.sql",
				dbChecksum,
				"1d7e3f980ad52f821ef84b5a644bb9a53c4dfe57d061f502ac73743a2cf18341",
			)
			require.True(t, ok)
		}
	})

	t.Run("157 historical checksum is compatible with embedded custom subscription migration", func(t *testing.T) {
		content, err := migrations.FS.ReadFile("157_custom_subscription_multiplier.sql")
		require.NoError(t, err)
		sum := sha256.Sum256(content)

		ok := isMigrationChecksumCompatible(
			"157_custom_subscription_multiplier.sql",
			"ae7eadf2b5c908d46a89c7176c0e6204987ff2bf38ef7aee6fb2e75dfb334213",
			hex.EncodeToString(sum[:]),
		)
		require.True(t, ok)
	})

	t.Run("157 historical checksum is compatible with LF checkout", func(t *testing.T) {
		content, err := migrations.FS.ReadFile("157_custom_subscription_multiplier.sql")
		require.NoError(t, err)
		lfContent := strings.ReplaceAll(string(content), "\r\n", "\n")
		sum := sha256.Sum256([]byte(lfContent))

		ok := isMigrationChecksumCompatible(
			"157_custom_subscription_multiplier.sql",
			"ae7eadf2b5c908d46a89c7176c0e6204987ff2bf38ef7aee6fb2e75dfb334213",
			hex.EncodeToString(sum[:]),
		)
		require.True(t, ok)
	})

	t.Run("157 previously applied checksum is compatible with embedded custom subscription migration", func(t *testing.T) {
		content, err := migrations.FS.ReadFile("157_custom_subscription_multiplier.sql")
		require.NoError(t, err)
		sum := sha256.Sum256(content)
		fileChecksum := hex.EncodeToString(sum[:])

		for _, dbChecksum := range []string{
			"d235b7e458372c21e4a27100d9089b2ce6e91af424ed8a8221b6e0f3d2f0a023",
			"ab8d5a06dc3f638fa399bc5c22ab3a8a35b21ee21974366e2333b51128e63cef",
		} {
			ok := isMigrationChecksumCompatible(
				"157_custom_subscription_multiplier.sql",
				dbChecksum,
				fileChecksum,
			)
			require.True(t, ok)
		}
	})

	t.Run("119未知checksum不兼容", func(t *testing.T) {
		ok := isMigrationChecksumCompatible(
			"119_enforce_payment_orders_out_trade_no_unique.sql",
			"ebd2c67cce0116393fb4f1b5d5116a67c6aceb73820dfb5133d1ff6f36d72d34",
			"ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		)
		require.False(t, ok)
	})
}
