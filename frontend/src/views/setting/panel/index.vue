<template>
    <div v-loading="loading">
        <LayoutContent :title="$t('setting.panel')" :divider="true">
            <template #main>
                <el-form :model="form" :label-position="isMobile ? 'top' : 'left'" label-width="150px">
                    <el-row>
                        <el-col :span="1"><br /></el-col>
                        <el-col :xs="24" :sm="20" :md="15" :lg="12" :xl="12">
                            <el-form-item :label="$t('setting.theme')" prop="theme">
                                <div class="flex justify-center items-center sm:gap-6 gap-2">
                                    <div class="sm:contents hidden">
                                        <el-radio-group @change="onSave('Theme', form.theme)" v-model="form.theme">
                                            <el-radio-button value="light">
                                                <span>{{ $t('setting.light') }}</span>
                                            </el-radio-button>
                                            <el-radio-button value="dark">
                                                <span>{{ $t('setting.dark') }}</span>
                                            </el-radio-button>
                                            <el-radio-button value="auto">
                                                <span>{{ $t('setting.auto') }}</span>
                                            </el-radio-button>
                                        </el-radio-group>
                                    </div>
                                    <div class="sm:hidden block w-32 !h-[33.5px]">
                                        <el-select @change="onSave('Theme', form.theme)" v-model="form.theme">
                                            <el-option key="light" value="light" :label="$t('setting.light')">
                                                {{ $t('setting.light') }}
                                            </el-option>
                                            <el-option key="dark" value="dark" :label="$t('setting.dark')">
                                                {{ $t('setting.dark') }}
                                            </el-option>
                                            <el-option key="auto" value="auto" :label="$t('setting.auto')">
                                                {{ $t('setting.auto') }}
                                            </el-option>
                                        </el-select>
                                    </div>
                                    <div>
                                        <el-button
                                            @click="onChangeThemeColor"
                                            icon="Setting"
                                            class="!h-[32px] sm:!h-[33.5px]"
                                        >
                                            <span>{{ $t('container.custom') }}</span>
                                        </el-button>
                                    </div>
                                </div>
                            </el-form-item>

                            <el-form-item :label="$t('setting.menuTabs')" prop="menuTabs">
                                <el-radio-group @change="onSave('MenuTabs', form.menuTabs)" v-model="form.menuTabs">
                                    <el-radio-button value="Enable">
                                        <span>{{ $t('commons.button.enable') }}</span>
                                    </el-radio-button>
                                    <el-radio-button value="Disable">
                                        <span>{{ $t('commons.button.disable') }}</span>
                                    </el-radio-button>
                                </el-radio-group>
                            </el-form-item>

                            <el-form-item :label="$t('setting.watermark')" prop="watermark">
                                <el-radio-group class="w-full" @change="onChangeWatermark" v-model="form.watermarkShow">
                                    <el-radio-button value="Enable">
                                        <span>{{ $t('commons.button.enable') }}</span>
                                    </el-radio-button>
                                    <el-radio-button value="Disable">
                                        <span>{{ $t('commons.button.disable') }}</span>
                                    </el-radio-button>
                                </el-radio-group>
                                <div v-if="form.watermarkShow === 'Enable'">
                                    <div>
                                        <el-button link type="primary" @click="onChangeWatermark">
                                            {{ $t('commons.button.view') }}
                                        </el-button>
                                    </div>
                                </div>
                            </el-form-item>

                            <el-form-item :label="$t('setting.title')" prop="panelName">
                                <el-input disabled v-model="form.panelName">
                                    <template #append>
                                        <el-button icon="Setting" @click="onChangeTitle">
                                            {{ $t('commons.button.set') }}
                                        </el-button>
                                    </template>
                                </el-input>
                            </el-form-item>

                            <el-form-item :label="$t('setting.brandText')" prop="title">
                                <el-input
                                    class="sm:!w-1/2 !w-full"
                                    v-model="form.title"
                                    maxlength="64"
                                    @change="onSaveBranding('Title', form.title)"
                                />
                                <span class="input-help">{{ $t('setting.brandTextHelper') }}</span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.masterAlias')" prop="masterAlias">
                                <el-input
                                    class="sm:!w-1/2 !w-full"
                                    v-model="form.masterAlias"
                                    maxlength="64"
                                    @change="onSaveBranding('MasterAlias', form.masterAlias)"
                                />
                            </el-form-item>

                            <el-form-item :label="$t('setting.loginBgTypeLabel')" prop="loginBgType">
                                <el-select
                                    class="sm:!w-1/2 !w-full"
                                    v-model="form.loginBgType"
                                    @change="onSaveBranding('LoginBgType', form.loginBgType)"
                                >
                                    <el-option value="image" :label="$t('setting.loginBgTypeImage')" />
                                    <el-option value="color" :label="$t('setting.loginBgTypeColor')" />
                                </el-select>
                            </el-form-item>

                            <el-form-item
                                v-if="form.loginBgType === 'color'"
                                :label="$t('setting.loginBackgroundColor')"
                                prop="loginBackground"
                            >
                                <el-color-picker
                                    v-model="form.loginBackground"
                                    @change="onSaveBranding('LoginBackground', form.loginBackground)"
                                />
                            </el-form-item>

                            <el-form-item
                                v-if="form.loginBgType === 'image'"
                                :label="$t('setting.brandLoginBgImage')"
                                prop="loginBackground"
                            >
                                <div class="brand-asset">
                                    <img
                                        v-if="assetValue('loginBackground')"
                                        :src="assetPreviewUrl('loginBackground')"
                                        class="brand-asset__preview"
                                        alt=""
                                    />
                                    <el-upload
                                        :show-file-list="false"
                                        accept="image/png,image/jpeg,image/gif,image/webp"
                                        :before-upload="(file) => beforeAssetUpload('loginBackground', file)"
                                        :http-request="(options) => onUploadAsset('loginBackground', options.file)"
                                    >
                                        <el-button icon="Upload">{{ $t('setting.brandUpload') }}</el-button>
                                    </el-upload>
                                    <el-button
                                        v-if="assetValue('loginBackground')"
                                        link
                                        type="danger"
                                        @click="onResetAsset('loginBackground')"
                                    >
                                        {{ $t('commons.button.reset') }}
                                    </el-button>
                                </div>
                                <span class="input-help">{{ $t('setting.brandImageHelper') }}</span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.loginBtnLinkColorLabel')" prop="loginBtnLinkColor">
                                <el-color-picker
                                    v-model="form.loginBtnLinkColor"
                                    @change="onSaveBranding('LoginBtnLinkColor', form.loginBtnLinkColor)"
                                />
                            </el-form-item>

                            <el-form-item v-for="asset in brandAssets" :key="asset.key" :label="$t(asset.label)">
                                <div class="brand-asset">
                                    <img
                                        v-if="assetValue(asset.key)"
                                        :src="assetPreviewUrl(asset.key)"
                                        class="brand-asset__preview"
                                        alt=""
                                    />
                                    <el-upload
                                        :show-file-list="false"
                                        :accept="asset.accept"
                                        :before-upload="(file) => beforeAssetUpload(asset.key, file)"
                                        :http-request="(options) => onUploadAsset(asset.key, options.file)"
                                    >
                                        <el-button icon="Upload">{{ $t('setting.brandUpload') }}</el-button>
                                    </el-upload>
                                    <el-button
                                        v-if="assetValue(asset.key)"
                                        link
                                        type="danger"
                                        @click="onResetAsset(asset.key)"
                                    >
                                        {{ $t('commons.button.reset') }}
                                    </el-button>
                                </div>
                                <span class="input-help">{{ $t(asset.helper) }}</span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.language')" prop="language">
                                <el-select
                                    class="sm:!w-1/2 !w-full"
                                    @change="onSave('Language', form.language)"
                                    v-model="form.language"
                                >
                                    <el-option
                                        v-for="option in languageOptions"
                                        :key="option.value"
                                        :value="option.value"
                                        :label="option.label"
                                    >
                                        {{ option.label }}
                                    </el-option>
                                </el-select>
                            </el-form-item>

                            <el-form-item :label="$t('setting.sessionTimeout')" prop="sessionTimeout">
                                <el-input disabled v-model.number="form.sessionTimeout">
                                    <template #append>
                                        <el-button @click="onChangeTimeout" icon="Setting">
                                            {{ $t('commons.button.set') }}
                                        </el-button>
                                    </template>
                                </el-input>
                                <span class="input-help">
                                    {{ $t('setting.sessionTimeoutHelper', [form.sessionTimeout]) }}
                                </span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.systemIP')" prop="systemIP">
                                <el-input disabled v-if="form.systemIP" v-model="form.systemIP">
                                    <template #append>
                                        <el-button @click="onChangeSystemIP" icon="Setting">
                                            {{ $t('commons.button.set') }}
                                        </el-button>
                                    </template>
                                </el-input>
                                <el-input disabled v-if="!form.systemIP" v-model="unset">
                                    <template #append>
                                        <el-button @click="onChangeSystemIP" icon="Setting">
                                            {{ $t('commons.button.set') }}
                                        </el-button>
                                    </template>
                                </el-input>
                                <span class="input-help">{{ $t('setting.systemIPHelper') }}</span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.proxy')" prop="proxyShow">
                                <el-input disabled v-model="form.proxyShow">
                                    <template #append>
                                        <el-button @click="onChangeProxy" icon="Setting">
                                            {{ $t('commons.button.set') }}
                                        </el-button>
                                    </template>
                                </el-input>
                            </el-form-item>

                            <el-form-item
                                v-if="!isEnterprise"
                                :label="$t('setting.developerMode')"
                                prop="developerMode"
                            >
                                <el-radio-group
                                    @change="onSave('DeveloperMode', form.developerMode)"
                                    v-model="form.developerMode"
                                >
                                    <el-radio-button value="Enable">
                                        <span>{{ $t('commons.button.enable') }}</span>
                                    </el-radio-button>
                                    <el-radio-button value="Disable">
                                        <span>{{ $t('commons.button.disable') }}</span>
                                    </el-radio-button>
                                </el-radio-group>
                                <span class="input-help">{{ $t('setting.developerModeHelper') }}</span>
                            </el-form-item>

                            <el-form-item :label="$t('setting.menuSetting')">
                                <el-button v-show="!show" @click="onChangeHideMenus" icon="Setting">
                                    {{ $t('commons.button.set') }}
                                </el-button>
                            </el-form-item>

                            <el-form-item :label="$t('setting.runtimeEnv')" prop="edition">
                                <el-button icon="Setting" @click="onChangeRegion">
                                    {{ runtimeEnvLabel() }}
                                </el-button>
                            </el-form-item>
                        </el-col>
                    </el-row>
                </el-form>
            </template>
        </LayoutContent>

        <PanelName ref="panelNameRef" @search="search()" />
        <SystemIP ref="systemIPRef" @search="search()" />
        <Proxy ref="proxyRef" @search="search()" />
        <Timeout ref="timeoutRef" @search="search()" />
        <HideMenu ref="hideMenuRef" @search="search()" />
        <ThemeColor ref="themeColorRef" />
        <Watermark ref="watermarkRef" @search="search()" />
        <Edition ref="editionRef" @search="search()" />
    </div>
