<template>
    <DrawerPro v-model="drawerVisible" :header="title + $t('setting.backupAccount')" @close="handleClose" size="large">
        <el-form @submit.prevent ref="formRef" v-loading="loading" label-position="top" :model="dialogData.rowData">
            <el-form-item :label="$t('commons.table.name')" prop="name" :rules="Rules.requiredInput">
                <el-tag v-if="dialogData.title === 'edit'">
                    {{ dialogData.rowData!.name === 'localhost' ? $t('terminal.local') : dialogData.rowData!.name }}
                </el-tag>
                <el-input v-else v-model="dialogData.rowData!.name" />
            </el-form-item>
            <el-form-item
                v-if="isProductPro"
                :label="$t('setting.scope')"
                prop="isPublic"
                :rules="Rules.requiredSelect"
            >
                <el-tag v-if="dialogData.title === 'edit'">
                    {{ dialogData.rowData!.isPublic ? $t('setting.public') : $t('setting.private') }}
                </el-tag>
                <el-radio-group v-else v-model="dialogData.rowData!.isPublic">
                    <el-radio :value="true" size="large">{{ $t('setting.public') }}</el-radio>
                    <el-radio :value="false" size="large">{{ $t('setting.private') }}</el-radio>
                </el-radio-group>
                <span class="input-help">
                    {{ dialogData.rowData!.isPublic ? $t('setting.publicHelper') : $t('setting.privateHelper') }}
                </span>
            </el-form-item>
            <el-form-item :label="$t('commons.table.type')" prop="type" :rules="Rules.requiredSelect">
                <el-tag v-if="dialogData.title === 'edit'">{{ $t('setting.' + dialogData.rowData!.type) }}</el-tag>
                <el-select v-else v-model="dialogData.rowData!.type" @change="changeType">
                    <el-option :label="$t('setting.OSS')" value="OSS"></el-option>
                    <el-option :label="$t('setting.COS')" value="COS"></el-option>
                    <el-option :label="$t('setting.S3')" value="S3"></el-option>
                    <el-option :label="$t('setting.OneDrive')" value="OneDrive"></el-option>
                    <el-option :label="$t('setting.GoogleDrive')" value="GoogleDrive"></el-option>
                    <el-option :label="$t('setting.ALIYUN')" value="ALIYUN"></el-option>
                    <el-option :label="$t('setting.MINIO')" value="MINIO"></el-option>
                    <el-option :label="$t('setting.WebDAV')" value="WebDAV"></el-option>
                    <el-option :label="$t('setting.SFTP')" value="SFTP"></el-option>
                    <el-option :label="$t('setting.KODO')" value="KODO"></el-option>
                    <el-option :label="$t('setting.UPYUN')" value="UPYUN"></el-option>
                </el-select>
                <span v-if="isALIYUNYUN()" class="input-help">{{ $t('setting.ALIYUNHelper') }}</span>
                <span v-if="dialogData.rowData?.type === 'GoogleDrive' && !isFxplay" class="input-help">
                    {{ $t('setting.googleHelper', [$t('setting.' + dialogData.rowData?.type)]) }}
                    <el-link
                        style="font-size: 12px; margin-left: 5px"
                        icon="Position"
                        @click="toDoc('google-drive')"
                        type="primary"
                    >
                        {{ $t('firewall.quickJump') }}
                    </el-link>
                </span>
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'S3'"
                :label="$t('setting.mode')"
                prop="varsJson.mode"
                :rules="Rules.requiredSelect"
            >
                <el-radio-group v-model="dialogData.rowData!.varsJson['mode']">
                    <el-radio value="virtual hosted">Virtual Hosted</el-radio>
                    <el-radio value="path">Path</el-radio>
                </el-radio-group>
            </el-form-item>
            <el-form-item v-if="hasAccessKey()" label="Access Key ID" prop="accessKey" :rules="Rules.requiredInput">
                <el-input clearable v-model.trim="dialogData.rowData!.accessKey" />
            </el-form-item>
            <el-form-item v-if="hasAccessKey()" label="Secret Key" prop="credential" :rules="Rules.requiredInput">
                <el-input show-password clearable v-model.trim="dialogData.rowData!.credential" />
            </el-form-item>
            <div v-if="isUPYUN()">
                <el-form-item :label="$t('setting.operator')" prop="accessKey" :rules="Rules.requiredInput">
                    <el-input clearable v-model.trim="dialogData.rowData!.accessKey" />
                </el-form-item>
                <el-form-item :label="$t('commons.login.password')" prop="credential" :rules="Rules.requiredInput">
                    <el-input show-password clearable v-model.trim="dialogData.rowData!.credential" />
                </el-form-item>
            </div>
            <el-form-item
                v-if="dialogData.rowData!.type === 'WebDAV'"
                :label="$t('setting.address')"
                prop="varsJson.address"
                :rules="Rules.requiredInput"
            >
                <el-input v-model="dialogData.rowData!.varsJson['address']" />
                <span class="input-help" v-if="!isFxplay">
                    {{ $t('setting.WebDAVAlist') }}
                    <el-link
                        style="font-size: 12px; margin-left: 5px"
                        icon="Position"
                        @click="toDoc('webdav')"
                        type="primary"
                    >
                        {{ $t('firewall.quickJump') }}
                    </el-link>
                </span>
            </el-form-item>
            <div v-if="dialogData.rowData!.type === 'SFTP'">
                <el-form-item :label="$t('setting.address')" prop="varsJson.address" :rules="Rules.host">
                    <el-input v-model.trim="dialogData.rowData!.varsJson['address']" clearable />
                </el-form-item>
                <el-form-item :label="$t('commons.table.port')" prop="varsJson.port" :rules="[Rules.port]">
                    <el-input-number :min="0" :max="65535" v-model.number="dialogData.rowData!.varsJson['port']" />
                </el-form-item>
            </div>
            <div v-if="hasPassword()">
                <el-form-item :label="$t('commons.login.username')" prop="accessKey" :rules="[Rules.requiredInput]">
                    <el-input v-model.trim="dialogData.rowData!.accessKey" />
                </el-form-item>

                <div v-if="dialogData.rowData!.type === 'SFTP'">
                    <el-form-item :label="$t('terminal.authMode')" prop="varsJson.authMode">
                        <el-radio-group v-model="dialogData.rowData!.varsJson['authMode']">
                            <el-radio value="password">{{ $t('terminal.passwordMode') }}</el-radio>
                            <el-radio value="key">{{ $t('terminal.keyMode') }}</el-radio>
                        </el-radio-group>
                    </el-form-item>
                </div>
                <div v-if="dialogData.rowData!.type === 'SFTP' && dialogData.rowData!.varsJson['authMode'] === 'key'">
                    <el-form-item :label="$t('terminal.key')" prop="credential" :rules="[Rules.requiredInput]">
                        <el-input type="textarea" v-model="dialogData.rowData!.credential" />
                    </el-form-item>
                    <el-form-item :label="$t('terminal.keyPassword')" prop="varsJson.passPhrase">
                        <el-input
                            type="password"
                            show-password
                            clearable
                            v-model="dialogData.rowData!.varsJson['passPhrase']"
                        />
                    </el-form-item>
                </div>
                <el-form-item
                    v-else
                    :label="$t('commons.login.password')"
                    prop="credential"
                    :rules="[Rules.requiredInput]"
                >
                    <el-input type="password" clearable show-password v-model.trim="dialogData.rowData!.credential" />
                </el-form-item>
            </div>
            <el-form-item v-if="hasRemember()" prop="rememberAuth">
                <el-checkbox v-model="dialogData.rowData!.rememberAuth">
                    {{ $t('terminal.rememberPassword') }}
                </el-checkbox>
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'COS'"
                label="Region"
                prop="varsJson.region"
                :rules="Rules.requiredInput"
            >
                <el-checkbox v-model="regionInput" :label="$t('container.input')" />
                <el-select v-if="!regionInput" v-model="dialogData.rowData!.varsJson['region']" filterable clearable>
                    <el-option v-for="item in cities" :key="item.value" :label="item.label" :value="item.value">
                        <span class="float-left">{{ item.label }}</span>
                        <span class="option-help">
                            {{ item.value }}
                        </span>
                    </el-option>
                </el-select>
                <el-input v-else v-model.trim="dialogData.rowData!.varsJson['region']" />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'S3'"
                label="Region"
                prop="varsJson.region"
                :rules="Rules.requiredInput"
            >
                <el-input v-model.trim="dialogData.rowData!.varsJson['region']" />
            </el-form-item>
            <el-form-item
                v-if="hasAccessKey()"
                :label="dialogData.rowData!.type === 'KODO' ? $t('setting.domain') : 'Endpoint'"
                prop="varsJson.endpointItem"
                :rules="Rules.requiredInput"
            >
                <el-input v-model.trim="dialogData.rowData!.varsJson['endpointItem']">
                    <template #prepend>
                        <el-select v-model.trim="domainProto" class="p-w-100">
                            <el-option label="http" value="http" />
                            <el-option label="https" value="https" />
                        </el-select>
                    </template>
                </el-input>
            </el-form-item>
            <el-form-item v-if="hasAccessKey()" label="Bucket" prop="bucket" :rules="Rules.requiredInput">
                <el-checkbox v-model="dialogData.rowData!.bucketInput" :label="$t('container.input')" />
                <el-input clearable v-if="dialogData.rowData!.bucketInput" v-model="dialogData.rowData!.bucket" />
                <div v-else class="w-full">
                    <el-select class="!w-4/5" v-model="dialogData.rowData!.bucket">
                        <el-option v-for="item in buckets" :key="item" :value="item" />
                    </el-select>
                    <el-button class="!w-1/5" plain @click="getBuckets(formRef)">
                        {{ $t('setting.loadBucket') }}
                    </el-button>
                </div>
            </el-form-item>
            <el-form-item
                v-if="isUPYUN()"
                :label="$t('setting.serviceName')"
                prop="bucket"
                :rules="Rules.requiredInput"
            >
                <el-input v-model="dialogData.rowData!.bucket" />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'COS'"
                :label="$t('setting.scType')"
                prop="varsJson.scType"
                :rules="[Rules.requiredSelect]"
            >
                <el-select v-model="dialogData.rowData!.varsJson['scType']">
                    <el-option value="DEFAULT" :label="$t('setting.scLighthouse')" />
                    <el-option value="Standard" :label="$t('setting.scStandard')" />
                    <el-option value="Standard_IA" :label="$t('setting.scStandard_IA')" />
                    <el-option value="Archive" :label="$t('setting.scArchive')" />
                    <el-option value="Deep_Archive" :label="$t('setting.scDeep_Archive')" />
                </el-select>
                <el-alert
                    v-if="
                        dialogData.rowData!.varsJson['scType'] === 'Archive' ||
                        dialogData.rowData!.varsJson['scType'] === 'Deep_Archive'
                    "
                    class="mt-2.5"
                    :closable="false"
                    type="warning"
                    :title="$t('setting.archiveHelper')"
                />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'OSS'"
                :label="$t('setting.scType')"
                prop="varsJson.scType"
                :rules="[Rules.requiredSelect]"
            >
                <el-select v-model="dialogData.rowData!.varsJson['scType']">
                    <el-option value="Standard" :label="$t('setting.scStandard')" />
                    <el-option value="IA" :label="$t('setting.scStandard_IA')" />
                    <el-option value="Archive" :label="$t('setting.scArchive')" />
                    <el-option value="ColdArchive" :label="$t('setting.scDeep_Archive')" />
                </el-select>
                <el-alert
                    v-if="
                        dialogData.rowData!.varsJson['scType'] === 'Archive' ||
                        dialogData.rowData!.varsJson['scType'] === 'ColdArchive'
                    "
                    class="mt-2.5"
                    :closable="false"
                    type="warning"
                    :title="$t('setting.archiveHelper')"
                />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'S3'"
                :label="$t('setting.scType')"
                prop="varsJson.scType"
                :rules="[Rules.requiredSelect]"
            >
                <el-select v-model="dialogData.rowData!.varsJson['scType']">
                    <el-option value="STANDARD" :label="$t('setting.scStandard')" />
                    <el-option value="STANDARD_IA" :label="$t('setting.scStandard_IA')" />
                    <el-option value="GLACIER" :label="$t('setting.scArchive')" />
                    <el-option value="DEEP_ARCHIVE" :label="$t('setting.scDeep_Archive')" />
                </el-select>
                <el-alert
                    v-if="
                        dialogData.rowData!.varsJson['scType'] === 'GLACIER' ||
                        dialogData.rowData!.varsJson['scType'] === 'DEEP_ARCHIVE'
                    "
                    class="mt-2.5"
                    :closable="false"
                    type="warning"
                    :title="$t('setting.archiveHelper')"
                />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'KODO'"
                :label="$t('cronjob.requestExpirationTime')"
                prop="varsJson.timeout"
            >
                <el-input-number
                    style="width: 200px"
                    :min="1"
                    step-strictly
                    :step="1"
                    v-model.number="dialogData.rowData!.varsJson['timeout']"
                ></el-input-number>
            </el-form-item>
            <div v-if="isALIYUNYUN()">
                <el-form-item label="Token" prop="varsJson.token">
                    <div class="!w-full">
                        <el-input
                            style="width: calc(100% - 80px)"
                            :rows="3"
                            type="textarea"
                            clearable
                            v-model.trim="dialogData.rowData!.varsJson['token']"
                        />
                        <el-button class="append-button" @click="loadFromTokenForAliyun()">
                            {{ $t('setting.analysis') }}
                        </el-button>
                        <span class="input-help" v-if="!isFxplay">
                            {{ $t('setting.analysisHelper') }}
                            <el-link
                                style="font-size: 12px; margin-left: 5px"
                                icon="Position"
                                @click="toDoc('ali-pan')"
                                type="primary"
                            >
                                {{ $t('firewall.quickJump') }}
                            </el-link>
                        </span>
                    </div>
                </el-form-item>
                <el-form-item label="Drive ID" prop="varsJson.drive_id" :rules="Rules.requiredInput">
                    <el-input v-model.trim="dialogData.rowData!.varsJson['drive_id']" />
                </el-form-item>
                <el-form-item
                    label="Refresh Token"
                    prop="varsJson.refresh_token"
                    :rules="dialogData.title === 'create' ? Rules.requiredInput : []"
                >
                    <el-input v-model.trim="dialogData.rowData!.varsJson['refresh_token']" />
                </el-form-item>
            </div>

            <div v-if="hasClient()">
                <el-divider content-position="left">{{ $t('setting.oauthApplication') }}</el-divider>
                <el-form-item v-if="isOneDrive()">
                    <el-radio-group v-model="dialogData.rowData!.varsJson['isCN']" @change="changeOAuthCloud">
                        <el-radio-button :value="false">{{ $t('setting.isNotCN') }}</el-radio-button>
                        <el-radio-button :value="true">{{ $t('setting.isCN') }}</el-radio-button>
                    </el-radio-group>
                    <span class="input-help" v-if="!isFxplay">
                        {{ $t('setting.onedrive_helper') }}
                        <el-link
                            style="font-size: 12px; margin-left: 5px"
                            icon="Position"
                            @click="toDoc('onedrive')"
                            type="primary"
                        >
                            {{ $t('firewall.quickJump') }}
                        </el-link>
                    </span>
                </el-form-item>
                <el-form-item :label="$t('commons.table.status')">
                    <div class="oauth-status-line">
                        <el-tag :type="oauthStatusMeta.tagType">{{ $t(oauthStatusMeta.labelKey) }}</el-tag>
                        <span v-if="oauthClientIdDisplay" class="input-help">
                            {{ $t('setting.client_id') }}{{ $t('commons.colon') }}{{ oauthClientIdDisplay }}
                        </span>
                        <span v-if="oauthUpdatedAt" class="input-help">
                            {{ $t('commons.table.updatedAt') }}{{ $t('commons.colon') }}{{ oauthUpdatedAt }}
                        </span>
                    </div>
                </el-form-item>
                <el-alert
                    v-if="oauthStatus !== 'configured'"
                    class="mb-4"
                    :closable="false"
                    :title="$t(oauthStatusMeta.descriptionKey || oauthStatusMeta.labelKey)"
                    :type="oauthStatusMeta.tagType === 'danger' ? 'error' : oauthStatusMeta.tagType"
                    show-icon
                />
                <el-form-item :label="$t('setting.client_id')">
                    <el-input
                        v-model.trim="oauthForm.clientId"
                        clearable
                        autocomplete="off"
                        :placeholder="oauthClientIdPlaceholder"
                    />
                </el-form-item>
                <el-form-item :label="$t('setting.client_secret')">
                    <el-input
                        v-model="oauthForm.clientSecret"
                        type="password"
                        show-password
                        clearable
                        autocomplete="new-password"
                        :placeholder="$t('setting.oauthSecretPlaceholder')"
                    />
                    <span v-if="!oauthRequiresCredentials" class="input-help">
                        {{ $t('setting.oauthSecretKeepHelper') }}
                    </span>
                </el-form-item>
                <el-form-item :label="$t('setting.redirect_uri')">
                    <el-input
                        v-model.trim="oauthForm.redirectUri"
                        clearable
                        autocomplete="off"
                        :placeholder="$t('setting.oauthRedirectPlaceholder')"
                    />
                    <span v-if="!oauthRequiresCredentials" class="input-help">
                        {{ $t('setting.oauthRedirectKeepHelper') }}
                    </span>
                </el-form-item>
                <el-form-item>
                    <div class="flex flex-wrap gap-2">
                        <el-button type="primary" :loading="oauthLoading" @click="onBeginOAuth">
                            {{ $t(oauthAuthorizeKey) }}
                        </el-button>
                        <el-button
                            v-if="canClearOAuth"
                            v-permission
                            plain
                            type="danger"
                            :disabled="oauthLoading"
                            @click="onClearOAuth"
                        >
                            {{ $t('commons.button.clean') }}
                        </el-button>
                    </div>
                </el-form-item>
                <el-form-item v-if="oauthForm.flowId" :label="$t('setting.oauthAuthorizationResponse')">
                    <div class="!w-full">
                        <el-button class="mb-2" plain @click="onOpenOAuthAuthorization">
                            {{ $t('setting.oauthOpenAuthorization') }}
                        </el-button>
                        <el-input
                            :rows="3"
                            type="textarea"
                            clearable
                            v-model.trim="oauthForm.authorizationResponse"
                            :placeholder="$t('setting.oauthAuthorizationResponseHelper')"
                        />
                        <el-button class="mt-2" :loading="oauthLoading" @click="onCompleteOAuth">
                            {{ $t('commons.button.confirm') }}
                        </el-button>
                        <span class="input-help">{{ $t('setting.oauthAuthorizationResponseHelper') }}</span>
                    </div>
                </el-form-item>
                <el-alert
                    v-if="oauthForm.sessionId"
                    class="mb-4"
                    :closable="false"
                    :title="$t('setting.oauthAuthorizationCompleted')"
                    type="success"
                    show-icon
                />
            </div>
            <el-form-item v-if="hasBackDir()" :label="$t('setting.backupDir')" prop="backupPath">
                <el-input clearable v-model.trim="dialogData.rowData!.backupPath" placeholder="/1panel" />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'SFTP'"
                :label="$t('setting.backupDir')"
                prop="backupPath"
                :rules="[Rules.requiredInput]"
            >
                <el-input v-model.trim="dialogData.rowData!.backupPath" />
            </el-form-item>
            <el-form-item
                v-if="dialogData.rowData!.type === 'LOCAL'"
                :label="$t('setting.backupDir')"
                prop="backupPath"
                :rules="Rules.requiredInput"
            >
                <el-input v-model="dialogData.rowData!.backupPath">
                    <template #prepend>
                        <el-button icon="Folder" @click="fileRef.acceptParams({ dir: true })" />
                    </template>
                </el-input>
            </el-form-item>
        </el-form>
        <template #footer>
            <el-button :disabled="loading || oauthLoading" @click="handleClose">
                {{ $t('commons.button.cancel') }}
            </el-button>
            <el-button v-permission :disabled="loading || oauthLoading" @click="onCheck(formRef)">
                {{ $t('terminal.testConn') }}
            </el-button>
            <el-button v-permission type="primary" :disabled="!isOK || loading || oauthLoading" @click="onSubmit()">
                {{ $t('commons.button.confirm') }}
            </el-button>
        </template>
    </DrawerPro>
    <FileList ref="fileRef" @choose="loadDir" />
