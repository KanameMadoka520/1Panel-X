<template>
    <div v-loading="loading">
        <LayoutContent :title="$t('setting.nodes')">
            <template #leftToolBar>
                <el-button type="primary" @click="onOpenCreate">
                    {{ $t('setting.addNode') }}
                </el-button>
            </template>
            <template #rightToolBar>
                <TableRefresh @search="search()" />
            </template>
            <template #main>
                <el-alert type="info" :closable="false" class="common-div">
                    <template #title>
                        <span>{{ $t('setting.nodeManageHelper') }}</span>
                    </template>
                </el-alert>
                <el-table :data="data" style="width: 100%">
                    <el-table-column
                        :label="$t('commons.table.name')"
                        prop="name"
                        min-width="120"
                        show-overflow-tooltip
                    />
                    <el-table-column label="IP" prop="addr" min-width="140" show-overflow-tooltip />
                    <el-table-column :label="$t('setting.nodePort')" prop="port" min-width="90" />
                    <el-table-column :label="$t('commons.table.status')" prop="status" min-width="110">
                        <template #default="{ row }">
                            <el-tag :type="statusType(row.status)">{{ statusLabel(row.status) }}</el-tag>
                        </template>
                    </el-table-column>
                    <el-table-column :label="$t('commons.table.operate')" prop="operate" min-width="120" fix="right">
                        <template #default="{ row }">
                            <el-button link type="primary" @click="onDelete(row)">
                                {{ $t('commons.button.delete') }}
                            </el-button>
                        </template>
                    </el-table-column>
                    <template #empty>
                        <span>{{ $t('setting.nodeEmpty') }}</span>
                    </template>
                </el-table>
            </template>
        </LayoutContent>

        <!-- Add node dialog -->
        <el-dialog v-model="createVisible" :title="$t('setting.addNode')" width="500px" :close-on-click-modal="false">
            <el-form ref="createFormRef" :model="createForm" :rules="rules" label-position="top">
                <el-form-item :label="$t('commons.table.name')" prop="name">
                    <el-input v-model.trim="createForm.name" :placeholder="$t('setting.nodeNamePlaceholder')" />
                </el-form-item>
                <el-form-item label="IP" prop="addr">
                    <el-input v-model.trim="createForm.addr" placeholder="10.0.0.2" />
                </el-form-item>
                <el-form-item :label="$t('setting.nodePort')" prop="port">
                    <el-input v-model.trim="createForm.port" placeholder="9999" />
                </el-form-item>
            </el-form>
            <template #footer>
                <el-button @click="createVisible = false">{{ $t('commons.button.cancel') }}</el-button>
                <el-button type="primary" :disabled="submitting" @click="onSubmitCreate">
                    {{ $t('commons.button.confirm') }}
                </el-button>
            </template>
        </el-dialog>

        <!-- Enrollment token dialog -->
        <el-dialog
            v-model="tokenVisible"
            :title="$t('setting.nodeEnrollToken')"
            width="560px"
            :close-on-click-modal="false"
        >
            <el-alert type="warning" :closable="false" class="mb-4">
                <template #title>
                    <span>{{ $t('setting.nodeEnrollTokenHelper') }}</span>
                </template>
            </el-alert>
            <el-input type="textarea" :rows="4" :model-value="token.token" readonly />
            <div class="mt-2 text-xs" v-if="token.expireAt">
                {{ $t('setting.nodeTokenExpire') }}: {{ formatExpire(token.expireAt) }}
            </div>
            <template #footer>
                <el-button @click="tokenVisible = false">{{ $t('commons.button.close') }}</el-button>
                <el-button type="primary" @click="onCopyToken">{{ $t('commons.button.copy') }}</el-button>
            </template>
        </el-dialog>
    </div>
</template>

<script lang="ts" setup>
import { reactive, ref, onMounted } from 'vue';
import type { FormInstance, FormRules } from 'element-plus';
import { ElMessageBox } from 'element-plus';
import i18n from '@/lang';
import { Setting } from '@/api/interface/setting';
import { listNodeOptions, createNode, deleteNode } from '@/api/modules/setting';
import { MsgSuccess } from '@/utils/message';
import { copyText } from '@/utils/clipboard';

const loading = ref(false);
const submitting = ref(false);
const data = ref<Setting.NodeItem[]>([]);

const createVisible = ref(false);
const tokenVisible = ref(false);
const createFormRef = ref<FormInstance>();
const createForm = reactive<Setting.NodeCreate>({ name: '', addr: '', port: '' });
const token = reactive<Setting.NodeEnrollToken>({ nodeID: 0, token: '', expireAt: 0, addr: '', port: '' });

const rules = reactive<FormRules>({
    name: [{ required: true, message: i18n.global.t('commons.rule.requiredInput'), trigger: 'blur' }],
    addr: [{ required: true, message: i18n.global.t('commons.rule.requiredInput'), trigger: 'blur' }],
    port: [{ required: true, message: i18n.global.t('commons.rule.requiredInput'), trigger: 'blur' }],
});

const search = async () => {
    loading.value = true;
    try {
        const res = await listNodeOptions('all');
        data.value = res.data || [];
    } catch (error) {
        data.value = [];
    } finally {
        loading.value = false;
    }
};

const onOpenCreate = () => {
    createForm.name = '';
    createForm.addr = '';
    createForm.port = '';
    createVisible.value = true;
};

const onSubmitCreate = async () => {
    if (!createFormRef.value) return;
    await createFormRef.value.validate(async (valid) => {
        if (!valid) return;
        submitting.value = true;
        try {
            const res = await createNode({ ...createForm });
            Object.assign(token, res.data);
            createVisible.value = false;
            tokenVisible.value = true;
            await search();
        } finally {
            submitting.value = false;
        }
    });
};

const onDelete = (row: Setting.NodeItem) => {
    ElMessageBox.confirm(
        i18n.global.t('setting.nodeDeleteHelper', [row.name]),
        i18n.global.t('commons.msg.infoTitle'),
        {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'warning',
        },
    ).then(async () => {
        await deleteNode(row.id);
        MsgSuccess(i18n.global.t('commons.msg.deleteSuccess'));
        await search();
    });
};

const onCopyToken = () => {
    copyText(token.token);
};

const statusLabel = (status: string) => {
    const key = 'setting.nodeStatus_' + (status || 'pending');
    const label = i18n.global.t(key);
    return label === key ? status : label;
};

const statusType = (status: string) => {
    switch (status) {
        case 'online':
            return 'success';
        case 'revoked':
        case 'offline':
            return 'danger';
        case 'enrolling':
            return 'warning';
        default:
            return 'info';
    }
};

const formatExpire = (unix: number) => {
    return new Date(unix * 1000).toLocaleString();
};

onMounted(() => {
    search();
});
</script>
