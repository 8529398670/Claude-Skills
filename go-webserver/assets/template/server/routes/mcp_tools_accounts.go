// whole-file feature:mcp
package routes

import (
	json "encoding/json"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	mcp "{{MODULE_PATH}}/server/mcp"
	models "{{MODULE_PATH}}/server/models"
)

// The MCP side of accounts and sign-in: login links, accounts, devices and
// invite links. Each tool is its HTTP route (mcp.ForRoute): the same handler,
// the same checks on what it is sent, so the rules (who may touch the owner,
// what a narrowed key may not do) hold whichever door a request comes through.
//
// A login link or invite link made here records the key that asked, and
// revoking or deleting that key voids it while it can still be used (see
// models.VoidLinksMintedByKeyTx). That is what keeps revoking a leaked key
// meaningful now that a key can hand out ways in.

// withLoginLinkURL is the call of a tool for a route that mints a login link:
// the route's reply with login_url added, the full address, from the address
// this agent reached /mcp at. The HTTP routes answer with login_path only
// (see CreateUser for why).
func withLoginLinkURL[ In any , Out any ]( route func() *apidoc.Endpoint[ In , Out ] ) ( call func( *mcp.Call , *In ) ( any , error ) ) {
	call = withFullURL( route , "login_path" , "login_url" )
	return
}

// withFullURL is the call of a tool whose route answers a path, such as a
// link to hand out: the route's reply with url_field added beside path_field,
// the full address, from the address this agent reached /mcp at.
func withFullURL[ In any , Out any ]( route func() *apidoc.Endpoint[ In , Out ] , path_field string , url_field string ) ( call func( *mcp.Call , *In ) ( any , error ) ) {
	endpoint := route()
	call = func( mcp_call *mcp.Call , in *In ) ( result any , err error ) {
		reply , serve_err := endpoint.Serve( mcp_call.Ctx , in )
		if result , err = mcp.Answer( reply , serve_err ); err != nil { return }
		encoded , err := json.Marshal( reply )
		if err != nil { return }
		answer := map[string]any{}
		if err = json.Unmarshal( encoded , &answer ); err != nil { return }
		if path , ok := answer[ path_field ].( string ); ok { answer[ url_field ] = mcp_call.Ctx.BaseURL() + path }
		result = answer
		return
	}
	return
}

const loginLinkAnswer = "Answers with login_url (open it to sign in; it works once unless max_uses says otherwise), expires_at and max_uses. Any older link the account could still use stops working; its signed-in devices stay signed in. Revoking or deleting this API key voids the link while it is unused, and signs out every device it signed in."