</template>

<script lang="ts" setup>
import { ref, watch, computed, onUnmounted } from 'vue';
import { Rules } from '@/global/form-rules';
import i18n from '@/lang';
import { ElForm, ElMessageBox } from 'element-plus';
import { Backup } from '@/api/interface/backup';
import FileList from '@/components/file-list/index.vue';
import {
    addBackup,
    beginOAuth,
    checkBackup,
    clearOAuth,
    completeOAuth,
    editBackup,
    listBucket,
} from '@/api/modules/backup';
import { cities } from './../helper';
import { dateFormat } from '@/utils/date';
import { deepCopy } from '@/utils/misc';
import { spliceHttp, splitHttp } from '@/utils/validate';
import { MsgError, MsgSuccess } from '@/utils/message';
import { Base64 } from 'js-base64';
import { useGlobalStore } from '@/composables/useGlobalStore';
import {
    buildOAuthBeginRequest,
    getOAuthStatusMeta,
    isOAuthSessionExpired,
    isOAuthProvider,
    mergeOAuthCredentialInfo,
    navigateOAuthAuthorization,
    openOAuthAuthorization,
    openOAuthAuthorizationPlaceholder,
    requiresOAuthCredentialInput,
    resolveOAuthStatus,
    sanitizeOAuthVars,
} from '../oauth';
const { docsUrl, isFxplay, isProductPro } = useGlobalStore();

