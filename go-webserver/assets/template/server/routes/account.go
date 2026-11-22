package routes

import (
	errors "errors"
	time "time" // feature:api_keys

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// meReply is GetMe's answer: who is asking, and what the page needs to know
// before it draws anything.
type meReply struct {
	Authenticated      bool                  `json:"authenticated"`
	ID                 uint64                `json:"id"`
	DisplayName        string                `json:"display_name"`
	Role               string                `json:"role" jsonschema:"enum=admin,enum=user,enum=viewer" jsonschema_description:"The effective role: never higher than an API key's own. A viewer may read, and change nothing."`
	AccountRole        string                `json:"account_role" jsonschema:"enum=admin,enum=user,enum=viewer"`
	Contract           int                   `json:"contract" jsonschema_description:"Which version of the portal contract this app speaks: the Portal contract section of the webserver skills' SKILL.md."`
	Capabilities       []string              `json:"capabilities" jsonschema_description:"The optional parts of that contract this app has on now: viewer_role, account_expiry, key_minting, invite_links, key_sign_in, service_accounts, key_grants, error_codes, key_rotation, admin_sessions."`
	MayMintKeys        bool                  `json:"may_mint_keys,omitempty" jsonschema_description:"API keys only: whether this key may make keys for other accounts." jsonschema_extras:"x-feature=api_keys"` // feature:api_keys
	IsOwner            bool                  `json:"is_owner"`
	Owner              bool                  `json:"owner" jsonschema_description:"Whether this is the owner's account: the same as is_owner, under portal contract v3's name."`
	Kind               string                `json:"kind" jsonschema:"enum=person,enum=service" jsonschema_description:"person, or service for an account that only has API keys and is never signed into (ProjectPortal's own)."`
	Maintenance        string                `json:"maintenance" jsonschema:"enum=none,enum=read_only,enum=full" jsonschema_extras:"x-feature=maintenance"` // feature:maintenance
	HasPIN             bool                  `json:"has_pin" jsonschema_extras:"x-feature=invite_links"` // feature:invite_links
	Username           string                `json:"username" jsonschema_extras:"x-feature=passwords"` // feature:passwords
	HasPassword        bool                  `json:"has_password" jsonschema_extras:"x-feature=passwords"` // feature:passwords
	PasswordBlocked    bool                  `json:"password_blocked" jsonschema_extras:"x-feature=passwords"` // feature:passwords
	TwoFactor          bool                  `json:"two_factor" jsonschema_extras:"x-feature=two_factor"` // feature:two_factor
	PasswordsEnabled   bool                  `json:"passwords_enabled" jsonschema_extras:"x-feature=passwords"` // feature:passwords
	TwoFactorEnabled   bool                  `json:"two_factor_enabled" jsonschema_extras:"x-feature=two_factor"` // feature:two_factor
	Auth               string                `json:"auth"`
	APIKeysEnabled     bool                  `json:"api_keys_enabled" jsonschema_extras:"x-feature=api_keys"` // feature:api_keys
	MCPEnabled         bool                  `json:"mcp_enabled" jsonschema_description:"server.mcp: whether POST /mcp answers." jsonschema_extras:"x-feature=mcp"` // feature:mcp
	FilesEnabled       bool                  `json:"files_enabled" jsonschema_description:"files.enabled: whether the Files screen and routes are on." jsonschema_extras:"x-feature=files"` // feature:files
	InviteLinksEnabled bool                  `json:"invite_links_enabled" jsonschema_description:"Whether invite links are on." jsonschema_extras:"x-feature=invite_links"` // feature:invite_links
	WebhooksEnabled    bool                  `json:"webhooks_enabled" jsonschema_description:"webhooks.enabled: whether webhooks are sent." jsonschema_extras:"x-feature=webhooks"` // feature:webhooks
	LogsEnabled        bool                  `json:"logs_enabled" jsonschema_extras:"x-feature=logs"` // feature:logs
	LogsUIClicks       bool                  `json:"logs_ui_clicks" jsonschema_extras:"x-feature=logs"` // feature:logs
	CSRFToken          string                `json:"csrf_token,omitempty" jsonschema_description:"Browser sessions only."`
	APIKeyID           string                `json:"api_key_id,omitempty" jsonschema_description:"API keys only." jsonschema_extras:"x-feature=api_keys"` // feature:api_keys
	APIKeyName         string                `json:"api_key_name,omitempty" jsonschema_description:"API keys only." jsonschema_extras:"x-feature=api_keys"` // feature:api_keys
	Key                meKey                 `json:"key,omitzero" jsonschema_description:"API keys only: what this key may do." jsonschema_extras:"x-feature=api_keys"` // feature:api_keys
	Version            string                `json:"version" jsonschema_description:"The running build. dev when none was stamped."`
	Skill              string                `json:"skill,omitempty" jsonschema_description:"The webserver skill this app was scaffolded from (go-webserver or python-webserver), from its .skill-origin.yaml. Absent when that is not known."`
	SkillCommit        string                `json:"skill_commit,omitempty" jsonschema_description:"The skill's commit this app was scaffolded from or last upgraded to, from its .skill-origin.yaml. Absent when that is not known."`
	Announcement       string                `json:"announcement" jsonschema_description:"server.announcement. Empty when there is none."`
	Repo               string                `json:"repo" jsonschema_description:"app.repo without a trailing / or .git: where the source lives. Empty when it is not set."`
	Description        string                `json:"description" jsonschema_description:"app.description. Empty when there is none."`
	ShowAbout          bool                  `json:"show_about" jsonschema_description:"app.show_about: whether the menu has About." jsonschema_extras:"x-feature=about"` // feature:about
	OpenapiURL         string                `json:"openapi_url" jsonschema_description:"Where to read this API's reference: config/api.json in app.repo, or /api/api.json when there is no repo." jsonschema_extras:"x-feature=openapi"` // feature:openapi
	MCPURL             string                `json:"mcp_url" jsonschema_description:"Where to read the MCP reference: config/mcp.json in app.repo, or /api/mcp.json when there is no repo. Only with server.mcp on." jsonschema_extras:"x-feature=mcp"` // feature:mcp
	APIDocURL          string                `json:"api_doc_url" jsonschema_description:"Where to read the API reference as a page for people: config/api.md in app.repo, or /docs/api when there is no repo." jsonschema_extras:"x-feature=openapi"` // feature:openapi
	MCPDocURL          string                `json:"mcp_doc_url" jsonschema_description:"Where to read the MCP tools as a page for people: config/mcp.md in app.repo, or /docs/mcp when there is no repo." jsonschema_extras:"x-feature=mcp"` // feature:mcp
	Shortcuts          map[string]string     `json:"shortcuts" jsonschema_description:"The keyboard shortcuts you changed, by action. Mod is Cmd on a Mac and Ctrl elsewhere." jsonschema_extras:"x-feature=shortcuts"` // feature:shortcuts
	Themes             []models.Theme        `json:"themes" jsonschema:"nullable" jsonschema_description:"Your themes (see the Theme schema): your own list, else the app's defaults, else null for the built-in ones." jsonschema_extras:"x-feature=appearance"` // feature:appearance
	ThemesCustom       bool                  `json:"themes_custom" jsonschema_description:"Whether you have your own list rather than following the defaults." jsonschema_extras:"x-feature=appearance"` // feature:appearance
	ThemesBuiltIns     int                   `json:"themes_builtins" jsonschema_description:"Which built-in themes that list has seen (see builtins on POST /api/account/themes)." jsonschema_extras:"x-feature=appearance"` // feature:appearance
	ThemeID            string                `json:"theme_id" jsonschema_description:"The theme you use. Empty: the first." jsonschema_extras:"x-feature=appearance"` // feature:appearance
	LoginLink          *models.LoginLinkInfo `json:"login_link" jsonschema_description:"Your own live login link, or null."`
}

func ( meReply ) APIName() ( name string ) { name = "Me"; return }

// >>> feature:api_keys
// meKey is what /api/me says about the calling key, so an outside tool knows
// before it clicks whether the key can make keys, and when it stops working.
type meKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Role        string    `json:"role" jsonschema:"enum=admin,enum=user,enum=viewer" jsonschema_description:"The key's own role. The role it acts with is the lower of this and the account's."`
	MayMintKeys bool      `json:"may_mint_keys" jsonschema_description:"Whether this key may make keys for other accounts."`
	MaySignIn   bool      `json:"may_sign_in" jsonschema_description:"Whether this key may make a login link for its own account."`
	ExpiresAt   time.Time `json:"expires_at,omitzero" jsonschema_description:"When the key stops working. Absent when it never does."`
}