</template>

<script lang="ts" setup>
import { ref, reactive, onMounted } from 'vue';
import { ElForm, ElMessageBox } from 'element-plus';
import { getSettingInfo, updateSetting, getSystemAvailable, getAgentSettingInfo } from '@/api/modules/setting';
import { uploadOpenEnhancementAsset, resetOpenEnhancementAsset } from '@/api/modules/enhancement';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { useTheme } from '@/global/use-theme';
import { MsgSuccess, MsgError } from '@/utils/message';
import ThemeColor from '@/views/setting/panel/theme-color/index.vue';
import Watermark from '@/views/setting/panel/watermark/index.vue';
import Edition from '@/views/setting/panel/edition/index.vue';
import Timeout from '@/views/setting/panel/timeout/index.vue';
import PanelName from '@/views/setting/panel/name/index.vue';
import SystemIP from '@/views/setting/panel/systemip/index.vue';
import Proxy from '@/views/setting/panel/proxy/index.vue';
import HideMenu from '@/views/setting/panel/hidemenu/index.vue';
import { getXpackProxyDocker } from '@/extensions/xpack';
import { getXpackSetting, updateXpackSettingByKey } from '@/utils/xpack';
import { setPrimaryColor } from '@/utils/theme';
import { codeEditorThemeStorageKey } from '@/utils/code-editor-theme';
import i18n from '@/lang';

