import { CommonModel, ReqPage } from '.';

export namespace Website {
    export interface Website extends CommonModel {
        primaryDomain: string;
        type: string;
        alias: string;
        remark: string;
        domains: string[];
        appType: string;
        appInstallId?: number;
        webSiteGroupId: number;
        otherDomains: string;
        defaultServer: boolean;
        protocol: string;
        autoRenew: boolean;
        appinstall?: NewAppInstall;
        webSiteSSL: SSL;
        runtimeID: number;
        rewrite: string;
        user: string;
        group: string;
        IPV6: boolean;
        accessLog?: boolean;
        errorLog?: boolean;
        childSites?: string[];
        dbID: number;
        dbType: string;
        favorite: boolean;
        streamPorts: string;
        udp: boolean;
    }

    export interface WebsiteDTO extends Website {
        errorLogPath: string;
        accessLogPath: string;
        sitePath: string;
        appName: string;
        runtimeName: string;
        runtimeType: string;
        openBaseDir: boolean;
        algorithm: string;
        servers: NginxUpstreamServer[];
    }

    export interface WebsiteStreamUpdate {
        websiteID: number;
        algorithm: string;
        streamPorts?: string;
        servers: NginxUpstreamServer[];
    }

    export interface WebsiteRes extends CommonModel {
        protocol: string;
        primaryDomain: string;
        type: string;
        alias: string;
        remark: string;
        status: string;
        expireDate: string;
        sitePath: string;
        appName: string;
        runtimeName: string;
        sslExpireDate: Date;
    }

    export interface NewAppInstall {
        name: string;
        appDetailId: number;
        params: any;
    }

    export interface WebSiteSearch extends ReqPage {
        name: string;
        orderBy: string;
        order: string;
        websiteGroupId: number;
    }

    export interface WebSiteDel {
        id: number;
        deleteApp: boolean;
        deleteBackup: boolean;
        forceDelete: boolean;
    }

    export interface WebSiteCreateReq {
        type: string;
        alias: string;
        remark: string;
        appType: string;
        appInstallId: number;
        webSiteGroupId: number;
        proxy: string;
        proxyType: string;
        ftpUser: string;
        ftpPassword: string;
        taskID: string;
        SSLID?: number;
        enableSSL: boolean;
        createDB?: boolean;
        dbName?: string;
        dbPassword?: string;
        dbFormat?: string;
        dbUser?: string;
        dbHost?: string;
        domains: SubDomain[];
        streamPorts?: string;
        name: string;
        algorithm: string;
        servers: NginxUpstreamServer[];
    }

    export interface WebSiteUpdateReq {
        id: number;
        primaryDomain: string;
        remark: string;
        webSiteGroupId: number;
        expireDate?: string;
        IPV6: boolean;
        favorite: boolean;
    }

    export interface WebSiteOp {
        id: number;
        operate: string;
    }

    export interface WebSiteOpLog {
        id: number;
        operate: string;
        logType: string;
    }

    export interface WebSiteLogReq {
        id: number;
        page?: number;
        pageSize?: number;
        logType: string;
    }

    export interface OptionReq {
        types?: string[];
    }

    export interface WebsiteOption {
        id: number;
        primaryDomain: string;
        alias: string;
    }

    export interface WebSiteLog {
        enable: boolean;
        content: string;
        end: boolean;
        path: string;
    }

    export interface Domain {
        websiteId: number;
        port: number;
        id: number;
        domain: string;
        ssl: boolean;
    }

    export interface DomainCreate {
        websiteID: number;
        domains: SubDomain[];
    }

    export interface DomainUpdate {
        id: number;
        ssl: boolean;
    }

    interface SubDomain {
        domain: string;
        port: number;
        ssl: boolean;
    }

    export interface DomainDelete {
        id: number;
    }

    export interface NginxConfigReq {
        operate: string;
        websiteId: number;
        scope: string;
        params?: any;
    }

    export interface NginxScopeReq {
        websiteId: number;
        scope: string;
    }

    export interface NginxParam {
        name: string;
        params: string[];
    }

    export interface NginxScopeConfig {
        enable: boolean;
        params: NginxParam[];
    }

    export interface DnsAccount extends CommonModel {
        name: string;
        type: string;
        authorization: object;
    }