func ( meKey ) APIName() ( name string ) { name = "MeKey"; return }
// <<< feature:api_keys

// getMe is the frontend's first call on every page load: it answers "who am
// I, what may I do, and what CSRF token do I echo back". Handing the token
// out here means no page needs a hidden form field or a template variable.
//
// "role" is the *effective* role, not the account's: a request made with a
// user-scoped API key belonging to an admin reports "user", because that is
// what it may actually do. account_role reports the account itself, for the
// rare caller that needs to tell the two apart. Reporting the account's role
// as "role" would have a client believe it has permissions the server will
// refuse to act on. The MCP whoami tool is this route, so the two cannot
// describe the same caller differently.
//
// @route("GET /api/me", summary="Who am I",
//     tag="Account",
//     description="The account this API key belongs to, the role it acts with (role -- never higher than "
//         "the key's own scope), and the account's own role.",
//     mcp="whoami", read_only=True,  // feature:mcp
// )
func ( handlers *Handlers ) getMe( c fiber.Ctx , in *apidoc.None ) ( reply *meReply , err error ) {
	actor := security.ActorFrom( c )
	user := actor.User

	// is_owner tells the UI which account-management buttons to offer; the
	// server checks ownership again on every request that depends on it.
	owner_id , err := models.OwnerID( handlers.Store )
	if err != nil { return }
	login_link , err := models.LiveLoginLink( handlers.Store , user.ID )
	if err != nil { return }

	// >>> feature:appearance
	themes , themes_custom , themes_builtins , err := handlers.myThemes( user )
	if err != nil { return }
	// <<< feature:appearance
	cfg := handlers.cfg()
	reply = &meReply{
		Authenticated:      true,
		ID:                 user.ID,
		DisplayName:        user.DisplayName,
		Role:               actor.Role,
		AccountRole:        user.Role,
		Contract:           portalContract,
		Capabilities:       handlers.capabilities(),
		IsOwner:            user.ID == owner_id,
		Owner:              user.ID == owner_id,
		Kind:               user.KindOrPerson(),
		LoginLink:          login_link,
		Maintenance:        cfg.MaintenanceMode(), // feature:maintenance
		Version:            handlers.Version,
		Skill:              handlers.Skill,
		SkillCommit:        handlers.SkillCommit,
		Announcement:       cfg.Announcement,
		Repo:               cfg.Repo,
		Description:        cfg.Description,
		ShowAbout:          cfg.ShowAbout, // feature:about
		OpenapiURL:         handlers.openAPIURL(), // feature:openapi
		MCPURL:             handlers.mcpURL(), // feature:mcp
		APIDocURL:          handlers.apiDocURL(), // feature:openapi
		MCPDocURL:          handlers.mcpDocURL(), // feature:mcp
		Shortcuts:          myShortcuts( user ), // feature:shortcuts
		Themes:             themes, // feature:appearance
		ThemesCustom:       themes_custom, // feature:appearance
		ThemesBuiltIns:     themes_builtins, // feature:appearance
		ThemeID:            user.ThemeID, // feature:appearance
		HasPIN:             user.HasPin(), // feature:invite_links
		Username:           user.Username, // feature:passwords
		HasPassword:        user.HasPassword(), // feature:passwords
		PasswordBlocked:    user.PasswordBlocked, // feature:passwords
		TwoFactor:          user.TwoFactorOn(), // feature:two_factor
		PasswordsEnabled:   cfg.PasswordsEnabled, // feature:passwords
		TwoFactorEnabled:   cfg.TwoFactorEnabled, // feature:two_factor
		Auth:               actor.Method(),
		APIKeysEnabled:     cfg.APIKeysEnabled, // feature:api_keys
		MCPEnabled:         cfg.MCPEnabled, // feature:mcp
		FilesEnabled:       cfg.Files.Enabled, // feature:files
		InviteLinksEnabled: handlers.inviteLinksOn(), // feature:invite_links
		WebhooksEnabled:    cfg.Webhooks.Enabled, // feature:webhooks
		// Whether track.js sends anything, and whether it sends clicks. // feature:logs
		LogsEnabled:        handlers.Logs.Enabled(), // feature:logs
		LogsUIClicks:       handlers.Logs.Enabled() && cfg.Logs.UIClicks, // feature:logs
	}
	// Only a session has a CSRF token. A key-authenticated request has no
	// ambient credential to protect, so there is nothing to hand back -- see
	// Guard.CheckCSRF.
	if actor.Session != nil {
		reply.CSRFToken = actor.Session.CSRFToken
	}
	// >>> feature:api_keys
	if actor.APIKey != nil {
		reply.APIKeyID = actor.APIKey.ID
		reply.APIKeyName = actor.APIKey.Name
		reply.MayMintKeys = actor.APIKey.MayMintKeys
		reply.Key = meKey{ ID: actor.APIKey.ID , Name: actor.APIKey.Name , Role: actor.APIKey.Role , MayMintKeys: actor.APIKey.MayMintKeys , MaySignIn: actor.APIKey.MaySignIn }
		if actor.APIKey.ExpiresAt != nil { reply.Key.ExpiresAt = actor.APIKey.ExpiresAt.UTC() }
	}
	// <<< feature:api_keys
	return
}

