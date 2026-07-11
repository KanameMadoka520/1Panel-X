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
