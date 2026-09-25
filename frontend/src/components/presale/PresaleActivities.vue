<template>
  <section id="presale-activities" class="activities-section" aria-labelledby="activities-title">
    <div class="activities-heading">
      <div><p class="activities-eyebrow">{{ t('presale.gift.collection') }}</p><h2 id="activities-title">{{ t(hasActiveActivity ? 'presale.gift.collectionTitle' : 'presale.gift.collectionUpcomingTitle') }}</h2></div>
      <p>{{ t('presale.gift.collectionIntro') }}</p>
    </div>
    <div class="activities-grid" :class="{ 'has-many': activities.length > 1 }">
      <article v-for="activity in activities" :key="activity.id" class="gift-activity" :class="{ 'is-upcoming': upcoming(activity) }" :data-activity-id="activity.id">
        <div class="activity-story">
          <div class="activity-kicker"><span class="activity-type">{{ t(isBalance(activity) ? 'presale.gift.balanceLabel' : 'presale.gift.daysLabel') }}</span><span class="activity-status"><i aria-hidden="true"/>{{ t(upcoming(activity) ? 'presale.gift.upcoming' : 'presale.gift.active') }}</span></div>
          <h3>{{ activity.name }}</h3>
          <p class="activity-intro">{{ t(isBalance(activity) ? 'presale.gift.intro' : 'presale.gift.daysIntro') }}</p>
          <div class="activity-dates">
            <div><span>{{ t('presale.gift.starts') }}</span><time :datetime="activity.starts_at">{{ date(activity.starts_at) }}</time></div>
            <span class="date-connector" aria-hidden="true"/>
            <div><span>{{ t('presale.gift.ends') }}</span><time :datetime="activity.ends_at">{{ date(activity.ends_at) }}</time></div>
          </div>
          <div class="gift-terms">
            <span>{{ t('presale.gift.limit', { count: activity.max_uses_per_user }) }}</span>
            <template v-if="isBalance(activity)"><span>{{ t('presale.gift.stack') }}</span><span>{{ t('presale.gift.fixedShort') }}</span></template>
          </div>
          <details v-if="isBalance(activity)" class="activity-rules"><summary>{{ t('presale.gift.rules') }}<span aria-hidden="true">+</span></summary><p>{{ t('presale.gift.endExclusive') }} <template v-if="activity.bonus_currency === 'CNY'">{{ t('presale.gift.conversionRule') }} </template>{{ t('presale.gift.refundRule') }}</p></details>
        </div>
        <div class="activity-rewards">
          <div class="reward-heading"><span>{{ t('presale.gift.rewardHeading') }}</span><svg class="reward-symbol" viewBox="0 0 64 54" fill="none" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <template v-if="isBalance(activity)"><path d="M13 27h33l-3 12c-1 6-6 9-13 9s-13-3-14-9Z M46 30h4c10 0 8 12-6 12 M9 51h42"/><g class="reward-steam"><path d="M24 20c-8-8 9-12 2-19M35 21c-7-8 9-13 2-18"/></g></template>
            <template v-else><rect x="10" y="12" width="42" height="36" rx="5"/><path d="M10 24h42M21 7v10M42 7v10M25 35h12M31 29v12"/></template>
          </svg></div>
          <a v-for="reward in rewards(activity)" :key="reward.plan_id" :href="`#presale-plan-${reward.plan_id}`" class="reward-row">
            <span class="reward-plan">{{ reward.name }}<small>{{ t('presale.gift.viewPlan') }} <span aria-hidden="true">↗</span></small></span>
            <span class="reward-value" :class="{ 'is-large-value': reward.amount >= 10000 }"><span class="reward-plus">+</span><span v-if="isBalance(activity)" class="reward-currency">{{ activity.bonus_currency === 'CNY' ? '￥' : '$' }}</span><strong>{{ amount(reward.amount) }}</strong><small v-if="!isBalance(activity) || activity.bonus_currency !== 'CNY'">{{ isBalance(activity) ? 'USD' : t('presale.gift.daysUnit') }}</small></span>
          </a>
          <p class="reward-note">{{ t(upcoming(activity) ? 'presale.gift.startsLater' : isBalance(activity) ? 'presale.gift.instant' : 'presale.gift.daysNote') }}</p>
        </div>
      </article>
    </div>
  </section>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { PresaleActivity } from '@/api/presale'
