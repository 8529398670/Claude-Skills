package routes

import (
	time "time"

)

// The named shapes the API docs share: components/schemas in
// config/openapi.json. See server/apidoc for what the struct tags mean.

// >>> feature:api_keys
type apiKeySchema struct {
	ID string `json:"id"`
	UserID int `json:"user_id"`
	Name string `json:"name"`
	Role string `json:"role" enum:"admin|user"`
	CreatedAt time.Time `json:"created_at"`
	Revoked bool `json:"revoked"`
	Expired bool `json:"expired"`
	Live bool `json:"live"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	LastUsedAt time.Time `json:"last_used_at,omitempty"`
	RevokedAt time.Time `json:"revoked_at,omitempty"`
	DisplayName string `json:"display_name,omitempty" doc:"The key's owner. On the admin list only."`
	CreatedBy int `json:"created_by,omitempty" doc:"The admin who made this key for its owner. Absent if the owner made it."`
	CreatedByName string `json:"created_by_name,omitempty"`
}

func ( apiKeySchema ) APIName() ( name string ) { name = "APIKey"; return }
// <<< feature:api_keys

// >>> feature:invite_links
type inviteLinkSchema struct {
	ID string `json:"id"`
	Label string `json:"label"`
	MaxUses int `json:"max_uses"`
	Used int `json:"used"`
	Remaining int `json:"remaining"`
	Live bool `json:"live"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Revoked bool `json:"revoked"`
	CreatedBy int `json:"created_by"`
	CreatedByName string `json:"created_by_name,omitempty"`
}

func ( inviteLinkSchema ) APIName() ( name string ) { name = "InviteLink"; return }
// <<< feature:invite_links

