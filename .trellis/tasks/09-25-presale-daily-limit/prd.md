# Test checkout daily purchase limit

The test preview retained a daily purchase cap of 1500. Today's test admin usage
counted toward this cap was 1418.20, leaving 81.80. The purchase rejection was
expected backend behavior, not a payment bypass or presale success UI failure.

For repeated isolated checkout testing, set only the test payment `daily_limit`
to 0 through the existing admin API and read it back. Keep production unchanged;
do not delete/reset orders, coupon usage or subscription uniqueness checks.

Presale checkout incorrectly looked up shared payment errors solely under
`presale.errors`, exposing `daily_limit_exceeded`. Select the presale namespace
only if that translation exists; otherwise use `payment.errors`. Retain backend
metadata interpolation, including remaining allowance, and use purchase rather
than recharge wording in both languages. Cover generic and presale-specific
errors in regression tests. Refresh only the test frontend after verification.
