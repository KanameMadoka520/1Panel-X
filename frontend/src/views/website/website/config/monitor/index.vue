<template>
    <div v-loading="loading">
        <div class="monitor-bar">
            <el-radio-group v-model="range" @change="loadAll" size="small">
                <el-radio-button label="24h">{{ $t('website.monitorRange24h') }}</el-radio-button>
                <el-radio-button label="7d">{{ $t('website.monitorRange7d') }}</el-radio-button>
                <el-radio-button label="30d">{{ $t('website.monitorRange30d') }}</el-radio-button>
            </el-radio-group>
            <el-button size="small" @click="loadAll">{{ $t('commons.button.refresh') }}</el-button>
        </div>

        <el-row :gutter="10" class="summary">
            <el-col :span="8">
                <el-card shadow="never">
                    <el-statistic :title="$t('website.monitorPv')" :value="totalPv" />
                </el-card>
            </el-col>
            <el-col :span="8">
                <el-card shadow="never">
                    <el-statistic :title="$t('website.monitorPeakUv')" :value="peakUv" />
                </el-card>
            </el-col>
            <el-col :span="8">
                <el-card shadow="never">
                    <el-statistic :title="$t('website.monitorBytes')" :value="totalBytesMB" suffix=" MB" />
                </el-card>
            </el-col>
        </el-row>

        <el-row :gutter="10" class="charts">
            <el-col :span="12">
                <el-card shadow="never">
                    <div class="chart-title">{{ $t('website.monitorTraffic') }}</div>
                    <v-charts v-if="trafficOption" height="240px" type="line" :option="trafficOption" />
                    <el-empty v-else :description="$t('website.monitorNoData')" :image-size="60" />
                </el-card>
            </el-col>
            <el-col :span="12">
                <el-card shadow="never">
                    <div class="chart-title">{{ $t('website.monitorStatus') }}</div>
                    <v-charts v-if="statusOption" height="240px" type="line" :option="statusOption" />
                    <el-empty v-else :description="$t('website.monitorNoData')" :image-size="60" />
                </el-card>
            </el-col>
        </el-row>

        <el-card shadow="never" class="rank-card">
            <div class="rank-head">
                <span class="chart-title">{{ $t('website.monitorRank') }}</span>
                <el-radio-group v-model="rankKind" @change="loadRank" size="small">
                    <el-radio-button label="uri">URL</el-radio-button>
                    <el-radio-button label="ip">IP</el-radio-button>
                    <el-radio-button label="referer">Referer</el-radio-button>
                    <el-radio-button label="region">{{ $t('website.monitorRegion') }}</el-radio-button>
                </el-radio-group>
            </div>
            <el-table :data="rankData" size="small" v-if="rankData.length">
                <el-table-column type="index" width="60" label="#" />
                <el-table-column prop="key" :label="$t('website.monitorRankKey')" show-overflow-tooltip />
                <el-table-column prop="count" :label="$t('website.monitorHits')" width="140" align="right" />
            </el-table>
            <el-empty v-else :description="$t('website.monitorNoData')" :image-size="60" />
        </el-card>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue';
import VCharts from '@/components/v-charts/index.vue';
import { LoadWebsiteAccessStat, LoadWebsiteAccessRank } from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import i18n from '@/lang';

const props = defineProps({
    id: {
        type: Number,
        default: 0,
    },
});

const loading = ref(false);
const range = ref('7d');
const rankKind = ref('uri');
const stats = ref<Website.AccessStat[]>([]);
const rankData = ref<Website.AccessRank[]>([]);
const trafficOption = ref<any>(null);
const statusOption = ref<any>(null);

const totalPv = computed(() => stats.value.reduce((s, i) => s + (i.pv || 0), 0));
const peakUv = computed(() => stats.value.reduce((m, i) => Math.max(m, i.uv || 0), 0));
const totalBytesMB = computed(() => {
    const bytes = stats.value.reduce((s, i) => s + (i.bytes || 0), 0);
    return Math.round((bytes / 1024 / 1024) * 100) / 100;
});

const rangeToTimes = () => {
    const end = new Date();
    const days = range.value === '24h' ? 1 : range.value === '30d' ? 30 : 7;
    const start = new Date(end.getTime() - days * 86400000);
    return { startTime: start.toISOString(), endTime: end.toISOString() };
};

// "2026-07-15T16:00:00Z" -> "07-15 16:00"
const fmtTime = (iso: string) => (iso || '').slice(5, 16).replace('T', ' ');

const loadStat = async () => {
    const { startTime, endTime } = rangeToTimes();
    const res = await LoadWebsiteAccessStat(props.id, { startTime, endTime });
    stats.value = res.data || [];
    if (!stats.value.length) {
        trafficOption.value = null;
        statusOption.value = null;
        return;
    }
    const x = stats.value.map((s) => fmtTime(s.time));
    trafficOption.value = {
        xData: x,
        yData: [
            { name: i18n.global.t('website.monitorPv'), data: stats.value.map((s) => s.pv) },
            { name: i18n.global.t('website.monitorUv'), data: stats.value.map((s) => s.uv) },
        ],
        formatStr: '',
    };
    statusOption.value = {
        xData: x,
        yData: [
            { name: '2xx', data: stats.value.map((s) => s.status2xx) },
            { name: '4xx', data: stats.value.map((s) => s.status4xx) },
            { name: '5xx', data: stats.value.map((s) => s.status5xx) },
        ],
        formatStr: '',
    };
};

const loadRank = async () => {
    const { startTime, endTime } = rangeToTimes();
    const res = await LoadWebsiteAccessRank(props.id, { startTime, endTime, kind: rankKind.value, top: 20 });
    rankData.value = res.data || [];
};

const loadAll = async () => {
    if (!props.id) {
        return;
    }
    loading.value = true;
    try {
        await Promise.all([loadStat(), loadRank()]);
    } finally {
        loading.value = false;
    }
};

onMounted(() => {
    loadAll();
});
</script>

<style lang="scss" scoped>
.monitor-bar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
}
.summary,
.charts {
    margin-bottom: 12px;
}
.chart-title {
    font-weight: 500;
    margin-bottom: 8px;
}
.rank-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 10px;
}
</style>