import type { SubscriptionPlan } from '@/types/payment'
const props = defineProps<{ activities: PresaleActivity[]; plans: SubscriptionPlan[]; now: number }>()
const { t, locale } = useI18n()
const hasActiveActivity = computed(() => props.activities.some(activity => Date.parse(activity.starts_at) <= props.now && props.now < Date.parse(activity.ends_at)))
const isBalance = (activity: PresaleActivity) => activity.type === 'presale_balance'
const upcoming = (activity: PresaleActivity) => Date.parse(activity.starts_at) > props.now
const amount = (value: number) => value.toLocaleString(locale.value, { maximumFractionDigits: 2 })
function rewards(activity: PresaleActivity) {
  return activity.plan_bonuses.flatMap(bonus => {
    const plan = props.plans.find(p => p.id === bonus.plan_id)
    const amount = isBalance(activity) ? bonus.bonus_balance : bonus.bonus_days
    return plan && amount && amount > 0 ? [{ plan_id: plan.id, name: plan.name, amount }] : []
  })
}
function date(value: string) {
  return new Intl.DateTimeFormat(locale.value.startsWith('zh') ? 'zh-CN' : 'en-GB', {
    timeZone: 'Asia/Shanghai', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23',
  }).format(new Date(value))
}
</script>
<style scoped>
.activities-section{margin:4px 0 64px;scroll-margin-top:120px}
.activities-heading{display:flex;justify-content:space-between;align-items:flex-end;gap:20px;margin-bottom:25px}
.activities-eyebrow{font:italic 14px Georgia,serif;color:var(--cafe-accent);letter-spacing:.025em}
.activities-heading h2{font:400 30px/1.5 Georgia,'Noto Serif CJK SC','Songti SC',serif;letter-spacing:-.035em;margin-top:6px}
.activities-heading>p{max-width:310px;font-size:12px;line-height:1.8;color:var(--cafe-muted)}
.activities-grid{display:grid;gap:22px}.activities-grid.has-many{grid-template-columns:repeat(2,minmax(0,1fr))}
.gift-activity{--gift-border:#ad864958;--gift-wash:#b999691a;--gift-value:#896231;position:relative;display:grid;grid-template-columns:minmax(0,1fr) minmax(250px,.58fr);border:1px solid var(--gift-border);border-radius:18px;overflow:visible;background:linear-gradient(115deg,var(--gift-wash),transparent 68%),var(--cafe-surface);box-shadow:0 14px 40px -28px #84602d55,inset 0 1px 0 #fff8}
.gift-activity::before{content:'';position:absolute;inset:0 14% auto;height:1px;background:linear-gradient(90deg,transparent,#caa56ca6,transparent);pointer-events:none}
.dark .gift-activity{--gift-border:#c5a06a66;--gift-wash:#cda3641c;--gift-value:#e1bf89;box-shadow:0 16px 44px -28px #bd965237,0 8px 20px -14px #0007,inset 0 1px 0 #e6c68b12}
.gift-activity.is-upcoming{--gift-border:var(--cafe-line);--gift-wash:#b9996908;--gift-value:var(--cafe-accent);box-shadow:none}
.is-upcoming::before{opacity:.3}
.activity-story{border-radius:17px 0 0 17px;padding:32px 36px 26px;background:radial-gradient(ellipse at 0 0,#b9996910,transparent 75%)}
.activity-kicker{display:flex;align-items:center;flex-wrap:wrap;gap:10px 20px;font-size:11px;letter-spacing:.05em}
.activity-type{color:var(--cafe-accent)}.activity-status{display:inline-flex;align-items:center;gap:7px;padding:4px 9px;border:1px solid #b999692e;border-radius:99px;background:#b9996912;color:var(--cafe-accent)}.activity-status i{width:5px;height:5px;background:var(--cafe-accent);border-radius:50%;box-shadow:0 0 0 4px #b9996912}
.is-upcoming .activity-status{border-color:var(--cafe-line);background:transparent;color:var(--cafe-muted)}
.is-upcoming .activity-status i{background:none;border:1px solid var(--cafe-muted);box-shadow:none}
.activity-story h3{font:400 clamp(22px,2.2vw,28px)/1.6 Georgia,'Noto Serif CJK SC','Songti SC',serif;margin-top:16px;overflow-wrap:anywhere}
.activity-intro{max-width:490px;font-size:13px;line-height:1.9;color:var(--cafe-muted);margin-top:12px}
.activity-dates{display:flex;align-items:center;gap:18px;margin-top:25px;max-width:390px}.activity-dates>div{display:flex;flex-direction:column;gap:6px}.activity-dates span{font-size:10px;color:var(--cafe-muted)}.activity-dates time{font-size:13px;font-variant-numeric:tabular-nums;white-space:nowrap}.date-connector{width:42px;border-top:1px solid var(--cafe-line);margin-top:19px;flex-shrink:1}
.gift-terms{display:flex;flex-wrap:wrap;gap:8px 18px;font-size:11px;color:var(--cafe-muted);margin-top:23px}.gift-terms>span+span{position:relative}.gift-terms>span+span:before{content:'·';position:absolute;left:-11px;color:var(--cafe-accent)}
.activity-rules{margin-top:22px;border-top:1px solid var(--cafe-line);padding-top:14px;font-size:11px;color:var(--cafe-muted)}.activity-rules summary{display:flex;align-items:center;justify-content:space-between;cursor:pointer;list-style:none;max-width:135px;gap:10px}.activity-rules summary::-webkit-details-marker{display:none}.activity-rules summary span{font-size:16px;transition:transform .25s}.activity-rules[open] summary span{transform:rotate(45deg)}.activity-rules p{margin-top:10px;max-width:510px;line-height:1.9}
.activity-rewards{position:relative;border-radius:0 17px 17px 0;display:flex;flex-direction:column;justify-content:center;padding:26px 32px;border-left:1px dashed var(--cafe-line);background:linear-gradient(145deg,#b9996918,#b9996908)}
/* Let each cutout cover the outer stroke as well as the fold line. Clipping the
   card itself at its padding box would leave a straight border across the notch.
   Panels own the corner radii; only the inward half of each cutout is visible. */
.activity-rewards:before,.activity-rewards:after{content:'';position:absolute;z-index:2;left:-10px;width:18px;height:18px;border:1px solid var(--gift-border);border-radius:50%;background:var(--cafe-page);pointer-events:none}
.activity-rewards:before{top:-10px;clip-path:inset(8px 0 0 0)}
.activity-rewards:after{bottom:-10px;clip-path:inset(0 0 8px 0)}
.reward-heading{display:flex;justify-content:space-between;align-items:center;gap:15px;margin-bottom:5px;color:var(--cafe-muted);font-size:11px;letter-spacing:.04em}.reward-symbol{width:58px;height:50px;color:var(--cafe-accent)}.reward-steam{animation:reward-breathe 5.5s ease-in-out infinite;transform-origin:30px 24px}
.reward-row{display:flex;align-items:center;justify-content:space-between;gap:16px;padding:22px 0;border-bottom:1px solid var(--cafe-line);text-decoration:none;color:inherit;transition:color .2s}.reward-row:hover,.reward-row:focus-visible{color:var(--cafe-accent)}.reward-plan{font-size:14px;overflow-wrap:anywhere}.reward-plan small{display:block;font-size:10px;color:var(--cafe-muted);margin-top:7px}.reward-value{display:flex;align-items:baseline;gap:3px;white-space:nowrap;color:var(--gift-value);font-variant-numeric:tabular-nums}.reward-plus{font-size:17px;font-weight:300;margin-right:1px}.reward-currency{font:400 22px Georgia,serif}.reward-value strong{font:400 43px/1 Georgia,serif;letter-spacing:-.045em}.reward-value small{font-size:9px;margin-left:5px;letter-spacing:.04em}.reward-note{margin-top:20px;font-size:11px;color:var(--cafe-muted);line-height:1.8}
.reward-value.is-large-value strong{font-size:clamp(20px,2.2vw,29px)}
.has-many .gift-activity{grid-template-columns:1fr;grid-template-rows:1fr auto}.has-many .activity-story{border-radius:17px 17px 0 0}.has-many .activity-rewards{border-radius:0 0 17px 17px;border-left:none;border-top:1px dashed var(--cafe-line)}.has-many .activity-rewards:before{left:-10px;top:-10px;clip-path:inset(0 0 0 8px)}.has-many .activity-rewards:after{left:auto;right:-10px;top:-10px;bottom:auto;clip-path:inset(0 8px 0 0)}
@keyframes reward-breathe{0%,100%{opacity:.45;transform:translateY(1px)}50%{opacity:.85;transform:translateY(-2px)}}
@media(max-width:760px){.activities-heading{align-items:flex-start;flex-direction:column;gap:8px}.activities-heading>p{max-width:none}.activities-grid.has-many{grid-template-columns:1fr}.gift-activity{grid-template-columns:1fr}.activity-story{border-radius:17px 17px 0 0;padding:25px 24px}.activity-rewards{border-radius:0 0 17px 17px;padding:20px 24px 23px;border-left:none;border-top:1px dashed var(--cafe-line)}.activity-rewards:before{left:-10px;top:-10px;clip-path:inset(0 0 0 8px)}.activity-rewards:after{left:auto;right:-10px;top:-10px;bottom:auto;clip-path:inset(0 8px 0 0)}.activity-dates{gap:12px}.activity-dates time{font-size:12px}.date-connector{width:25px}.reward-row{padding:18px 0}.reward-value strong{font-size:36px}.activities-section{margin-bottom:42px}}
@media(prefers-reduced-motion:reduce){.reward-steam{animation:none}}
</style>
