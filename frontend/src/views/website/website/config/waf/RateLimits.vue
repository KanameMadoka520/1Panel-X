<template>
    <div class="waf-rl">
        <div v-for="row in rows" :key="row.kind" class="waf-rl-row">
            <div class="waf-rl-head">
                <el-switch
                    :model-value="row.enabled"
                    :disabled="disabled"
                    @update:model-value="(v: boolean) => toggle(row.kind, v)"
                />
                <strong>{{ $t(`website.wafRl_${row.kind}`) }}</strong>
                <el-tag v-if="row.kind === 'attack'" size="small" type="info">
                    {{ $t('website.wafRlGatewayWide') }}
                </el-tag>
            </div>
            <span class="waf-rl-hint">{{ summary(row) }}</span>
            <div v-if="row.enabled" class="waf-rl-fields">
                <el-form-item v-if="row.kind === 'access'" :label="$t('website.wafRlMode')">
                    <el-select
                        :model-value="row.perUrl ? 'url' : 'global'"
                        :disabled="disabled"
                        size="small"
                        class="waf-rl-mode"
                        @update:model-value="(v: string) => patch(row.kind, { perUrl: v === 'url' })"
                    >
                        <el-option value="global" :label="$t('website.wafRlModeGlobal')" />
                        <el-option value="url" :label="$t('website.wafRlModeUrl')" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('website.wafRlPeriod')">
                    <el-input-number
                        :model-value="row.periodSec"
                        :min="1"
                        :max="3600"
                        :disabled="disabled"
                        size="small"
                        controls-position="right"
                        @update:model-value="(v: number) => patch(row.kind, { periodSec: v })"
                    />
                </el-form-item>
                <el-form-item :label="$t('website.wafRlThreshold')">
                    <el-input-number
                        :model-value="row.threshold"
                        :min="1"
                        :max="1000000"
                        :disabled="disabled"
                        size="small"
                        controls-position="right"
                        @update:model-value="(v: number) => patch(row.kind, { threshold: v })"
                    />
                </el-form-item>
                <el-form-item :label="$t('website.wafRlBan')">
                    <el-input-number
                        :model-value="row.banSec"
                        :min="0"
                        :max="86400"
                        :disabled="disabled"
                        size="small"
                        controls-position="right"
                        @update:model-value="(v: number) => patch(row.kind, { banSec: v })"
                    />
                </el-form-item>
            </div>
        </div>
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Website } from '@/api/interface/website';

const { t: $t } = useI18n();

const props = withDefaults(
    defineProps<{
        modelValue: Website.WafRateLimit[];
        // The attack limit is enforced gateway-wide (a rule match is only visible
        // through the shared engine's callback, which carries no Host), so it is
        // only offered on the global form.
        scope?: 'site' | 'global';
        disabled?: boolean;
    }>(),
    { scope: 'site', disabled: false },
);
const emit = defineEmits<{ (e: 'update:modelValue', value: Website.WafRateLimit[]): void }>();

// Defaults are the starting point when an operator switches a limit on; they are
// never applied implicitly, so a limit that is off stays absent from the policy.
const DEFAULTS: Record<Website.WafRateLimitKind, Website.WafRateLimit> = {
    access: { kind: 'access', periodSec: 10, threshold: 200, banSec: 600, perUrl: false },
    url: { kind: 'url', periodSec: 10, threshold: 100, banSec: 600, perUrl: true },
    notfound: { kind: 'notfound', periodSec: 60, threshold: 30, banSec: 600, perUrl: false },
    attack: { kind: 'attack', periodSec: 60, threshold: 10, banSec: 600, perUrl: false },
};

const kinds = computed<Website.WafRateLimitKind[]>(() =>
    props.scope === 'global' ? ['access', 'url', 'notfound', 'attack'] : ['access', 'url', 'notfound'],
);

type Row = Website.WafRateLimit & { enabled: boolean };

const rows = computed<Row[]>(() =>
    kinds.value.map((kind) => {
        const found = (props.modelValue || []).find((l) => l.kind === kind);
        return found ? { ...found, enabled: true } : { ...DEFAULTS[kind], enabled: false };
    }),
);

const summary = (row: Row) => {
    if (!row.enabled) return $t(`website.wafRlOff_${row.kind}`);
    const key = row.banSec > 0 ? 'website.wafRlSummary' : 'website.wafRlSummaryNoBan';
    return $t(key, [row.periodSec, row.threshold, row.banSec]);
};

// Only enabled limits are emitted: a limit that is off must be ABSENT from the
// policy, not present with a disabled flag, so the gateway never receives a rule
// it is not meant to enforce.
const write = (next: Row[]) => {
    const limits = next
        .filter((r) => r.enabled)
        .map((r) => ({
            kind: r.kind,
            periodSec: r.periodSec,
            threshold: r.threshold,
            banSec: r.banSec,
            perUrl: r.perUrl,
        }));
    emit('update:modelValue', limits);
};

const toggle = (kind: Website.WafRateLimitKind, on: boolean) => {
    write(rows.value.map((r) => (r.kind === kind ? { ...r, enabled: on } : r)));
};

const patch = (kind: Website.WafRateLimitKind, fields: Partial<Website.WafRateLimit>) => {
    write(rows.value.map((r) => (r.kind === kind ? { ...r, ...fields } : r)));
};
</script>

<style lang="scss" scoped>
.waf-rl {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.waf-rl-row {
    padding: 10px 12px;
    border: 1px solid var(--el-border-color-light);
}
.waf-rl-head {
    display: flex;
    align-items: center;
    gap: 10px;
}
.waf-rl-hint {
    display: block;
    margin-top: 4px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-rl-fields {
    display: flex;
    flex-wrap: wrap;
    gap: 0 16px;
    margin-top: 8px;
}
.waf-rl-fields :deep(.el-form-item) {
    margin-bottom: 6px;
}
.waf-rl-mode {
    width: 130px;
}
</style>
