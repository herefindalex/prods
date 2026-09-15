package localization

type AuthText struct {
	AdminTitle              string
	Catalog                 string
	RequestPart             string
	Footer                  string
	TemporaryToken          string
	Email                   string
	Password                string
	SignIn                  string
	POCHelp                 string
	InvalidTemporaryToken   string
	TooManyAttempts         string
	InvalidCredentials      string
	SetPasswordTitle        string
	PasswordSaved           string
	CanNow                  string
	SignInLink              string
	NewPassword             string
	ConfirmPassword         string
	PasswordRequirements    string
	SetPassword             string
	PasswordMismatch        string
	InvalidPassword         string
	InvalidSetPasswordToken string
}

var authCatalog = map[string]AuthText{
	"en-US": {
		AdminTitle:              "Prods Admin",
		Catalog:                 "Catalog",
		RequestPart:             "Request a part",
		Footer:                  "Product catalog and request for quotation",
		TemporaryToken:          "Temporary PoC admin token",
		Email:                   "Email",
		Password:                "Password",
		SignIn:                  "Sign in",
		POCHelp:                 "This temporary token is available only in the explicitly enabled isolated PoC mode.",
		InvalidTemporaryToken:   "Invalid temporary token.",
		TooManyAttempts:         "Too many sign-in attempts. Try again shortly.",
		InvalidCredentials:      "Invalid email or password.",
		SetPasswordTitle:        "Set your Prods password",
		PasswordSaved:           "Password saved for",
		CanNow:                  "You can now",
		SignInLink:              "sign in",
		NewPassword:             "New password",
		ConfirmPassword:         "Confirm password",
		PasswordRequirements:    "Use at least 8 characters with at least one letter and one number.",
		SetPassword:             "Set password",
		PasswordMismatch:        "Passwords do not match.",
		InvalidPassword:         "Use at least 8 characters with at least one letter and one number.",
		InvalidSetPasswordToken: "This set-password link is invalid, expired, or already used. Ask an Owner for a new link.",
	},
	"zh-TW": {
		AdminTitle:              "Prods 管理後台",
		Catalog:                 "產品型錄",
		RequestPart:             "提出詢價",
		Footer:                  "產品型錄與詢價系統",
		TemporaryToken:          "暫時性 POC 管理員權杖",
		Email:                   "電子郵件",
		Password:                "密碼",
		SignIn:                  "登入",
		POCHelp:                 "此暫時性權杖只適用於明確啟用的隔離 POC 模式。",
		InvalidTemporaryToken:   "暫時性權杖無效。",
		TooManyAttempts:         "登入嘗試次數過多，請稍後再試。",
		InvalidCredentials:      "電子郵件或密碼不正確。",
		SetPasswordTitle:        "設定 Prods 密碼",
		PasswordSaved:           "已為以下帳號儲存密碼：",
		CanNow:                  "你現在可以",
		SignInLink:              "登入",
		NewPassword:             "新密碼",
		ConfirmPassword:         "確認密碼",
		PasswordRequirements:    "至少 8 個字元，且至少包含一個英文字母與一個數字。",
		SetPassword:             "設定密碼",
		PasswordMismatch:        "兩次輸入的密碼不一致。",
		InvalidPassword:         "密碼至少需要 8 個字元，且至少包含一個英文字母與一個數字。",
		InvalidSetPasswordToken: "此密碼設定連結無效、已過期或已使用。請向 Owner 索取新的連結。",
	},
}

func AuthFor(locale string) AuthText {
	if text, ok := authCatalog[locale]; ok {
		return text
	}
	return authCatalog["en-US"]
}