    export interface DnsAccountCreate {
        name: string;
        type: string;
        authorization: object;
    }

    export interface DnsAccountUpdate {
        id: number;
        name: string;
        type: string;
        authorization: object;
    }

    export interface SSL extends CommonModel {
        primaryDomain: string;
        privateKey: string;
        pem: string;
        otherDomains: string;
        certURL: string;
        type: string;
        issuerName: string;
        expireDate: string;
        startDate: string;
        provider: string;
        websites?: Website.Website[];
        autoRenew: boolean;
        acmeAccountId: number;
        status: string;
        domains: string;
        description: string;
        dnsAccountId?: number;
        pushDir: boolean;
        dir: string;
        keyType: string;
        nameserver1: string;
        nameserver2: string;
        disableCNAME: boolean;
        skipDNS: boolean;
        execShell: boolean;
        shell: string;
        pushNode: boolean;
        nodes: string;
        privateKeyPath: string;
        certPath: string;
        isIP: boolean;
    }

    export interface SSLDTO extends SSL {
        logPath: string;
    }

    export interface SSLCreate {
        primaryDomain: string;
        otherDomains: string;
        provider: string;
        acmeAccountId: number;
        dnsAccountId: number;
        id?: number;
        description: string;
        isIP: boolean;
    }

    export interface SSLApply {
        websiteId: number;
        SSLId: number;
    }

    export interface SSLRenew {
        SSLId: number;
    }

    export interface SSLUpdate {
        id: number;
        autoRenew: boolean;
        description: string;
        primaryDomain: string;
        otherDomains: string;
        acmeAccountId: number;
        provider: string;
        dnsAccountId?: number;
        keyType: string;
        pushDir: boolean;
        dir: string;
        pushNode?: boolean;
        nodes?: string;
    }

    export interface SSLPush {
        id: number;
        pushNode: boolean;
        nodes: string;
        taskID: string;
    }

    export interface AcmeAccount extends CommonModel {
        email: string;
        url: string;
        type: string;
        useProxy: boolean;
    }

    export interface AcmeAccountCreate {
        email: string;
        useProxy: boolean;
    }

    export interface AcmeAccountUpdate {
        id: number;
        useProxy: boolean;
    }

    export interface DNSResolveReq {
        acmeAccountId: number;
        websiteSSLId: number;
    }

    export interface DNSResolve {
        resolve: string;
        value: string;
        domain: string;
        err: string;
    }

    export interface SSLReq {
        name?: string;
        acmeAccountID?: string;
    }

    export interface HTTPSReq {
        websiteId: number;
        enable: boolean;
        websiteSSLId?: number;
        type: string;
        certificate?: string;
        privateKey?: string;
        httpConfig: string;
        SSLProtocol: string[];
        algorithm: string;
        http3: boolean;
    }

    export interface HTTPSConfig {
        enable: boolean;
        SSL: SSL;
        httpConfig: string;
        SSLProtocol: string[];
        algorithm: string;
        hsts: boolean;
        hstsIncludeSubDomains: boolean;
        httpsPort?: string;
        http3: boolean;
    }

    export interface BatchSetHttps {
        ids: number[];
        taskID: string;
        enable: boolean;
        websiteSSLId?: number;
        type: string;
        certificate?: string;
        privateKey?: string;
        httpConfig: string;
        SSLProtocol: string[];
        algorithm: string;
        http3: boolean;
    }

    export interface CheckReq {
        installIds?: number[];
    }

    export interface CheckRes {
        name: string;
        status: string;
        version: string;
        appName: string;
    }

    export interface DelReq {
        id: number;
    }

    export interface NginxUpdate {
        id: number;
        content: string;
    }

    export interface DefaultServerUpdate {
        id: number;
    }
    export interface RewriteReq {
        websiteID: number;
        name: string;
    }

    export interface RewriteRes {
        content: string;
    }

    export interface RewriteUpdate {
        websiteID: number;
        name: string;
        content: string;
    }

    export interface CustomRewrite {
        operate: string;
        name: string;
        content: string;
    }

    export interface DirUpdate {
        id: number;
        siteDir: string;
    }

    export interface DirPermissionUpdate {
        id: number;
        user: string;
        group: string;
    }

    export interface ProxyReq {
        id: number;
    }

