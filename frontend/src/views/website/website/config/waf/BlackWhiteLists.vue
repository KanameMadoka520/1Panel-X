<template>
    <div class="waf-bw">
        <el-alert :title="$t('website.wafListsTip')" type="info" :closable="false" class="waf-tip" />

        <div class="waf-bw-bar">
            <el-radio-group v-model="target" size="small">
                <el-radio-button label="ip">IP</el-radio-button>
                <el-radio-button label="url">URL</el-radio-button>
                <el-radio-button label="ua">User-Agent</el-radio-button>
                <el-radio-button label="ipgroup">{{ $t('website.wafListIpGroup') }}</el-radio-button>
            </el-radio-group>
            <el-radio-group v-if="target !== 'ipgroup'" v-model="list" size="small">
                <el-radio-button label="deny">{{ $t('website.wafListDeny') }}</el-radio-button>
                <el-radio-button label="allow">{{ $t('website.wafListAllow') }}</el-radio-button>
            </el-radio-group>
            <el-button type="primary" size="small" :loading="saving" @click="openCreate">
                {{ $t('commons.button.create') }}
            </el-button>
        </div>

        <span class="waf-bw-hint">{{ scopeHint }}</span>

        <!-- IP groups are a different shape from list entries, so they get their
             own table rather than a column that is blank for every other tab. -->
        <el-table v-if="target === 'ipgroup'" :data="groups" size="small" class="waf-bw-table">
            <el-table-column :label="$t('website.wafListGroupName')" prop="name" min-width="140" />
            <el-table-column :label="$t('website.wafListGroupMembers')" min-width="240">
                <template #default="{ row }">{{ (row.entries || []).join(', ') }}</template>
            </el-table-column>
            <el-table-column :label="$t('website.wafListRemark')" prop="remark" min-width="140" />
            <el-table-column :label="$t('commons.table.operate')" width="140">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openGroup(row)">
                        {{ $t('commons.button.edit') }}
                    </el-button>
                    <el-button link type="danger" :loading="saving" @click="removeGroup(row)">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <el-table v-else :data="visibleEntries" size="small" class="waf-bw-table">
            <el-table-column :label="$t('website.wafListMatch')" width="120">
                <template #default="{ row }">{{ matchLabel(row) }}</template>
            </el-table-column>
            <el-table-column
                :label="$t('website.wafListPattern')"
                prop="pattern"
                min-width="240"
                show-overflow-tooltip
            />
            <el-table-column :label="$t('commons.table.status')" width="90">
                <template #default="{ row }">
                    <el-switch
                        :model-value="row.enabled"
                        :loading="saving"
                        @update:model-value="(v: boolean) => toggleEntry(row, v)"
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafListRemark')" prop="remark" min-width="140" show-overflow-tooltip />
            <el-table-column :label="$t('commons.table.operate')" width="140">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openEntry(row)">
                        {{ $t('commons.button.edit') }}
                    </el-button>
                    <el-button link type="danger" :loading="saving" @click="removeEntry(row)">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <el-dialog v-model="entryOpen" :title="$t('website.wafListEntryTitle')" width="560px">
            <el-form label-width="110px">
                <el-form-item :label="$t('website.wafListWhich')">
                    <el-select v-model="entryForm.list" size="small" class="waf-bw-field">
                        <el-option value="deny" :label="$t('website.wafListDeny')" />
                        <el-option value="allow" :label="$t('website.wafListAllow')" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="$t('website.wafListTarget')">
                    <el-select v-model="entryForm.target" size="small" class="waf-bw-field">
                        <el-option value="ip" label="IP" />
                        <el-option value="url" label="URL" />
                        <el-option value="ua" label="User-Agent" />
                        <el-option value="ipgroup" :label="$t('website.wafListIpGroup')" />
                    </el-select>
                </el-form-item>
                <el-form-item v-if="isTextTarget" :label="$t('website.wafListMatch')">
                    <el-select v-model="entryForm.match" size="small" class="waf-bw-field">
                        <el-option value="contains" :label="$t('website.wafListMatchContains')" />
                        <el-option value="prefix" :label="$t('website.wafListMatchPrefix')" />
                        <el-option value="suffix" :label="$t('website.wafListMatchSuffix')" />
                        <el-option value="exact" :label="$t('website.wafListMatchExact')" />
                        <el-option value="regex" :label="$t('website.wafListMatchRegex')" />
                    </el-select>
                </el-form-item>
                <el-form-item :label="patternLabel">
                    <el-select
                        v-if="entryForm.target === 'ipgroup'"
                        v-model="entryForm.pattern"
                        size="small"
                        class="waf-bw-field"
                    >
                        <el-option v-for="g in groups" :key="g.id" :value="g.name" :label="g.name" />
                    </el-select>
                    <el-input v-else v-model="entryForm.pattern" size="small" :placeholder="patternPlaceholder" />
                </el-form-item>
                <el-form-item :label="$t('website.wafListRemark')">
                    <el-input v-model="entryForm.remark" size="small" maxlength="256" />
                </el-form-item>
                <el-form-item :label="$t('commons.table.status')">
                    <el-switch v-model="entryForm.enabled" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="entryOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="submitEntry">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </el-dialog>

        <el-dialog v-model="groupOpen" :title="$t('website.wafListGroupTitle')" width="560px">
            <el-form label-width="110px">
                <el-form-item :label="$t('website.wafListGroupName')">
                    <el-input v-model="groupForm.name" size="small" maxlength="64" />
                </el-form-item>
                <el-form-item :label="$t('website.wafListGroupMembers')">
                    <el-input
                        v-model="groupMembersText"
                        type="textarea"
                        :rows="6"
                        resize="vertical"
                        placeholder="203.0.113.10&#10;198.51.100.0/24"
                    />
                </el-form-item>
                <el-form-item :label="$t('website.wafListRemark')">
                    <el-input v-model="groupForm.remark" size="small" maxlength="256" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="groupOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="submitGroup">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
    DeleteWafIPGroups,
    DeleteWafListEntries,
    GetWafLists,
    SaveWafIPGroup,
    SaveWafListEntry,
} from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';