const loading = ref(false);
const oauthLoading = ref(false);
type FormInstance = InstanceType<typeof ElForm>;
const formRef = ref<FormInstance>();
const buckets = ref();
const fileRef = ref();

const isOK = ref();
const stopWatch = ref();

const regionInput = ref();

const domainProto = ref('http');
const emit = defineEmits(['search']);

interface DialogProps {
    title: string;
    rowData?: Backup.BackupInfo;
}
const title = ref<string>('');
const drawerVisible = ref(false);
const dialogData = ref<DialogProps>({
    title: '',
});

interface OAuthFormState {
    clientId: string;
    clientSecret: string;
    redirectUri: string;
    authorizationUrl: string;
    authorizationResponse: string;
    flowId: string;
    flowRedirectUri: string;
    sessionId: string;
    sessionAccountName: string;
    sessionIsPublic: boolean | null;
    expiresAt: string;
    clientIdDisplay: string;
}

const emptyOAuthForm = (): OAuthFormState => ({
    clientId: '',
    clientSecret: '',
    redirectUri: '',
    authorizationUrl: '',
    authorizationResponse: '',
    flowId: '',
    flowRedirectUri: '',
    sessionId: '',
    sessionAccountName: '',
    sessionIsPublic: null,
    expiresAt: '',
    clientIdDisplay: '',
});
const oauthForm = ref<OAuthFormState>(emptyOAuthForm());
const oauthCloudChanged = ref(false);

