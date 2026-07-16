<template>
    <div>
        <el-alert :title="$t('website.wafTip')" type="info" :closable="false" class="waf-tip" />
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
import { onMounted, reactive, ref } from 'vue';
import { LoadWafEvents } from '@/api/modules/website';
import { Website } from '@/api/interface/website';

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

onMounted(() => {
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
.waf-cat {
    width: 150px;
}
</style>
