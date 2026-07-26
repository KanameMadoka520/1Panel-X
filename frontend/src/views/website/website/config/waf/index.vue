<template>
    <div>
        <el-alert :title="$t('website.wafTip')" type="info" :closable="false" class="waf-tip" />
        <div class="waf-control">
            <div class="waf-control-copy">
                <strong>{{ $t('website.wafProtection') }}</strong>
                <span>{{ protectionDescription }}</span>
            </div>
            <el-switch
                v-model="enabled"
                :loading="saving"
                :disabled="loadingStatus || !status.supported"
                @change="updateConfig"
            />
            <el-select
                v-model="mode"
                :disabled="saving || !enabled"
                size="small"
                class="waf-mode"
                @change="updateConfig"
            >
                <el-option value="inherit" :label="inheritModeLabel" />
                <el-option value="detection" :label="$t('website.wafDetectionMode')" />
                <el-option value="block" :label="$t('website.wafBlockMode')" />
            </el-select>
            <el-tag v-if="status.protected" type="success">{{ $t('website.wafProtected') }}</el-tag>
            <el-tag v-else-if="enabled" type="warning">{{ $t('website.wafPending') }}</el-tag>
            <el-tag v-else type="info">{{ $t('website.wafDisabled') }}</el-tag>
            <el-button size="small" :disabled="globalLoading" @click="globalOpen = true">
                {{ $t('website.wafGlobalSettings') }}
            </el-button>
        </div>
        <el-alert v-if="status.lastError" :title="status.lastError" type="error" :closable="false" class="waf-error" />

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafAccessControl') }}</strong>
                <span>{{ $t('website.wafAccessControlTip') }}</span>
            </div>
            <div class="waf-acl-lists">
                <div class="waf-acl-col">
                    <label>{{ $t('website.wafDenyList') }}</label>
                    <span class="waf-acl-hint">{{ $t('website.wafDenyListTip') }}</span>
                    <el-input
                        v-model="denyText"
                        type="textarea"
                        :rows="6"
                        :disabled="!status.supported"
                        :placeholder="aclPlaceholder"
                        resize="vertical"
                    />
                </div>
                <div class="waf-acl-col">
                    <label>{{ $t('website.wafAllowList') }}</label>
                    <span class="waf-acl-hint">{{ $t('website.wafAllowListTip') }}</span>
                    <el-input
                        v-model="allowText"
                        type="textarea"
                        :rows="6"
                        :disabled="!status.supported"
                        :placeholder="aclPlaceholder"
                        resize="vertical"
                    />
                </div>
            </div>
            <div class="waf-acl-actions">
                <el-button type="primary" size="small" :loading="saving" :disabled="!status.supported" @click="saveAcl">
                    {{ $t('commons.button.save') }}
                </el-button>
            </div>
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafDefaultRules') }}</strong>
                <span>{{ $t('website.wafDefaultRulesTip') }}</span>
            </div>
            <DefaultRules
                v-model="siteRules"
                :effective="status.effectiveRules"
                scope="site"
                :disabled="!status.supported"
            />
            <div class="waf-acl-actions">
                <el-button type="primary" size="small" :loading="saving" :disabled="!status.supported" @click="saveAcl">
                    {{ $t('commons.button.save') }}
                </el-button>
            </div>
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafRegion') }}</strong>
                <span>{{ $t('website.wafRegionTip') }}</span>
            </div>
            <RegionAccess
                v-model="siteRegion"
                :effective="status.effectiveRegion"
                :geo-available="status.geoAvailable"
                scope="site"
                :disabled="!status.supported"
            />
            <div class="waf-acl-actions">
                <el-button type="primary" size="small" :loading="saving" :disabled="!status.supported" @click="saveAcl">
                    {{ $t('commons.button.save') }}
                </el-button>
            </div>
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafBans') }}</strong>
                <span>{{ $t('website.wafBansScope') }}</span>
            </div>
            <BanRecords />
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafLists') }}</strong>
                <span>{{ $t('website.wafListsScope') }}</span>
            </div>
            <BlackWhiteLists />
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafRulesCustom') }}</strong>
                <span>{{ $t('website.wafListsScope') }}</span>
            </div>
            <CustomRules />
        </div>

        <div class="waf-acl">
            <div class="waf-acl-head">
                <strong>{{ $t('website.wafRateLimit') }}</strong>
                <span>{{ $t('website.wafRateLimitTip') }}</span>
            </div>
            <RateLimits v-model="rateLimits" scope="site" :disabled="!status.supported" />
            <div class="waf-acl-actions">
                <el-button type="primary" size="small" :loading="saving" :disabled="!status.supported" @click="saveAcl">
                    {{ $t('commons.button.save') }}
                </el-button>
            </div>
        </div>

        <div class="waf-bar">
            <el-radio-group v-model="range" @change="search" size="small">
                <el-radio-button label="24h">{{ $t('website.monitorRange24h') }}</el-radio-button>
                <el-radio-button label="7d">{{ $t('website.monitorRange7d') }}</el-radio-button>
                <el-radio-button label="30d">{{ $t('website.monitorRange30d') }}</el-radio-button>
            </el-radio-group>
            <el-select
                v-model="category"
                @change="search"
                size="small"
                clearable
                :placeholder="$t('website.wafCategoryAll')"
                class="waf-cat"
            >
                <el-option v-for="c in categories" :key="c" :label="c" :value="c" />
            </el-select>
        </div>

        <ComplexTable :data="data" :pagination-config="paginationConfig" @search="search()">
            <el-table-column :label="$t('commons.table.date')" prop="time" width="160">
                <template #default="{ row }">{{ fmtTime(row.time) }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafSourceIp')" prop="sourceIP" width="140" show-overflow-tooltip />
            <el-table-column label="Method" prop="method" width="90" />
            <el-table-column label="URI" prop="uri" min-width="200" show-overflow-tooltip />
            <el-table-column :label="$t('website.wafRule')" min-width="180" show-overflow-tooltip>
                <template #default="{ row }">
                    <span>[{{ row.ruleID }}] {{ row.ruleMsg }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafCategory')" prop="category" width="100">
                <template #default="{ row }">
                    <el-tag v-if="row.category" size="small">{{ row.category }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafSeverity')" prop="severity" width="100">
                <template #default="{ row }">
                    <el-tag v-if="row.severity" size="small" :type="sevType(row.severity)">{{ row.severity }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafAction')" prop="action" width="100">
                <template #default="{ row }">
                    <el-tag size="small" :type="row.action === 'blocked' ? 'danger' : 'warning'">
                        {{ row.action === 'blocked' ? $t('website.wafBlocked') : $t('website.wafDetected') }}
                    </el-tag>
                </template>
            </el-table-column>
        </ComplexTable>

        <el-dialog v-model="globalOpen" :title="$t('website.wafGlobalSettings')" width="680px">
            <el-alert :title="$t('website.wafGlobalTip')" type="info" :closable="false" class="waf-tip" />
            <div class="waf-global-mode">
                <label>{{ $t('website.wafGlobalDefaultMode') }}</label>
                <el-select v-model="globalForm.defaultMode" size="small" class="waf-mode">
                    <el-option value="detection" :label="$t('website.wafDetectionMode')" />
                    <el-option value="block" :label="$t('website.wafBlockMode')" />
                </el-select>
            </div>
            <div class="waf-acl-lists">
                <div class="waf-acl-col">
                    <label>{{ $t('website.wafDenyList') }}</label>
                    <span class="waf-acl-hint">{{ $t('website.wafDenyListTip') }}</span>
                    <el-input
                        v-model="globalDenyText"
                        type="textarea"
                        :rows="6"
                        :placeholder="aclPlaceholder"
                        resize="vertical"
                    />
                </div>
                <div class="waf-acl-col">
                    <label>{{ $t('website.wafAllowList') }}</label>
                    <span class="waf-acl-hint">{{ $t('website.wafAllowListTip') }}</span>
                    <el-input
                        v-model="globalAllowText"
                        type="textarea"
                        :rows="6"
                        :placeholder="aclPlaceholder"
                        resize="vertical"
                    />
                </div>
            </div>
            <div class="waf-global-rl">
                <label>{{ $t('website.wafRateLimit') }}</label>
                <span class="waf-acl-hint">{{ $t('website.wafGlobalRateLimitTip') }}</span>
                <RateLimits v-model="globalForm.rateLimits" scope="global" />
            </div>
            <div class="waf-global-rl">
                <label>{{ $t('website.wafDefaultRules') }}</label>
                <span class="waf-acl-hint">{{ $t('website.wafGlobalDefaultRulesTip') }}</span>
                <DefaultRules v-model="globalRules" scope="global" />
            </div>
            <div class="waf-global-rl">
                <label>{{ $t('website.wafRegion') }}</label>
                <span class="waf-acl-hint">{{ $t('website.wafGlobalRegionTip') }}</span>
                <RegionAccess v-model="globalRegion" :geo-available="globalGeoAvailable" scope="global" />
            </div>
            <div class="waf-global-rl">
                <WafSettings :block-page="globalBlockPage" :log="globalLog" :record-kinds="globalRecordKinds" />
            </div>
            <template #footer>
                <el-button @click="globalOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="globalSaving" @click="saveGlobal">
                    {{ $t('commons.button.save') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { GetWafGlobal, GetWafStatus, LoadWafEvents, UpdateWafGlobal, UpdateWafSite } from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';
import RateLimits from './RateLimits.vue';
import BlackWhiteLists from './BlackWhiteLists.vue';
import BanRecords from './BanRecords.vue';
import DefaultRules from './DefaultRules.vue';
import CustomRules from './CustomRules.vue';
import RegionAccess from './RegionAccess.vue';
import WafSettings from './WafSettings.vue';

const { t: $t } = useI18n();

const props = defineProps({
    id: {
        type: Number,
        default: 0,
    },
});

const range = ref('7d');
const category = ref('');
const categories = ['sqli', 'xss', 'rce', 'lfi', 'rfi', 'php', 'protocol', 'scanner'];
const data = ref<Website.WafEvent[]>([]);
const status = reactive<Website.WafSiteStatus>({
    websiteID: props.id,
    supported: false,
    enabled: false,
    mode: 'inherit',
    effectiveMode: 'detection',
    allowList: [],
    denyList: [],
    rateLimits: [],
    effectiveRateLimits: [],
    rules: null,
    effectiveRules: {
        disableSqli: false,
        disableXss: false,
        strict: false,
        allowedMethods: [],
        bannedUploadExts: [],
    },
    region: null,
    effectiveRegion: { mode: 'deny', regions: [] },
    geoAvailable: false,
    installed: false,
    ready: false,
    routed: false,
    protected: false,
    lastError: '',
});
const enabled = ref(false);
const mode = ref<Website.WafSiteUpdate['mode']>('inherit');
const allowText = ref('');
const denyText = ref('');
const rateLimits = ref<Website.WafRateLimit[]>([]);
const siteRules = ref<Website.WafRulePolicy | null>(null);
const siteRegion = ref<Website.WafRegionPolicy | null>(null);
const globalRules = ref<Website.WafRulePolicy>({
    disableSqli: false,
    disableXss: false,
    strict: false,
    allowedMethods: [],
    bannedUploadExts: [],
});
const aclPlaceholder = '203.0.113.10\n198.51.100.0/24\n2001:db8::/32';
const loadingStatus = ref(false);
const saving = ref(false);
const globalOpen = ref(false);
const globalSaving = ref(false);
const globalLoading = ref(false);
const globalForm = reactive<Website.WafGlobalConfig>({
    defaultMode: 'detection',
    allowList: [],
    denyList: [],
    rateLimits: [],
});
const globalRegion = ref<Website.WafRegionPolicy>({ mode: 'deny', regions: [] });
// Reported by the server: false means the IP address database region control
// needs is not installed, so the control is shown as unavailable rather than as
// a switch that cannot take effect.
const globalGeoAvailable = ref(true);
const globalBlockPage = reactive<Website.WafBlockPage>({ status: 403, html: '' });
const globalLog = reactive<Website.WafLogSettings>({ retentionDays: 30, excludedKinds: [] });
// Supplied by the server so the UI can never offer a record kind the data plane
// does not know.
const globalRecordKinds = ref<string[]>([]);
const globalAllowText = ref('');
const globalDenyText = ref('');
const inheritModeLabel = computed(() => {
    const label = globalForm.defaultMode === 'block' ? $t('website.wafBlockMode') : $t('website.wafDetectionMode');
    return $t('website.wafModeInheritWith', [label]);
});
const protectionDescription = computed(() => {
    if (!status.supported) return $t('website.wafProtectionDescriptionUnsupported');
    if (status.protected) return $t('website.wafProtectionDescriptionProtected');
    return $t('website.wafProtectionDescriptionEnable');
});
const paginationConfig = reactive({
    cacheSizeKey: 'waf-event-page-size',
    currentPage: 1,
    pageSize: Number(localStorage.getItem('waf-event-page-size')) || 20,
    total: 0,
});

const rangeToTimes = () => {
    const end = new Date();
    const days = range.value === '24h' ? 1 : range.value === '30d' ? 30 : 7;
    const start = new Date(end.getTime() - days * 86400000);
    return { startTime: start.toISOString(), endTime: end.toISOString() };
};

// "2026-07-15T16:00:00Z" -> "2026-07-15 16:00:00"
const fmtTime = (iso: string) => (iso || '').slice(0, 19).replace('T', ' ');

const sevType = (s: string) => (s === 'critical' ? 'danger' : s === 'error' ? 'warning' : 'info');

const search = () => {
    if (!props.id) {
        return;
    }
    const { startTime, endTime } = rangeToTimes();
    const req: Website.WafEventReq = {
        startTime,
        endTime,
        category: category.value,
        page: paginationConfig.currentPage,
        pageSize: paginationConfig.pageSize,
    };
    LoadWafEvents(props.id, req).then((res) => {
        data.value = res.data.items;
        paginationConfig.total = res.data.total;
    });
};

// textarea (one entry per line) -> trimmed, blank-dropped array; the agent is the
// authority that canonicalizes/validates/dedupes, so we only strip obvious noise.
const linesToList = (text: string) =>
    text
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => l.length > 0);

const syncFromStatus = (data: Website.WafSiteStatus) => {
    Object.assign(status, data);
    enabled.value = data.enabled;
    mode.value = data.mode;
    allowText.value = (data.allowList || []).join('\n');
    denyText.value = (data.denyList || []).join('\n');
    rateLimits.value = data.rateLimits || [];
    siteRules.value = data.rules ?? null;
    siteRegion.value = data.region ?? null;
};

const loadStatus = async () => {
    if (!props.id) return;
    loadingStatus.value = true;
    try {
        const res = await GetWafStatus(props.id);
        syncFromStatus(res.data);
    } finally {
        loadingStatus.value = false;
    }
};

const updateConfig = async () => {
    if (!props.id || saving.value) return;
    saving.value = true;
    try {
        const res = await UpdateWafSite(props.id, {
            enabled: enabled.value,
            mode: mode.value,
            allowList: linesToList(allowText.value),
            denyList: linesToList(denyText.value),
            rateLimits: rateLimits.value,
            rules: siteRules.value,
            region: siteRegion.value,
        });
        syncFromStatus(res.data);
    } catch {
        await loadStatus();
    } finally {
        saving.value = false;
    }
};

const saveAcl = () => updateConfig();

const syncGlobal = (data: Website.WafGlobalConfig) => {
    globalForm.defaultMode = data.defaultMode === 'block' ? 'block' : 'detection';
    globalForm.allowList = data.allowList || [];
    globalForm.denyList = data.denyList || [];
    globalForm.rateLimits = data.rateLimits || [];
    globalRules.value = data.rules || {
        disableSqli: false,
        disableXss: false,
        strict: false,
        allowedMethods: [],
        bannedUploadExts: [],
    };
    globalRegion.value = data.region || { mode: 'deny', regions: [] };
    globalGeoAvailable.value = data.geoAvailable;
    globalBlockPage.status = data.blockPage?.status || 403;
    globalBlockPage.html = data.blockPage?.html || '';
    globalLog.retentionDays = data.log?.retentionDays || 30;
    globalLog.excludedKinds = data.log?.excludedKinds || [];
    globalRecordKinds.value = data.recordKinds || [];
    globalAllowText.value = (data.allowList || []).join('\n');
    globalDenyText.value = (data.denyList || []).join('\n');
};

const loadGlobal = async () => {
    globalLoading.value = true;
    try {
        const res = await GetWafGlobal();
        syncGlobal(res.data);
    } finally {
        globalLoading.value = false;
    }
};

const saveGlobal = async () => {
    if (globalSaving.value) return;
    globalSaving.value = true;
    try {
        const res = await UpdateWafGlobal({
            defaultMode: globalForm.defaultMode,
            allowList: linesToList(globalAllowText.value),
            denyList: linesToList(globalDenyText.value),
            rateLimits: globalForm.rateLimits,
            rules: globalRules.value,
            region: globalRegion.value,
            blockPage: { ...globalBlockPage },
            log: { retentionDays: globalLog.retentionDays, excludedKinds: [...globalLog.excludedKinds] },
        });
        syncGlobal(res.data);
        globalOpen.value = false;
        MsgSuccess($t('commons.msg.operationSuccess'));
        // The effective mode of inherit sites may have changed.
        await loadStatus();
    } finally {
        globalSaving.value = false;
    }
};

onMounted(() => {
    loadStatus();
    search();
    loadGlobal();
});
</script>

<style lang="scss" scoped>
.waf-tip {
    margin-bottom: 12px;
}
.waf-bar {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 12px;
}
.waf-control {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
    padding: 12px 14px;
    margin-bottom: 12px;
    border: 1px solid var(--el-border-color-light);
}
.waf-control-copy {
    display: flex;
    flex-direction: column;
    gap: 3px;
    flex: 1;
    min-width: 240px;
}
.waf-control-copy span {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-mode {
    width: 145px;
}
.waf-error {
    margin-bottom: 12px;
}
.waf-cat {
    width: 150px;
}
.waf-acl {
    padding: 12px 14px;
    margin-bottom: 12px;
    border: 1px solid var(--el-border-color-light);
}
.waf-acl-head {
    display: flex;
    flex-direction: column;
    gap: 3px;
    margin-bottom: 10px;
}
.waf-acl-head span {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-acl-lists {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
}
.waf-acl-col {
    display: flex;
    flex-direction: column;
    gap: 4px;
    flex: 1;
    min-width: 260px;
}
.waf-acl-col label {
    font-weight: 500;
}
.waf-acl-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-acl-actions {
    margin-top: 10px;
}
.waf-global-mode {
    display: flex;
    gap: 12px;
    align-items: center;
    margin-bottom: 12px;
}
.waf-global-mode label {
    font-weight: 500;
}
.waf-global-rl {
    margin-top: 14px;
}
.waf-global-rl label {
    font-weight: 500;
}
</style>
