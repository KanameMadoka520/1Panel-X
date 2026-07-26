<template>
    <div>
        <el-alert :title="$t('website.wafBansTip')" type="info" :closable="false" class="waf-tip" />

        <div class="waf-ban-bar">
            <el-button size="small" :loading="loading" @click="load">{{ $t('commons.button.refresh') }}</el-button>
            <span v-if="state.counterOverflow" class="waf-ban-warn">{{ $t('website.wafBansOverflow') }}</span>
        </div>

        <el-alert v-if="unreachable" :title="unreachable" type="warning" :closable="false" class="waf-tip" />

        <el-table v-else :data="state.bans" size="small" v-loading="loading">
            <el-table-column :label="$t('website.wafSourceIp')" prop="ip" min-width="150" />
            <el-table-column :label="$t('website.wafBanReason')" min-width="150">
                <template #default="{ row }">{{ reasonLabel(row.kind) }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafBanSite')" min-width="150">
                <template #default="{ row }">{{ row.host || '-' }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafBanAt')" min-width="160">
                <template #default="{ row }">{{ fmtTime(row.bannedAt) }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafBanUntil')" min-width="160">
                <template #default="{ row }">{{ fmtTime(row.expiresAt) }}</template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.operate')" width="110">
                <template #default="{ row }">
                    <el-button link type="primary" :loading="loading" @click="release(row)">
                        {{ $t('website.wafBanRelease') }}
                    </el-button>
                </template>
            </el-table-column>
            <template #empty>
                <span>{{ $t('website.wafBansEmpty') }}</span>
            </template>
        </el-table>
    </div>
</template>

<script lang="ts" setup>
import { onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import { GetWafBans, ReleaseWafBan } from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';

const { t: $t } = useI18n();

const state = reactive<Website.WafBanState>({ bans: [], trackedCounters: 0, counterOverflow: false });
const loading = ref(false);
// Bans live in the gateway process, so an unreachable gateway means the data is
// genuinely unavailable — shown as such rather than as an empty list, which
// would read as "nobody is banned".
const unreachable = ref('');

const fmtTime = (iso: string) => (iso || '').slice(0, 19).replace('T', ' ');

const reasonLabel = (kind: string) => {
    const key = `website.wafRl_${kind}`;
    const label = $t(key);
    return label === key ? kind : label;
};

const apply = (data: Website.WafBanState) => {
    state.bans = data.bans || [];
    state.trackedCounters = data.trackedCounters || 0;
    state.counterOverflow = !!data.counterOverflow;
    unreachable.value = '';
};

const load = async () => {
    loading.value = true;
    try {
        const res = await GetWafBans();
        apply(res.data);
    } catch (e: any) {
        state.bans = [];
        unreachable.value = e?.message || $t('website.wafBansUnavailable');
    } finally {
        loading.value = false;
    }
};

const release = async (row: Website.WafBan) => {
    loading.value = true;
    try {
        const res = await ReleaseWafBan(row.ip);
        apply(res.data);
        MsgSuccess($t('commons.msg.operationSuccess'));
    } finally {
        loading.value = false;
    }
};

onMounted(load);
</script>

<style lang="scss" scoped>
.waf-ban-bar {
    display: flex;
    align-items: center;
    gap: 12px;
    margin-bottom: 10px;
}
.waf-ban-warn {
    color: var(--el-color-warning);
    font-size: 12px;
}
</style>