const {
    globalStore,
    isEnterprise,
    isIntl,
    isMobile,
    menuAccordion,
    openMenuTabs,
    themeConfig,
    watermark,
    watermarkShow,
} = useGlobalStore();

const loading = ref(false);

const { switchTheme } = useTheme();

interface ThemeColor {
    light: string;
    dark: string;
    themePredefineColors: {
        light: string[];
        dark: string[];
    };
}

const form = reactive({
    panelName: '',
    theme: '',
    watermark: '',
    watermarkShow: '',
    title: '',
    masterAlias: '',
    loginBgType: '',
    loginBackground: '',
    loginBtnLinkColor: '',
    logo: '',
    logoWithText: '',
    favicon: '',
    loginImage: '',
    themeColor: {} as ThemeColor,
    menuTabs: '',
    menuAccordion: '',
    language: '',
    sessionTimeout: 0,
    docSource: 'withByRegion',
    edition: '',
    developerMode: '',
    systemIP: '',

    proxyShow: '',
    proxyUrl: '',
    proxyType: '',
    proxyPort: '',
    proxyUser: '',
    proxyPasswd: '',
    proxyPasswdKeep: '',
    proxyDocker: '',

    hideMenu: '',
});

const show = ref();

const panelNameRef = ref();
const systemIPRef = ref();
const proxyRef = ref();
const timeoutRef = ref();
const hideMenuRef = ref();
const watermarkRef = ref();
const themeColorRef = ref();
const editionRef = ref();
const unset = ref(i18n.global.t('setting.unSetting'));

