<template>
    <div class="flex items-center justify-center min-h-screen relative bg-gray-100">
        <div class="absolute inset-0 bg-cover bg-center bg-no-repeat" :style="backgroundStyle"></div>
        <div
            :style="{ opacity: backgroundOpacity, width: containerWidth, height: containerHeight }"
            class="bg-white shadow-lg relative z-10 border border-gray-200 flex overflow-hidden"
            id="login-container"
        >
            <div class="grid items-stretch w-full" :style="loginGridStyle">
                <div v-if="showLogo" class="flex justify-center" :style="{ height: containerHeight }">
                    <img
                        v-show="imgLoaded"
                        :src="loadImage('loginImage')"
                        class="max-w-full max-h-full object-cover bg-cover bg-center"
                        alt="1panel"
                        @load="onImgLoad"
                        @error="onImgError"
                    />
                </div>
                <div :class="loginFormClass">
                    <LoginForm ref="loginRef"></LoginForm>
                </div>
            </div>
        </div>
    </div>
</template>

<script setup lang="ts">
import LoginForm from './components/login-form.vue';
import { ref, onMounted } from 'vue';
import { useGlobalStore } from '@/composables/useGlobalStore';
import { preloadImage } from '@/utils/browser';
defineOptions({ name: 'Login' });
const { entrance, isEnterprise, themeConfig } = useGlobalStore();
const backgroundOpacity = ref(1);
const defaultLoginImage = new URL('@/assets/images/1panel-login.jpg', import.meta.url).href;
const defaultEnterpriseLoginImage = new URL('@/assets/images/1panel-login-enterprise.png', import.meta.url).href;
const defaultLoginBgImage = new URL('@/assets/images/1panel-login-bg.jpg', import.meta.url).href;
const loadedLoginImage = ref<string | null>(null);
const loadedBackgroundImage = ref<string | null>(null);
const backgroundStyle = ref<{ backgroundImage?: string; backgroundColor?: string }>({});
const imgLoaded = ref(false);
const currentDefaultLoginImage = computed(() => (isEnterprise.value ? defaultEnterpriseLoginImage : defaultLoginImage));

function onImgLoad() {
    imgLoaded.value = true;
}
const mySafetyCode = defineProps({
    code: {
        type: String,
        default: '',
    },
});

const getStatus = async () => {
    let code = mySafetyCode.code;
    if (code != '') {
        entrance.value = code;
    }
};

const loadImage = (name: string) => {
    const { loginImage, loginBackground, loginBgType } = themeConfig.value;
    if (name === 'loginImage') {
        if (loginImage === 'loginImage') {
            return loadedLoginImage.value || currentDefaultLoginImage.value;
        }
        if (loginImage) {
            return loginImage;
        }
        return currentDefaultLoginImage.value;
    }
    if (name === 'loginBackground') {
        if (loginBgType === 'image') {
            if (loginBackground === 'loginBackground') {
                return loadedBackgroundImage.value || defaultLoginBgImage;
            }
            if (loginBackground) {
                return loginBackground;
            }
            return defaultLoginBgImage;
        }
        if (loginBgType === 'color') {
            return loginBackground;
        }
        return defaultLoginBgImage;
    }
    return '';
};

const onImgError = (event: any) => {
    event.target.src = currentDefaultLoginImage.value;
    imgLoaded.value = true;
};

// Apply the custom login image / background. Runs reactively (watch below)
// because themeConfig is populated asynchronously from the public settings
// AFTER mount; a one-shot onMounted read would miss the uploaded assets and
// fall back to the defaults.
// brandingRun guards against overlapping reactive runs: a stale async callback
// (image preload / img.onload) from an earlier run must not clobber the result
// of the latest run (e.g. a slow default-bg onload overwriting a configured
// solid color).
let brandingRun = 0;
const applyLoginBranding = async () => {
    const run = ++brandingRun;
    const { loginImage, loginBgType, loginBackground } = themeConfig.value;
    if (loginImage === 'loginImage') {
        const src = await preloadImage(`/api/v2/images/loginImage?t=${Date.now()}`);
        if (run === brandingRun) loadedLoginImage.value = src;
    }
    if (loginBgType === 'image' && loginBackground === 'loginBackground') {
        const src = await preloadImage(`/api/v2/images/loginBackground?t=${Date.now()}`);
        if (run === brandingRun) loadedBackgroundImage.value = src;
    }
    if (loginBgType === 'color') {
        backgroundStyle.value = { backgroundColor: loginBackground };
    } else {
        const url = loadImage('loginBackground');
        const img = new Image();
        img.onload = () => {
            if (run === brandingRun) backgroundStyle.value = { backgroundImage: `url(${url})` };
        };
        img.onerror = () => {
            if (run === brandingRun) backgroundStyle.value = { backgroundImage: `url(${defaultLoginBgImage})` };
        };
        img.src = url;
    }
};

onMounted(async () => {
    await getStatus();
});

watch(
    () => [themeConfig.value.loginImage, themeConfig.value.loginBgType, themeConfig.value.loginBackground],
    () => applyLoginBranding(),
    { immediate: true },
);

const FIXED_WIDTH = 1000;
const FIXED_HEIGHT = 415;
const useWindowSize = () => {
    const width = ref(window.innerWidth);
    const height = ref(window.innerHeight);

    const updateSize = () => {
        width.value = window.innerWidth;
        height.value = window.innerHeight;
    };

    onMounted(() => window.addEventListener('resize', updateSize));
    onUnmounted(() => window.removeEventListener('resize', updateSize));

    return { width, height };
};
const { width } = useWindowSize();
const showLogo = computed(() => width.value >= FIXED_WIDTH);
const containerWidth = computed(() => `${FIXED_WIDTH}px`);
const containerHeight = computed(() => `${FIXED_HEIGHT}px`);
const loginGridStyle = computed(() => ({
    gridTemplateColumns: showLogo.value ? 'repeat(2, minmax(0, 1fr))' : 'minmax(0, 1fr)',
}));
const loginFormClass = computed(() => {
    return showLogo.value
        ? 'flex items-center justify-center p-4 min-w-0'
        : 'flex items-center justify-center p-4 w-full min-w-0';
});
</script>
