package localization

import (
	"strings"

	"golang.org/x/text/language"
)

type InstallerMessages struct {
	Language                    string
	ClaimTitle                  string
	ClaimInstructions           string
	BootstrapToken              string
	ClaimInstallation           string
	ClaimedTitle                string
	ClaimedInstructions         string
	SetupTitle                  string
	SetupInstructions           string
	OwnerEmail                  string
	OwnerDisplayName            string
	Password                    string
	ConfirmPassword             string
	PasswordRequirements        string
	DefaultLocale               string
	SupportedLocales            string
	SiteTimeZone                string
	CompleteInstallation        string
	CompleteTitle               string
	CompleteInstructions        string
	InvalidBootstrapToken       string
	InvalidInstallerSession     string
	InvalidCSRF                 string
	InvalidOwnerEmail           string
	PasswordMismatch            string
	InvalidPassword             string
	InvalidDefaultLocale        string
	InvalidSupportedLocale      string
	DefaultMissingFromSupported string
	InvalidTimeZone             string
	DataDirectoryNotWritable    string
	BackupDirectoryNotWritable  string
}

var installerEnglish = InstallerMessages{
	Language: "Language", ClaimTitle: "Claim Prods installation",
	ClaimInstructions: "Use the bootstrap token shown in the Prods process console to claim this installation.",
	BootstrapToken:    "Bootstrap token", ClaimInstallation: "Claim installation",
	ClaimedTitle:        "Installer already claimed",
	ClaimedInstructions: "Another browser already owns the installation session. Finish there, or restart Prods to invalidate that session and issue a new bootstrap token.",
	SetupTitle:          "Set up Prods",
	SetupInstructions:   "Create the first Owner and minimum site settings. Product data, website configuration, SMTP, and public URL settings remain post-install work.",
	OwnerEmail:          "Owner email", OwnerDisplayName: "Owner display name", Password: "Password", ConfirmPassword: "Confirm password",
	PasswordRequirements: "Password minimum: 8 characters with at least one letter and one number.",
	DefaultLocale:        "Default locale", SupportedLocales: "Supported locales (comma-separated)", SiteTimeZone: "Site time zone",
	CompleteInstallation: "Complete installation", CompleteTitle: "Installation complete",
	CompleteInstructions: "Installation settings were saved atomically. Restart Prods to enter Normal mode.", InvalidBootstrapToken: "Invalid bootstrap token.",
	InvalidInstallerSession: "Installer session is not valid.", InvalidCSRF: "Invalid CSRF token.",
	InvalidOwnerEmail: "Enter a valid Owner email address.", PasswordMismatch: "Password confirmation does not match.",
	InvalidPassword:      "Password must contain at least 8 characters, including a letter and a number.",
	InvalidDefaultLocale: "Default locale is not valid.", InvalidSupportedLocale: "A supported locale is not valid.",
	DefaultMissingFromSupported: "Supported locales must include the default locale.", InvalidTimeZone: "Site time zone is not valid.",
	DataDirectoryNotWritable: "Data directory is not writable", BackupDirectoryNotWritable: "Backup directory is not writable",
}

var installerTraditionalChinese = InstallerMessages{
	Language: "介面語言", ClaimTitle: "認領 Prods 安裝程序",
	ClaimInstructions: "請使用 Prods 程序主控台顯示的 bootstrap token 認領此安裝程序。",
	BootstrapToken:    "Bootstrap token", ClaimInstallation: "認領安裝程序",
	ClaimedTitle:        "安裝程序已被認領",
	ClaimedInstructions: "另一個瀏覽器已持有安裝 session。請在該處完成，或重新啟動 Prods 使 session 失效並取得新的 bootstrap token。",
	SetupTitle:          "設定 Prods",
	SetupInstructions:   "建立第一位 Owner 與最低限度的站點設定。產品資料、網站設定、SMTP 與公開 URL 可在安裝後處理。",
	OwnerEmail:          "Owner 電子郵件", OwnerDisplayName: "Owner 顯示名稱", Password: "密碼", ConfirmPassword: "確認密碼",
	PasswordRequirements: "密碼至少 8 個字元，並包含至少一個英文字母與一個數字。",
	DefaultLocale:        "預設語系", SupportedLocales: "支援語系（以逗號分隔）", SiteTimeZone: "站點時區",
	CompleteInstallation: "完成安裝", CompleteTitle: "安裝完成",
	CompleteInstructions: "安裝設定已原子保存。請重新啟動 Prods 以進入 Normal 模式。", InvalidBootstrapToken: "Bootstrap token 無效。",
	InvalidInstallerSession: "安裝 session 無效。", InvalidCSRF: "CSRF token 無效。",
	InvalidOwnerEmail: "請輸入有效的 Owner 電子郵件。", PasswordMismatch: "兩次輸入的密碼不一致。",
	InvalidPassword:      "密碼至少需要 8 個字元，並包含英文字母與數字。",
	InvalidDefaultLocale: "預設語系無效。", InvalidSupportedLocale: "支援語系中包含無效值。",
	DefaultMissingFromSupported: "支援語系必須包含預設語系。", InvalidTimeZone: "站點時區無效。",
	DataDirectoryNotWritable: "資料目錄無法寫入", BackupDirectoryNotWritable: "備份目錄無法寫入",
}

func InstallerFor(locale string) InstallerMessages {
	tag, err := language.Parse(strings.TrimSpace(locale))
	if err == nil {
		base, _ := tag.Base()
		if base.String() == "zh" {
			return installerTraditionalChinese
		}
	}
	return installerEnglish
}
