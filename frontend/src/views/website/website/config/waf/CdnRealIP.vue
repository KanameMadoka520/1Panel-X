<template>
    <div class="waf-cdn">
        <label>{{ $t('website.wafRealIp') }}</label>
        <span class="waf-cdn-hint">{{ $t('website.wafRealIpTip') }}</span>

        <el-select :model-value="value.mode" :disabled="disabled" class="waf-cdn-mode" @update:model-value="setMode">
            <el-option value="" :label="$t('website.wafRealIpModeDefault')" />
            <el-option value="header" :label="$t('website.wafRealIpModeHeader')" />
            <el-option value="header-list" :label="$t('website.wafRealIpModeList')" />
            <el-option value="xff-1" :label="$t('website.wafRealIpModeXff1')" />
            <el-option value="xff-2" :label="$t('website.wafRealIpModeXff2')" />
            <el-option value="xff-3" :label="$t('website.wafRealIpModeXff3')" />
        </el-select>

        <span class="waf-cdn-hint">{{ modeHint }}</span>

        <el-input
            v-if="value.mode === 'header'"
            :model-value="value.header"
            :disabled="disabled"
            class="waf-cdn-header"
            placeholder="X-Real-IP"
            @update:model-value="(v: string) => write({ header: v })"
        />

        <!-- The list is what the server actually reads, returned by the API
             rather than restated here, so the two cannot drift. -->
        <template v-if="value.mode === 'header-list'">
            <label class="waf-cdn-sub">CDN Headers</label>
            <div class="waf-cdn-list">
                <div v-for="h in cdnHeaders" :key="h">{{ h }}</div>
            </div>
        </template>

        <el-alert v-if="trustWarning" :title="trustWarning" type="warning" :closable="false" class="waf-cdn-warn" />
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Website } from '@/api/interface/website';

const { t: $t } = useI18n();

const props = withDefaults(
    defineProps<{
        modelValue: Website.WafRealIP | null;
        cdnHeaders?: string[];
        disabled?: boolean;
    }>(),
    { cdnHeaders: () => [], disabled: false },
);
const emit = defineEmits<{ (e: 'update:modelValue', value: Website.WafRealIP | null): void }>();

const value = computed<Website.WafRealIP>(() => props.modelValue ?? { mode: '', header: '' });

const modeHint = computed(() => {
    switch (value.value.mode) {
        case 'header':
            return $t('website.wafRealIpModeHeaderHint');
        case 'header-list':
            return $t('website.wafRealIpModeListHint');
        case 'xff-1':
        case 'xff-2':
        case 'xff-3':
            return $t('website.wafRealIpModeXffHint');
        default:
            return $t('website.wafRealIpModeDefaultHint');
    }
});

// Any mode that reads a header a client could have written deserves saying so
// out loud: every explicit control keys off the recovered address.
const trustWarning = computed(() => {
    if (value.value.mode === 'header-list' || value.value.mode === 'header') {
        return $t('website.wafRealIpTrustWarning');
    }
    return '';
});

const write = (patch: Partial<Website.WafRealIP>) => {
    emit('update:modelValue', { ...value.value, ...patch });
};

const setMode = (mode: string | number | boolean | undefined) => {
    const next = String(mode ?? '');
    // Modes that read a fixed source take no header name; clearing it here keeps
    // the form from showing a value nothing reads.
    emit('update:modelValue', {
        mode: next,
        header: next === 'header' ? value.value.header || 'X-Real-IP' : '',
    });
};
</script>

<style lang="scss" scoped>
.waf-cdn {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.waf-cdn-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-cdn-sub {
    margin-top: 6px;
}
.waf-cdn-mode {
    width: 100%;
    max-width: 460px;
}
.waf-cdn-header {
    width: 100%;
    max-width: 460px;
}
.waf-cdn-list {
    max-height: 220px;
    overflow-y: auto;
    padding: 8px 12px;
    border: 1px solid var(--el-border-color-light);
    color: var(--el-text-color-secondary);
    font-size: 12px;
    line-height: 1.9;
}
.waf-cdn-warn {
    margin-top: 4px;
}
</style>
