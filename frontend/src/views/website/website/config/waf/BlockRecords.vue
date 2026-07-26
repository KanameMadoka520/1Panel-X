<template>
    <div class="waf-br">
        <div class="waf-br-bar">
            <el-radio-group v-model="range" size="small" @change="search()">
                <el-radio-button label="24h">{{ $t('website.monitorRange24h') }}</el-radio-button>
                <el-radio-button label="7d">{{ $t('website.monitorRange7d') }}</el-radio-button>
                <el-radio-button label="30d">{{ $t('website.monitorRange30d') }}</el-radio-button>
            </el-radio-group>
            <el-select
                v-model="kind"
                size="small"
                clearable
                class="waf-br-kind"
                :placeholder="$t('website.wafBlockAllKinds')"
                @change="search()"
            >
                <el-option v-for="k in KINDS" :key="k" :value="k" :label="kindLabel(k)" />
            </el-select>
            <span class="waf-br-hint">{{ $t('website.wafBlockTip') }}</span>
        </div>

        <el-table :data="rows" size="small" v-loading="loading" class="waf-br-table">
            <el-table-column :label="$t('commons.table.date')" width="160">
                <template #default="{ row }">{{ formatTime(row.time) }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafBlockKind')" width="130">
                <template #default="{ row }">{{ kindLabel(row.kind) }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafBlockAction')" width="100">
                <template #default="{ row }">
                    <el-tag size="small" :type="actionTagType(row.action)">{{ actionLabel(row.action) }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column label="IP" prop="sourceIP" width="140" />
            <el-table-column :label="$t('website.wafBlockRule')" prop="rule" min-width="160" show-overflow-tooltip />
            <el-table-column label="URI" min-width="220" show-overflow-tooltip>
                <template #default="{ row }">{{ row.method }} {{ row.uri }}</template>
            </el-table-column>
        </el-table>

        <el-pagination
            v-model:current-page="page"
            v-model:page-size="pageSize"
            :total="total"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            size="small"
            class="waf-br-page"
            @size-change="search()"
            @current-change="search()"
        />
    </div>
</template>

<script lang="ts" setup>
import { onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import { LoadWafBlocks } from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { dateFormat } from '@/utils/date';

const { t: $t } = useI18n();

const props = defineProps<{ id: number }>();

// The closed set the gateway can write. Kept in the same order the pipeline
// evaluates them, so the filter reads like the request path.
const KINDS = [
    'acl-deny',
    'custom-rule',
    'challenge',
    'region',
    'ratelimit',
    'ban',
    'banned',
    'ban-released',
    'unknown-host',
    'oversize-body',
];

const rows = ref<Website.WafBlockRecord[]>([]);
const total = ref(0);
const page = ref(1);
const pageSize = ref(20);
const range = ref('24h');
const kind = ref('');
const loading = ref(false);

const kindLabel = (k: string) => {
    const key = `website.wafKind_${k.replace(/-/g, '_')}`;
    const label = $t(key);
    // An unknown kind shows its raw name rather than a missing translation key,
    // so a record written by a newer gateway is still readable.
    return label === key ? k : label;
};

const actionLabel = (a: string) => {
    const key = `website.wafAction_${a}`;
    const label = $t(key);
    return label === key ? a : label;
};

const actionTagType = (a: string) => {
    switch (a) {
        case 'blocked':
        case 'banned':
            return 'danger';
        case 'challenged':
            return 'warning';
        case 'released':
            return 'success';
        default:
            return 'info';
    }
};

const formatTime = (t: string) => dateFormat(0, 0, t);

const rangeStart = () => {
    const now = new Date();
    const hours = range.value === '24h' ? 24 : range.value === '7d' ? 24 * 7 : 24 * 30;
    return new Date(now.getTime() - hours * 3600 * 1000).toISOString();
};

const search = async () => {
    if (!props.id || loading.value) return;
    loading.value = true;
    try {
        const res = await LoadWafBlocks(props.id, {
            startTime: rangeStart(),
            endTime: new Date().toISOString(),
            kind: kind.value || undefined,
            page: page.value,
            pageSize: pageSize.value,
        });
        rows.value = res.data.items || [];
        total.value = res.data.total || 0;
    } finally {
        loading.value = false;
    }
};

watch(
    () => props.id,
    () => search(),
);
onMounted(search);
</script>

<style lang="scss" scoped>
.waf-br-bar {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 10px;
}
.waf-br-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-br-kind {
    width: 180px;
}
.waf-br-table {
    width: 100%;
}
.waf-br-page {
    margin-top: 10px;
    justify-content: flex-end;
}
</style>