const languageOptions = ref([
    { value: 'zh', label: '中文(简体)' },
    { value: 'zh-Hant', label: '中文(繁體)' },
    ...(!isIntl.value ? [{ value: 'en', label: 'English' }] : []),
    { value: 'ja', label: '日本語' },
    { value: 'pt-BR', label: 'Português (Brasil)' },
    { value: 'ko', label: '한국어' },
    { value: 'ru', label: 'Русский' },
    { value: 'ms', label: 'Bahasa Melayu' },
    { value: 'tr', label: 'Turkish' },
    { value: 'es-ES', label: 'España - Español' },
    { value: 'fa', label: 'فارسی' },
]);

if (isIntl.value) {
    languageOptions.value.unshift({ value: 'en', label: 'English' });
}

const search = async () => {
    const agentRes = await getAgentSettingInfo();
    form.systemIP = agentRes.data.systemIP;

    const res = await getSettingInfo();
    form.theme = res.data.theme;
    form.menuTabs = res.data.menuTabs;
    form.menuAccordion = res.data.menuAccordion || 'Disable';
    menuAccordion.value = form.menuAccordion === 'Enable';
    form.panelName = res.data.panelName;
    form.language = res.data.language;
    form.sessionTimeout = Number(res.data.sessionTimeout || 0);
    form.docSource = res.data.docSource || 'withByRegion';
    form.edition = res.data.edition;

    form.proxyUrl = res.data.proxyUrl;
    form.proxyType = res.data.proxyType;
    form.proxyPort = res.data.proxyPort;
    form.proxyShow = form.proxyUrl ? form.proxyUrl + ':' + form.proxyPort : unset.value;
    form.proxyUser = res.data.proxyUser;
    form.proxyPasswd = res.data.proxyPasswd;
    form.proxyPasswdKeep = res.data.proxyPasswdKeep;

    form.developerMode = res.data.developerMode;
    form.hideMenu = res.data.hideMenu;

    const [xpackRes, proxyDockerRes] = await Promise.all([
        getXpackSetting(true),
        getXpackProxyDocker().catch(() => null),
    ]);
    if (xpackRes) {
        form.theme = xpackRes.data.theme || themeConfig.value.theme || 'light';
        form.themeColor = JSON.parse(xpackRes.data.themeColor || '{"light":"#005eeb","dark":"#F0BE96"}');
        themeConfig.value.themeColor = xpackRes.data.themeColor
            ? xpackRes.data.themeColor
            : '{"light":"#005eeb","dark":"#F0BE96"}';
        themeConfig.value.theme = form.theme;
        form.watermark = xpackRes.data.watermark;
        form.watermarkShow = xpackRes.data.watermarkShow;
        form.title = xpackRes.data.title || '';
        form.masterAlias = xpackRes.data.masterAlias || '';
        form.loginBgType = xpackRes.data.loginBgType || '';
        form.loginBackground = xpackRes.data.loginBackground || '';
        form.loginBtnLinkColor = xpackRes.data.loginBtnLinkColor || '';
        form.logo = xpackRes.data.logo || '';
        form.logoWithText = xpackRes.data.logoWithText || '';
        form.favicon = xpackRes.data.favicon || '';
        form.loginImage = xpackRes.data.loginImage || '';
        try {
            watermark.value = JSON.parse(xpackRes.data.watermark);
        } catch {
            watermark.value = null;
        }
    }
    form.proxyDocker = proxyDockerRes?.data?.proxyDocker || '';
};

