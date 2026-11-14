// whole-file feature:mcp
package routes

import (
	strconv "strconv" // feature:files

	config "{{MODULE_PATH}}/server/config" // feature:files
	mcp "{{MODULE_PATH}}/server/mcp"
	models "{{MODULE_PATH}}/server/models" // feature:files
)

// The tools /mcp offers (see server/mcp for the protocol and the rules a tool
// follows). A tool for a declared route is that route (mcp.ForRoute): the same
// handler, the same checks on its arguments, so the two doors answer the same
// way and refuse for the same reasons. A tool that adds to its route's answer
// (where to send a file's bytes) passes ForRoute a call that does.
//
// The rule: every route an API key may call has a tool here, made with
// mcp.ForRoute, or an MCPHTTP reason in its declaration for staying HTTP
// only. mcp_parity_test.go fails until that holds. What a key may not call
// -- managing API keys, setting your own password, PIN or two-factor, other
// people's files, and the owner's screens -- needs a browser session (see
// RequireSession), so it has no tool either. Accounts, login links, devices
// and invite links are in mcp_tools_accounts.go.
//
// After adding or changing a tool, run scripts/api_docs.sh to rewrite
// config/mcp.json and config/api.md; api_docs_test.go fails until they are
// current.
//
// An app's own tools go in a new file in this package (mcp_tools_<feature>.go)
// with a registerXTools function called from registerMCPTools below.