// portalContract is the version of ProjectPortal's contract this app
// speaks: what it may call from outside, field for field (the "Portal
// contract" section of the go-webserver skill's SKILL.md, which ProjectPortal's
// CROSS_PROJECT.md points at). The webserver-upgrade skill's status reads it.
const portalContract = 3

// capabilities are the optional parts of that contract this app has on right
// now. The portal turns each one on only when it is listed here.
func ( handlers *Handlers ) capabilities() ( result []string ) {
	// One line per capability, in the contract's order, so an app that left
	// one of them out drops its line and nothing else.
	result = []string{}
	keys := false
	keys = handlers.cfg().APIKeysEnabled // feature:api_keys
	result = append( result , "viewer_role" )
	result = append( result , "account_expiry" )
	if keys { result = append( result , "key_minting" ) }
	if handlers.inviteLinksOn() { result = append( result , "invite_links" ) } // feature:invite_links
	if keys { result = append( result , "key_sign_in" ) }
	if keys { result = append( result , "service_accounts" ) }
	if keys { result = append( result , "key_grants" ) }
	result = append( result , "error_codes" )
	if keys { result = append( result , "key_rotation" ) }
	result = append( result , "admin_sessions" ) // feature:sessions
	return
}

// >>> feature:shortcuts
// myShortcuts is never null, so the frontend can read it without checking.
func myShortcuts( user *models.User ) ( result map[string]string ) {
	result = user.Shortcuts
	if result == nil { result = map[string]string{} }
	return
}
// <<< feature:shortcuts

