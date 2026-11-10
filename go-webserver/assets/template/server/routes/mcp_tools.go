// whole-file feature:mcp
package routes

import (
	strconv "strconv" // feature:files

	fiber "github.com/gofiber/fiber/v3"

	config "{{MODULE_PATH}}/server/config" // feature:files
	mcp "{{MODULE_PATH}}/server/mcp"
	models "{{MODULE_PATH}}/server/models" // feature:invite_links|appearance|files
)

// The tools /mcp offers (see server/mcp for the protocol and the rules a tool
// follows). Each one is a thin wrapper over the helper its HTTP route uses --
// meRow, teamRows, setUserDisabled, createFile, ... -- so the two doors answer
// the same way and refuse for the same reasons.
//
// The rule: every route an API key may call has a tool here, named in its
// Route, or an "x-mcp-http" reason in config/openapi.json for staying HTTP
// only. mcp_parity_test.go fails until that holds. What a key may not call
// -- managing users, keys and sign-in, and erasing anything for good -- needs
// a browser session (see RequireSession), so it has no tool either.
//
// After adding or changing a tool, run scripts/mcp_json.sh to rewrite
// config/mcp.json; mcp_manifest_test.go fails until it is current.
//
// An app's own tools go in a new file in this package (mcp_tools_<feature>.go)
// with a registerXTools function called from registerMCPTools below.

var userIDSchema = fiber.Map{ "type": "integer" , "minimum": 1 , "description": "The account's id, from list_team or admin_list_users." }