const onChangeTitle = () => {
    panelNameRef.value.acceptParams({ panelName: form.panelName });
};
const onChangeTimeout = () => {
    timeoutRef.value.acceptParams({ sessionTimeout: form.sessionTimeout });
};
const onChangeSystemIP = () => {
    systemIPRef.value.acceptParams({ systemIP: form.systemIP });
};
const onChangeProxy = () => {
    proxyRef.value.acceptParams({
        url: form.proxyUrl,
        type: form.proxyType,
        port: form.proxyPort,
        user: form.proxyUser,
        passwd: form.proxyPasswd,
        passwdKeep: form.proxyPasswdKeep,
        proxyDocker: form.proxyDocker,
    });
};

const onChangeHideMenus = () => {
    hideMenuRef.value.acceptParams({
        hideMenu: form.hideMenu,
        menuAccordion: form.menuAccordion,
    });
};

const onChangeRegion = () => {
    editionRef.value.acceptParams({ edition: form.edition, docSource: form.docSource });
};

const runtimeEnvLabel = () => {
    const editionLabel = form.edition === 'cn' ? i18n.global.t('setting.cn') : i18n.global.t('setting.intl');
    const docSourceLabel = i18n.global.t(`setting.${form.docSource || 'withByRegion'}`);
    return `${editionLabel} / ${docSourceLabel}`;
};

const onChangeThemeColor = () => {
    const themeColor: ThemeColor = JSON.parse(themeConfig.value.themeColor);
    themeColorRef.value.acceptParams({ themeColor: themeColor, theme: themeConfig.value.theme });
};

const onChangeWatermark = async () => {
    if (form.watermarkShow === 'Enable') {
        watermarkRef.value.acceptParams(form.watermark);
        return;
    }
    ElMessageBox.confirm(i18n.global.t('setting.watermarkCloseHelper'), i18n.global.t('setting.watermark'), {
        confirmButtonText: i18n.global.t('commons.button.confirm'),
        cancelButtonText: i18n.global.t('commons.button.cancel'),
    })
        .then(async () => {
            loading.value = true;
            await updateXpackSettingByKey('WatermarkShow', 'Disable')
                .then(() => {
                    loading.value = false;
                    watermark.value = null;
                    watermarkShow.value = false;
                    search();
                    MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
                })
                .catch(() => {
                    loading.value = false;
                });
        })
        .catch(() => {
            form.watermarkShow = 'Enable';
        });
};

const handleThemeChange = async (val: string) => {
    localStorage.removeItem(codeEditorThemeStorageKey);
    themeConfig.value.theme = val;
    switchTheme();
    await updateXpackSettingByKey('Theme', val);
    let color: string;
    const themeColor: ThemeColor = JSON.parse(themeConfig.value.themeColor);
    if (val === 'auto') {
        const prefersDark = window.matchMedia('(prefers-color-scheme: dark)');
        color = prefersDark.matches ? themeColor.dark : themeColor.light;
    } else {
        color = val === 'dark' ? themeColor.dark : themeColor.light;
    }
    themeConfig.value.primary = color;
    setPrimaryColor(color);
};
const onSaveBranding = async (key: string, val: string) => {
    loading.value = true;
    try {
        await updateXpackSettingByKey(key, (val ?? '') + '');
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } catch (error) {
        await search();
    } finally {
        loading.value = false;
    }
};

