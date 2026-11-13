// whole-file feature:mcp
package routes

import (
	fiber "github.com/gofiber/fiber/v3"

	mcp "{{MODULE_PATH}}/server/mcp"
	models "{{MODULE_PATH}}/server/models"
)

// The MCP side of accounts and sign-in: login links, accounts, devices and
// invite links. Each tool calls the helper its HTTP route calls, so the rules
// (who may touch the owner, what a narrowed key may not do) hold whichever
// door a request comes through.
//
// A login link or invite link made here records the key that asked, and
// revoking or deleting that key voids it while it can still be used (see
// models.VoidLinksMintedByKeyTx). That is what keeps revoking a leaked key
// meaningful now that a key can hand out ways in.

// loginLinkArgsSchema is the arguments every tool that mints a login link takes
// besides its own, the same two as the HTTP body (models.LoginLinkRequest).
var loginLinkArgsSchema = fiber.Map{
	"max_uses":           fiber.Map{ "type": "integer" , "minimum": 0 , "description": "How many sign-ins the link allows. Leave it out for once; 0 is unlimited until it expires." },
	"expires_in_minutes": fiber.Map{ "type": "integer" , "minimum": 0 , "description": "How long the link works. Leave it out for the configured default; 0 never expires." },
}

// loginLinkProperties is loginLinkArgsSchema plus a tool's own arguments.
func loginLinkProperties( own fiber.Map ) ( properties fiber.Map ) {
	properties = fiber.Map{}
	for name , value := range loginLinkArgsSchema { properties[ name ] = value }
	for name , value := range own { properties[ name ] = value }
	return
}

// withLoginURL hands back a login-link payload with the full address added,
// from the address this agent reached /mcp at, the way fileURL does for files.
// The HTTP routes answer with login_path only (see CreateUser for why).
func withLoginURL( call *mcp.Call , payload fiber.Map , refused *refusal ) ( result any , err error ) {
	if refused == nil {
		if path , ok := payload[ "login_path" ].( string ); ok { payload[ "login_url" ] = call.Ctx.BaseURL() + path }
	}
	result , err = answer( payload , refused )
	return
}

const loginLinkAnswer = "Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. Any older link the account could still use stops working; its signed-in devices stay signed in. Revoking or deleting this API key voids the link while it is unused."

