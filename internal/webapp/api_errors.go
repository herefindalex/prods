package webapp

import (
	"log/slog"
	"net/http"
	"strings"

	"prods/internal/localization"
)

const (
	apiCodeConflict             = "conflict"
	apiCodeCategoryCycle        = "category_cycle"
	apiCodeDisabledReference    = "disabled_reference"
	apiCodeForbidden            = "forbidden"
	apiCodeImportConflict       = "import_conflict"
	apiCodeInvalidImportMapping = "invalid_import_mapping"
	apiCodeInvalidSiteRoute     = "invalid_site_route"
	apiCodeInternal             = "internal_error"
	apiCodeInvalidCSRF          = "invalid_csrf"
	apiCodeInvalidJSON          = "invalid_json"
	apiCodeJobNotRetryable      = "job_not_retryable"
	apiCodeLastActiveOwner      = "last_active_owner"
	apiCodeNotFound             = "not_found"
	apiCodeRequestTooLarge      = "request_too_large"
	apiCodeResourceUnavailable  = "resource_unavailable"
	apiCodeRevisionConflict     = "revision_conflict"
	apiCodeServiceUnavailable   = "service_unavailable"
	apiCodeBrandCaptureDenied   = "brand_capture_source_access_denied"
	apiCodeSMTPNotConfigured    = "smtp_not_configured"
	apiCodeUnauthorized         = "unauthorized"
	apiCodeUnsupportedMediaType = "unsupported_media_type"
	apiCodeUseHideAction        = "use_hide_action"
	apiCodeValidationFailed     = "validation_failed"
)

type apiErrorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

var apiErrorMessages = map[string]map[string]string{
	"en-US": {
		apiCodeConflict:             "The request conflicts with the current state.",
		apiCodeCategoryCycle:        "This move would create a category cycle.",
		apiCodeDisabledReference:    "The selected reference is disabled. Enable it or choose another value.",
		apiCodeForbidden:            "You do not have permission to perform this action.",
		apiCodeImportConflict:       "The import preview no longer matches current data. Create a new preview and try again.",
		apiCodeInvalidImportMapping: "Check the import column mapping and try again.",
		apiCodeInvalidSiteRoute:     "The website route configuration is invalid. Review the preview and fix the conflicts.",
		apiCodeInternal:             "An internal error occurred.",
		apiCodeInvalidCSRF:          "The security token is invalid or expired. Refresh the page and try again.",
		apiCodeInvalidJSON:          "The request data is invalid.",
		apiCodeJobNotRetryable:      "This job cannot be retried safely. Start a new operation instead.",
		apiCodeLastActiveOwner:      "The last active Owner cannot be disabled or demoted.",
		apiCodeNotFound:             "The requested resource was not found.",
		apiCodeRequestTooLarge:      "The uploaded file is too large.",
		apiCodeResourceUnavailable:  "The required storage resource is unavailable; no data was accepted.",
		apiCodeRevisionConflict:     "This record changed since it was loaded. Refresh and try again.",
		apiCodeServiceUnavailable:   "This service is temporarily unavailable.",
		apiCodeBrandCaptureDenied:   "The source website rejected automated access. Enter the brand details manually or try an authorized static page.",
		apiCodeSMTPNotConfigured:    "SMTP is not configured. Configure email delivery or use the manual sharing workflow.",
		apiCodeUnauthorized:         "Your session is missing or expired. Sign in again.",
		apiCodeUnsupportedMediaType: "The uploaded file type is not supported.",
		apiCodeUseHideAction:        "Use the Hide action so public access is revoked synchronously.",
		apiCodeValidationFailed:     "Check the entered values and try again.",
	},
	"zh-TW": {
		apiCodeConflict:             "此請求與目前狀態衝突。",
		apiCodeCategoryCycle:        "此移動會造成分類循環。",
		apiCodeDisabledReference:    "選取的參照已停用，請先啟用或改選其他項目。",
		apiCodeForbidden:            "你沒有執行此操作的權限。",
		apiCodeImportConflict:       "匯入預覽已不符合目前資料，請重新建立預覽後再試。",
		apiCodeInvalidImportMapping: "請檢查匯入欄位對應後再試。",
		apiCodeInvalidSiteRoute:     "網站路由設定無效，請查看預覽並修正衝突。",
		apiCodeInternal:             "系統發生內部錯誤。",
		apiCodeInvalidCSRF:          "安全權杖無效或已過期，請重新整理頁面後再試。",
		apiCodeInvalidJSON:          "請求資料格式無效。",
		apiCodeJobNotRetryable:      "此工作無法安全重試，請改為開始新的操作。",
		apiCodeLastActiveOwner:      "不能停用或降級最後一位有效的 Owner。",
		apiCodeNotFound:             "找不到要求的資源。",
		apiCodeRequestTooLarge:      "上傳的檔案太大。",
		apiCodeResourceUnavailable:  "必要的儲存資源目前無法使用，系統未接受任何資料。",
		apiCodeRevisionConflict:     "此資料在載入後已被變更，請重新整理後再試。",
		apiCodeServiceUnavailable:   "此服務目前暫時無法使用。",
		apiCodeBrandCaptureDenied:   "來源網站拒絕自動讀取。請改用人工填寫品牌資料，或嘗試已授權的靜態頁面。",
		apiCodeSMTPNotConfigured:    "尚未設定 SMTP，請設定郵件傳送或使用手動分享流程。",
		apiCodeUnauthorized:         "登入工作階段不存在或已過期，請重新登入。",
		apiCodeUnsupportedMediaType: "不支援此上傳檔案類型。",
		apiCodeUseHideAction:        "請使用「隱藏」操作，以同步撤銷公開存取。",
		apiCodeValidationFailed:     "請檢查輸入內容後再試。",
	},
}

func (s *Server) adminAPILocale(r *http.Request) string {
	if s.store == nil {
		return "en-US"
	}
	settings, err := s.store.SiteSettings(r.Context())
	if err != nil {
		return "en-US"
	}
	locale, err := localization.Resolve(
		settings.DefaultLocale,
		settings.SupportedLocales,
		"",
		r.Header.Get("Accept-Language"),
	)
	if err != nil || !localization.IsAvailable(locale) {
		return "en-US"
	}
	return locale
}

func apiErrorForLocale(locale, code string) apiErrorResponse {
	message := apiErrorMessages[locale][code]
	if strings.TrimSpace(message) == "" {
		message = apiErrorMessages["en-US"][apiCodeInternal]
		code = apiCodeInternal
	}
	return apiErrorResponse{Code: code, Error: message}
}

func (s *Server) localizedAPIError(w http.ResponseWriter, r *http.Request, code string) apiErrorResponse {
	locale := s.adminAPILocale(r)
	w.Header().Set("Content-Language", locale)
	w.Header().Set("Vary", "Accept-Language")
	return apiErrorForLocale(locale, code)
}

func (s *Server) writeAPIError(w http.ResponseWriter, r *http.Request, status int, code string) {
	writeJSON(w, status, s.localizedAPIError(w, r, code))
}

func (s *Server) internalAPIError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "error", err)
	s.writeAPIError(w, r, http.StatusInternalServerError, apiCodeInternal)
}