const oauthVars = computed(() => dialogData.value.rowData?.varsJson || {});
const oauthStatus = computed(() => resolveOAuthStatus(oauthVars.value));
const oauthStatusMeta = computed(() => getOAuthStatusMeta(oauthVars.value));
const oauthClientIdDisplay = computed(
    () => oauthForm.value.clientIdDisplay || String(oauthVars.value.oauth_client_id_display || ''),
);
const oauthUpdatedAt = computed(() => String(oauthVars.value.oauth_updated_at || ''));
const oauthRequiresCredentials = computed(() => {
    return !dialogData.value.rowData?.id || oauthCloudChanged.value || requiresOAuthCredentialInput(oauthVars.value);
});
const oauthClientIdPlaceholder = computed(() => {
    if (oauthClientIdDisplay.value && !oauthRequiresCredentials.value) {
        return i18n.global.t('setting.oauthClientIdKeepPlaceholder', [oauthClientIdDisplay.value]);
    }
    return i18n.global.t('setting.oauthClientIdPlaceholder');
});
const oauthAuthorizeKey = computed(() => {
    return oauthStatus.value === 'configured' || oauthStatus.value === 'reauthorization_required'
        ? 'setting.oauthReauthorize'
        : 'setting.oauthAuthorize';
});
const canClearOAuth = computed(() => {
    return !!dialogData.value.rowData?.id && oauthStatus.value !== 'unconfigured';
});