type renameAccountRequest struct {
	DisplayName string `json:"display_name" jsonschema:"minLength=1,maxLength=80" error:"display name must be 1-80 characters"`
	// Expect is optional: the name the caller expects to replace. The
	// frontend always sends it, so its undo cannot overwrite a rename made
	// elsewhere.
	Expect *string `json:"expect,omitempty"`
}

// renameAccount changes the caller's own display name.
//
// @route("POST /api/account/rename", summary="Rename your account",
//     tag="Account", errors=(409,),
//     description="Change your own display name (1-80 characters). expect, when given, is the name you "
//         "think it has now; if someone changed it meanwhile the rename is refused with the current name, "
//         "so nothing is overwritten unseen.",
//     mcp="rename_account",  // feature:mcp
// )
func ( handlers *Handlers ) renameAccount( c fiber.Ctx , in *renameAccountRequest ) ( reply *renameUserReply , err error ) {
	if models.ValidDisplayName( in.DisplayName ) == false {
		err = refuse( fiber.StatusBadRequest , "display name must be 1-80 characters" )
		return
	}
	user := security.UserFrom( c )
	rename_err := models.RenameUser( handlers.Store , user.ID , in.DisplayName , in.Expect )
	var conflict_err *models.RenameConflictError
	if errors.As( rename_err , &conflict_err ) {
		err = refuse( fiber.StatusConflict , conflict_err.Error() , fiber.Map{ "display_name": conflict_err.Current } )
		return
	}
	if rename_err != nil {
		err = rename_err
		return
	}
	handlers.audit( c , logs.KindAuth , "renamed own account" , fiber.Map{ "name": in.DisplayName } )
	reply = &renameUserReply{ Ok: true , DisplayName: in.DisplayName }
	return
}

