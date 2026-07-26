<template>
    <div class="waf-up">
        <div class="waf-up-bar">
            <el-button type="primary" size="small" :loading="saving" @click="openRule()">
                {{ $t('commons.button.create') }}
            </el-button>
            <el-switch :model-value="enabled" :loading="saving" @update:model-value="toggle" />
            <span>{{ $t('website.wafUploadTitle') }}</span>
            <span class="waf-up-hint">{{ stateHint }}</span>
        </div>

        <el-table :data="rules" size="small" class="waf-up-table">
            <el-table-column :label="$t('website.wafUploadRule')" prop="rule" min-width="160" />
            <el-table-column :label="$t('commons.table.status')" width="90">
                <template #default="{ row }">
                    <el-switch
                        :model-value="row.enabled"
                        :loading="saving"
                        @update:model-value="(v: boolean) => setRowEnabled(row, v)"
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafListRemark')" prop="remark" min-width="180" show-overflow-tooltip />
            <el-table-column :label="$t('commons.table.operate')" width="140">
                <template #default="{ row }">
                    <el-button link type="primary" @click="openRule(row)">
                        {{ $t('commons.button.edit') }}
                    </el-button>
                    <el-button link type="danger" :loading="saving" @click="remove(row)">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <el-dialog v-model="ruleOpen" :title="$t('website.wafUploadTitle')" width="520px">
            <el-form label-width="80px">
                <el-form-item :label="$t('website.wafUploadRule')" required>
                    <el-input v-model="form.rule" size="small" maxlength="32" placeholder="php" />
                    <span class="waf-up-hint">{{ $t('website.wafUploadFuzzy') }}</span>
                </el-form-item>
                <el-form-item :label="$t('website.wafListRemark')">
                    <el-input v-model="form.remark" size="small" maxlength="256" />
                </el-form-item>
                <el-form-item :label="$t('commons.table.status')">
                    <el-switch v-model="form.enabled" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="ruleOpen = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :loading="saving" @click="submit">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, reactive, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
    DeleteWafUploadRules,
    GetWafUploadRules,
    SaveWafUploadRule,
    ToggleWafUploadRules,
} from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';

const { t: $t } = useI18n();

const props = defineProps<{ id: number }>();

const rules = ref<Website.WafUploadRule[]>([]);
const enabled = ref(false);
const saving = ref(false);
const ruleOpen = ref(false);
const form = reactive<Website.WafUploadRule>({ id: 0, rule: '', remark: '', enabled: true });

// The switch and the list say different things, and an operator needs both: an
// armed control with no rules catches nothing, and rules with the control off
// are stored but not enforced.
const stateHint = computed(() => {
    if (!enabled.value) return $t('website.wafUploadOff');
    return rules.value.some((r) => r.enabled) ? $t('website.wafUploadOn') : $t('website.wafUploadOnButEmpty');
});

const apply = (data: Website.WafUploadRules) => {
    rules.value = data.rules || [];
    enabled.value = data.enabled;
};

const load = async () => {
    if (!props.id) return;
    const res = await GetWafUploadRules(props.id);
    apply(res.data);
};

// Every mutation returns the whole set, so the table always reflects what the
// gateway was actually handed rather than an optimistic local guess.
const run = async (action: () => Promise<{ data: Website.WafUploadRules }>) => {
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

const openRule = (row?: Website.WafUploadRule) => {
    Object.assign(form, {
        id: row?.id ?? 0,
        rule: row?.rule ?? '',
        remark: row?.remark ?? '',
        enabled: row ? row.enabled : true,
    });
    ruleOpen.value = true;
};

const submit = async () => {
    const ok = await run(() => SaveWafUploadRule(props.id, { ...form }));
    if (ok) ruleOpen.value = false;
};

const setRowEnabled = (row: Website.WafUploadRule, on: boolean) =>
    run(() => SaveWafUploadRule(props.id, { ...row, enabled: on }));

const remove = (row: Website.WafUploadRule) => run(() => DeleteWafUploadRules(props.id, [row.id]));

const toggle = (on: boolean) => run(() => ToggleWafUploadRules(props.id, on));

watch(() => props.id, load);
onMounted(load);
</script>

<style lang="scss" scoped>
.waf-up-bar {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 10px;
}
.waf-up-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-up-table {
    width: 100%;
}
</style>
