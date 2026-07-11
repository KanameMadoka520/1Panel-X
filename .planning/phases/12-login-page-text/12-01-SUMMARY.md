# Phase 12 Summary: Open Login-Page Text (+ login-hero fix)

**Requirement:** LOGIN-TEXT-01
**Date:** 2026-07-11
**Status:** Implemented + automatically verified; adversarial review recorded; live login render human UAT pending.

## What shipped

- **Backend** (`core`): added `LoginWelcome`, `LoginSubtitle`, `Copyright` to `EnhancementSettingInfo` + `PublicEnhancementSettingInfo`; extended `EnhancementSettingUpdate.oneof`. Validator reuses `validateBrandingText` — `LoginWelcome`/`LoginSubtitle` ≤128 runes, `Copyright` ≤200; reject `<>` + control chars; empty = unset; fail-closed default preserved. Both getters populate the three.
- **Login render** (`login/components/login-form.vue`): welcome (`text-2xl`) + subtitle (`text-sm`) block at the top of the login card, copyright footer at the bottom — all `{{ }}` interpolation, **no `v-html`**. `themeConfig` carries the values.
- **themeConfig population**: `store/interface/index.ts` type + `store/modules/global.ts` default gained the three; `utils/xpack.ts` (`getXpackSettingForTheme`) + `global/use-logo.ts` set them from the public response.
- **Community form** (`setting/panel/index.vue`): three `el-input` (maxlength 128/128/200) wired to `onSaveBranding`; loaded in `search()`.
- **i18n** (`en.ts` + `zh.ts`): `loginWelcomeLabel/loginSubtitleLabel/copyrightLabel/copyrightHelper`.
- **`LOGIN-HERO-RENDER` fix** (`login/index.vue`): replaced the one-shot `onMounted` preload of loginImage/loginBackground with a reactive `watch(() => [themeConfig.loginImage, themeConfig.loginBgType, themeConfig.loginBackground], applyLoginBranding, { immediate: true })`, so the uploaded login image/background swap in once the async public settings populate. This closes the gap found in the v1.3 live VPS UAT.

## Tests
Extended `enhancement_test.go`: the subset test now includes `loginWelcome/loginSubtitle/copyright` (present in both anon + authed, still excludes watermark/paths); the validator table adds accept (empty, valid, CJK) and reject (markup `<b>`, control char `\n`, over-length copyright). Focused + full core `go test ./...` pass; gofmt/vet clean. Frontend: changed-file ESLint clean (0 errors); `npm run build:pro` passes.

## Deferred
Rich/multi-line/HTML login text; v1.5+ secure multi-node.