func ( handlers *Handlers ) registerAccountTools( server *mcp.Server ) {
	server.Register( mcp.ForRoute( handlers.CreateMyLoginLink , mcp.Tool{
		Name:        "create_my_login_link",
		Title:       "A login link for yourself",
		Description: "A link that signs your own account in on a browser or another device. Only a key with your account's full role may ask: one scoped down to user on an admin account is refused. " + loginLinkAnswer,
	} , withLoginLinkURL[ models.LoginLinkRequest , loginLinkReply ]( handlers.CreateMyLoginLink ) ) )

	server.Register( mcp.ForRoute( handlers.CancelMyLoginLink , mcp.Tool{
		Name:        "cancel_my_login_link",
		Title:       "Stop your own login link working",
		Description: "Void your account's live login link without making a new one. Devices it already signed in stay signed in. Answers with removed: 0 or 1.",
		Destructive: true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.CreateUser , mcp.Tool{
		Name:        "admin_create_user",
		Title:       "Create an account (admin)",
		Description: "Make a new account and its first login link, to hand to the person. role is admin or user. Answers with user_id and the link. " + loginLinkAnswer,
		AdminOnly:   true,
	} , withLoginLinkURL[ createUserRequest , createUserReply ]( handlers.CreateUser ) ) )

	server.Register( mcp.ForRoute( handlers.ReissueLogin , mcp.Tool{
		Name:        "admin_reissue_login",
		Title:       "A new login link for an account (admin)",
		Description: "For someone who lost their device or their link: a fresh login link for their account. " + loginLinkAnswer,
		AdminOnly:   true,
	} , withLoginLinkURL[ loginLinkRequest , loginLinkReply ]( handlers.ReissueLogin ) ) )

	server.Register( mcp.ForRoute( handlers.CancelLoginLink , mcp.Tool{
		Name:        "admin_cancel_login_link",
		Title:       "Stop an account's login link working (admin)",
		Description: "Void an account's live login link without making a new one: a link that went somewhere it should not have. Devices it already signed in stay signed in; admin_revoke_access ends those too. Answers with removed: 0 or 1.",
		Destructive: true,
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RenameUser , mcp.Tool{
		Name:        "admin_rename_user",
		Title:       "Rename someone (admin)",
		Description: "Change another account's display name (1-80 characters). Only the owner renames an admin. expect, when given, is the name you think it has now; if it changed meanwhile the rename is refused with the current name.",
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SetUserRole , mcp.Tool{
		Name:        "admin_set_user_role",
		Title:       "Change an account's role (admin)",
		Description: "Make an account an admin or an ordinary user. Only the owner takes an admin's rights away; the owner is always an admin, and nobody changes their own role.",
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.DeleteUser , mcp.Tool{
		Name:        "admin_delete_user",
		Title:       "Delete an account (admin)",
		Description: "Remove an account for good, with its sessions, login link and API keys. Cannot be undone: admin_set_user_disabled is the reversible version. Only the owner deletes an admin; nobody deletes the owner or themselves.",
		Destructive: true,
		AdminOnly:   true,
	} , nil ) )

	// >>> feature:two_factor
	server.Register( mcp.ForRoute( handlers.ResetTwoFactor , mcp.Tool{
		Name:        "admin_reset_two_factor",
		Title:       "Turn off an account's two-factor (admin)",
		Description: "For someone who lost both their phone and their recovery codes: switches their two-factor sign-in off so a login link lets them in again. Only the owner resets an admin's; the owner's is reset from a shell.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     func() bool { return handlers.cfg().TwoFactorEnabled },
	} , nil ) )
	// <<< feature:two_factor

	// >>> feature:sessions
	server.Register( mcp.ForRoute( handlers.ListMySessions , mcp.Tool{
		Name:        "list_my_sessions",
		Title:       "Your signed-in devices",
		Description: "Where your account is signed in, a page at a time: id, when it started and was last seen, the browser, and the name you gave it. Through an API key no device is current. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.LabelMySession , mcp.Tool{
		Name:        "label_my_session",
		Title:       "Name one of your devices",
		Description: "Give one of your devices a name (at most 80 characters; empty clears it). expect, when given, is the name you think it has now.",
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RevokeMySession , mcp.Tool{
		Name:        "revoke_my_session",
		Title:       "Sign out one of your devices",
		Description: "End one of your signed-in sessions; that browser has to sign in again.",
		Destructive: true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RevokeMyOtherSessions , mcp.Tool{
		Name:        "revoke_my_other_sessions",
		Title:       "Sign out all your devices",
		Description: "End every session your account has: through an API key there is no current device to keep, so every browser signs in again. API keys keep working. Answers with how many were removed.",
		Destructive: true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.ListUserSessions , mcp.Tool{
		Name:        "admin_list_user_sessions",
		Title:       "An account's signed-in devices (admin)",
		Description: "Where an account is signed in: every session, with when it started and was last seen. Addresses are shown to the owner only, and only the owner sees the owner's.",
		ReadOnly:    true,
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RevokeUserSession , mcp.Tool{
		Name:        "admin_revoke_user_session",
		Title:       "Sign out one of an account's devices (admin)",
		Description: "End one of an account's sessions, rather than all of them with admin_revoke_access.",
		Destructive: true,
		AdminOnly:   true,
	} , nil ) )
	// <<< feature:sessions

	// >>> feature:invite_links
	server.Register( mcp.ForRoute( handlers.CreateInviteLink , mcp.Tool{
		Name:        "admin_create_invite_link",
		Title:       "Create an invite link (admin)",
		Description: "One link for a group: each person who opens it joins as a new user, up to max_uses people, for days days. Answers with join_url (shown only now) and the link's details. Revoking or deleting this API key switches the link off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , withFullURL[ inviteLinkRequest , createdInviteLinkReply ]( handlers.CreateInviteLink , "join_path" , "join_url" ) ) )

	server.Register( mcp.ForRoute( handlers.RenameInviteLink , mcp.Tool{
		Name:        "admin_rename_invite_link",
		Title:       "Rename an invite link (admin)",
		Description: "Change the label people see on an invite link's join page (1-80 characters). expect, when given, is the label you think it has now.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SetInviteLinkLimits , mcp.Tool{
		Name:        "admin_set_invite_link_limits",
		Title:       "Change an invite link's places and expiry (admin)",
		Description: "Set how many people the link may take in (never fewer than have joined) and, when days is given, a new expiry counted from now. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SwitchOnInviteLink , mcp.Tool{
		Name:        "admin_switch_on_invite_link",
		Title:       "Switch an invite link back on (admin)",
		Description: "Undo admin_revoke_invite_link: anyone still holding the link can use it again. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.DeleteInviteLink , mcp.Tool{
		Name:        "admin_delete_invite_link",
		Title:       "Delete an invite link (admin)",
		Description: "Take an invite link off the list and stop it working. The accounts it made stay. admin_restore_invite_link undoes it for a few minutes.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RestoreInviteLink , mcp.Tool{
		Name:        "admin_restore_invite_link",
		Title:       "Undo deleting an invite link (admin)",
		Description: "Bring back an invite link deleted in the last few minutes, working as before. The link then counts as this key's: revoking the key switches it off.",
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )
	// <<< feature:invite_links
}