// Branding images are presence-only on the client: the value is a sentinel and
// the bytes are fetched from the fixed /api/v2/images/<key> path. previewTick
// busts the cache after an upload or reset.
const previewTick = ref(Date.now());

const brandAssets = [
    {
        key: 'logo',
        label: 'setting.brandLogo',
        accept: 'image/png,image/jpeg,image/gif,image/webp',
        maxKB: 2048,
        helper: 'setting.brandImageHelper',
    },
    {
        key: 'logoWithText',
        label: 'setting.brandLogoWithText',
        accept: 'image/png,image/jpeg,image/gif,image/webp',
        maxKB: 2048,
        helper: 'setting.brandImageHelper',
    },
    {
        key: 'favicon',
        label: 'setting.brandFavicon',
        accept: 'image/png',
        maxKB: 256,
        helper: 'setting.brandFaviconHelper',
    },
    {
        key: 'loginImage',
        label: 'setting.brandLoginImage',
        accept: 'image/png,image/jpeg,image/gif,image/webp',
        maxKB: 2048,
        helper: 'setting.brandImageHelper',
    },
];

const assetValue = (key: string): string => {
    switch (key) {
        case 'logo':
            return form.logo;
        case 'logoWithText':
            return form.logoWithText;
        case 'favicon':
            return form.favicon;
        case 'loginImage':
            return form.loginImage;
        case 'loginBackground':
            return form.loginBackground === 'loginBackground' ? form.loginBackground : '';
        default:
            return '';
    }
};

const assetPreviewUrl = (key: string) => `/api/v2/images/${key}?t=${previewTick.value}`;

const beforeAssetUpload = (key: string, file: File) => {
    const accept = key === 'favicon' ? 'image/png' : 'image/png,image/jpeg,image/gif,image/webp';
    if (!accept.split(',').includes(file.type)) {
        MsgError(i18n.global.t('setting.brandImageBadType'));
        return false;
    }
    const maxKB = key === 'favicon' ? 256 : 2048;
    if (file.size > maxKB * 1024) {
        MsgError(i18n.global.t('setting.brandImageTooLarge'));
        return false;
    }
    return true;
};

const onUploadAsset = async (key: string, file: File) => {
    loading.value = true;
    try {
        await uploadOpenEnhancementAsset(key, file);
        await search();
        previewTick.value = Date.now();
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } finally {
        loading.value = false;
    }
};

const onResetAsset = async (key: string) => {
    loading.value = true;
    try {
        await resetOpenEnhancementAsset(key);
        await search();
        previewTick.value = Date.now();
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
    } finally {
        loading.value = false;
    }
};

const onSave = async (key: string, val: any) => {
    loading.value = true;
    let param = {
        key: key,
        value: val + '',
    };
    try {
        if (key === 'Theme') {
            await handleThemeChange(val);
        } else {
            await updateSetting(param);
        }
        switch (key) {
            case 'MenuTabs':
                openMenuTabs.value = val === 'Enable';
                break;
            case 'MenuAccordion':
                menuAccordion.value = val === 'Enable';
                break;
            case 'Language':
                await globalStore.updateLanguage(val);
                location.reload();
                break;
        }
        MsgSuccess(i18n.global.t('commons.msg.operationSuccess'));
        search();
    } catch (error) {
        loading.value = false;
        return false;
    }
    loading.value = false;
    return true;
};

onMounted(() => {
    search();
    getSystemAvailable();
});
</script>

<style scoped lang="scss">
:deep(.el-radio-group) {
    min-width: max-content;
}
.brand-asset {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
}
.brand-asset__preview {
    max-width: 120px;
    max-height: 40px;
    object-fit: contain;
    border: 1px solid var(--el-border-color);
    border-radius: 4px;
    padding: 2px;
    background: var(--el-fill-color-light);
}
</style>
