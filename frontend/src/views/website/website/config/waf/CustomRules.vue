<template>
    <div class="waf-cr">
        <el-alert :title="$t('website.wafRulesCustomTip')" type="info" :closable="false" class="waf-tip" />

        <div class="waf-cr-bar">
            <el-button type="primary" size="small" :loading="saving" @click="openRule()">
                {{ $t('commons.button.create') }}
            </el-button>
            <span class="waf-cr-hint">{{ $t('website.wafRulesCustomOrderHint') }}</span>
        </div>

        <el-table :data="rules" size="small" class="waf-cr-table">
            <el-table-column :label="$t('website.wafRulesCustomOrder')" width="96">
                <template #default="{ $index }">
                    <div class="waf-cr-order">
                        <span>{{ $index + 1 }}</span>
                        <el-button link type="primary" :disabled="$index === 0 || saving" @click="move($index, -1)">
                            ↑
                        </el-button>
                        <el-button
                            link
                            type="primary"
                            :disabled="$index === rules.length - 1 || saving"
                            @click="move($index, 1)"
                        >
                            ↓
                        </el-button>
                    </div>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafRulesCustomName')" min-width="140" show-overflow-tooltip>
                <template #default="{ row }">
                    <span>{{ row.name || $t('website.wafRulesCustomUnnamed') }}</span>
                    <!-- A row whose stored conditions could not be read is called
                         out rather than rendered as a rule with no conditions,
                         which would read as "matches nothing". -->
                    <el-tag v-if="row.invalid" size="small" type="danger" class="waf-cr-badge">
                        {{ $t('website.wafRulesCustomBroken') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafRulesCustomAction')" width="110">
                <template #default="{ row }">
                    <el-tag size="small" :type="actionTagType(row.action)">{{ actionLabel(row.action) }}</el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafRulesCustomConditions')" min-width="280">
                <template #default="{ row }">
                    <span v-if="row.invalid" class="waf-cr-broken">{{ row.invalid }}</span>
                    <span v-else>{{ describe(row.conditions) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('commons.table.status')" width="90">
                <template #default="{ row }">
                    <el-switch
                        :model-value="row.enabled"
                        :loading="saving"
                        :disabled="!!row.invalid"
                        @update:model-value="(v: boolean) => toggle(row, v)"
                    />
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafListRemark')" prop="remark" min-width="120" show-overflow-tooltip />
            <el-table-column :label="$t('commons.table.operate')" width="140">
                <template #default="{ row }">
                    <el-button link type="primary" :disabled="!!row.invalid" @click="openRule(row)">
                        {{ $t('commons.button.edit') }}
                    </el-button>
                    <el-button link type="danger" :loading="saving" @click="remove(row)">
                        {{ $t('commons.button.delete') }}
                    </el-button>
                </template>
            </el-table-column>
        </el-table>

        <el-dialog v-model="ruleOpen" :title="$t('website.wafRulesCustomTitle')" width="720px">
            <el-form label-width="110px">
                <el-form-item :label="$t('website.wafRulesCustomName')">
                    <el-input v-model="form.name" size="small" maxlength="64" class="waf-cr-field" />
                </el-form-item>
                <el-form-item :label="$t('website.wafRulesCustomAction')">
                    <el-select v-model="form.action" size="small" class="waf-cr-field">
                        <el-option value="deny" :label="$t('website.wafRulesActionDeny')" />
                        <el-option value="allow" :label="$t('website.wafRulesActionAllow')" />
                        <el-option value="log" :label="$t('website.wafRulesActionLog')" />
                    </el-select>
                    <span class="waf-cr-hint">{{ actionHint }}</span>
                </el-form-item>
                <el-form-item :label="$t('website.wafRulesCustomConditions')">
                    <div class="waf-cr-conds">
                        <span class="waf-cr-hint">{{ $t('website.wafRulesCustomAndHint') }}</span>
                        <div v-for="(cond, i) in form.conditions" :key="i" class="waf-cr-cond">
                            <el-select
                                v-model="cond.field"
                                size="small"
                                class="waf-cr-cond-field"
                                @change="() => onFieldChange(cond)"
                            >
                                <el-option v-for="f in FIELDS" :key="f" :value="f" :label="fieldLabel(f)" />
                            </el-select>
                            <el-input
                                v-if="needsName(cond.field)"
                                v-model="cond.name"
                                size="small"
                                class="waf-cr-cond-name"
                                :placeholder="$t('website.wafRulesCustomFieldName')"
                            />
                            <el-select
                                v-if="cond.field !== 'ip'"
                                v-model="cond.match"
                                size="small"
                                class="waf-cr-cond-match"
                            >
                                <el-option value="contains" :label="$t('website.wafListMatchContains')" />
                                <el-option value="prefix" :label="$t('website.wafListMatchPrefix')" />
                                <el-option value="suffix" :label="$t('website.wafListMatchSuffix')" />
                                <el-option value="exact" :label="$t('website.wafListMatchExact')" />
                                <el-option value="regex" :label="$t('website.wafListMatchRegex')" />
                            </el-select>
                            <el-input
                                v-model="cond.pattern"
                                size="small"
                                class="waf-cr-cond-pattern"
                                :placeholder="patternPlaceholder(cond.field)"
                            />
                            <el-checkbox v-model="cond.negate" size="small">
                                {{ $t('website.wafRulesCustomNegate') }}
                            </el-checkbox>
                            <el-button
                                link
                                type="danger"
                                :disabled="form.conditions.length <= 1"
                                @click="form.conditions.splice(i, 1)"
                            >
                                {{ $t('commons.button.delete') }}
                            </el-button>
                        </div>
                        <el-button
                            link
                            type="primary"
                            :disabled="form.conditions.length >= MAX_CONDITIONS"
                            @click="addCondition"
                        >
                            {{ $t('website.wafRulesCustomAddCondition') }}
                        </el-button>
                    </div>
                </el-form-item>
                <el-form-item :label="$t('website.wafListRemark')">
                    <el-input v-model="form.remark" size="small" maxlength="256" class="waf-cr-field" />
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
import { computed, onMounted, reactive, ref } from 'vue';
import { useI18n } from 'vue-i18n';
import {
    DeleteWafCustomRules,
    GetWafCustomRules,
    ReorderWafCustomRules,
    SaveWafCustomRule,
} from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';

const { t: $t } = useI18n();

const FIELDS = ['ip', 'host', 'method', 'uri', 'path', 'query', 'ua', 'referer', 'header', 'cookie'];
const MAX_CONDITIONS = 8;

const rules = ref<Website.WafCustomRule[]>([]);
const saving = ref(false);
const ruleOpen = ref(false);
const form = reactive<Website.WafCustomRule>(blankRule());

function blankRule(): Website.WafCustomRule {
    return {
        id: 0,
        name: '',
        action: 'deny',
        conditions: [{ field: 'path', match: 'contains', pattern: '', negate: false }],
        remark: '',
        enabled: true,
    };
}

const needsName = (field: string) => field === 'header' || field === 'cookie';

const fieldLabel = (field: string) => $t(`website.wafRulesField_${field}`);

const actionLabel = (action: string) => {
    switch (action) {
        case 'allow':
            return $t('website.wafRulesActionAllow');
        case 'log':
            return $t('website.wafRulesActionLog');
        default:
            return $t('website.wafRulesActionDeny');
    }
};

const actionTagType = (action: string) => {
    switch (action) {
        case 'allow':
            return 'success';
        case 'log':
            return 'info';
        default:
            return 'danger';
    }
};

const actionHint = computed(() => {
    switch (form.action) {
        case 'allow':
            return $t('website.wafRulesActionAllowHint');
        case 'log':
            return $t('website.wafRulesActionLogHint');
        default:
            return $t('website.wafRulesActionDenyHint');
    }
});

const patternPlaceholder = (field: string) => {
    switch (field) {
        case 'ip':
            return '203.0.113.10 / 198.51.100.0/24';
        case 'method':
            return 'POST';
        case 'path':
            return '/admin';
        case 'ua':
            return 'BadBot';
        default:
            return '';
    }
};

const describe = (conditions: Website.WafRuleCondition[]) =>
    (conditions || [])
        .map((c) => {
            const field = needsName(c.field) ? `${fieldLabel(c.field)}[${c.name}]` : fieldLabel(c.field);
            const not = c.negate ? $t('website.wafRulesCustomNot') + ' ' : '';
            const match = c.field === 'ip' ? '' : `${$t(`website.wafListMatch${cap(c.match || 'contains')}`)} `;
            return `${field} ${not}${match}${c.pattern}`;
        })
        .join('  &  ');

const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1);

const load = async () => {
    const res = await GetWafCustomRules();
    rules.value = res.data || [];
};

// Every mutation returns the whole ordered list, so the table always reflects
// what the gateway was actually handed rather than an optimistic local guess.
const run = async (action: () => Promise<{ data: Website.WafCustomRule[] }>) => {
    if (saving.value) return false;
    saving.value = true;
    try {
        const res = await action();
        rules.value = res.data || [];
        MsgSuccess($t('commons.msg.operationSuccess'));
        return true;
    } finally {
        saving.value = false;
    }
};

const openRule = (row?: Website.WafCustomRule) => {
    const blank = blankRule();
    Object.assign(form, {
        ...blank,
        ...(row ? { ...row, conditions: row.conditions.map((c) => ({ ...c })) } : {}),
    });
    if (!form.conditions.length) form.conditions = blank.conditions;
    ruleOpen.value = true;
};

const addCondition = () => {
    if (form.conditions.length >= MAX_CONDITIONS) return;
    form.conditions.push({ field: 'path', match: 'contains', pattern: '', negate: false });
};

// The server refuses a match type on an address condition, so switching the
// field clears it here rather than letting the save fail on something the form
// no longer shows.
const onFieldChange = (cond: Website.WafRuleCondition) => {
    if (cond.field === 'ip') {
        cond.match = '';
        cond.name = '';
        return;
    }
    if (!cond.match) cond.match = 'contains';
    if (!needsName(cond.field)) cond.name = '';
};

const submit = async () => {
    const ok = await run(() => SaveWafCustomRule({ ...form, conditions: form.conditions.map((c) => ({ ...c })) }));
    if (ok) ruleOpen.value = false;
};

const toggle = (row: Website.WafCustomRule, enabled: boolean) => run(() => SaveWafCustomRule({ ...row, enabled }));

const remove = (row: Website.WafCustomRule) => run(() => DeleteWafCustomRules([row.id]));

// The whole id sequence is sent, so the stored evaluation order can never drift
// from what is on screen.
const move = (index: number, delta: number) => {
    const next = index + delta;
    if (next < 0 || next >= rules.value.length) return;
    const ids = rules.value.map((r) => r.id);
    [ids[index], ids[next]] = [ids[next], ids[index]];
    return run(() => ReorderWafCustomRules(ids));
};

onMounted(load);
</script>

<style lang="scss" scoped>
.waf-cr-bar {
    display: flex;
    gap: 12px;
    align-items: center;
    flex-wrap: wrap;
    margin-bottom: 10px;
}
.waf-cr-hint {
    color: var(--el-text-color-secondary);
    font-size: 12px;
}
.waf-cr-table {
    width: 100%;
}
.waf-cr-order {
    display: flex;
    align-items: center;
    gap: 2px;
}
.waf-cr-badge {
    margin-left: 6px;
}
.waf-cr-broken {
    color: var(--el-color-danger);
}
.waf-cr-field {
    width: 240px;
}
.waf-cr-conds {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
}
.waf-cr-cond {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-wrap: wrap;
}
.waf-cr-cond-field {
    width: 120px;
}
.waf-cr-cond-name {
    width: 130px;
}
.waf-cr-cond-match {
    width: 110px;
}
.waf-cr-cond-pattern {
    width: 200px;
}
</style>