// >>> feature:jobs
type jobSchema struct {
	Name string `json:"name"`
	IntervalSeconds int `json:"interval_seconds"`
	Running bool `json:"running"`
	Interrupted bool `json:"interrupted"`
	Runs int `json:"runs"`
	NextDue time.Time `json:"next_due"`
	LastStarted time.Time `json:"last_started,omitempty"`
	LastFinished time.Time `json:"last_finished,omitempty"`
	LastOk bool `json:"last_ok,omitempty"`
	LastNote string `json:"last_note,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

func ( jobSchema ) APIName() ( name string ) { name = "Job"; return }
// <<< feature:jobs

// >>> feature:invite_links
type joinPathSchema struct {
	JoinPath string `json:"join_path" doc:"The link to hand out, shown in this response only. A credential: treat it like a password."`
}

func ( joinPathSchema ) APIName() ( name string ) { name = "JoinPath"; return }
// <<< feature:invite_links

// >>> feature:invite_links
type joinedSchemaShape1Doc struct {
	Ok bool `json:"ok"`
	DisplayName string `json:"display_name"`
}
// <<< feature:invite_links

// >>> feature:invite_links
type joinedSchemaShape2Doc struct {
	TwoFactor bool `json:"two_factor"`
	Challenge string `json:"challenge"`
}
// <<< feature:invite_links

// >>> feature:invite_links
type joinedSchema struct{}

func ( joinedSchema ) APIName() ( name string ) { name = "Joined"; return }

func ( joinedSchema ) APIOneOf() ( shapes []any ) {
	shapes = []any{ joinedSchemaShape1Doc{} , joinedSchemaShape2Doc{} }
	return
}

func ( joinedSchema ) APIDoc() ( text string ) { text = "Either the sign-in finished, or a two-factor code is needed."; return }
// <<< feature:invite_links

// >>> feature:logs
type logDaySchema struct {
	Date string `json:"date"`
	Entries []map[string]any `json:"entries"`
	Next *int `json:"next,omitempty"`
}

func ( logDaySchema ) APIName() ( name string ) { name = "LogDay"; return }
// <<< feature:logs

// >>> feature:logs
type logDaysSchema struct {
	Days []map[string]any `json:"days"`
	TotalBytes int `json:"total_bytes,omitempty"`
	Dropped int `json:"dropped,omitempty"`
	Settings map[string]any `json:"settings,omitempty"`
}

func ( logDaysSchema ) APIName() ( name string ) { name = "LogDays"; return }
// <<< feature:logs

type meSchema struct {
	Authenticated bool `json:"authenticated"`
	ID int `json:"id"`
	DisplayName string `json:"display_name"`
	Role string `json:"role" doc:"The effective role: never higher than an API key's own."`
	AccountRole string `json:"account_role"`
	IsOwner bool `json:"is_owner"`
	Maintenance string `json:"maintenance" enum:"none|read_only|full"` // feature:maintenance
	HasPIN bool `json:"has_pin,omitempty"` // feature:invite_links
	Username string `json:"username,omitempty"` // feature:passwords
	HasPassword bool `json:"has_password,omitempty"` // feature:passwords
	PasswordBlocked bool `json:"password_blocked,omitempty"` // feature:passwords
	TwoFactor bool `json:"two_factor,omitempty"` // feature:two_factor
	PasswordsEnabled bool `json:"passwords_enabled,omitempty"` // feature:passwords
	TwoFactorEnabled bool `json:"two_factor_enabled,omitempty"` // feature:two_factor
	Auth string `json:"auth,omitempty"`
	APIKeysEnabled bool `json:"api_keys_enabled,omitempty"` // feature:api_keys
	MCPEnabled bool `json:"mcp_enabled,omitempty" doc:"server.mcp: whether POST /mcp answers."` // feature:mcp
	FilesEnabled bool `json:"files_enabled,omitempty" doc:"files.enabled: whether the Files screen and routes are on."` // feature:files
	InviteLinksEnabled bool `json:"invite_links_enabled,omitempty" doc:"Whether invite links are on."` // feature:invite_links
	WebhooksEnabled bool `json:"webhooks_enabled,omitempty" doc:"webhooks.enabled: whether webhooks are sent."` // feature:webhooks
	LogsEnabled bool `json:"logs_enabled,omitempty"` // feature:logs
	LogsUIClicks bool `json:"logs_ui_clicks,omitempty"` // feature:logs
	CSRFToken string `json:"csrf_token,omitempty" doc:"Browser sessions only."`
	APIKeyID string `json:"api_key_id,omitempty" doc:"API keys only."`
	APIKeyName string `json:"api_key_name,omitempty" doc:"API keys only."`
	Version string `json:"version,omitempty" doc:"The running build. dev when none was stamped."`
	Announcement string `json:"announcement,omitempty" doc:"server.announcement. Empty when there is none."`
	Repo string `json:"repo,omitempty" doc:"app.repo without a trailing / or .git: where the source lives. Empty when it is not set."`
	Description string `json:"description,omitempty" doc:"app.description. Empty when there is none."`
	ShowAbout bool `json:"show_about,omitempty" doc:"app.show_about: whether the menu has About."` // feature:about
	OpenapiURL string `json:"openapi_url,omitempty" doc:"Where to read this API's reference: config/openapi.json in app.repo, or /api/openapi.json when there is no repo."` // feature:openapi
	MCPURL string `json:"mcp_url,omitempty" doc:"Where to read the MCP reference: config/mcp.json in app.repo, or /api/mcp.json when there is no repo. Only with server.mcp on."` // feature:mcp
	Shortcuts map[string]string `json:"shortcuts,omitempty" doc:"The keyboard shortcuts you changed, by action. Mod is Cmd on a Mac and Ctrl elsewhere."` // feature:shortcuts
	Themes *[]map[string]any `json:"themes,omitempty" doc:"Your themes (see the Theme schema): your own list, else the app's defaults, else null for the built-in ones."` // feature:appearance
	ThemesCustom bool `json:"themes_custom,omitempty" doc:"Whether you have your own list rather than following the defaults."` // feature:appearance
	ThemeID string `json:"theme_id,omitempty" doc:"The theme you use. Empty: the first."` // feature:appearance
	LoginLink *loginLinkSchema `json:"login_link,omitempty" doc:"Your own live login link, or null."`
}

func ( meSchema ) APIName() ( name string ) { name = "Me"; return }

type memberSchema struct {
	ID int `json:"id"`
	DisplayName string `json:"display_name"`
}

func ( memberSchema ) APIName() ( name string ) { name = "Member"; return }

// >>> feature:api_keys
type newAPIKeySchema struct {
	apiKeySchema
	Credential string `json:"credential" doc:"Shown in this response only, and never recoverable."`
}

func ( newAPIKeySchema ) APIName() ( name string ) { name = "NewAPIKey"; return }
// <<< feature:api_keys

// >>> feature:logs
type pruneResultSchema struct {
	Compressed int `json:"compressed,omitempty"`
	Deleted int `json:"deleted,omitempty"`
	BytesBefore int `json:"bytes_before,omitempty"`
	BytesAfter int `json:"bytes_after,omitempty"`
}

func ( pruneResultSchema ) APIName() ( name string ) { name = "PruneResult"; return }
// <<< feature:logs

type revokedSchema struct {
	Sessions int `json:"sessions,omitempty"`
	LoginLinks int `json:"login_links,omitempty"`
	APIKeys int `json:"api_keys,omitempty"`
	Passwords int `json:"passwords,omitempty"`
}

func ( revokedSchema ) APIName() ( name string ) { name = "Revoked"; return }

// >>> feature:sessions
type sessionSchema struct {
	ID string `json:"id"`
	Current bool `json:"current"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty"`
	IP string `json:"ip,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
	Label string `json:"label,omitempty" doc:"The name you gave this device."`
	IPName string `json:"ip_name,omitempty" doc:"What the owner calls this address on the Logs screen. The owner's own list only."` // feature:logs
}

func ( sessionSchema ) APIName() ( name string ) { name = "Session"; return }
// <<< feature:sessions

// >>> feature:settings
type settingsResultSchema struct {
	Changes []map[string]any `json:"changes,omitempty" nullable:"true"`
	Applied []string `json:"applied,omitempty" nullable:"true"`
	Restart []string `json:"restart,omitempty" nullable:"true"`
	Scripts []string `json:"scripts,omitempty" nullable:"true"`
	Unchanged []string `json:"unchanged,omitempty" nullable:"true"`
	Version string `json:"version,omitempty"`
}

func ( settingsResultSchema ) APIName() ( name string ) { name = "SettingsResult"; return }
// <<< feature:settings

// >>> feature:settings
type settingsSnapshotSchema struct {
	File string `json:"file,omitempty"`
	SecretFile string `json:"secret_file,omitempty"`
	Version string `json:"version,omitempty" doc:"Send it back when saving: a changed file is refused with a 409."`
	Writable bool `json:"writable,omitempty"`
	Stale bool `json:"stale,omitempty"`
	InContainer bool `json:"in_container,omitempty"`
	Error *settingsSnapshotSchemaErrorDoc `json:"error,omitempty"`
	Sections []settingsSnapshotSchemaSectionsItemDoc `json:"sections,omitempty"`
}

func ( settingsSnapshotSchema ) APIName() ( name string ) { name = "SettingsSnapshot"; return }
// <<< feature:settings

// >>> feature:settings
type settingsSnapshotSchemaErrorDoc struct {
	Message string `json:"message,omitempty"`
	At time.Time `json:"at,omitempty"`
	Origin string `json:"origin,omitempty"`
}
// <<< feature:settings

// >>> feature:settings
type settingsSnapshotSchemaSectionsItemDoc struct {
	Name string `json:"name,omitempty"`
	Help string `json:"help,omitempty"`
	Keys []settingsSnapshotSchemaSectionsItemDocKeysItem `json:"keys,omitempty"`
}
// <<< feature:settings

// >>> feature:settings
type settingsSnapshotSchemaSectionsItemDocKeysItem struct {
	Key string `json:"key,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
	Value any `json:"value,omitempty"`
	Default any `json:"default,omitempty"`
	InFile bool `json:"in_file,omitempty"`
	Help string `json:"help,omitempty"`
	Apply string `json:"apply,omitempty"`
	PendingRestart bool `json:"pending_restart,omitempty"`
	Running any `json:"running,omitempty"`
	Locked bool `json:"locked,omitempty"`
}
// <<< feature:settings