const formWatcher = computed(() => {
    const { name, type, isPublic, accessKey, bucket, credential, backupPath, bucketInput, varsJson, oauthSession } =
        dialogData.value.rowData || {};
    return {
        name,
        type,
        isPublic,
        accessKey,
        bucket,
        credential,
        backupPath,
        bucketInput,
        varsJson,
        oauthSession,
        oauthClientId: oauthForm.value.clientId,
        oauthClientSecret: oauthForm.value.clientSecret,
        oauthRedirectUri: oauthForm.value.redirectUri,
    };
});
const startWatcher = () => {
    if (stopWatch.value) {
        stopWatcher();
    }
    stopWatch.value = watch(
        () => formWatcher.value,
        () => {
            stopWatcher();
            isOK.value = false;
        },
        { deep: true },
    );
};
const stopWatcher = () => {
    if (stopWatch.value) {
        stopWatch.value();
        stopWatch.value = null;
    }
};

const resetOAuthForm = (vars: Record<string, any> = {}) => {
    oauthForm.value = {
        ...emptyOAuthForm(),
        redirectUri: String(vars.redirect_uri || ''),
        clientIdDisplay: String(vars.oauth_client_id_display || ''),
    };
    oauthCloudChanged.value = false;
    if (dialogData.value.rowData) {
        dialogData.value.rowData.oauthSession = undefined;
    }
};

const clearOAuthBrowserState = () => {
    oauthForm.value = emptyOAuthForm();
    oauthCloudChanged.value = false;
    if (dialogData.value.rowData) {
        dialogData.value.rowData.oauthSession = undefined;
    }
};

const clearOAuthPendingFlow = () => {
    oauthForm.value.flowId = '';
    oauthForm.value.flowRedirectUri = '';
    oauthForm.value.authorizationUrl = '';
    oauthForm.value.authorizationResponse = '';
    oauthForm.value.expiresAt = '';
};

const acceptParams = (params: DialogProps): void => {
    dialogData.value = params;
    dialogData.value.rowData.varsJson = dialogData.value.rowData!.vars
        ? JSON.parse(dialogData.value.rowData!.vars)
        : {};
    if (isOAuthProvider(dialogData.value.rowData!.type)) {
        dialogData.value.rowData!.varsJson = mergeOAuthCredentialInfo(
            dialogData.value.rowData!.varsJson,
            dialogData.value.rowData!.oauth,
        );
        resetOAuthForm(dialogData.value.rowData!.varsJson);
    } else {
        clearOAuthBrowserState();
    }
    isOK.value = false;
    title.value = i18n.global.t('commons.button.' + dialogData.value.title);
    if (dialogData.value.title === 'create') {
        dialogData.value.rowData!.type = 'OSS';
        changeType();
        drawerVisible.value = true;
        return;
    }
    buckets.value = [];
    if (hasAccessKey()) {
        let itemJson = dialogData.value.rowData!.varsJson['endpoint'];
        if (dialogData.value.rowData!.type === 'KODO') {
            itemJson = dialogData.value.rowData!.varsJson['domain'];
        }
        let httpItem = splitHttp(itemJson);
        dialogData.value.rowData!.varsJson['endpointItem'] = httpItem.url;
        domainProto.value = httpItem.proto;
    }
    if (dialogData.value.rowData!.rememberAuth) {
        dialogData.value.rowData!.accessKey = Base64.decode(dialogData.value.rowData!.accessKey);
        dialogData.value.rowData!.credential = Base64.decode(dialogData.value.rowData!.credential);
    }
    if (dialogData.value.rowData!.varsJson['timeout'] === undefined) {
        dialogData.value.rowData!.varsJson['timeout'] = 1;
    }
    drawerVisible.value = true;
};
const toDoc = (type: string) => {
    let uri = '';
    switch (type) {
        case 'onedrive':
            uri = '#42-onedrive';
            break;
        case 'onedrive-bind':
            uri = '#43-onedrive';
            break;
        case 'ali-pan':
            uri = '#44';
            break;
        case 'google-drive':
            uri = '#45';
            break;
        case 'webdav':
            uri = '#46-webdav-alist';
            break;
    }
    window.open(docsUrl.value + '/user_manual/settings/' + uri, '_blank', 'noopener,noreferrer');
};

