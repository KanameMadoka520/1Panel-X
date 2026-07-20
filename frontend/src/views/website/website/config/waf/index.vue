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
            <el-select v-model="mode" :disabled="saving || !enabled" size="small" class="waf-mode" @change="updateConfig">
                <el-option value="detection" :label="$t('website.wafDetectionMode')" />
                <el-option value="block" :label="$t('website.wafBlockMode')" />
            </el-select>
            <el-tag v-if="status.protected" type="success">{{ $t('website.wafProtected') }}</el-tag>
            <el-tag v-else-if="enabled" type="warning">{{ $t('website.wafPending') }}</el-tag>
            <el-tag v-else type="info">{{ $t('website.wafDisabled') }}</el-tag>
        </div>
        <el-alert v-if="status.lastError" :title="status.lastError" type="error" :closable="false" class="waf-error" />
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
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref, computed } from 'vue';
import { useI18n } from 'vue-i18n';
import { GetWafStatus, LoadWafEvents, UpdateWafSite } from '@/api/modules/website';
import { Website } from '@/api/interface/website';

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
    mode: 'detection',
    installed: false,
    ready: false,
    routed: false,
    protected: false,
    lastError: '',
});
const enabled = ref(false);
const mode = ref<Website.WafSiteUpdate['mode']>('detection');
const loadingStatus = ref(false);
const saving = ref(false);
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

const loadStatus = async () => {
    if (!props.id) return;
    loadingStatus.value = true;
    try {
        const res = await GetWafStatus(props.id);
        Object.assign(status, res.data);
        enabled.value = res.data.enabled;
        mode.value = res.data.mode;
    } finally {
        loadingStatus.value = false;
    }
};

const updateConfig = async () => {
    if (!props.id || saving.value) return;
    saving.value = true;
    try {
        const res = await UpdateWafSite(props.id, { enabled: enabled.value, mode: mode.value });
        Object.assign(status, res.data);
        enabled.value = res.data.enabled;
        mode.value = res.data.mode;
    } catch {
        await loadStatus();
    } finally {
        saving.value = false;
    }
};

onMounted(() => {
    loadStatus();
    search();
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
</style>
