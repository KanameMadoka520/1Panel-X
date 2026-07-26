<template>
    <div class="waf-cfg">
        <div class="waf-cfg-block">
            <label>{{ $t('website.wafBlockPage') }}</label>
            <span class="waf-cfg-hint">{{ $t('website.wafBlockPageTip') }}</span>
            <div class="waf-cfg-row">
                <span class="waf-cfg-label">{{ $t('website.wafBlockPageStatus') }}</span>
                <el-select v-model="blockPage.status" size="small" class="waf-cfg-status">
                    <el-option :value="403" label="403" />
                    <el-option :value="404" label="404" />
                    <el-option :value="200" label="200" />
                </el-select>
                <span class="waf-cfg-hint">{{ statusHint }}</span>
            </div>
            <el-input
                v-model="blockPage.html"
                type="textarea"
                :rows="8"
                resize="vertical"
                :maxlength="MAX_PAGE_BYTES"
                show-word-limit
                :placeholder="$t('website.wafBlockPagePlaceholder')"
            />
            <span class="waf-cfg-hint">{{ $t('website.wafBlockPagePlaceholders') }}</span>
        </div>

        <div class="waf-cfg-block">
            <label>{{ $t('website.wafLogSettings') }}</label>
            <span class="waf-cfg-hint">{{ $t('website.wafLogSettingsTip') }}</span>
            <div class="waf-cfg-row">
                <span class="waf-cfg-label">{{ $t('website.wafLogRetention') }}</span>
                <el-input-number
                    v-model="log.retentionDays"
                    :min="1"
                    :max="3650"
                    size="small"
                    controls-position="right"
                />
                <span class="waf-cfg-hint">{{ $t('website.wafLogRetentionTip') }}</span>
            </div>
            <div class="waf-cfg-row">
                <span class="waf-cfg-label">{{ $t('website.wafLogMaxSize') }}</span>
                <el-input-number v-model="log.maxMb" :min="0" :max="8192" size="small" controls-position="right" />
                <span class="waf-cfg-hint">{{ $t('website.wafLogMaxSizeTip') }}</span>
            </div>
            <div class="waf-cfg-row waf-cfg-row-top">
                <span class="waf-cfg-label">{{ $t('website.wafLogExcluded') }}</span>
                <div class="waf-cfg-kinds">
                    <el-checkbox
                        v-for="kind in recordKinds"
                        :key="kind"
                        :model-value="log.excludedKinds.includes(kind)"
                        @update:model-value="(on: boolean) => toggleKind(kind, on)"
                    >
                        {{ kindLabel(kind) }}
                    </el-checkbox>
                </div>
            </div>
            <span class="waf-cfg-hint">{{ $t('website.wafLogExcludedTip') }}</span>
        </div>
    </div>
</template>

<script lang="ts" setup>
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { Website } from '@/api/interface/website';

const { t: $t } = useI18n();

const MAX_PAGE_BYTES = 65536;

const props = withDefaults(
    defineProps<{
        blockPage: Website.WafBlockPage;
        log: Website.WafLogSettings;
        // Supplied by the server so the UI can never offer a kind the data plane
        // does not know.
        recordKinds?: string[];
    }>(),
    { recordKinds: () => [] },
);

// Both objects are edited in place by the parent's reactive form, so the child
// reads them directly rather than round-tripping through v-model.
const blockPage = computed(() => props.blockPage);
const log = computed(() => props.log);
const recordKinds = computed(() => props.recordKinds);

const statusHint = computed(() =>
    blockPage.value.status === 200 ? $t('website.wafBlockPageStatus200Hint') : $t('website.wafBlockPageStatusHint'),
);

const kindLabel = (kind: string) => {
    const key = `website.wafKind_${kind.replace(/-/g, '_')}`;
    const label = $t(key);
    // An unknown kind is shown as its raw name rather than as a missing
    // translation key, so a new server-side kind is still usable.
    return label === key ? kind : label;
};

const toggleKind = (kind: string, on: boolean) => {
    const next = new Set(log.value.excludedKinds);
    if (on) {
        next.add(kind);
    } else {
        next.delete(kind);
    }
    log.value.excludedKinds = recordKinds.value.filter((k) => next.has(k));
};
</script>

<style lang="scss" scoped>
.waf-cfg {
    display: flex;
    flex-direction: column;
    gap: 18px;
}
.waf-cfg-block {
    display: flex;
    flex-direction: column;
    gap: 6px;
}
.waf-cfg-row {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}
.waf-cfg-row-top {
    align-items: flex-start;
}
.waf-cfg-label {
    min-width: 92px;
}
.waf-cfg-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-cfg-status {
    width: 100px;
}
.waf-cfg-kinds {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 16px;
}
</style>