const hasPendingOAuthChanges = () => {
    const storedRedirectUri = String(oauthVars.value.redirect_uri || '');
    return (
        oauthCloudChanged.value ||
        !!oauthForm.value.clientId ||
        !!oauthForm.value.clientSecret ||
        (!!oauthForm.value.redirectUri && oauthForm.value.redirectUri !== storedRedirectUri)
    );
};

const hasCurrentOAuthSession = () => {
    const row = dialogData.value.rowData;
    return (
        !!row &&
        !!oauthForm.value.sessionId &&
        !isOAuthSessionExpired(oauthForm.value.expiresAt) &&
        oauthForm.value.sessionAccountName === row.name.trim() &&
        oauthForm.value.sessionIsPublic === row.isPublic &&
        !hasPendingOAuthChanges()
    );
};

const prepareOAuthVars = () => {
    if (!dialogData.value.rowData || !hasClient()) return;
    const vars = sanitizeOAuthVars(dialogData.value.rowData.varsJson);
    vars.isCN = isOneDrive() && (vars.isCN === true || vars.isCN === 'true');
    if (oauthForm.value.redirectUri.trim()) {
        vars.redirect_uri = oauthForm.value.redirectUri.trim();
    }
    if (oauthClientIdDisplay.value) {
        vars.oauth_client_id_display = oauthClientIdDisplay.value;
    }
    if (oauthForm.value.sessionId) {
        vars.oauth_status = 'configured';
        vars.oauth_configured = true;
        vars.oauth_authorized = true;
        dialogData.value.rowData.oauthSession = oauthForm.value.sessionId;
    }
    dialogData.value.rowData.varsJson = vars;
};

const requireOAuthAuthorization = () => {
    if (!hasClient()) return true;
    if (oauthForm.value.sessionId && isOAuthSessionExpired(oauthForm.value.expiresAt)) {
        isOK.value = false;
        MsgError(i18n.global.t('setting.oauthSessionExpired'));
        return false;
    }
    const hasSession = hasCurrentOAuthSession();
    if (
        (oauthStatusMeta.value.requiresAuthorization || hasPendingOAuthChanges() || !!oauthForm.value.flowId) &&
        !hasSession
    ) {
        MsgError(i18n.global.t('setting.oauthAuthorizationRequired'));
        return false;
    }
    return true;
};

const onBeginOAuth = async () => {
    const row = dialogData.value.rowData;
    if (!row || !isOAuthProvider(row.type)) return;
    const clientId = oauthForm.value.clientId.trim();
    const clientSecret = oauthForm.value.clientSecret;
    const redirectUri = oauthForm.value.redirectUri.trim();
    if (!row.name?.trim()) {
        MsgError(i18n.global.t('commons.rule.requiredInput'));
        return;
    }
    if (oauthRequiresCredentials.value && (!clientId || !clientSecret || !redirectUri)) {
        MsgError(i18n.global.t('setting.oauthCredentialsRequired'));
        return;
    }

    const authorizationWindow = openOAuthAuthorizationPlaceholder();
    oauthLoading.value = true;
    try {
        const res = await beginOAuth(
            buildOAuthBeginRequest({
                provider: row.type,
                accountId: row.id || 0,
                accountName: row.name,
                isPublic: row.isPublic,
                isCN: isOneDrive() && (row.varsJson['isCN'] === true || row.varsJson['isCN'] === 'true'),
                clientId,
                clientSecret,
                redirectUri,
            }),
        );
        oauthForm.value.flowId = res.data.flowId;
        oauthForm.value.sessionId = '';
        oauthForm.value.sessionAccountName = row.name.trim();
        oauthForm.value.sessionIsPublic = row.isPublic;
        oauthForm.value.authorizationUrl = res.data.authorizationUrl;
        oauthForm.value.flowRedirectUri = redirectUri;
        oauthForm.value.authorizationResponse = '';
        oauthForm.value.expiresAt = res.data.expiresAt;
        oauthForm.value.clientIdDisplay = res.data.clientIdDisplay;
        oauthForm.value.clientId = '';
        oauthForm.value.clientSecret = '';
        row.oauthSession = undefined;
        isOK.value = false;
        if (!navigateOAuthAuthorization(authorizationWindow, res.data.authorizationUrl)) {
            openOAuthAuthorization(res.data.authorizationUrl);
        }
    } catch {
        authorizationWindow?.close();
    } finally {
        oauthForm.value.clientSecret = '';
        oauthLoading.value = false;
    }
};

const onOpenOAuthAuthorization = () => {
    if (!oauthForm.value.authorizationUrl) return;
    openOAuthAuthorization(oauthForm.value.authorizationUrl);
};

