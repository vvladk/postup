package i18n

type Translations map[string]string

var locales = map[string]Translations{
	"uk": {
		"nav.retros":    "Ретроспективи",
		"nav.teams":     "Команди",
		"nav.users":     "Користувачі",
		"nav.templates": "Шаблони",
		"nav.settings":  "Налаштування",
		"nav.timer":     "⏱ Таймер",
		"nav.logout":    "Вийти",

		"settings.title":        "Налаштування",
		"settings.system":       "Системні налаштування",
		"settings.system_desc":  "Впливають на всіх учасників",
		"settings.personal":     "Особисті налаштування",
		"settings.ip":           "Зовнішній IP",
		"settings.ip_hint":      "Залишіть порожнім — буде визначено автоматично",
		"settings.port":         "Порт",
		"settings.lang":         "Мова інтерфейсу",
		"settings.theme":        "Тема оформлення",
		"settings.theme_dark":   "🌙 Темна",
		"settings.theme_light":  "☀ Світла",
		"settings.save_system":  "Зберегти системні",
		"settings.save_personal": "Зберегти особисті",
		"settings.saved":        "Збережено",
		"settings.lang_uk":      "Українська",
		"settings.lang_en":      "English",

		"btn.save":   "Зберегти",
		"btn.cancel": "Скасувати",
		"btn.delete": "Видалити",
		"btn.add":    "Додати",

		"err.required":  "Поле обовʼязкове",
		"err.too_short": "Занадто коротко",
		"err.forbidden": "Доступ заборонено",

		"empty.retros":    "Ще немає ретроспектив",
		"empty.teams":     "Ще немає команд",
		"empty.users":     "Ще немає користувачів",
		"empty.templates": "Ще немає шаблонів",
	},
	"en": {
		"nav.retros":    "Retrospectives",
		"nav.teams":     "Teams",
		"nav.users":     "Users",
		"nav.templates": "Templates",
		"nav.settings":  "Settings",
		"nav.timer":     "⏱ Timer",
		"nav.logout":    "Logout",

		"settings.title":        "Settings",
		"settings.system":       "System settings",
		"settings.system_desc":  "Affect all participants",
		"settings.personal":     "Personal settings",
		"settings.ip":           "External IP",
		"settings.ip_hint":      "Leave blank — will be auto-detected",
		"settings.port":         "Port",
		"settings.lang":         "Interface language",
		"settings.theme":        "Theme",
		"settings.theme_dark":   "🌙 Dark",
		"settings.theme_light":  "☀ Light",
		"settings.save_system":  "Save system settings",
		"settings.save_personal": "Save personal settings",
		"settings.saved":        "Saved",
		"settings.lang_uk":      "Ukrainian",
		"settings.lang_en":      "English",

		"btn.save":   "Save",
		"btn.cancel": "Cancel",
		"btn.delete": "Delete",
		"btn.add":    "Add",

		"err.required":  "Field is required",
		"err.too_short": "Too short",
		"err.forbidden": "Access denied",

		"empty.retros":    "No retrospectives yet",
		"empty.teams":     "No teams yet",
		"empty.users":     "No users yet",
		"empty.templates": "No templates yet",
	},
}

func T(lang, key string) string {
	if t, ok := locales[lang]; ok {
		if v, ok := t[key]; ok {
			return v
		}
	}
	return key
}

func Available() []string {
	return []string{"uk", "en"}
}
