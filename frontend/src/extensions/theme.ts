type XpackThemeModule = {
    setPrimaryColor?: (color: string) => any;
};

function findModule<T>(modules: Record<string, T>, suffix: string): T | null {
    for (const path in modules) {
        if (path.endsWith(suffix)) {
            return modules[path];
        }
    }
    return null;
}

function setOpenPrimaryColor(color: string) {
    if (typeof document === 'undefined') {
        return;
    }
    const root = document.documentElement;
    const mixTarget = root.classList.contains('dark') ? '#141414' : '#ffffff';
    root.style.setProperty('--panel-color-primary', color);
    root.style.setProperty('--el-color-primary', color);
    root.style.setProperty('--el-color-primary-dark-2', `color-mix(in srgb, ${color} 80%, #000000)`);
    for (let index = 1; index <= 9; index += 1) {
        const primaryWeight = 100 - index * 10;
        const mixed = `color-mix(in srgb, ${color} ${primaryWeight}%, ${mixTarget})`;
        root.style.setProperty(`--panel-color-primary-light-${index}`, mixed);
        root.style.setProperty(`--el-color-primary-light-${index}`, mixed);
    }
}

export function loadXpackStyles() {
    const xpackModules = import.meta.glob('@/xpack/styles/index.scss');
    const xpackLoader = findModule(xpackModules, '/styles/index.scss');
    xpackLoader?.();

    const enterpriseModules = import.meta.glob('@/enterprise/styles/index.scss');
    const enterpriseLoader = findModule(enterpriseModules, '/styles/index.scss');
    enterpriseLoader?.();
}

export function setXpackPrimaryColor(color: string) {
    setOpenPrimaryColor(color);
    const xpackModules = import.meta.glob('@/xpack/utils/theme/tool.ts', { eager: true }) as Record<
        string,
        XpackThemeModule
    >;
    const xpackModule = findModule(xpackModules, '/utils/theme/tool.ts');
    xpackModule?.setPrimaryColor?.(color);

    const enterpriseModules = import.meta.glob('@/enterprise/utils/theme/tool.ts', { eager: true }) as Record<
        string,
        XpackThemeModule
    >;
    const enterpriseModule = findModule(enterpriseModules, '/utils/theme/tool.ts');
    enterpriseModule?.setPrimaryColor?.(color);
}

export const loadExtensionStyles = loadXpackStyles;
export const setExtensionPrimaryColor = setXpackPrimaryColor;
