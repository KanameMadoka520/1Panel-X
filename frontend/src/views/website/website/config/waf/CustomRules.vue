<template>
    <div class="waf-cr">
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
                    <span>{{ row.name }}</span>
                    <!-- A row whose stored conditions could not be read is called
                         out rather than rendered as a rule with no conditions,
                         which would read as "matches nothing". -->
                    <el-tag v-if="row.invalid" size="small" type="danger" class="waf-cr-badge">
                        {{ $t('website.wafRulesCustomBroken') }}
                    </el-tag>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafRulesCustomRule')" min-width="280">
                <template #default="{ row }">
                    <span v-if="row.invalid" class="waf-cr-broken">{{ row.invalid }}</span>
                    <span v-else>{{ describe(row.conditions) }}</span>
                </template>
            </el-table-column>
            <el-table-column :label="$t('website.wafRulesCustomAction')" width="110">
                <template #default="{ row }">
                    <el-tag size="small" :type="actionTagType(row.action)">{{ actionLabel(row.action) }}</el-tag>
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

        <el-dialog v-model="ruleOpen" :title="dialogTitle" width="760px">
            <el-form label-position="top">
                <el-form-item :label="$t('website.wafRulesCustomName')" required>
                    <!-- Fixed after creation: an enforcement record already
                         written under the old name would otherwise point at a
                         rule that no longer answers to it. -->
                    <el-input v-model="form.name" size="small" maxlength="64" :disabled="form.id !== 0" />
                    <span v-if="form.id !== 0" class="waf-cr-hint">{{ $t('website.wafRulesCustomNameFixed') }}</span>
                </el-form-item>

                <div v-for="(cond, i) in form.conditions" :key="i" class="waf-cr-cond">
                    <div class="waf-cr-cond-col">
                        <label>{{ i === 0 ? $t('website.wafRulesCustomObject') : '' }}</label>
                        <el-select
                            v-model="cond.field"
                            size="small"
                            class="waf-cr-cond-field"
                            @change="() => onFieldChange(cond)"
                        >
                            <el-option v-for="f in FIELDS" :key="f" :value="f" :label="fieldLabel(f)" />
                        </el-select>
                        <el-input
                            v-if="cond.field === 'header'"
                            v-model="cond.name"
                            size="small"
                            class="waf-cr-cond-field"
                            :placeholder="$t('website.wafRulesCustomFieldName')"
                        />
                    </div>
                    <div class="waf-cr-cond-col">
                        <label>{{ i === 0 ? $t('website.wafRulesCustomCondition') : '' }}</label>
                        <el-select
                            :model-value="operatorOf(cond)"
                            size="small"
                            class="waf-cr-cond-op"
                            @update:model-value="(op: string) => setOperator(cond, op)"
                        >
                            <el-option
                                v-for="op in operatorsFor(cond.field)"
                                :key="op"
                                :value="op"
                                :label="$t(`website.wafRulesOp_${op}`)"
                            />
                        </el-select>
                    </div>
                    <div class="waf-cr-cond-col waf-cr-cond-grow">
                        <label>{{ i === 0 ? $t('website.wafRulesCustomValue') : '' }}</label>
                        <el-input v-model="cond.pattern" size="small" :placeholder="patternPlaceholder(cond.field)" />
                    </div>
                    <el-button
                        link
                        type="danger"
                        class="waf-cr-cond-del"
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
                    + {{ $t('commons.button.add') }}
                </el-button>
                <span class="waf-cr-hint waf-cr-and">{{ $t('website.wafRulesCustomAndHint') }}</span>

                <el-form-item :label="$t('website.wafRulesCustomAction')" required>
                    <el-select v-model="form.action" size="small" class="waf-cr-field">
                        <el-option value="allow" :label="$t('website.wafRulesActionAllow')" />
                        <el-option value="captcha" :label="$t('website.wafRulesActionCaptcha')" />
                        <el-option value="js" :label="$t('website.wafRulesActionJs')" />
                        <el-option value="deny" :label="$t('website.wafRulesActionDeny')" />
                        <el-option value="log" :label="$t('website.wafRulesActionLog')" />
                    </el-select>
                </el-form-item>
                <span class="waf-cr-hint">{{ actionHint }}</span>

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
    DeleteWafCustomRules,
    GetWafCustomRules,
    ReorderWafCustomRules,
    SaveWafCustomRule,
} from '@/api/modules/website';
import { Website } from '@/api/interface/website';
import { MsgSuccess } from '@/utils/message';

const { t: $t } = useI18n();

const props = defineProps<{ id: number }>();

// The match objects the panel offers, matching the upstream product. The data
// plane's evaluator understands more; exposing only these keeps the form a
// faithful replica while a stored rule using another field still evaluates.
const FIELDS = ['url', 'ip', 'header', 'host', 'method'];

