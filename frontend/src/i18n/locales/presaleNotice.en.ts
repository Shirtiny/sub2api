export default {
  errors: {
    "PRESALE_NOTICE_URL_INVALID": "Configure a valid frontend URL first.",
    "PRESALE_NOTICE_CONFIRM_REQUIRED": "Confirm the content and recipients first.",
    "PRESALE_NOTICE_CHANGED": "The notice, audience or month changed. Refresh and review again.",
    "PRESALE_NOTICE_NOT_OPEN": "The presale is not open. Publish eligible plans first, or send a test email.",
    "PRESALE_NOTICE_EMAIL_INVALID": "Enter a valid recipient email address.",
    "PRESALE_NOTICE_TEST_COOLDOWN": "Wait 60 seconds before sending another test email.",
    "PRESALE_NOTICE_RECEIPT_FAILED": "The delivery record is unconfirmed. Check your email provider; do not resend.",
    "PRESALE_NOTICE_SEND_UNCONFIRMED": "Delivery is unconfirmed and sending has stopped. Check your provider records. No automatic retry will occur."
},
  entry: 'Presale notice', title: 'Announce the next presale', intro: 'An invitation for the month ahead. Preview first, then confirm delivery.',
  audience: 'Recipients', access: 'Users with site access', includeRestricted: 'Include users without site access', audienceHint: 'Includes matching waitlist addresses. Does not grant access or publish any plans.',
  language: 'Email language', month: 'Presale month', preview: 'Email preview', refresh: 'Refresh preview',
  eligible: 'Mailboxes', sent: 'Submitted', pending: 'Pending', skipped: 'Unsubscribed', uncertain: 'Unconfirmed', sendingCount: 'Processing',
  draft: 'The presale is not open. Preview and test emails are available; formal notices require payments enabled and published presale plans.',
  testEmail: 'Test recipient', test: 'Send test email', testSuccess: 'Test email submitted. Please check your inbox.', invalidEmail: 'Enter a valid email address',
  send: 'Send announcement', confirmTitle: 'Confirm presale announcement',
  confirm: 'Notify {count} pending mailboxes about the {month} presale. {audience} Each mailbox is notified once per month.',
  includeSummary: 'Includes users without site access.', accessSummary: 'Only users with site access.',
  safety: 'Messages are sent individually, without exposing other recipients. Unsubscribed recipients are skipped. Unconfirmed deliveries are never retried automatically.',
  keepOpen: 'Keep this window open. Pause or close it to stop after the current message; remaining recipients can be resumed later.',
  stop: 'Pause sending', stopping: 'Pausing…', complete: 'This send is complete', paused: 'Paused. You can resume the remaining recipients.',
  failed: 'The operation did not complete. Refresh the preview before trying again.', loadFailed: 'Unable to load the email preview.', noPending: 'No pending recipients in the selected audience.',
  receiptWarning: 'Some deliveries are processing or unconfirmed. Check your email provider records; these will not be retried automatically.'
}
