<template>
    <div class="waf-dr">
        <div v-if="inherited" class="waf-dr-inherit">
            <el-tag size="small" type="info">{{ $t('website.wafRulesInherited') }}</el-tag>
            <span class="waf-dr-hint">{{ $t('website.wafRulesInheritedTip') }}</span>
            <el-button link type="primary" :disabled="disabled" @click="detach">
                {{ $t('website.wafRulesOverride') }}
            </el-button>
        </div>
        <div v-else-if="scope === 'site'" class="waf-dr-inherit">
            <el-tag size="small" type="warning">{{ $t('website.wafRulesOwn') }}</el-tag>
            <el-button link type="primary" :disabled="disabled" @click="reattach">
                {{ $t('website.wafRulesFollowGlobal') }}
            </el-button>
        </div>

        <div class="waf-dr-row">
            <el-switch :model-value="!value.disableSqli" :disabled="locked" @update:model-value="setSqli" />
            <div class="waf-dr-copy">
                <strong>{{ $t('website.wafRuleSqli') }}</strong>
                <span>{{ $t('website.wafRuleSqliTip') }}</span>
            </div>
        </div>

        <div class="waf-dr-row">
            <el-switch :model-value="!value.disableXss" :disabled="locked" @update:model-value="setXss" />
            <div class="waf-dr-copy">
                <strong>{{ $t('website.wafRuleXss') }}</strong>
                <span>{{ $t('website.wafRuleXssTip') }}</span>
            </div>
        </div>

        <div class="waf-dr-row">
            <el-switch :model-value="value.strict" :disabled="locked" @update:model-value="setStrict" />
            <div class="waf-dr-copy">
                <strong>{{ $t('website.wafRuleStrict') }}</strong>
                <span>{{ $t('website.wafRuleStrictTip') }}</span>
            </div>
        </div>

        <div class="waf-dr-methods">
            <div class="waf-dr-copy">
                <strong>{{ $t('website.wafRuleMethods') }}</strong>
                <span>{{ $t('website.wafRuleMethodsTip') }}</span>
            </div>
            <el-checkbox
                :model-value="methodFilterOn"
                :disabled="locked"
                class="waf-dr-methods-toggle"
                @update:model-value="toggleMethodFilter"
            >
                {{ $t('website.wafRuleMethodsEnable') }}
            </el-checkbox>
            <div v-if="methodFilterOn" class="waf-dr-method-grid">
                <el-checkbox
                    v-for="m in METHODS"
                    :key="m"
                    :model-value="value.allowedMethods.includes(m)"
                    :disabled="locked"
                    @update:model-value="(on: boolean) => setMethod(m, on)"
                >
                    {{ m }}
                </el-checkbox>
            </div>
        </div>

        <div class="waf-dr-methods">
            <div class="waf-dr-copy">
                <strong>{{ $t('website.wafRuleUploads') }}</strong>
                <span>{{ $t('website.wafRuleUploadsTip') }}</span>
            </div>
            <el-select
                :model-value="value.bannedUploadExts"
                :disabled="locked"
                multiple
                filterable
                allow-create
                default-first-option
                class="waf-dr-ext-input"
                :placeholder="$t('website.wafRuleUploadsPlaceholder')"
                @update:model-value="setUploadExts"
            />
            <el-button
                v-if="!locked && value.bannedUploadExts.length === 0"
                link
                type="primary"
                class="waf-dr-methods-toggle"
                @click="setUploadExts(COMMON_BANNED_EXTS)"
            >
                {{ $t('website.wafRuleUploadsPreset') }}
            </el-button>
            <div v-if="extError" class="waf-dr-error">{{ extError }}</div>
        </div>
    </div>
</template>

