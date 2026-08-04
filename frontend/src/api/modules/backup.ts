import http from '@/api';
import { deepCopy } from '@/utils/misc';
import { encodeBase64Fields } from '@/utils/base64';
import { ResPage } from '../interface';
import { Backup } from '../interface/backup';
import { TimeoutEnum } from '@/enums/http-enum';
import { GlobalStore } from '@/store';

// backup-agent
export const getLocalBackupDir = (node?: string) => {
    const params = node ? `?operateNode=${node}` : '';
    return http.get<string>(`/backups/local${params}`);
};
export const searchBackup = (params: Backup.SearchWithType) => {
    return http.post<ResPage<Backup.BackupInfo>>(`/backups/search`, params);
};
export const checkBackup = (params: Backup.BackupOperate) => {
    let request = deepCopy(params) as Backup.BackupOperate;
    encodeBase64Fields(request, ['accessKey', 'credential']);
    return http.post<Backup.CheckResult>(`/backups/conn/check`, request);
};
export const listBucket = (params: Backup.ForBucket) => {
    const globalStore = GlobalStore();
    let request = deepCopy(params) as Backup.BackupOperate;
    encodeBase64Fields(request, ['accessKey', 'credential']);
    if (!params.isPublic || !globalStore.isProductPro) {
        return http.postLocalNode('/backups/buckets', request, TimeoutEnum.T_40S);
    }
    return http.post('/backups/buckets', request, TimeoutEnum.T_40S);
};
export const handleBackup = (params: Backup.Backup, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/backup${query}`, params, TimeoutEnum.T_10M);
};
export const listBackupOptions = () => {
    return http.get<Array<Backup.BackupOption>>(`/backups/options`);
};
export const handleRecover = (params: Backup.Recover, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/recover${query}`, params, TimeoutEnum.T_10M);
};
export const handleRecoverByUpload = (params: Backup.Recover, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/recover/byupload${query}`, params, TimeoutEnum.T_10M);
};
export const downloadBackupRecord = (params: Backup.RecordDownload, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post<string>(`/backups/record/download${query}`, params, TimeoutEnum.T_10M);
};
export const deleteBackupRecord = (params: { ids: number[] }, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/record/del${query}`, params);
};
export const updateRecordDescription = (id: number, description: string, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/record/description/update${query}`, { id: id, description: description });
};
export const uploadByRecover = (filePath: string, targetDir: string, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post(`/backups/upload${query}`, { filePath: filePath, targetDir: targetDir });
};
export const searchBackupRecords = (params: Backup.SearchBackupRecord, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post<ResPage<Backup.RecordInfo>>(`/backups/record/search${query}`, params, TimeoutEnum.T_5M);
};
export const loadRecordSize = (param: Backup.SearchForSize, node?: string) => {
    const query = node ? `?operateNode=${node}` : '';
    return http.post<Array<Backup.RecordFileSize>>(`/backups/record/size${query}`, param);
};
export const searchBackupRecordsByCronjob = (params: Backup.SearchBackupRecordByCronjob) => {
    return http.post<ResPage<Backup.RecordInfo>>(`/backups/record/search/bycronjob`, params, TimeoutEnum.T_5M);
};
export const getFilesFromBackup = (id: number) => {
    return http.post<Array<any>>(`/backups/search/files`, { id: id });
};

// backup-core
export const refreshToken = (params: { id: number; name: string; isPublic: boolean }) => {
    if (!params.isPublic) {
        return http.post('/backups/refresh/token', { id: params.id });
    }
    return http.post<Backup.BackupSyncOperationResult>('/core/backups/refresh/token', { name: params.name });
};
export const listBackupSyncStatuses = () => {
    return http.get<Array<Backup.BackupSyncStatus>>('/core/backups/sync/status');
};
export const retryBackupSync = (name: string) => {
    return http.post<Backup.BackupSyncOperationResult>('/core/backups/sync/retry', { name });
};
export const beginOAuth = (params: Backup.OAuthBegin) => {
    const url = params.isPublic ? '/core/backups/oauth/begin' : '/backups/oauth/begin';
    return http.post<Backup.OAuthBeginResponse>(url, params, TimeoutEnum.T_60S);
};
export const completeOAuth = (params: Backup.OAuthComplete, isPublic: boolean) => {
    const url = isPublic ? '/core/backups/oauth/complete' : '/backups/oauth/complete';
    return http.post<Backup.OAuthCompleteResponse>(url, params, TimeoutEnum.T_60S);
};
export const clearOAuth = (params: { id: number; name: string; isPublic: boolean }) => {
    if (params.isPublic) {
        return http.post<Backup.BackupSyncOperationResult>('/core/backups/oauth/credential/clear', {
            name: params.name,
        });
    }
    return http.post('/backups/oauth/credential/clear', { id: params.id });
};
export const addBackup = (params: Backup.BackupOperate) => {
    let request = deepCopy(params) as Backup.BackupOperate;
    encodeBase64Fields(request, ['accessKey', 'credential']);
    if (!params.isPublic) {
        return http.post('/backups', request, TimeoutEnum.T_60S);
    }
    return http.post<Backup.BackupSyncOperationResult>('/core/backups', request, TimeoutEnum.T_60S);
};
export const editBackup = (params: Backup.BackupOperate) => {
    let request = deepCopy(params) as Backup.BackupOperate;
    encodeBase64Fields(request, ['accessKey', 'credential']);
    if (!params.isPublic) {
        return http.post('/backups/update', request, TimeoutEnum.T_60S);
    }
    return http.post<Backup.BackupSyncOperationResult>('/core/backups/update', request, TimeoutEnum.T_60S);
};
export const deleteBackup = (params: { id: number; name: string; isPublic: boolean }) => {
    if (!params.isPublic) {
        return http.post('/backups/del', { id: params.id });
    }
    return http.post<Backup.BackupSyncOperationResult>('/core/backups/del', { name: params.name });
};