// The five operators the upstream product offers. Internally each is a (match,
// negate) pair, which is what the data plane already speaks — so "not equal" and
// "not contains" need no new evaluator code.
const OPERATORS: Record<string, { match: string; negate: boolean }> = {
    eq: { match: 'exact', negate: false },
    ne: { match: 'exact', negate: true },
    contains: { match: 'contains', negate: false },
    notContains: { match: 'contains', negate: true },
    regex: { match: 'regex', negate: false },
};

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
        conditions: [{ field: 'url', match: 'contains', pattern: '', negate: false }],
        remark: '',
        enabled: true,
    };
}

const dialogTitle = computed(() => (form.id === 0 ? $t('commons.button.create') : $t('commons.button.edit')));

// An address is always a network membership test, so the text operators would
// only be a way to get it subtly wrong. The server refuses them outright.
const operatorsFor = (field: string) => (field === 'ip' ? [] : Object.keys(OPERATORS));

const operatorOf = (cond: Website.WafRuleCondition) => {
    for (const [key, spec] of Object.entries(OPERATORS)) {
        if (spec.match === (cond.match || 'contains') && !!spec.negate === !!cond.negate) return key;
    }
    return 'contains';
};

const setOperator = (cond: Website.WafRuleCondition, op: string) => {
    const spec = OPERATORS[op];
    if (!spec) return;
    cond.match = spec.match;
    cond.negate = spec.negate;
};

const fieldLabel = (field: string) => $t(`website.wafRulesField_${field}`);

const actionLabel = (action: string) => {
    switch (action) {
        case 'allow':
            return $t('website.wafRulesActionAllow');
        case 'log':
            return $t('website.wafRulesActionLog');
        case 'captcha':
            return $t('website.wafRulesActionCaptcha');
        case 'js':
            return $t('website.wafRulesActionJs');
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
        case 'captcha':
        case 'js':
            return 'warning';
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
        case 'captcha':
            return $t('website.wafRulesActionCaptchaHint');
        case 'js':
            return $t('website.wafRulesActionJsHint');
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
        case 'url':
            return '/admin';
        case 'host':
            return 'example.com';
        default:
            return '';
    }
};

const describe = (conditions: Website.WafRuleCondition[]) =>
    (conditions || [])
        .map((c) => {
            const field = c.field === 'header' ? `${fieldLabel(c.field)}[${c.name}]` : fieldLabel(c.field);
            if (c.field === 'ip') return `${field} ${c.pattern}`;
            return `${field} ${$t(`website.wafRulesOp_${operatorOf(c)}`)} ${c.pattern}`;
        })
        .join('  &  ');

const load = async () => {
    if (!props.id) return;
    const res = await GetWafCustomRules(props.id);
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
    form.conditions.push({ field: 'url', match: 'contains', pattern: '', negate: false });
};

// The server refuses a match type on an address condition, so switching the
// field clears it here rather than letting the save fail on something the form
// no longer shows.
const onFieldChange = (cond: Website.WafRuleCondition) => {
    if (cond.field === 'ip') {
        cond.match = '';
        cond.name = '';
        cond.negate = false;
        return;
    }
    if (!cond.match) cond.match = 'contains';
    if (cond.field !== 'header') cond.name = '';
};

const submit = async () => {
    const ok = await run(() =>
        SaveWafCustomRule(props.id, { ...form, conditions: form.conditions.map((c) => ({ ...c })) }),
    );
    if (ok) ruleOpen.value = false;
};

const toggle = (row: Website.WafCustomRule, on: boolean) =>
    run(() => SaveWafCustomRule(props.id, { ...row, enabled: on }));

const remove = (row: Website.WafCustomRule) => run(() => DeleteWafCustomRules(props.id, [row.id]));

// The whole id sequence is sent, so the stored evaluation order can never drift
// from what is on screen.
const move = (index: number, delta: number) => {
    const next = index + delta;
    if (next < 0 || next >= rules.value.length) return;
    const ids = rules.value.map((r) => r.id);
    [ids[index], ids[next]] = [ids[next], ids[index]];
    return run(() => ReorderWafCustomRules(props.id, ids));
};

watch(() => props.id, load);
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
.waf-cr-and {
    margin-left: 12px;
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
.waf-cr-cond {
    display: flex;
    align-items: flex-end;
    gap: 8px;
    margin-bottom: 8px;
}
.waf-cr-cond-col {
    display: flex;
    flex-direction: column;
    gap: 4px;
}
.waf-cr-cond-col label {
    font-size: 12px;
    min-height: 16px;
}
.waf-cr-cond-grow {
    flex: 1;
}
.waf-cr-cond-field {
    width: 150px;
}
.waf-cr-cond-op {
    width: 130px;
}
.waf-cr-cond-del {
    margin-bottom: 4px;
}
</style>