// >>> feature:invite_links
type pinRequest struct {
	Pin string `json:"pin"`
}

type pinReply struct {
	Ok     bool `json:"ok"`
	HasPIN bool `json:"has_pin"`
}

// setPin sets, or with an empty pin clears, the PIN asked for when this
// account signs back in through an invite link (see models/pin.go).
//
// @route("POST /api/account/pin", summary="Set or clear your PIN",
//     tag="Account", access=SESSION_ONLY, feature="invite_links",
// )
func ( handlers *Handlers ) setPin( c fiber.Ctx , in *pinRequest ) ( reply *pinReply , err error ) {
	set_err := models.SetUserPin( handlers.Store , security.UserFrom( c ).ID , in.Pin )
	if errors.Is( set_err , models.ErrBadPin ) {
		err = refuse( fiber.StatusBadRequest , "bad_pin" )
		return
	}
	if set_err != nil {
		err = set_err
		return
	}
	handlers.audit( c , logs.KindAuth , "pin changed" , fiber.Map{ "has_pin": in.Pin != "" } )
	reply = &pinReply{ Ok: true , HasPIN: in.Pin != "" }
	return
}
// <<< feature:invite_links

// createMyLoginLink mints a login link for the caller's own account: the way
// to sign in on another device without asking an admin. Once by default, or
// reusable, like ReissueLogin -- it is ReissueLogin pointed at yourself, so it
// has the same effect: any link the account could still use stops working,
// and existing sessions stay signed in.
//
// A browser session may always ask. An API key may only when its account
// turned that on for it, from a session (POST /api/keys/{key_id}/sign-in):
// the link becomes an ordinary browser session, which opens everything kept
// from keys (the API keys card, the owner's screens, minting keys) to whoever
// holds the key. A key's link works once and for at most 15 minutes,
// whatever it asked for, and revoking the key voids it while it is unused.
//
// @route("POST /api/account/login-link", summary="A login link for yourself",
//     tag="Account",
//     description="A link that signs your own account in on a browser or another device. An API key may "
//         "ask only if your account let it, from a browser session (POST /api/keys/{key_id}/sign-in); "
//         "its link works once, for at most 15 minutes.",
//     mcp_note="Only works if this key was allowed to make sign-in links, on the API keys card. Answers "  // feature:mcp
//         "with login_url (open it to sign in; it works once, for at most 15 minutes), expires_at and "  // feature:mcp
//         "max_uses. Any older link the account could still use stops working; its signed-in devices "  // feature:mcp
//         "stay signed in. Revoking or deleting this API key voids the link while it is unused, and signs "  // feature:mcp
//         "out every device it signed in.",  // feature:mcp
//     mcp="create_my_login_link", call=withLoginLinkURL,  // feature:mcp
// )
func ( handlers *Handlers ) createMyLoginLink( c fiber.Ctx , in *models.LoginLinkRequest ) ( reply *loginLinkReply , err error ) {
	actor := security.ActorFrom( c )
	// >>> feature:api_keys
	if actor.ViaAPIKey() && actor.APIKey.MaySignIn == false {
		err = refuse( fiber.StatusForbidden , "this key may not make sign-in links: turn that on for it on the API keys card" )
		return
	}
	// <<< feature:api_keys
	options , refused := handlers.loginLinkOptions( c , *in )
	if refused != nil {
		err = refused
		return
	}
	// >>> feature:api_keys
	if actor.ViaAPIKey() {
		options.MaxUses , options.Unlimited = 1 , false
		if options.TTL <= 0 || options.TTL > models.KeyLoginLinkTTL { options.TTL = models.KeyLoginLinkTTL }
	}
	// <<< feature:api_keys
	credential , info , err := models.IssueLoginLink( handlers.Store , actor.User.ID , options )
	if err != nil { return }
	handlers.audit( c , logs.KindAuth , "own login link created" , fiber.Map{ "max_uses": info.MaxUses , "expires_at": info.ExpiresAt } )
	made := newLoginLinkReply( credential , info )
	reply = &made
	return
}

