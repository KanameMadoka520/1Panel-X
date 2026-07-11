<template>
    <div>
        <DrawerPro
            v-model="drawerVisible"
            :header="$t('aiTools.agents.agentLimitTitle')"
            @close="handleClose"
            size="small"
        >
            <el-form
                ref="formRef"
                label-position="top"
                :model="form"
                :rules="rules"
                @submit.prevent
                v-loading="loading"
            >
                <el-form-item :label="$t('aiTools.agents.agentLimitLabel')" prop="limit">
                    <el-input-number
                        v-model="form.limit"
                        :min="0"
                        :max="1000"
                        :step="1"
                        :precision="0"
                        controls-position="right"
                        :placeholder="$t('aiTools.agents.agentLimitPlaceholder')"
                    />
                    <span class="input-help">{{ $t('aiTools.agents.agentLimitHelper') }}</span>
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="drawerVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button :disabled="loading" type="primary" @click="onSubmit(formRef)">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </DrawerPro>
    </div>
</template>
<script lang="ts" setup>
import { reactive, ref } from 'vue';
import i18n from '@/lang';
import { MsgSuccess } from '@/utils/message';
import { getAgentSettingInfo, updateAgentSetting } from '@/api/modules/setting';
import { FormInstance } from 'element-plus';

const drawerVisible = ref(false);
const loading = ref(false);
const formRef = ref<FormInstance>();

const form = reactive({
    limit: 0,
});

const rules = reactive({
    limit: [{ required: true, validator: checkLimit, trigger: 'blur' }],
});

function checkLimit(rule: any, value: any, callback: any) {
    if (!Number.isInteger(value) || value < 0 || value > 1000) {
        return callback(new Error(i18n.global.t('aiTools.agents.agentLimitRangeError')));
    }
    callback();
}

const acceptParams = async (): Promise<void> => {
    form.limit = 0;
    drawerVisible.value = true;
    loading.value = true;
    try {
        const res = await getAgentSettingInfo();
        const raw = Number(res.data?.aiAgentLimit);
        form.limit = Number.isFinite(raw) && raw > 0 ? raw : 0;
    } catch (error) {
        form.limit = 0;
    } finally {
        loading.value = false;
    }
};

const onSubmit = async (formEl: FormInstance | undefined) => {
    if (!formEl) return;
    formEl.validate(async (valid) => {
        if (!valid) return;
        loading.value = true;
        await updateAgentSetting({ key: 'AIAgentLimit', value: String(form.limit) })
            .then(() => {
                loading.value = false;
                handleClose();
                MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            })
            .catch(() => {
                loading.value = false;
            });
    });
};

const handleClose = () => {
    drawerVisible.value = false;
};

defineExpose({
    acceptParams,
});
</script>
