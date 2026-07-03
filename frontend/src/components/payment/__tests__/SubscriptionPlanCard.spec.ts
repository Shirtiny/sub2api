import { mount } from "@vue/test-utils";
import { describe, expect, it } from "vitest";
import { createI18n } from "vue-i18n";
import SubscriptionPlanCard from "../SubscriptionPlanCard.vue";

const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackWarn: false,
  missingWarn: false,
  messages: {
    en: {
      payment: {
        days: "days",
        renewNow: "Renew",
        subscribeNow: "Subscribe now",
        planCard: {
          currentMultiplier: "Current multiplier",
          dailyLimit: "Daily",
          models: "Models",
          monthlyLimit: "Monthly",
          multiplier: "Multiplier",
          quota: "Quota",
          rate: "Rate",
          unlimited: "Unlimited",
          weeklyLimit: "Weekly",
        },
      },
    },
  },
});

const SelectStub = {
  name: "Select",
  props: ["modelValue", "options"],
  emits: ["update:modelValue"],
  template: `
    <select
      data-testid="multiplier-select"
      :value="modelValue"
      @change="$emit('update:modelValue', Number($event.target.value))"
    >
      <option v-for="option in options" :key="option.value" :value="option.value">
        {{ option.label }}
      </option>
    </select>
  `,
};

const mountPlanCard = (groupPlatform: string) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: {
        id: 1,
        group_id: 10,
        group_platform: groupPlatform,
        name: "Pro",
        price: 10,
        amount: 1000,
        features: [],
        rate_multiplier: 1,
        validity_days: 30,
        validity_unit: "day",
        supported_model_scopes: ["claude", "gemini_text", "gemini_image"],
        is_active: true,
      },
    },
    global: { plugins: [i18n], stubs: { Select: SelectStub } },
  });

const customPlan = () => ({
  id: 2,
  group_id: 20,
  group_platform: "openai",
  name: "Custom Pro",
  price: 100,
  original_price: 120,
  amount: 1000,
  features: [],
  rate_multiplier: 1,
  daily_limit_usd: 10,
  weekly_limit_usd: 20,
  monthly_limit_usd: 30,
  validity_days: 30,
  validity_unit: "day",
  supported_model_scopes: [],
  is_active: true,
  custom_multiplier_enabled: true,
  custom_multiplier_min: 2,
  custom_multiplier_max: 4,
});

const mountCustomPlanCard = (props = {}) =>
  mount(SubscriptionPlanCard, {
    props: {
      plan: customPlan(),
      ...props,
    },
    global: { plugins: [i18n], stubs: { Select: SelectStub } },
  });

describe("SubscriptionPlanCard", () => {
  it("does not show Antigravity model scopes for OpenAI plans", () => {
    const text = mountPlanCard("openai").text();

    expect(text).not.toContain("Claude");
    expect(text).not.toContain("Gemini");
    expect(text).not.toContain("Imagen");
  });

  it("shows model scopes for Antigravity plans", () => {
    const text = mountPlanCard("antigravity").text();

    expect(text).toContain("Claude");
    expect(text).toContain("Gemini");
    expect(text).toContain("Imagen");
  });

  it("scales price and limits and emits selected custom multiplier", async () => {
    const wrapper = mountCustomPlanCard();

    expect(wrapper.text()).toContain("\u00a5200");
    expect(wrapper.text()).toContain("$20");
    expect(wrapper.text()).toContain("$40");
    expect(wrapper.text()).toContain("$60");

    await wrapper.find('[data-testid="multiplier-select"]').setValue("4");

    expect(wrapper.text()).toContain("\u00a5400");
    expect(wrapper.text()).toContain("$40");
    expect(wrapper.text()).toContain("$80");
    expect(wrapper.text()).toContain("$120");

    await wrapper.find('button[type="button"]').trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[1]).toBe(4);
  });

  it("allows custom multiplier minimum of 1x", async () => {
    const wrapper = mountCustomPlanCard({
      plan: {
        ...customPlan(),
        custom_multiplier_min: 1,
        custom_multiplier_max: 3,
      },
    });

    expect(wrapper.text()).toContain("\u00a5100");

    await wrapper.find('[data-testid="multiplier-select"]').setValue("3");

    expect(wrapper.text()).toContain("\u00a5300");
    await wrapper.find('button[type="button"]').trigger("click");
    expect(wrapper.emitted("select")?.[0]?.[1]).toBe(3);
  });

  it("locks custom renewal multiplier and hides selector", () => {
    const wrapper = mountCustomPlanCard({
      activeSubscriptions: [
        {
          id: 1,
          user_id: 7,
          group_id: 99,
          status: "active",
          expires_at: new Date(Date.now() + 86400000).toISOString(),
          group: {
            id: 99,
            name: "Custom Pro-user",
            rate_multiplier: 1,
            is_custom_subscription_group: true,
            custom_source_plan_id: 2,
            custom_multiplier: 3,
          },
        },
      ],
    });

    expect(wrapper.find('[data-testid="multiplier-select"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("3x");
    expect(wrapper.text()).toContain("\u00a5300");
  });

  it("does not treat a source-group subscription as a locked custom renewal", () => {
    const wrapper = mountCustomPlanCard({
      activeSubscriptions: [
        {
          id: 1,
          user_id: 7,
          group_id: 20,
          status: "active",
          expires_at: new Date(Date.now() + 86400000).toISOString(),
          group: {
            id: 20,
            name: "Source Group",
            rate_multiplier: 1,
            is_custom_subscription_group: false,
          },
        },
      ],
    });

    expect(wrapper.find('[data-testid="multiplier-select"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("\u00a5200");
  });

  it("does not lock selector for expired custom subscriptions", () => {
    const wrapper = mountCustomPlanCard({
      activeSubscriptions: [
        {
          id: 1,
          user_id: 7,
          group_id: 99,
          status: "active",
          expires_at: new Date(Date.now() - 86400000).toISOString(),
          group: {
            id: 99,
            name: "Custom Pro-user",
            rate_multiplier: 1,
            is_custom_subscription_group: true,
            custom_source_plan_id: 2,
            custom_multiplier: 3,
          },
        },
      ],
    });

    expect(wrapper.find('[data-testid="multiplier-select"]').exists()).toBe(true);
    expect(wrapper.text()).toContain("\u00a5200");
  });

  it("shows coupon pay amount with the effective price struck through", () => {
    const wrapper = mountCustomPlanCard({ couponPayAmount: 150 });

    expect(wrapper.text()).toContain("\u00a5200");
    expect(wrapper.text()).toContain("\u00a5150");
    expect(wrapper.find('.line-through').exists()).toBe(true);
  });
});