// cancelMyLoginLink stops the caller's own live login link working.
//
// @route("POST /api/account/login-link/cancel", summary="Stop your own login link working",
//     tag="Account",
//     description="Void your account's live login link without making a new one. Devices it already signed "
//         "in stay signed in. Answers with removed: 0 or 1.",
//     mcp="cancel_my_login_link", destructive=True,  // feature:mcp
// )
func ( handlers *Handlers ) cancelMyLoginLink( c fiber.Ctx , in *apidoc.None ) ( reply *cancelLoginLinkReply , err error ) {
	user_id := security.UserFrom( c ).ID
	removed , err := models.CancelLoginLink( handlers.Store , user_id , user_id )
	if err != nil { return }
	handlers.audit( c , logs.KindAuth , "own login link cancelled" , fiber.Map{ "removed": removed } )
	reply = &cancelLoginLinkReply{ Ok: true , Removed: removed }
	return
}

// logout deletes the session server-side as well as clearing the cookie, so
// a copy of the cookie captured earlier is dead too.
//
// It is held to the CSRF token like every other state change. SameSite=Strict
// already stops another site from sending the cookie here, but not a page on
// a sibling subdomain, and being signed out by someone else's page is the
// first step of tricking a person into signing in to the wrong account.
//
// Called with an API key it does nothing and says so by succeeding: there is
// no session to destroy. A key is ended by revoking it, not by logging out.
//
// @route("POST /api/logout", summary="Sign out",
//     tag="Account",
//     mcp_http="Ends a browser session; a key has none to end.",
// )
func ( handlers *Handlers ) logout( c fiber.Ctx , in *apidoc.None ) ( reply *okReply , err error ) {
	handlers.audit( c , logs.KindAuth , "signed out" , nil )
	handlers.destroyCookieSession( c )
	handlers.Guard.ClearSessionCookie( c )
	reply = &okReply{ Ok: true }
	return
}

// >>> feature:openapi
// openAPIURL is where the API reference is read: config/api.json in
// app.repo when that is set, so it opens as a page there, else this server's
// own copy, which needs the same sign-in.
func ( handlers *Handlers ) openAPIURL() ( result string ) {
	result = handlers.cfg().RepoFile( "config/api.json" )
	if result == "" {
		result = "/api/api.json"
	}
	return
}
// <<< feature:openapi

// >>> feature:mcp
// mcpURL is where the MCP reference is read: config/mcp.json in app.repo when
// that is set, else this server's own copy, which needs the same sign-in.
func ( handlers *Handlers ) mcpURL() ( result string ) {
	result = handlers.cfg().RepoFile( "config/mcp.json" )
	if result == "" {
		result = "/api/mcp.json"
	}
	return
}
// <<< feature:mcp