const { t: $t } = useI18n();

const target = ref<Website.WafListTarget>('ip');
const list = ref<Website.WafListName>('deny');
const entries = ref<Website.WafListEntry[]>([]);
const groups = ref<Website.WafIPGroup[]>([]);
const saving = ref(false);

const entryOpen = ref(false);
const entryForm = reactive<Partial<Website.WafListEntry>>({});
const groupOpen = ref(false);
const groupForm = reactive<Partial<Website.WafIPGroup>>({});
const groupMembersText = ref('');

const isTextTarget = computed(() => entryForm.target === 'url' || entryForm.target === 'ua');

const visibleEntries = computed(() => entries.value.filter((e) => e.target === target.value && e.list === list.value));

const scopeHint = computed(() => {
    if (target.value === 'ipgroup') return $t('website.wafListGroupHint');
    return list.value === 'deny' ? $t('website.wafListDenyHint') : $t('website.wafListAllowHint');
});

const patternLabel = computed(() =>
    entryForm.target === 'ipgroup' ? $t('website.wafListIpGroup') : $t('website.wafListPattern'),
);

const patternPlaceholder = computed(() => {
    switch (entryForm.target) {
        case 'ip':
            return '203.0.113.10 / 198.51.100.0/24';
        case 'url':
            return '/wp-admin';
        default:
            return 'BadBot';
    }
});

const matchLabel = (row: Website.WafListEntry) => {
    if (row.target === 'ip') return $t('website.wafListMatchIp');
    if (row.target === 'ipgroup') return $t('website.wafListIpGroup');
    return $t(`website.wafListMatch${row.match.charAt(0).toUpperCase()}${row.match.slice(1)}`);
};

const apply = (data: Website.WafLists) => {
    entries.value = data.entries || [];
    groups.value = data.ipGroups || [];
};

const load = async () => {
    const res = await GetWafLists();
    apply(res.data);
};

// Every mutation returns the whole list set, so the table always reflects what
// the gateway was actually handed rather than an optimistic local guess.
const run = async (action: () => Promise<{ data: Website.WafLists }>) => {
    if (saving.value) return false;
    saving.value = true;
    try {
        const res = await action();
        apply(res.data);
        MsgSuccess($t('commons.msg.operationSuccess'));
        return true;
    } finally {
        saving.value = false;
    }
};

const openCreate = () => {
    if (target.value === 'ipgroup') {
        openGroup();
        return;
    }
    openEntry();
};

const openEntry = (row?: Website.WafListEntry) => {
    Object.assign(entryForm, {
        id: row?.id ?? 0,
        list: row?.list ?? list.value,
        target: row?.target ?? (target.value === 'ipgroup' ? 'ip' : target.value),
        match: row?.match || 'contains',
        pattern: row?.pattern ?? '',
        remark: row?.remark ?? '',
        enabled: row ? row.enabled : true,
    });
    entryOpen.value = true;
};

const submitEntry = async () => {
    const ok = await run(() => SaveWafListEntry({ ...entryForm } as Website.WafListEntry));
    if (ok) entryOpen.value = false;
};

const toggleEntry = (row: Website.WafListEntry, enabled: boolean) => run(() => SaveWafListEntry({ ...row, enabled }));

const removeEntry = (row: Website.WafListEntry) => run(() => DeleteWafListEntries([row.id]));

const openGroup = (row?: Website.WafIPGroup) => {
    Object.assign(groupForm, { id: row?.id ?? 0, name: row?.name ?? '', remark: row?.remark ?? '' });
    groupMembersText.value = (row?.entries || []).join('\n');
    groupOpen.value = true;
};

const submitGroup = async () => {
    const members = groupMembersText.value
        .split('\n')
        .map((l) => l.trim())
        .filter((l) => l.length > 0);
    const ok = await run(() => SaveWafIPGroup({ ...groupForm, entries: members }));
    if (ok) groupOpen.value = false;
};

const removeGroup = (row: Website.WafIPGroup) => run(() => DeleteWafIPGroups([row.id]));

onMounted(load);
</script>

<style lang="scss" scoped>
.waf-bw-bar {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 6px;
}
.waf-bw-hint {
    display: block;
    margin-bottom: 10px;
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-bw-table {
    width: 100%;
}
.waf-bw-field {
    width: 200px;
}
</style>
