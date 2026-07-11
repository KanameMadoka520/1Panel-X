import http from '@/api';

export const getPublicOpenEnhancementSetting = () => {
    return http.get('/core/settings/enhancements/public');
};

export const getOpenEnhancementSetting = () => {
    return http.get('/core/settings/enhancements/search');
};

export const updateOpenEnhancementSetting = (key: string, value: string) => {
    return http.post('/core/settings/enhancements/update', { key, value });
};

export const uploadOpenEnhancementAsset = (key: string, file: File) => {
    const formData = new FormData();
    formData.append('key', key);
    formData.append('file', file);
    return http.upload('/core/settings/enhancements/asset', formData);
};

export const resetOpenEnhancementAsset = (key: string) => {
    return http.post('/core/settings/enhancements/asset/reset', { key });
};