<script lang="ts" setup>
import { computed, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { Website } from '@/api/interface/website';

const { t: $t } = useI18n();

const props = withDefaults(
    defineProps<{
        // null on a site means "follows the panel default"; the global form always
        // has a concrete policy.
        modelValue: Website.WafRulePolicy | null;
        // effective is what the gateway actually enforces, shown read-only while
        // the site is following the panel default.
        effective?: Website.WafRulePolicy | null;
        scope?: 'site' | 'global';
        disabled?: boolean;
    }>(),
    { scope: 'site', disabled: false, effective: null },
);
const emit = defineEmits<{ (e: 'update:modelValue', value: Website.WafRulePolicy | null): void }>();

// The methods the panel offers. GET/POST/HEAD/OPTIONS are what a plain web
// application needs; the rest are offered because an API or WebDAV endpoint may
// legitimately require them.
const METHODS = [
    'GET',
    'POST',
    'HEAD',
    'OPTIONS',
    'PUT',
    'DELETE',
    'PATCH',
    'TRACE',
    'CONNECT',
    'PROPFIND',
    'PROPPATCH',
    'MKCOL',
    'COPY',
    'MOVE',
    'LOCK',
    'UNLOCK',
    'SEARCH',
];

// The extensions most often used to drop a web shell. Offered as a one-click
// starting point rather than applied by default, because banning uploads a site
// legitimately accepts is its own kind of outage.
const COMMON_BANNED_EXTS = ['php', 'jsp', 'asp', 'aspx', 'exe', 'sh'];

// Mirrors the server-side charset check so a bad entry is refused while the
// operator is still looking at the form instead of failing the whole save.
const EXT_PATTERN = /^[A-Za-z0-9]{1,15}$/;

const DEFAULT_POLICY: Website.WafRulePolicy = {
    disableSqli: false,
    disableXss: false,
    strict: false,
    allowedMethods: [],
    bannedUploadExts: [],
};

const inherited = computed(() => props.scope === 'site' && props.modelValue === null);

// While a site follows the panel default its switches show the effective policy
// but cannot be changed — editing them in place would silently fork the site off
// the default without the operator saying so.
const locked = computed(() => props.disabled || inherited.value);

const value = computed<Website.WafRulePolicy>(() => {
    const source = props.modelValue ?? props.effective ?? DEFAULT_POLICY;
    return {
        ...DEFAULT_POLICY,
        ...source,
        allowedMethods: source.allowedMethods ?? [],
        bannedUploadExts: source.bannedUploadExts ?? [],
    };
});

const extError = ref('');

const methodFilterOn = computed(() => value.value.allowedMethods.length > 0);

const write = (patch: Partial<Website.WafRulePolicy>) => {
    emit('update:modelValue', { ...value.value, ...patch });
};

const setSqli = (on: boolean) => write({ disableSqli: !on });
const setXss = (on: boolean) => write({ disableXss: !on });
const setStrict = (on: boolean) => write({ strict: on });

const toggleMethodFilter = (on: boolean) => {
    // Turning the filter on starts from the methods an ordinary site needs
    // rather than from an empty list, which would refuse every request.
    write({ allowedMethods: on ? ['GET', 'HEAD', 'POST', 'OPTIONS'] : [] });
};

const setMethod = (method: string, on: boolean) => {
    const next = new Set(value.value.allowedMethods);
    if (on) {
        next.add(method);
    } else {
        next.delete(method);
    }
    write({ allowedMethods: METHODS.filter((m) => next.has(m)) });
};

const setUploadExts = (raw: string[]) => {
    const cleaned: string[] = [];
    const rejected: string[] = [];
    for (const entry of raw) {
        const ext = String(entry).trim().replace(/^\./, '');
        if (!ext) continue;
        if (!EXT_PATTERN.test(ext)) {
            rejected.push(entry);
            continue;
        }
        const lower = ext.toLowerCase();
        if (!cleaned.includes(lower)) cleaned.push(lower);
    }
    extError.value = rejected.length ? $t('website.wafRuleUploadsInvalid', [rejected.join(', ')]) : '';
    write({ bannedUploadExts: cleaned.sort() });
};

// Detaching copies the currently effective policy so switching to an override
// changes nothing until the operator actually edits something.
const detach = () => emit('update:modelValue', { ...value.value });
const reattach = () => emit('update:modelValue', null);
</script>

<style lang="scss" scoped>
.waf-dr {
    display: flex;
    flex-direction: column;
    gap: 12px;
}
.waf-dr-inherit {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
}
.waf-dr-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-dr-row {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--el-border-color-light);
}
.waf-dr-copy {
    display: flex;
    flex-direction: column;
    gap: 3px;
}
.waf-dr-copy span {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-dr-methods {
    padding: 10px 12px;
    border: 1px solid var(--el-border-color-light);
}
.waf-dr-methods-toggle {
    margin-top: 8px;
}
.waf-dr-method-grid {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 16px;
    margin-top: 6px;
}
.waf-dr-ext-input {
    margin-top: 8px;
    width: 100%;
}
.waf-dr-error {
    margin-top: 6px;
    color: var(--el-color-danger);
    font-size: 12px;
}
</style>