const onCompleteOAuth = async () => {
    const row = dialogData.value.rowData;
    const authorizationResponse = oauthForm.value.authorizationResponse.trim();
    oauthForm.value.authorizationResponse = '';
    if (!row || !oauthForm.value.flowId || !authorizationResponse) {
        MsgError(i18n.global.t('setting.oauthAuthorizationResponseRequired'));
        return;
    }
    if (
        oauthForm.value.sessionAccountName !== row.name.trim() ||
        oauthForm.value.sessionIsPublic !== row.isPublic ||
        !!oauthForm.value.clientId.trim() ||
        !!oauthForm.value.clientSecret ||
        oauthForm.value.redirectUri.trim() !== oauthForm.value.flowRedirectUri
    ) {
        MsgError(i18n.global.t('setting.oauthAuthorizationRequired'));
        return;
    }
    try {
        const parsed = new URL(authorizationResponse);
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
            throw new Error('unsupported protocol');
        }
    } catch {
        MsgError(i18n.global.t('setting.oauthInvalidAuthorizationResponse'));
        return;
    }

    oauthLoading.value = true;
    try {
        const res = await completeOAuth({ flowId: oauthForm.value.flowId, authorizationResponse }, row.isPublic);
        oauthForm.value.sessionId = res.data.sessionId;
        oauthForm.value.clientIdDisplay = res.data.clientIdDisplay;
        oauthForm.value.expiresAt = res.data.expiresAt;
        oauthForm.value.flowId = '';
        const authorizedRedirectUri = oauthForm.value.flowRedirectUri;
        oauthForm.value.flowRedirectUri = '';
        oauthForm.value.authorizationUrl = '';
        row.oauthSession = res.data.sessionId;
        row.varsJson = {
            ...sanitizeOAuthVars(row.varsJson),
            oauth_status: 'configured',
            oauth_configured: true,
            oauth_authorized: true,
            oauth_client_id_display: res.data.clientIdDisplay,
            redirect_uri: authorizedRedirectUri || row.varsJson.redirect_uri,
        };
        oauthCloudChanged.value = false;
        isOK.value = true;
        startWatcher();
        MsgSuccess(i18n.global.t('setting.oauthAuthorizationCompleted'));
    } catch {
        // The HTTP client already presents a sanitized API error. Consuming the
        // rejection here prevents Axios request data from reaching browser logs.
        clearOAuthPendingFlow();
    } finally {
        oauthLoading.value = false;
    }
};

const onClearOAuth = async () => {
    const row = dialogData.value.rowData;
    if (!row?.id) return;
    try {
        await ElMessageBox.confirm(i18n.global.t('setting.oauthClearConfirm'), i18n.global.t('commons.button.clean'), {
            confirmButtonText: i18n.global.t('commons.button.confirm'),
            cancelButtonText: i18n.global.t('commons.button.cancel'),
            type: 'warning',
        });
    } catch {
        return;
    }

    oauthLoading.value = true;
    try {
        await clearOAuth({ id: row.id, name: row.name, isPublic: row.isPublic });
        row.varsJson = {
            isCN: isOneDrive() && (row.varsJson['isCN'] === true || row.varsJson['isCN'] === 'true'),
            oauth_status: 'unconfigured',
            oauth_configured: false,
            oauth_authorized: false,
        };
        resetOAuthForm(row.varsJson);
        isOK.value = false;
        emit('search');
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } finally {
        oauthLoading.value = false;
    }
};

function callback(error: any) {
    if (error) {
        return error.message;
    } else {
        return;
    }
}

const loadFromTokenForAliyun = () => {
    const obj = JSON.parse(dialogData.value.rowData!.varsJson['token']);
    dialogData.value.rowData!.varsJson['drive_id'] = obj.default_drive_id;
    dialogData.value.rowData!.varsJson['refresh_token'] = obj.refresh_token;
};
const hasRemember = () => {
    return (
        dialogData.value.rowData!.type !== 'LOCAL' &&
        dialogData.value.rowData!.type !== 'OneDrive' &&
        dialogData.value.rowData!.type !== 'ALIYUN' &&
        dialogData.value.rowData!.type !== 'GoogleDrive'
    );
};
const hasClient = () => {
    let itemType = dialogData.value.rowData!.type;
    return isOAuthProvider(itemType);
};
const isOneDrive = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType === 'OneDrive';
};
const isUPYUN = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType === 'UPYUN';
};
const isALIYUNYUN = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType === 'ALIYUN';
};
const hasAccessKey = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType === 'COS' || itemType === 'KODO' || itemType === 'MINIO' || itemType === 'OSS' || itemType === 'S3';
};
const hasPassword = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType === 'SFTP' || itemType === 'WebDAV';
};

const hasBackDir = () => {
    let itemType = dialogData.value.rowData!.type;
    return itemType !== 'LOCAL' && itemType !== 'SFTP';
};

const loadDir = async (path: string) => {
    dialogData.value.rowData!.backupPath = path;
};