    export interface ProxyConfig {
        id: number;
        operate: string;
        enable: boolean;
        cache: boolean;
        cacheTime: number;
        cacheUnit: string;
        serverCacheTime: number;
        serverCacheUnit: string;
        name: string;
        modifier: string;
        match: string;
        proxyPass: string;
        proxyHost: string;
        filePath?: string;
        replaces?: ProxReplace;
        content?: string;
        proxyAddress?: string;
        proxyProtocol?: string;
        sni?: boolean;
        proxySSLName: string;
        sslVerify?: boolean;
        cors: boolean;
        allowOrigins: string;
        allowMethods: string;
        allowHeaders: string;
        allowCredentials: boolean;
        preflight: boolean;
        browserCache?: 'enable' | 'disable' | 'noModify';
    }

    export interface ProxyDel {
        id: number;
        name: string;
    }

    export interface ProxyStatusUpdate {
        id: number;
        name: string;
        status: string;
    }

    export interface ProxReplace {
        [key: string]: string;
    }

    export interface ProxyFileUpdate {
        websiteID: number;
        name: string;
        content: string;
    }

    export interface AuthReq {
        websiteID: number;
    }

    export interface NginxAuth {
        username: string;
        remark: string;
    }

    export interface AuthConfig {
        enable: boolean;
        items: NginxAuth[];
    }

    export interface NginxAuthConfig {
        websiteID: number;
        operate: string;
        username: string;
        password: string;
        remark: string;
        scope: string;
        path?: '';
        name?: '';
    }

    export interface NginxPathAuthConfig {
        websiteID: number;
        operate: string;
        path: string;
        username: string;
        password: string;
        name: string;
    }

    export interface LeechConfig {
        enable: boolean;
        cache: boolean;
        cacheTime: number;
        cacheUint: string;
        extends: string;
        return: string;
        serverNames: string[];
        noneRef: boolean;
        logEnable: boolean;
        blocked: boolean;
        websiteID?: number;
    }

    export interface LeechReq {
        websiteID: number;
    }

    export interface WebsiteReq {
        websiteID: number;
    }

    export interface RedirectConfig {
        operate: string;
        websiteID: number;
        domains?: string[];
        enable: boolean;
        name: string;
        keepPath: boolean;
        type: string;
        redirect: string;
        path?: string;
        target: string;
        redirectRoot?: boolean;
        filePath?: string;
        content?: string;
    }

    export interface RedirectFileUpdate {
        websiteID: number;
        name: string;
        content: string;
    }

    export interface PHPVersionChange {
        websiteID: number;
        runtimeID: number;
    }

    export interface DirConfig {
        dirs: string[];
        user: string;
        userGroup: string;
        msg: string;
    }

    export interface SSLUpload {
        privateKey: string;
        certificate: string;
        privateKeyPath: string;
        certificatePath: string;
        type: string;
        sslID: number;
        description?: string;
        pushNode?: boolean;
        nodes?: string;
    }

    export interface SSLObtain {
        ID: number;
    }

    export interface CA extends CommonModel {
        name: string;
        csr: string;
        privateKey: string;
        keyType: string;
    }

    export interface CACreate {
        name: string;
        commonName: string;
        country: string;
        organization: string;
        organizationUint: string;
        keyType: string;
        province: string;
        city: string;
    }

    export interface CADTO extends CA {
        commonName: string;
        country: string;
        organization: string;
        organizationUint: string;
        province: string;
        city: string;
    }

    export interface SSLObtainByCA {
        id: number;
        domains: string;
        keyType: string;
        time: number;
        unit: string;
        pushDir: boolean;
        dir: string;
        description: string;
        pushNode?: boolean;
        nodes?: string;
    }

    export interface RenewSSLByCA {
        SSLID: number;
    }

    export interface SSLDownload {
        id: number;
    }

    export interface WebsiteHtml {
        content: string;
    }
    export interface WebsiteHtmlUpdate {
        type: string;
        content: string;
        sync: boolean;
    }

    export interface NginxUpstream {
        name: string;
        algorithm: string;
        servers: NginxUpstreamServer[];
        content?: string;
        websiteID?: number;
    }

    export interface NginxUpstreamFile {
        name: string;
        content: string;
        websiteID: number;
    }

    export interface LoadBalanceReq {
        websiteID: number;
        name: string;
        algorithm: string;
        servers: NginxUpstreamServer[];
    }

