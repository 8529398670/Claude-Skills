// whole-file feature:mcp
package routes

import (
	strconv "strconv" // feature:files

	fiber "github.com/gofiber/fiber/v3" // feature:files

	config "{{MODULE_PATH}}/server/config" // feature:files
	mcp "{{MODULE_PATH}}/server/mcp"
	models "{{MODULE_PATH}}/server/models" // feature:files
)

// The tools /mcp offers (see server/mcp for the protocol and the rules a tool
// follows). A tool is declared with its route, by mcp="tool_name" in the
// handler's @route(...), and the route table (routes_generated.go) registers it with
// mcp.ForRoute: the same handler, the same checks on its arguments, so the
// two doors answer the same way and refuse for the same reasons. A tool that
// adds to its route's answer (where to send a file's bytes) names a call=
// that does; those live here and in mcp_tools_accounts.go.
//
// The rule: every route an API key may call has a tool, or an mcp_http
// reason in its declaration for staying HTTP only. mcp_parity_test.go fails
// until that holds. What a key may not call -- managing API keys, setting
// your own password, PIN or two-factor, other people's files, and the
// owner's screens -- needs a browser session (access=SessionOnly), so it has
// no tool either.
//
// After adding or changing a tool, run scripts/api_docs.sh to rewrite the
// route table, config/mcp.json and config/api.md; the tests fail until they
// are current.
//
// An app's tools that stand for no route go in a new file in this package
// (mcp_tools_<feature>.go), registered from registerMCPTools below.

func ( handlers *Handlers ) registerMCPTools( server *mcp.Server ) {
	handlers.registerDeclaredTools( server )
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

// filesOn is whether the files tools are offered: files.enabled can change
// while the server runs.
func ( handlers *Handlers ) filesOn() ( result bool ) {
	result = handlers.cfg().Files.Enabled
	return
}

// startUploadTool is files_start_upload's answer: the route's, and where and
// how to send the pieces. Uploading and downloading are two steps: a tool
// says where, and the bytes go to /api/files over HTTP with the same
// Authorization header.
func ( handlers *Handlers ) startUploadTool( call *mcp.Call , in *createFileRequest , serve func( fiber.Ctx , *createFileRequest ) ( *uploadReply , error ) ) ( result any , err error ) {
	reply , serve_err := serve( call.Ctx , in )
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
}

// downloadURLTool is files_download_url's answer: where the file's bytes are.
func ( handlers *Handlers ) downloadURLTool( call *mcp.Call , in *fileIDPath ) ( result any , err error ) {
	reply , serve_err := handlers.getMyFile( call.Ctx , in )
	if _ , err = mcp.Answer( reply , serve_err ); err != nil { return }
	if reply.State != models.FileReady {
		err = mcp.Refuse( "that file has not finished uploading" )
		return
	}
	result = &fileDownload{ ID: in.FileID , Name: reply.Name , Size: reply.Size , URL: fileURL( call , "/api/files/" + in.FileID + "/download" ) }
	return
}
// <<< feature:files