func ( handlers *Handlers ) registerAccountTools( server *mcp.Server ) {
	server.Register( &mcp.Tool{
		Name:        "create_my_login_link",
		Route:       "POST /api/account/login-link",
		Title:       "A login link for yourself",
		Description: "A link that signs your own account in on a browser or another device. Only a key with your account's full role may ask: one scoped down to user on an admin account is refused. " + loginLinkAnswer,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           loginLinkProperties( nil ),
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args models.LoginLinkRequest
			if err = call.Bind( &args ); err != nil { return }
			payload , refused := handlers.createMyLoginLink( call.Ctx , args )
			result , err = withLoginURL( call , payload , refused )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "cancel_my_login_link",
		Route:       "POST /api/account/login-link/cancel",
		Title:       "Stop your own login link working",
		Description: "Void your account's live login link without making a new one. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
		Destructive: true,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = answer( handlers.cancelMyLoginLink( call.Ctx ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_create_user",
		Route:       "POST /api/admin/users",
		Title:       "Create an account (admin)",
		Description: "Make a new account and its first login link, to hand to the person. role is admin or user. Answers with user_id and the link. " + loginLinkAnswer,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": loginLinkProperties( fiber.Map{
				"display_name": fiber.Map{ "type": "string" , "description": "1-80 characters." },
				"role":         fiber.Map{ "type": "string" , "enum": []string{ models.RoleAdmin , models.RoleUser } },
			} ),
			"required":             []string{ "display_name" , "role" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				DisplayName string `json:"display_name"`
				Role        string `json:"role"`
				models.LoginLinkRequest
			}
			if err = call.Bind( &args ); err != nil { return }
			payload , refused := handlers.createUser( call.Ctx , args.DisplayName , args.Role , args.LoginLinkRequest )
			result , err = withLoginURL( call , payload , refused )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_reissue_login",
		Route:       "POST /api/admin/users/{user_id}/reissue-login",
		Title:       "A new login link for an account (admin)",
		Description: "For someone who lost their device or their link: a fresh login link for their account. " + loginLinkAnswer,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           loginLinkProperties( fiber.Map{ "user_id": userIDSchema } ),
			"required":             []string{ "user_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
				models.LoginLinkRequest
			}
			if err = call.Bind( &args ); err != nil { return }
			payload , refused := handlers.reissueLogin( call.Ctx , args.UserID , args.LoginLinkRequest )
			result , err = withLoginURL( call , payload , refused )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_cancel_login_link",
		Route:       "POST /api/admin/users/{user_id}/cancel-login-link",
		Title:       "Stop an account's login link working (admin)",
		Description: "Void an account's live login link without making a new one: a link that went somewhere it should not have. Devices it already signed in stay signed in; admin_revoke_access ends those too. Answers with removed: 0 or 1.",
		Destructive: true,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "user_id": userIDSchema },
			"required":             []string{ "user_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.cancelLoginLink( call.Ctx , args.UserID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_rename_user",
		Route:       "POST /api/admin/users/{user_id}/rename",
		Title:       "Rename someone (admin)",
		Description: "Change another account's display name (1-80 characters). Only the owner renames an admin. expect, when given, is the name you think it has now; if it changed meanwhile the rename is refused with the current name.",
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"user_id":      userIDSchema,
				"display_name": fiber.Map{ "type": "string" },
				"expect":       fiber.Map{ "type": "string" , "description": "The current name, from admin_list_users. Optional." },
			},
			"required":             []string{ "user_id" , "display_name" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID      uint64  `json:"user_id"`
				DisplayName string  `json:"display_name"`
				Expect      *string `json:"expect"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.renameUser( call.Ctx , args.UserID , args.DisplayName , args.Expect ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_set_user_role",
		Route:       "POST /api/admin/users/{user_id}/role",
		Title:       "Change an account's role (admin)",
		Description: "Make an account an admin or an ordinary user. Only the owner takes an admin's rights away; the owner is always an admin, and nobody changes their own role.",
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"user_id": userIDSchema,
				"role":    fiber.Map{ "type": "string" , "enum": []string{ models.RoleAdmin , models.RoleUser } },
			},
			"required":             []string{ "user_id" , "role" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
				Role   string `json:"role"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setUserRole( call.Ctx , args.UserID , args.Role ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_delete_user",
		Route:       "POST /api/admin/users/{user_id}/delete",
		Title:       "Delete an account (admin)",
		Description: "Remove an account for good, with its sessions, login link and API keys. Cannot be undone: admin_set_user_disabled is the reversible version. Only the owner deletes an admin; nobody deletes the owner or themselves.",
		Destructive: true,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "user_id": userIDSchema },
			"required":             []string{ "user_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.deleteUser( call.Ctx , args.UserID ) )
			return
		},
	} )

	// >>> feature:two_factor
	server.Register( &mcp.Tool{
		Name:        "admin_reset_two_factor",
		Route:       "POST /api/admin/users/{user_id}/reset-two-factor",
		Title:       "Turn off an account's two-factor (admin)",
		Description: "For someone who lost both their phone and their recovery codes: switches their two-factor sign-in off so a login link lets them in again. Only the owner resets an admin's; the owner's is reset from a shell.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     func() bool { return handlers.cfg().TwoFactorEnabled },
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "user_id": userIDSchema },
			"required":             []string{ "user_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.resetTwoFactor( call.Ctx , args.UserID ) )
			return
		},
	} )
	// <<< feature:two_factor

	// >>> feature:sessions
	sessionIDSchema := fiber.Map{ "type": "string" , "description": "The device's id, from list_my_sessions or admin_list_user_sessions." }

	server.Register( &mcp.Tool{
		Name:        "list_my_sessions",
		Route:       "GET /api/account/sessions",
		Title:       "Your signed-in devices",
		Description: "Where your account is signed in, a page at a time: id, when it started and was last seen, the browser, and the name you gave it. Through an API key no device is current. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "sessions" , func() ( []fiber.Map , error ) { return handlers.mySessionRows( call.Ctx ) } )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "label_my_session",
		Route:       "POST /api/account/sessions/{session_id}/label",
		Title:       "Name one of your devices",
		Description: "Give one of your devices a name (at most 80 characters; empty clears it). expect, when given, is the name you think it has now.",
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"session_id": sessionIDSchema,
				"label":      fiber.Map{ "type": "string" },
				"expect":     fiber.Map{ "type": "string" },
			},
			"required":             []string{ "session_id" , "label" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				SessionID string  `json:"session_id"`
				Label     string  `json:"label"`
				Expect    *string `json:"expect"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.labelMySession( call.Ctx , args.SessionID , args.Label , args.Expect ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "revoke_my_session",
		Route:       "POST /api/account/sessions/{session_id}/revoke",
		Title:       "Sign out one of your devices",
		Description: "End one of your signed-in sessions; that browser has to sign in again.",
		Destructive: true,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "session_id": sessionIDSchema },
			"required":             []string{ "session_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				SessionID string `json:"session_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.revokeMySession( call.Ctx , args.SessionID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "revoke_my_other_sessions",
		Route:       "POST /api/account/sessions/revoke-others",
		Title:       "Sign out all your devices",
		Description: "End every session your account has: through an API key there is no current device to keep, so every browser signs in again. API keys keep working. Answers with how many were removed.",
		Destructive: true,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = answer( handlers.revokeMyOtherSessions( call.Ctx ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_list_user_sessions",
		Route:       "GET /api/admin/users/{user_id}/sessions",
		Title:       "An account's signed-in devices (admin)",
		Description: "Where an account is signed in: every session, with when it started and was last seen. Addresses are shown to the owner only, and only the owner sees the owner's.",
		ReadOnly:    true,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "user_id": userIDSchema },
			"required":             []string{ "user_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID uint64 `json:"user_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			rows , refused := handlers.userSessionRows( call.Ctx , args.UserID )
			result , err = answer( fiber.Map{ "sessions": rows } , refused )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_revoke_user_session",
		Route:       "POST /api/admin/users/{user_id}/sessions/{session_id}/revoke",
		Title:       "Sign out one of an account's devices (admin)",
		Description: "End one of an account's sessions, rather than all of them with admin_revoke_access.",
		Destructive: true,
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"user_id":    userIDSchema,
				"session_id": sessionIDSchema,
			},
			"required":             []string{ "user_id" , "session_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID    uint64 `json:"user_id"`
				SessionID string `json:"session_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.revokeUserSession( call.Ctx , args.UserID , args.SessionID ) )
			return
		},
	} )
	// <<< feature:sessions

	// >>> feature:invite_links
	linkIDSchema := fiber.Map{ "type": "string" , "description": "The id from admin_list_invite_links." }
	linkIDInput := fiber.Map{
		"type":                 "object",
		"properties":           fiber.Map{ "link_id": linkIDSchema },
		"required":             []string{ "link_id" },
		"additionalProperties": false,
	}
	type linkIDArgs struct {
		LinkID string `json:"link_id"`
	}

	server.Register( &mcp.Tool{
		Name:        "admin_create_invite_link",
		Route:       "POST /api/admin/invite-links",
		Title:       "Create an invite link (admin)",
		Description: "One link for a group: each person who opens it joins as a new user, up to max_uses people, for days days. Answers with join_url (shown only now) and the link's details. Revoking or deleting this API key switches the link off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"label":    fiber.Map{ "type": "string" , "description": "1-80 characters; people see it on the join page." },
				"max_uses": fiber.Map{ "type": "integer" , "minimum": 1 , "maximum": models.InviteLinkMaxUses },
				"days":     fiber.Map{ "type": "integer" , "minimum": 1 , "maximum": models.InviteLinkMaxDays },
			},
			"required":             []string{ "label" , "max_uses" , "days" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				Label   string `json:"label"`
				MaxUses int    `json:"max_uses"`
				Days    int    `json:"days"`
			}
			if err = call.Bind( &args ); err != nil { return }
			payload , refused := handlers.createInviteLink( call.Ctx , args.Label , args.MaxUses , args.Days )
			if refused == nil { payload[ "join_url" ] = call.Ctx.BaseURL() + payload[ "join_path" ].( string ) }
			result , err = answer( payload , refused )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_rename_invite_link",
		Route:       "POST /api/admin/invite-links/{link_id}/rename",
		Title:       "Rename an invite link (admin)",
		Description: "Change the label people see on an invite link's join page (1-80 characters). expect, when given, is the label you think it has now.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"link_id": linkIDSchema,
				"label":   fiber.Map{ "type": "string" },
				"expect":  fiber.Map{ "type": "string" },
			},
			"required":             []string{ "link_id" , "label" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				LinkID string  `json:"link_id"`
				Label  string  `json:"label"`
				Expect *string `json:"expect"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.renameInviteLink( call.Ctx , args.LinkID , args.Label , args.Expect ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_set_invite_link_limits",
		Route:       "POST /api/admin/invite-links/{link_id}/limits",
		Title:       "Change an invite link's places and expiry (admin)",
		Description: "Set how many people the link may take in (never fewer than have joined) and, when days is given, a new expiry counted from now. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"link_id":  linkIDSchema,
				"max_uses": fiber.Map{ "type": "integer" , "minimum": 1 , "maximum": models.InviteLinkMaxUses },
				"days":     fiber.Map{ "type": "integer" , "minimum": 1 , "maximum": models.InviteLinkMaxDays , "description": "Leave it out to keep the expiry." },
			},
			"required":             []string{ "link_id" , "max_uses" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				LinkID  string `json:"link_id"`
				MaxUses int    `json:"max_uses"`
				Days    *int   `json:"days"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setInviteLinkLimits( call.Ctx , args.LinkID , args.MaxUses , args.Days ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_switch_on_invite_link",
		Route:       "POST /api/admin/invite-links/{link_id}/switch-on",
		Title:       "Switch an invite link back on (admin)",
		Description: "Undo admin_revoke_invite_link: anyone still holding the link can use it again. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: linkIDInput,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args linkIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.switchOnInviteLink( call.Ctx , args.LinkID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_delete_invite_link",
		Route:       "POST /api/admin/invite-links/{link_id}/delete",
		Title:       "Delete an invite link (admin)",
		Description: "Take an invite link off the list and stop it working. The accounts it made stay. admin_restore_invite_link undoes it for a few minutes.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: linkIDInput,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args linkIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.deleteInviteLink( call.Ctx , args.LinkID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_restore_invite_link",
		Route:       "POST /api/admin/invite-links/{link_id}/restore",
		Title:       "Undo deleting an invite link (admin)",
		Description: "Bring back an invite link deleted in the last few minutes, working as before. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: linkIDInput,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args linkIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.restoreInviteLink( call.Ctx , args.LinkID ) )
			return
		},
	} )
	// <<< feature:invite_links
}