const changeType = async () => {
    buckets.value = [];
    clearOAuthBrowserState();
    dialogData.value.rowData!.varsJson = {};
    dialogData.value.rowData!.rememberAuth = false;
    switch (dialogData.value.rowData!.type) {
        case 'COS':
        case 'OSS':
            dialogData.value.rowData.varsJson['scType'] = 'Standard';
            break;
        case 'S3':
            dialogData.value.rowData.varsJson['scType'] = 'STANDARD';
            dialogData.value.rowData.varsJson['mode'] = 'virtual hosted';
            break;
        case 'KODO':
            dialogData.value.rowData!.varsJson['timeout'] = 1;
            break;
        case 'OneDrive':
            dialogData.value.rowData.varsJson = {
                isCN: false,
                oauth_status: 'unconfigured',
                oauth_configured: false,
                oauth_authorized: false,
            };
            resetOAuthForm(dialogData.value.rowData.varsJson);
            break;
        case 'GoogleDrive':
            dialogData.value.rowData.varsJson = {
                oauth_status: 'unconfigured',
                oauth_configured: false,
                oauth_authorized: false,
            };
            resetOAuthForm(dialogData.value.rowData.varsJson);
            break;
        case 'SFTP':
            dialogData.value.rowData.varsJson['port'] = 22;
            dialogData.value.rowData.varsJson['authMode'] = 'password';
            break;
    }
};
const changeOAuthCloud = () => {
    const isCN = dialogData.value.rowData.varsJson['isCN'] === true;
    dialogData.value.rowData.varsJson = {
        isCN,
        oauth_status: 'unconfigured',
        oauth_configured: false,
        oauth_authorized: false,
    };
    resetOAuthForm(dialogData.value.rowData.varsJson);
    oauthCloudChanged.value = true;
    isOK.value = false;
};

const handleClose = () => {
    stopWatcher();
    clearOAuthBrowserState();
    emit('search');
    drawerVisible.value = false;
};

const getBuckets = async (formEl: FormInstance | undefined) => {
    if (!formEl) return;
    const result1 = await formEl.validateField('varsJson.endpointItem', callback);
    const result2 = await formEl.validateField('accessKey', callback);
    const result3 = await formEl.validateField('credential', callback);
    const result4 = await formEl.validateField('varsJson.region', callback);
    if (!result1 || !result2 || !result3 || !result4) {
        return;
    }
    loading.value = true;
    let item = deepCopy(dialogData.value.rowData!.varsJson);
    let itemEndpoint = loadEndpoint();
    if (dialogData.value.rowData!.type === 'KODO') {
        item['domain'] = itemEndpoint;
    } else {
        item['endpoint'] = itemEndpoint;
    }
    item['endpointItem'] = undefined;
    listBucket({
        isPublic: dialogData.value.rowData!.isPublic,
        type: dialogData.value.rowData!.type,
        vars: JSON.stringify(item),
        accessKey: dialogData.value.rowData!.accessKey,
        credential: dialogData.value.rowData!.credential,
    })
        .then((res) => {
            loading.value = false;
            buckets.value = res.data;
        })
        .catch(() => {
            buckets.value = [];
            loading.value = false;
        });
};

const loadEndpoint = () => {
    let item = splitHttp(dialogData.value.rowData!.varsJson['endpointItem']);
    if (item.proto) {
        domainProto.value = item.proto;
        dialogData.value.rowData!.varsJson['endpointItem'] = item.url;
    }
    return spliceHttp(domainProto.value, dialogData.value.rowData!.varsJson['endpointItem']);
};

const onCheck = async (formEl: FormInstance | undefined) => {
    if (!formEl) return;
    formEl.validate(async (valid) => {
        if (!valid) return;
        if (!dialogData.value.rowData) return;
        if (!requireOAuthAuthorization()) return;
        if (hasClient() && hasCurrentOAuthSession()) {
            prepareOAuthVars();
            isOK.value = true;
            MsgSuccess(i18n.global.t('terminal.connTestOk'));
            startWatcher();
            return;
        }
        if (hasAccessKey()) {
            let itemEndpoint = loadEndpoint();
            if (dialogData.value.rowData!.type === 'KODO') {
                dialogData.value.rowData!.varsJson['domain'] = itemEndpoint;
            } else {
                dialogData.value.rowData!.varsJson['endpoint'] = itemEndpoint;
            }
        }
        if (isALIYUNYUN()) {
            dialogData.value.rowData!.varsJson['token'] = undefined;
        }
        prepareOAuthVars();
        dialogData.value.rowData.vars = JSON.stringify(dialogData.value.rowData!.varsJson);
        loading.value = true;
        await checkBackup(dialogData.value.rowData)
            .then((res) => {
                loading.value = false;
                if (res.data.isOk) {
                    isOK.value = true;
                    MsgSuccess(i18n.global.t('terminal.connTestOk'));
                    if (isALIYUNYUN()) {
                        dialogData.value.rowData!.varsJson['refresh_status'] = 'Success';
                        dialogData.value.rowData!.varsJson['refresh_time'] = dateFormat(null, null, new Date());
                    }
                    startWatcher();
                    return;
                }
                isOK.value = false;
                MsgError(
                    hasClient()
                        ? i18n.global.t('terminal.connTestFailed')
                        : i18n.global.t('terminal.connTestFailed') + ':' + res.data.msg,
                );
            })
            .catch(() => {
                loading.value = false;
                isOK.value = false;
            });
    });
};

const onSubmit = async () => {
    if (!requireOAuthAuthorization()) return;
    prepareOAuthVars();
    dialogData.value.rowData.vars = JSON.stringify(dialogData.value.rowData!.varsJson);
    loading.value = true;
    if (dialogData.value.title === 'create') {
        await addBackup(dialogData.value.rowData)
            .then(() => {
                loading.value = false;
                MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
                clearOAuthBrowserState();
                drawerVisible.value = false;
            })
            .catch(() => {
                loading.value = false;
            });
        return;
    }
    await editBackup(dialogData.value.rowData)
        .then(() => {
            loading.value = false;
            MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
            clearOAuthBrowserState();
            drawerVisible.value = false;
        })
        .catch(() => {
            loading.value = false;
        });
};

onUnmounted(() => {
    if (stopWatch.value) stopWatcher();
    clearOAuthBrowserState();
});

defineExpose({
    acceptParams,
});
</script>

<style scoped lang="scss">
.option-help {
    float: right;
    font-size: 12px;
    word-break: break-all;
    color: #8f959e;
}

.oauth-status-line {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
}
</style>
