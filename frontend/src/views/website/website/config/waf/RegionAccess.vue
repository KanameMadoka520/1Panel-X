<template>
    <div class="waf-rg">
        <!-- Region control needs the panel's IP address database. Saying so is
             the whole point: a switch that silently does nothing would be worse
             than not offering one. -->
        <el-alert
            v-if="!geoAvailable"
            :title="$t('website.wafRegionNoDatabase')"
            type="warning"
            :closable="false"
            class="waf-tip"
        />

        <div v-if="inherited" class="waf-rg-inherit">
            <el-tag size="small" type="info">{{ $t('website.wafRulesInherited') }}</el-tag>
            <span class="waf-rg-hint">{{ effectiveSummary }}</span>
            <el-button link type="primary" :disabled="disabled" @click="detach">
                {{ $t('website.wafRulesOverride') }}
            </el-button>
        </div>
        <div v-else-if="scope === 'site'" class="waf-rg-inherit">
            <el-tag size="small" type="warning">{{ $t('website.wafRulesOwn') }}</el-tag>
            <el-button link type="primary" :disabled="disabled" @click="reattach">
                {{ $t('website.wafRulesFollowGlobal') }}
            </el-button>
        </div>

        <div class="waf-rg-row">
            <el-switch
                :model-value="value.enabled"
                :disabled="locked"
                @update:model-value="(on: boolean) => write({ enabled: on })"
            />
            <span class="waf-rg-hint">{{ enabledHint }}</span>
        </div>

        <div class="waf-rg-row">
            <el-radio-group :model-value="value.mode" :disabled="locked" size="small" @update:model-value="setMode">
                <el-radio-button label="deny">{{ $t('website.wafRegionModeDeny') }}</el-radio-button>
                <el-radio-button label="allow">{{ $t('website.wafRegionModeAllow') }}</el-radio-button>
            </el-radio-group>
            <span class="waf-rg-hint">{{ modeHint }}</span>
        </div>

        <el-select
            :model-value="value.regions"
            :disabled="locked"
            multiple
            filterable
            collapse-tags
            collapse-tags-tooltip
            class="waf-rg-select"
            :placeholder="$t('website.wafRegionPlaceholder')"
            @update:model-value="setRegions"
        >
            <el-option v-for="c in COUNTRIES" :key="c.code" :value="c.code" :label="`${label(c)} (${c.code})`" />
        </el-select>

        <span class="waf-rg-hint">{{ $t('website.wafRegionGranularity') }}</span>
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Website } from '@/api/interface/website';
import { COUNTRIES, Country } from './countries';

const { t: $t, locale } = useI18n();

const props = withDefaults(
    defineProps<{
        // null on a site means "follows the panel default"; the global form
        // always has a concrete policy.
        modelValue: Website.WafRegionPolicy | null;
        effective?: Website.WafRegionPolicy | null;
        scope?: 'site' | 'global';
        disabled?: boolean;
        geoAvailable?: boolean;
    }>(),
    { scope: 'site', disabled: false, effective: null, geoAvailable: true },
);
const emit = defineEmits<{ (e: 'update:modelValue', value: Website.WafRegionPolicy | null): void }>();

const DEFAULT_POLICY: Website.WafRegionPolicy = { mode: 'deny', regions: [], enabled: false };

const inherited = computed(() => props.scope === 'site' && props.modelValue === null);

// Editing in place while the site follows the panel default would silently fork
// it off without the operator saying so. The database being absent locks it too:
// nothing saved here could be enforced.
const locked = computed(() => props.disabled || inherited.value || !props.geoAvailable);

const value = computed<Website.WafRegionPolicy>(() => {
    const source = props.modelValue ?? props.effective ?? DEFAULT_POLICY;
    return { mode: source.mode || 'deny', regions: source.regions ?? [], enabled: !!source.enabled };
});

// The toggle switches the control off WITHOUT discarding the list, so an
// operator can park a policy and turn it back on later.
const enabledHint = computed(() =>
    value.value.enabled
        ? value.value.regions.length
            ? $t('website.wafRegionOn')
            : $t('website.wafRegionOnButEmpty')
        : $t('website.wafRegionOff'),
);

const label = (c: Country) => (locale.value.startsWith('zh') ? c.zh : c.en);

const modeHint = computed(() =>
    value.value.mode === 'allow' ? $t('website.wafRegionModeAllowHint') : $t('website.wafRegionModeDenyHint'),
);

const effectiveSummary = computed(() => {
    const eff = props.effective;
    if (!eff || !eff.regions?.length) return $t('website.wafRegionNone');
    const names = eff.regions.map((code) => {
        const found = COUNTRIES.find((c) => c.code === code);
        return found ? label(found) : code;
    });
    return (
        (eff.mode === 'allow' ? $t('website.wafRegionModeAllow') : $t('website.wafRegionModeDeny')) +
        ': ' +
        names.join(', ')
    );
});

const write = (patch: Partial<Website.WafRegionPolicy>) => {
    emit('update:modelValue', { ...value.value, ...patch });
};

const setMode = (mode: string | number | boolean | undefined) => write({ mode: String(mode) });
// Picking regions turns the control on: an operator who has just chosen
// countries has said what they want, and leaving the switch off would quietly
// enforce nothing.
const setRegions = (regions: string[]) =>
    write({ regions: [...regions].sort(), enabled: value.value.enabled || regions.length > 0 });

// Detaching copies the currently effective policy so switching to an override
// changes nothing until the operator actually edits something.
const detach = () => emit('update:modelValue', { ...value.value });
const reattach = () => emit('update:modelValue', null);
</script>

<style lang="scss" scoped>
.waf-rg {
    display: flex;
    flex-direction: column;
    gap: 10px;
}
.waf-rg-inherit {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}
.waf-rg-row {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
}
.waf-rg-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-rg-select {
    width: 100%;
}
</style>