func ( handlers *Handlers ) registerMCPTools( server *mcp.Server ) {
	server.Register( &mcp.Tool{
		Name:        "whoami",
		Route:       "GET /api/me",
		Title:       "Who am I",
		Description: "The account this API key belongs to, the role it acts with (role -- never higher than the key's own scope), and the account's own role.",
		ReadOnly:    true,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = handlers.meRow( call.Actor )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "list_team",
		Route:       "GET /api/team",
		Title:       "List team",
		Description: "People with an active account, a page at a time: id and display name. Use the id wherever another tool asks for a user. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "members" , handlers.teamRows )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_list_users",
		Route:       "GET /api/admin/users",
		Title:       "List users (admin)",
		Description: "Accounts, a page at a time, including disabled ones, with each one's role and which is the owner. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "users" , handlers.userRows )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_set_user_disabled",
		Route:       "POST /api/admin/users/{user_id}/disabled",
		Title:       "Disable or enable a user (admin)",
		Description: "Switch an account off (disabled: true) or back on (false). Disabling signs it out everywhere and voids its live login link; its API keys stop working while it is off. Reversible. The owner and your own account cannot be disabled.",
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"user_id":  userIDSchema,
				"disabled": fiber.Map{ "type": "boolean" },
			},
			"required":             []string{ "user_id" , "disabled" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				UserID   uint64 `json:"user_id"`
				Disabled *bool  `json:"disabled"`
			}
			if err = call.Bind( &args ); err != nil { return }
			if args.UserID == 0 || args.Disabled == nil {
				err = mcp.Refuse( "user_id and disabled are required" )
				return
			}
			if refused := handlers.setUserDisabled( call.Actor.User.ID , args.UserID , *args.Disabled ); refused != nil {
				err = refusalError( refused )
				return
			}
			result = fiber.Map{ "ok": true }
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_revoke_access",
		Route:       "POST /api/admin/users/{user_id}/revoke-access",
		Title:       "Revoke a user's access (admin)",
		Description: "For an account that may be compromised: ends every session and voids its live login link and every API key it holds, at once. The account stays enabled; someone with a browser session must issue a new login link to let its owner back in. The owner and your own account cannot be revoked.",
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
			if args.UserID == 0 {
				err = mcp.Refuse( "user_id is required" )
				return
			}
			revoked , refused := handlers.revokeAccess( call.Actor.User.ID , args.UserID )
			if refused != nil {
				err = refusalError( refused )
				return
			}
			result = fiber.Map{ "ok": true , "revoked": revoked }
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "rename_account",
		Route:       "POST /api/account/rename",
		Title:       "Rename your account",
		Description: "Change your own display name (1-80 characters). expect, when given, is the name you think it has now; if someone changed it meanwhile the rename is refused with the current name, so nothing is overwritten unseen.",
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"display_name": fiber.Map{ "type": "string" },
				"expect":       fiber.Map{ "type": "string" , "description": "The current name, from whoami. Optional." },
			},
			"required":             []string{ "display_name" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				DisplayName string  `json:"display_name"`
				Expect      *string `json:"expect"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.renameAccount( call.Ctx , args.DisplayName , args.Expect ) )
			return
		},
	} )

	// >>> feature:shortcuts
	server.Register( &mcp.Tool{
		Name:        "set_shortcuts",
		Route:       "POST /api/account/shortcuts",
		Title:       "Change your keyboard shortcuts",
		Description: "Replace your changed keyboard shortcuts: action name to binding, such as \"search\": \"Mod+K\" (Mod is Cmd on a Mac, Ctrl elsewhere). Send the whole set; an empty binding switches a key off, and {} puts every key back to its default. whoami shows the current set.",
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"shortcuts": fiber.Map{ "type": "object" , "additionalProperties": fiber.Map{ "type": "string" } },
			},
			"required":             []string{ "shortcuts" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				Shortcuts map[string]string `json:"shortcuts"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setMyShortcuts( call.Ctx , args.Shortcuts ) )
			return
		},
	} )
	// <<< feature:shortcuts

	// >>> feature:appearance
	server.Register( &mcp.Tool{
		Name:        "set_themes",
		Route:       "POST /api/account/themes",
		Title:       "Replace your own themes",
		Description: "Replace your whole list of color themes (1 to 40) and pick the one you use. themes: null goes back to following the app's defaults. A theme is the Theme schema in the API reference (config/openapi.json): id, name, mode light|dark, accent, second, and optional colors as #rrggbb. whoami shows the current list.",
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"themes":   fiber.Map{ "type": []string{ "array" , "null" } , "items": fiber.Map{ "type": "object" } },
				"theme_id": fiber.Map{ "type": "string" },
			},
			"required":             []string{ "themes" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				Themes  []models.Theme `json:"themes"`
				ThemeID string         `json:"theme_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setMyThemes( call.Ctx , args.Themes , args.ThemeID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "set_theme",
		Route:       "POST /api/account/theme",
		Title:       "Pick your theme",
		Description: "Switch to another theme from your list (or the defaults), by its id.",
		InputSchema: fiber.Map{
			"type":                 "object",
			"properties":           fiber.Map{ "theme_id": fiber.Map{ "type": "string" } },
			"required":             []string{ "theme_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				ThemeID string `json:"theme_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setMyTheme( call.Ctx , args.ThemeID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_get_theme_defaults",
		Route:       "GET /api/admin/theme-defaults",
		Title:       "Default themes (admin)",
		Description: "The theme list everyone without their own list sees. themes is null while the built-in ones apply.",
		ReadOnly:    true,
		AdminOnly:   true,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = answer( handlers.themeDefaults() )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_set_theme_defaults",
		Route:       "POST /api/admin/theme-defaults",
		Title:       "Replace the default themes (admin)",
		Description: "Replace the theme list everyone without their own list sees, 1 to 40 themes in the Theme schema of the API reference. null puts the built-in themes back.",
		AdminOnly:   true,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"themes": fiber.Map{ "type": []string{ "array" , "null" } , "items": fiber.Map{ "type": "object" } },
			},
			"required":             []string{ "themes" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				Themes []models.Theme `json:"themes"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.setThemeDefaults( call.Ctx , args.Themes ) )
			return
		},
	} )
	// <<< feature:appearance

	handlers.registerFileTools( server ) // feature:files

	// >>> feature:invite_links
	server.Register( &mcp.Tool{
		Name:        "admin_list_invite_links",
		Route:       "GET /api/admin/invite-links",
		Title:       "List invite links (admin)",
		Description: "Group invite links, a page at a time: label, places used and left, expiry, and whether each still works. The links themselves are never shown again after creation. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "invite_links" , handlers.inviteLinkRows )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_revoke_invite_link",
		Route:       "POST /api/admin/invite-links/{link_id}/revoke",
		Title:       "Switch off an invite link (admin)",
		Description: "Stop an invite link from letting anyone else join or sign in through it. Accounts it already made keep working. Cannot be switched back on.",
		Destructive: true,
		AdminOnly:   true,
		Enabled:     handlers.inviteLinksOn,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"link_id": fiber.Map{ "type": "string" , "description": "The id from admin_list_invite_links." },
			},
			"required":             []string{ "link_id" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				LinkID string `json:"link_id"`
			}
			if err = call.Bind( &args ); err != nil { return }
			if models.RevokeInviteLink( handlers.Store , args.LinkID ) != nil {
				err = mcp.Refuse( "no such invite link" )
				return
			}
			handlers.Webhooks.Emit( models.EventInviteLinkRevoked , map[string]any{ "link_id": args.LinkID } ) // feature:webhooks
			result = fiber.Map{ "ok": true }
			return
		},
	} )
	// <<< feature:invite_links
}

// answer hands a shared helper's result to MCP: the payload, or its refusal
// in the words the HTTP route would use.
func answer( payload fiber.Map , refused *refusal ) ( result any , err error ) {
	if refused != nil {
		err = refusalError( refused )
		return
	}
	result = payload
	return
}

// >>> feature:files
var fileIDSchema = fiber.Map{ "type": "string" , "description": "The file's id, from files_list or files_start_upload." }

// fileIDArgs is the arguments of a tool that takes only a file.
type fileIDArgs struct {
	FileID string `json:"file_id"`
}

func fileIDInput() ( schema fiber.Map ) {
	schema = fiber.Map{
		"type":                 "object",
		"properties":           fiber.Map{ "file_id": fileIDSchema },
		"required":             []string{ "file_id" },
		"additionalProperties": false,
	}
	return
}

// fileURL is where a file route is, from the address this agent reached /mcp
// at: the bytes of a file go there over plain HTTP with the same key, never
// through an MCP message (clients cap those well below a file's size).
func fileURL( call *mcp.Call , path string ) ( result string ) {
	result = call.Ctx.BaseURL() + path
	return
}

// registerFileTools is everything the Files screen does. Uploading and
// downloading are two steps: a tool says where, and the bytes go to
// /api/files over HTTP with the same Authorization header.
func ( handlers *Handlers ) registerFileTools( server *mcp.Server ) {
	files_on := func() ( result bool ) {
		result = handlers.cfg().Files.Enabled
		return
	}

	server.Register( &mcp.Tool{
		Name:        "files_list",
		Route:       "GET /api/files",
		Title:       "List your files",
		Description: "Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) and when. next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		Enabled:     files_on,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "files" , func() ( []fiber.Map , error ) { return handlers.myFileRows( call.Ctx ) } )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_get",
		Route:       "GET /api/files/{file_id}",
		Title:       "One of your files",
		Description: "One file: its details, and for an unfinished upload how many pieces have arrived (received), which is where to resume.",
		ReadOnly:    true,
		Enabled:     files_on,
		InputSchema: fileIDInput(),
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args fileIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.getMyFile( call.Ctx , args.FileID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_start_upload",
		Route:       "POST /api/files",
		Title:       "Start uploading a file",
		Description: "Reserve room for a file and get where to send it. The bytes do not go through MCP: POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does all of it. Then call files_finish_upload.",
		Enabled:     files_on,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"name": fiber.Map{ "type": "string" , "description": "The file's name, as it should be shown." },
				"size": fiber.Map{ "type": "integer" , "minimum": 1 , "description": "Exact size in bytes." },
				"type": fiber.Map{ "type": "string" , "description": "Its media type, such as application/pdf. Optional." },
			},
			"required":             []string{ "name" , "size" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				Name string `json:"name"`
				Size int64  `json:"size"`
				Type string `json:"type"`
			}
			if err = call.Bind( &args ); err != nil { return }
			row , refused := handlers.createFile( call.Ctx , args.Name , args.Type , args.Size )
			if refused != nil {
				err = refusalError( refused )
				return
			}
			id , _ := row[ "id" ].( string )
			chunks , _ := row[ "chunks" ].( int )
			chunk_url := fileURL( call , "/api/files/" + id + "/chunks/{index}" )
			row[ "chunk_url" ] = chunk_url
			row[ "finish_tool" ] = "files_finish_upload"
			row[ "curl" ] = "for ((i = 0; i < " + strconv.Itoa( chunks ) + "; i++)); do dd if=FILE bs=" + strconv.Itoa( config.FileChunkBytes ) +
				" skip=$i count=1 2>/dev/null | curl -sf -o /dev/null -H \"Authorization: Bearer $KEY\" -H 'Content-Type: application/octet-stream' --data-binary @- \"" +
				fileURL( call , "/api/files/" + id + "/chunks/" ) + "$i\"; done"
			result = row
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_finish_upload",
		Route:       "POST /api/files/{file_id}/finish",
		Title:       "Finish an upload",
		Description: "Seal an upload once every piece has arrived; the file is then ready to download. Asked again, it answers the same. If pieces are missing it says how many arrived (received).",
		Enabled:     files_on,
		InputSchema: fileIDInput(),
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args fileIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.finishFile( call.Ctx , args.FileID ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_download_url",
		Route:       "GET /api/files/{file_id}/download",
		Title:       "Where to download a file",
		Description: "The address of one of your files. GET it over HTTP with the same Authorization: Bearer header this MCP connection uses; the bytes do not come through MCP.",
		ReadOnly:    true,
		Enabled:     files_on,
		InputSchema: fileIDInput(),
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args fileIDArgs
			if err = call.Bind( &args ); err != nil { return }
			row , refused := handlers.getMyFile( call.Ctx , args.FileID )
			if refused != nil {
				err = refusalError( refused )
				return
			}
			if row[ "state" ] != models.FileReady {
				err = mcp.Refuse( "that file has not finished uploading" )
				return
			}
			result = fiber.Map{ "id": args.FileID , "name": row[ "name" ] , "size": row[ "size" ] , "url": fileURL( call , "/api/files/" + args.FileID + "/download" ) }
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_rename",
		Route:       "POST /api/files/{file_id}/rename",
		Title:       "Rename a file",
		Description: "Give one of your files a new name. expect, when given, is the name you think it has now; a rename made meanwhile is refused with the current name.",
		Enabled:     files_on,
		InputSchema: fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"file_id": fileIDSchema,
				"name":    fiber.Map{ "type": "string" },
				"expect":  fiber.Map{ "type": "string" },
			},
			"required":             []string{ "file_id" , "name" },
			"additionalProperties": false,
		},
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args struct {
				FileID string  `json:"file_id"`
				Name   string  `json:"name"`
				Expect *string `json:"expect"`
			}
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.renameFileAs( call.Ctx , args.FileID , args.Name , args.Expect , true ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_delete",
		Route:       "POST /api/files/{file_id}/delete",
		Title:       "Delete a file",
		Description: "Move one of your files to the bin. files_restore brings it back for a while; after that it is gone.",
		Destructive: true,
		Enabled:     files_on,
		InputSchema: fileIDInput(),
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args fileIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.deleteFileAs( call.Ctx , args.FileID , true ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "files_restore",
		Route:       "POST /api/files/{file_id}/restore",
		Title:       "Restore a deleted file",
		Description: "Bring back a file you deleted, while it is still in the bin and your storage quota has room.",
		Enabled:     files_on,
		InputSchema: fileIDInput(),
		Call: func( call *mcp.Call ) ( result any , err error ) {
			var args fileIDArgs
			if err = call.Bind( &args ); err != nil { return }
			result , err = answer( handlers.restoreFileAs( call.Ctx , args.FileID , false ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "my_storage",
		Route:       "GET /api/account/storage",
		Title:       "Your storage",
		Description: "How many bytes your files use, and your quota (0 is no limit).",
		ReadOnly:    true,
		Enabled:     files_on,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = answer( handlers.myStorage( call.Ctx ) )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_list_files",
		Route:       "GET /api/admin/files",
		Title:       "List everyone's files (admin)",
		Description: "Every account's files, a page at a time, with whose each one is (user_id, user_name). next is the offset of the following page, or null on the last one.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     files_on,
		InputSchema: pageSchema,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			result , err = toolPage( call , "files" , handlers.allFileRows )
			return
		},
	} )

	server.Register( &mcp.Tool{
		Name:        "admin_storage",
		Route:       "GET /api/admin/storage",
		Title:       "Storage for everyone (admin)",
		Description: "How many bytes everyone's files use together, and the disk's free and total bytes where the system says.",
		ReadOnly:    true,
		AdminOnly:   true,
		Enabled:     files_on,
		Call: func( call *mcp.Call ) ( result any , err error ) {
			if err = call.Bind( &struct{}{} ); err != nil { return }
			result , err = answer( handlers.allStorage() )
			return
		},
	} )
}
// <<< feature:files

// refusalError hands a shared rule's refusal to MCP in the same words the
// HTTP route would use. A server-side failure stays a plain error, so it is
// logged and reported only as "internal error".
func refusalError( refused *refusal ) ( err error ) {
	if refused.status >= 500 {
		err = refused.cause
		if err == nil {
			err = fiber.NewError( refused.status , refused.message )
		}
		return
	}
	err = mcp.Refuse( refused.message )
	return
}