type signInSchemaShape1Doc struct {
	Ok bool `json:"ok"`
}

type signInSchemaShape2Doc struct {
	TwoFactor bool `json:"two_factor"`
	Challenge string `json:"challenge"`
}

type signInSchema struct{}

func ( signInSchema ) APIName() ( name string ) { name = "SignIn"; return }

func ( signInSchema ) APIOneOf() ( shapes []any ) {
	shapes = []any{ signInSchemaShape1Doc{} , signInSchemaShape2Doc{} }
	return
}

func ( signInSchema ) APIDoc() ( text string ) { text = "Either the sign-in finished, or a two-factor code is needed: send it with the challenge to /api/login/two-factor."; return }

type userSchema struct {
	ID int `json:"id"`
	DisplayName string `json:"display_name"`
	Username string `json:"username,omitempty" doc:"Empty until a password is set."` // feature:passwords
	Role string `json:"role" enum:"admin|user"`
	CreatedAt time.Time `json:"created_at,omitempty"`
	Disabled bool `json:"disabled"`
	Owner bool `json:"owner"`
	HasPassword bool `json:"has_password,omitempty"` // feature:passwords
	PasswordBlocked bool `json:"password_blocked,omitempty"` // feature:passwords
	TwoFactor bool `json:"two_factor,omitempty"` // feature:two_factor
	LoginLink *loginLinkSchema `json:"login_link,omitempty" doc:"The account's live login link, or null."`
}

func ( userSchema ) APIName() ( name string ) { name = "User"; return }