func ( handlers *Handlers ) registerMCPTools( server *mcp.Server ) {
	server.Register( mcp.ForRoute( handlers.GetMe , mcp.Tool{
		Name:        "whoami",
		Title:       "Who am I",
		Description: "The account this API key belongs to, the role it acts with (role -- never higher than the key's own scope), and the account's own role.",
		ReadOnly:    true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.ListTeam , mcp.Tool{
		Name:        "list_team",
		Title:       "List team",
		Description: "People with an active account, a page at a time: id and display name. Use the id wherever another tool asks for a user. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.ListUsers , mcp.Tool{
		Name:        "admin_list_users",
		Title:       "List users (admin)",
		Description: "Accounts, a page at a time, including disabled ones, with each one's role and which is the owner. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SetUserDisabled , mcp.Tool{
		Name:        "admin_set_user_disabled",
		Title:       "Disable or enable a user (admin)",
		Description: "Switch an account off (disabled: true) or back on (false). Disabling signs it out everywhere and voids its live login link; its API keys stop working while it is off. Reversible. The owner and your own account cannot be disabled.",
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RevokeAccess , mcp.Tool{
		Name:        "admin_revoke_access",
		Title:       "Revoke a user's access (admin)",
		Description: "For an account that may be compromised: ends every session and voids its live login link and every API key it holds, at once. The account stays enabled; admin_reissue_login lets its owner back in. The owner and your own account cannot be revoked.",
		Destructive: true,
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RenameAccount , mcp.Tool{
		Name:        "rename_account",
		Title:       "Rename your account",
		Description: "Change your own display name (1-80 characters). expect, when given, is the name you think it has now; if someone changed it meanwhile the rename is refused with the current name, so nothing is overwritten unseen.",
	} , nil ) )

	// >>> feature:shortcuts
	server.Register( mcp.ForRoute( handlers.SetMyShortcuts , mcp.Tool{
		Name:        "set_shortcuts",
		Title:       "Change your keyboard shortcuts",
		Description: "Replace your changed keyboard shortcuts: action name to binding, such as \"search\": \"Mod+K\" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} puts every key back to its default. whoami shows the current set.",
	} , nil ) )
	// <<< feature:shortcuts

	// >>> feature:appearance
	server.Register( mcp.ForRoute( handlers.SetMyThemes , mcp.Tool{
		Name:        "set_themes",
		Title:       "Replace your own themes",
		Description: "Replace your whole list of color themes (1 to 40) and pick the one you use. themes: null goes back to following the app's defaults. A theme is the Theme schema in the API reference (config/openapi.json): id, name, mode light|dark, accent, second, and optional colors as #rrggbb. whoami shows the current list.",
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SetMyTheme , mcp.Tool{
		Name:        "set_theme",
		Title:       "Pick your theme",
		Description: "Switch to another theme from your list (or the defaults), by its id.",
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.GetThemeDefaults , mcp.Tool{
		Name:        "admin_get_theme_defaults",
		Title:       "Default themes (admin)",
		Description: "The theme list everyone without their own list sees. themes is null while the built-in ones apply.",
		ReadOnly:    true,
		AdminOnly:   true,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.SetThemeDefaults , mcp.Tool{
		Name:        "admin_set_theme_defaults",
		Title:       "Replace the default themes (admin)",
		Description: "Replace the theme list everyone without their own list sees, 1 to 40 themes in the Theme schema of the API reference. null puts the built-in themes back.",
		AdminOnly:   true,
	} , nil ) )
	// <<< feature:appearance

	handlers.registerAccountTools( server )
	handlers.registerFileTools( server ) // feature:files

	// >>> feature:invite_links
	server.Register( mcp.ForRoute( handlers.ListInviteLinks , mcp.Tool{
		Name:        "admin_list_invite_links",
		Title:       "List invite links (admin)",
		Description: "Group invite links, a page at a time: label, places used and left, expiry, and whether each still works. The links themselves are never shown again after creation. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RevokeInviteLink , mcp.Tool{
		Name:        "admin_revoke_invite_link",
		Title:       "Switch off an invite link (admin)",
		Description: "Stop an invite link from letting anyone else join or sign in through it. Accounts it already made keep working. Cannot be switched back on.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
	} , nil ) )
	// <<< feature:invite_links
}

// >>> feature:files
// fileURL is where a file route is, from the address this agent reached /mcp
// at: the bytes of a file go there over plain HTTP with the same key, never
// through an MCP message (clients cap those well below a file's size).
func fileURL( call *mcp.Call , path string ) ( result string ) {
	result = call.Ctx.BaseURL() + path
	return
}

// startedUpload is files_start_upload's answer: the route's, and where and
// how to send the pieces.
type startedUpload struct {
	uploadReply
	ChunkURL   string `json:"chunk_url"`
	FinishTool string `json:"finish_tool"`
	Curl       string `json:"curl"`
}

// fileDownload is files_download_url's answer.
type fileDownload struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// registerFileTools is everything the Files screen does. Uploading and
// downloading are two steps: a tool says where, and the bytes go to
// /api/files over HTTP with the same Authorization header.
func ( handlers *Handlers ) registerFileTools( server *mcp.Server ) {
	files_on := func() ( result bool ) {
		result = handlers.cfg().Files.Enabled
		return
	}

	server.Register( mcp.ForRoute( handlers.ListMyFiles , mcp.Tool{
		Name:        "files_list",
		Title:       "List your files",
		Description: "Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) and when. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.GetMyFile , mcp.Tool{
		Name:        "files_get",
		Title:       "One of your files",
		Description: "One file: its details, and for an unfinished upload how many pieces have arrived (received), which is where to resume.",
		ReadOnly:    true,
		Enabled:     files_on,
	} , nil ) )

	create_file := handlers.CreateFile()
	server.Register( mcp.ForRoute( handlers.CreateFile , mcp.Tool{
		Name:        "files_start_upload",
		Title:       "Start uploading a file",
		Description: "Reserve room for a file and get where to send it. The bytes do not go through MCP: POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does all of it. Then call files_finish_upload.",
		Enabled:     files_on,
	} , func( call *mcp.Call , in *createFileRequest ) ( result any , err error ) {
		reply , serve_err := create_file.Serve( call.Ctx , in )
		if _ , err = mcp.Answer( reply , serve_err ); err != nil { return }
		pieces := fileURL( call , "/api/files/" + reply.ID + "/chunks/" )
		result = &startedUpload{
			uploadReply: *reply,
			ChunkURL:    pieces + "{index}",
			FinishTool:  "files_finish_upload",
			Curl: "for ((i = 0; i < " + strconv.Itoa( reply.Chunks ) + "; i++)); do dd if=FILE bs=" + strconv.Itoa( config.FileChunkBytes ) +
				" skip=$i count=1 2>/dev/null | curl -sf -o /dev/null -H \"Authorization: Bearer $KEY\" -H 'Content-Type: application/octet-stream' --data-binary @- \"" +
				pieces + "$i\"; done",
		}
		return
	} ) )

	server.Register( mcp.ForRoute( handlers.FinishFile , mcp.Tool{
		Name:        "files_finish_upload",
		Title:       "Finish an upload",
		Description: "Seal an upload once every piece has arrived; the file is then ready to download. Asked again, it answers the same. If pieces are missing it says how many arrived (received).",
		Enabled:     files_on,
	} , nil ) )

	get_file := handlers.GetMyFile()
	server.Register( mcp.ForRoute( handlers.DownloadMyFile , mcp.Tool{
		Name:        "files_download_url",
		Title:       "Where to download a file",
		Description: "The address of one of your files. GET it over HTTP with the same Authorization: Bearer header this MCP connection uses; the bytes do not come through MCP.",
		ReadOnly:    true,
		Enabled:     files_on,
	} , func( call *mcp.Call , in *fileIDPath ) ( result any , err error ) {
		reply , serve_err := get_file.Serve( call.Ctx , in )
		if _ , err = mcp.Answer( reply , serve_err ); err != nil { return }
		if reply.State != models.FileReady {
			err = mcp.Refuse( "that file has not finished uploading" )
			return
		}
		result = &fileDownload{ ID: in.FileID , Name: reply.Name , Size: reply.Size , URL: fileURL( call , "/api/files/" + in.FileID + "/download" ) }
		return
	} ) )

	server.Register( mcp.ForRoute( handlers.RenameMyFile , mcp.Tool{
		Name:        "files_rename",
		Title:       "Rename a file",
		Description: "Give one of your files a new name. expect, when given, is the name you think it has now; a rename made meanwhile is refused with the current name.",
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.DeleteMyFile , mcp.Tool{
		Name:        "files_delete",
		Title:       "Delete a file",
		Description: "Move one of your files to the bin. files_restore brings it back for a while; after that it is gone.",
		Destructive: true,
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.RestoreMyFile , mcp.Tool{
		Name:        "files_restore",
		Title:       "Restore a deleted file",
		Description: "Bring back a file you deleted, while it is still in the bin and your storage quota has room.",
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.MyStorage , mcp.Tool{
		Name:        "my_storage",
		Title:       "Your storage",
		Description: "How many bytes your files use, and your quota (0 is no limit).",
		ReadOnly:    true,
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.ListAllFiles , mcp.Tool{
		Name:        "admin_list_files",
		Title:       "List everyone's files (admin)",
		Description: "Every account's files, a page at a time, with whose each one is (user_id, user_name). next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     files_on,
	} , nil ) )

	server.Register( mcp.ForRoute( handlers.AllStorage , mcp.Tool{
		Name:        "admin_storage",
		Title:       "Storage for everyone (admin)",
		Description: "How many bytes everyone's files use together, and the disk's free and total bytes where the system says.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     files_on,
	} , nil ) )
}
// <<< feature:files