    interface NginxUpstreamServer {
        server: string;
        weight: number;
        failTimeout: number;
        failTimeoutUnit: string;
        maxFails: number;
        maxConns: number;
        flag: string;
    }

    export interface LoadBalanceDel {
        websiteID: number;
        name: string;
    }

    export interface WebsiteLBUpdateFile {
        websiteID: number;
        name: string;
        content: string;
    }

    export interface WebsiteCacheConfig {
        open: boolean;
        cacheLimit: number;
        cacheLimitUnit: string;
        shareCache: number;
        shareCacheUnit: string;
        cacheExpire: number;
        cacheExpireUnit: string;
    }

    export interface WebsiteRealIPConfig {
        open: boolean;
        ipFrom: string;
        ipHeader: string;
        ipOther: string;
    }

    export interface WebsiteResource {
        name: string;
        type: string;
        resourceID: number;
        detail: any;
    }

    export interface WebsiteDatabase {
        type: string;
        databaseID: number;
        websiteID: number;
        from: string;
        databaseName: number;
    }

    export interface ChangeDatabase {
        websiteID: number;
        databaseID: number;
        databaseType: string;
    }

    export interface CrossSiteAccessOp {
        websiteID: number;
        operation: string;
    }

    export interface ExecComposer {
        websiteID: number;
        command: string;
        extCommand?: string;
        mirror: string;
        dir: string;
        user: string;
        taskID: string;
    }

    export interface BatchOperate {
        ids: number[];
        operate: string;
        taskID: string;
    }

    export interface CorsConfig {
        cors: boolean;
        allowOrigins: string;
        allowMethods: string;
        allowHeaders: string;
        allowCredentials: boolean;
        preflight: boolean;
    }

    export interface CorsConfigReq extends CorsConfig {
        websiteID: number;
    }

    export interface BatchSetGroup {
        ids: number[];
        groupID: number;
    }

    export interface AccessStatReq {
        startTime?: string;
        endTime?: string;
    }

    export interface AccessStat {
        websiteID: number;
        time: string;
        pv: number;
        uv: number;
        bytes: number;
        status2xx: number;
        status3xx: number;
        status4xx: number;
        status5xx: number;
    }

    export interface AccessRankReq {
        startTime?: string;
        endTime?: string;
        kind: string;
        top?: number;
    }

    export interface AccessRank {
        kind: string;
        key: string;
        count: number;
    }

    export type WafRateLimitKind = 'access' | 'url' | 'notfound' | 'attack';

    export interface WafRateLimit {
        kind: WafRateLimitKind;
        periodSec: number;
        threshold: number;
        // 0 means the limit is recorded but does not ban.
        banSec: number;
        perUrl: boolean;
    }

    export interface WafSiteStatus {
        websiteID: number;
        supported: boolean;
        enabled: boolean;
        mode: 'detection' | 'block' | 'inherit';
        effectiveMode: 'detection' | 'block';
        allowList: string[];
        denyList: string[];
        // rateLimits are this site's own overrides; effectiveRateLimits are what
        // the gateway actually enforces once the global defaults are merged in.
        rateLimits: WafRateLimit[];
        effectiveRateLimits: WafRateLimit[];
        // rules is this site's own detection policy (null when it follows the
        // panel default); effectiveRules is what the gateway actually enforces.
        rules: WafRulePolicy | null;
        effectiveRules: WafRulePolicy;
        // region follows the same whole-or-nothing rule as rules.
        region: WafRegionPolicy | null;
        effectiveRegion: WafRegionPolicy;
        // geoAvailable is false when the IP address database region control
        // needs is not installed, so the UI can say the control is unavailable
        // rather than offer a switch that cannot take effect.
        geoAvailable: boolean;
        // realIp is how this site recovers the client address from a CDN;
        // cdnHeaders is the list the header-list mode actually reads, returned
        // by the server so the UI shows exactly that.
        realIp: WafRealIP;
        cdnHeaders: string[];
        installed: boolean;
        ready: boolean;
        routed: boolean;
        protected: boolean;
        lastError: string;
    }