// >>> feature:webhooks
type webhookSchema struct {
	ID string `json:"id"`
	Name string `json:"name"`
	URL string `json:"url"`
	Events []string `json:"events"`
	Enabled bool `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func ( webhookSchema ) APIName() ( name string ) { name = "Webhook"; return }
// <<< feature:webhooks

// >>> feature:webhooks
type webhookDeliverySchema struct {
	ID string `json:"id"`
	Event string `json:"event"`
	Status string `json:"status"`
	Attempts int `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
	NextAttemptAt time.Time `json:"next_attempt_at,omitempty"`
	LastStatus int `json:"last_status,omitempty"`
	LastError string `json:"last_error,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

func ( webhookDeliverySchema ) APIName() ( name string ) { name = "WebhookDelivery"; return }
// <<< feature:webhooks

// >>> feature:files
type fileSchema struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int `json:"size"`
	State string `json:"state" enum:"uploading|ready"`
	Chunks int `json:"chunks"`
	Received int `json:"received"`
	CreatedAt time.Time `json:"created_at"`
}

func ( fileSchema ) APIName() ( name string ) { name = "File"; return }
// <<< feature:files

// >>> feature:logs
type ipNamesSchema struct {
	Names []ipNamesSchemaNamesItemDoc `json:"names,omitempty"`
}

func ( ipNamesSchema ) APIName() ( name string ) { name = "IPNames"; return }
// <<< feature:logs

// >>> feature:logs
type ipNamesSchemaNamesItemDoc struct {
	Target string `json:"target,omitempty"`
	Name string `json:"name,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}
// <<< feature:logs

type loginLinkSchema struct {
	MaxUses int `json:"max_uses" doc:"Sign-ins it allows. 0 when unlimited."`
	Unlimited bool `json:"unlimited"`
	Uses int `json:"uses" doc:"Sign-ins so far."`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at" doc:"null when it never expires."`
	LastUsedAt *time.Time `json:"last_used_at"`
}

func ( loginLinkSchema ) APIName() ( name string ) { name = "LoginLink"; return }

func ( loginLinkSchema ) APIDoc() ( text string ) { text = "A login link that can still be used. One per account at most."; return }

// >>> feature:appearance
type themeSchema struct {
	ID string `json:"id" doc:"Lower-case letters, digits, - and _, up to 32."`
	Name string `json:"name" doc:"1 to 60 characters."`
	Mode string `json:"mode" enum:"light|dark"`
	Accent string `json:"accent" doc:"The main color: what you can press."`
	Second string `json:"second" doc:"Says where you are: active nav item, headings, info."`
	Background string `json:"background,omitempty" doc:"The page. Left out: the mode's own."`
	Surface string `json:"surface,omitempty" doc:"Cards. Left out: the mode's own."`
	Overrides map[string]string `json:"overrides,omitempty" doc:"Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back."`
	Button themeSchemaButtonDoc `json:"button,omitempty" doc:"How the theme's buttons look. A choice left out is the default (rounded, normal, raised, normal)."`
	Light themeSchemaLightDoc `json:"light,omitempty" doc:"A dark theme's colors for light mode, kept while it is dark. Switching the theme to light swaps them in."`
	Dark themeSchemaDarkDoc `json:"dark,omitempty" doc:"A light theme's colors for dark mode, kept while it is light. Switching the theme to dark swaps them in. A light theme's light (or a dark theme's dark) is dropped."`
}

func ( themeSchema ) APIName() ( name string ) { name = "Theme"; return }

func ( themeSchema ) APIDoc() ( text string ) { text = "A color scheme on the Appearance screen. Colors are #rrggbb. Anything not given is worked out by static/js/theme.js at readable contrast."; return }
// <<< feature:appearance

// >>> feature:appearance
type themeSchemaButtonDoc struct {
	Shape string `json:"shape,omitempty" enum:"square|soft|rounded|pill"`
	Weight string `json:"weight,omitempty" enum:"normal|medium|bold"`
	Depth string `json:"depth,omitempty" enum:"flat|raised|glossy"`
	Case string `json:"case,omitempty" enum:"normal|upper"`
}
// <<< feature:appearance

// >>> feature:appearance
type themeSchemaLightDoc struct {
	Accent string `json:"accent" doc:"The main color: what you can press."`
	Second string `json:"second" doc:"Says where you are: active nav item, headings, info."`
	Background string `json:"background,omitempty" doc:"The page. Left out: the mode's own."`
	Surface string `json:"surface,omitempty" doc:"Cards. Left out: the mode's own."`
	Overrides map[string]string `json:"overrides,omitempty" doc:"Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back."`
}
// <<< feature:appearance

// >>> feature:appearance
type themeSchemaDarkDoc struct {
	Accent string `json:"accent" doc:"The main color: what you can press."`
	Second string `json:"second" doc:"Says where you are: active nav item, headings, info."`
	Background string `json:"background,omitempty" doc:"The page. Left out: the mode's own."`
	Surface string `json:"surface,omitempty" doc:"Cards. Left out: the mode's own."`
	Overrides map[string]string `json:"overrides,omitempty" doc:"Worked-out colors replaced by name: text, accent, accentFg, second, secondFg, secondText, link, navLink, background, surface, nested, chrome, info, infoBorder, border, and for each button variant V (Secondary, Success, Danger, Warning, Info, Light, Dark) btnV, btnVFg and btnVText. Leaving one out puts the readable color back."`
}
// <<< feature:appearance