    export interface WafSiteUpdate {
        enabled: boolean;
        mode: 'detection' | 'block' | 'inherit';
        allowList: string[];
        denyList: string[];
        rateLimits: WafRateLimit[];
        // Present makes it this site's own policy, replacing the panel default
        // wholesale; null keeps the site following the panel default.
        rules: WafRulePolicy | null;
        region: WafRegionPolicy | null;
        realIp: WafRealIP | null;
    }

    // The X-Forwarded-For modes are expressed as "how many proxy hops upstream",
    // never as "the first value": that header is appended to by each proxy, so
    // its leftmost entry is whatever the original caller wrote.
    export interface WafRealIP {
        mode: string;
        header: string;
    }

    export interface WafRulePolicy {
        // Stored as "disable" flags so the default object is the fully-protecting one.
        disableSqli: boolean;
        disableXss: boolean;
        strict: boolean;
        // Empty leaves the rule set's own method default in force.
        allowedMethods: string[];
    }

    // One upload restriction rule. Matching is FUZZY: the rule hits anywhere in
    // the uploaded file name.
    export interface WafUploadRule {
        id: number;
        rule: string;
        remark: string;
        enabled: boolean;
    }

    // The master switch plus the rules it governs, returned together because a
    // rule list means nothing without knowing whether the control is armed.
    export interface WafUploadRules {
        enabled: boolean;
        rules: WafUploadRule[];
    }

    // Geographic access control. Regions are ISO 3166-1 alpha-2 country codes;
    // an empty list means no region control at all, whatever mode says.
    export interface WafRegionPolicy {
        mode: string;
        regions: string[];
    }

    // A custom rule's conditions are ANDed; the rule list order is the
    // evaluation order, so it must never be re-sorted for display.
    export interface WafRuleCondition {
        field: string;
        name?: string;
        match?: string;
        pattern: string;
        negate?: boolean;
    }

    export interface WafCustomRule {
        id: number;
        name: string;
        action: string;
        conditions: WafRuleCondition[];
        remark: string;
        enabled: boolean;
        // Set when the stored conditions could not be read. Such a rule is shown
        // as broken rather than as one that matches nothing.
        invalid?: string;
    }

    export interface WafBan {
        ip: string;
        kind: string;
        host: string;
        websiteId: number;
        bannedAt: string;
        expiresAt: string;
    }

    export interface WafBanState {
        bans: WafBan[];
        trackedCounters: number;
        counterOverflow: boolean;
    }

    export type WafListName = 'deny' | 'allow';
    export type WafListTarget = 'ip' | 'ipgroup' | 'url' | 'ua';
    export type WafListMatch = 'exact' | 'prefix' | 'contains' | 'regex' | '';

    export interface WafListEntry {
        id: number;
        list: WafListName;
        target: WafListTarget;
        match: WafListMatch;
        pattern: string;
        remark: string;
        enabled: boolean;
    }

    export interface WafIPGroup {
        id: number;
        name: string;
        entries: string[];
        remark: string;
    }

    export interface WafLists {
        entries: WafListEntry[];
        ipGroups: WafIPGroup[];
    }

    export interface WafGlobalConfig {
        defaultMode: 'detection' | 'block';
        allowList: string[];
        denyList: string[];
        rateLimits: WafRateLimit[];
        rules: WafRulePolicy;
        region: WafRegionPolicy;
        geoAvailable: boolean;
        blockPage: WafBlockPage;
        log: WafLogSettings;
        // The closed set of record kinds the panel may offer for exclusion,
        // returned by the server so the two can never drift into offering a kind
        // the data plane does not know.
        recordKinds: string[];
    }

    // The status set is closed: a 5xx would blame the origin for a decision the
    // WAF made, and a 3xx would turn a refusal into a redirect.
    export interface WafBlockPage {
        status: number;
        html: string;
    }

    export interface WafLogSettings {
        retentionDays: number;
        excludedKinds: string[];
        // Caps the data plane's record file in MB. 0 keeps the built-in ceiling.
        maxMb: number;
    }

    export interface WafEventReq {
        startTime?: string;
        endTime?: string;
        category?: string;
        page: number;
        pageSize: number;
    }

    export interface WafEvent {
        id: number;
        websiteID: number;
        time: string;
        host: string;
        sourceIP: string;
        method: string;
        uri: string;
        ruleID: number;
        ruleMsg: string;
        category: string;
        severity: string;
        matchedData: string;
        hitCount: number;
        action: string;
    }
}
