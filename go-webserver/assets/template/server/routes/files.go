// whole-file feature:files
package routes

import (
	errors "errors"
	fmt "fmt"
	log "log"
	strconv "strconv"
	strings "strings"
	time "time"

	fiber "github.com/gofiber/fiber/v3"

	apidoc "{{MODULE_PATH}}/server/apidoc"
	config "{{MODULE_PATH}}/server/config"
	db "{{MODULE_PATH}}/server/db"
	disk "{{MODULE_PATH}}/server/disk"
	logs "{{MODULE_PATH}}/server/logs"
	models "{{MODULE_PATH}}/server/models"
	security "{{MODULE_PATH}}/server/security"
)

// Uploaded files. How they are stored is in models/file.go; here is who may do
// what. Everyone has their own, and someone else's answers 404, like a key that
// is not yours. Admins have their own routes for everyone's, and one opening a
// file that is not theirs is written to the log.
//
// An upload is three steps, so that every request stays under the global body
// limit: POST /api/files, one POST per piece, then POST .../finish.
//
// A piece's body is the raw bytes, so its csrf token cannot ride in the body
// like everywhere else, and comes in fileCSRFHeader instead.

const (
	fileCSRFHeader = "X-CSRF-Token"
	// fileWriteWait is how long a piece of a download may take to send.
	// server.WriteTimeout is one deadline for a whole response, which would
	// cut a big file off at 30 seconds.
	fileWriteWait = 60 * time.Second
)

// fileIDPath is one of your own files, under /api/files/{file_id}.
type fileIDPath struct {
	FileID string `json:"file_id" path:"file_id" jsonschema_description:"The file's id, from GET /api/files (files_list over MCP)."`
}

// anyFileIDPath is anyone's file, under /api/admin/files/{file_id}.
type anyFileIDPath struct {
	FileID string `json:"file_id" path:"file_id" jsonschema_description:"The file's id, from GET /api/admin/files (admin_list_files over MCP)."`
}

// RequireFiles makes /api/files answer 404 while files.enabled is off.
func ( handlers *Handlers ) RequireFiles( c fiber.Ctx ) ( err error ) {
	if handlers.cfg().Files.Enabled == false {
		err = notFound( c , "not found" )
		return
	}
	err = c.Next()
	return
}

// fileRow picks the fields by hand, so the sealed key cannot reach a response
// by way of a change to the model.
type fileRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	Size      int64     `json:"size"`
	State     string    `json:"state" jsonschema:"enum=uploading,enum=ready"`
	Chunks    int       `json:"chunks"`
	Received  int       `json:"received"`
	CreatedAt time.Time `json:"created_at"`
}

func ( fileRow ) APIName() ( name string ) { name = "File"; return }

func newFileRow( file *models.File ) ( row fileRow ) {
	row = fileRow{
		ID:        file.ID,
		Name:      file.Name,
		Type:      file.Type,
		Size:      file.Size,
		State:     file.State,
		Chunks:    file.Chunks,
		Received:  file.Received,
		CreatedAt: file.CreatedAt,
	}
	return
}

// uploadReply is a file with the size of the pieces an upload sends.
type uploadReply struct {
	fileRow
	ChunkBytes int `json:"chunk_bytes" jsonschema_description:"Every piece but the last is exactly this long."`
}

func newUploadReply( file *models.File ) ( reply *uploadReply ) {
	reply = &uploadReply{ fileRow: newFileRow( file ) , ChunkBytes: config.FileChunkBytes }
	return
}

// contentDisposition always says attachment: an uploaded page must never run
// as this site. The plain filename is the fallback for old clients, and
// filename* carries the real name (RFC 6266).
func contentDisposition( name string ) ( result string ) {
	plain := strings.Builder{}
	encoded := strings.Builder{}
	for _ , character := range name {
		safe := character < 0x80 && ( character == '.' || character == '-' || character == '_' ||
			( character >= '0' && character <= '9' ) || ( character >= 'a' && character <= 'z' ) || ( character >= 'A' && character <= 'Z' ) )
		if safe {
			plain.WriteRune( character )
		} else {
			plain.WriteByte( '_' )
		}
	}
	for _ , value := range []byte( name ) {
		safe := value == '.' || value == '-' || value == '_' || value == '~' ||
			( value >= '0' && value <= '9' ) || ( value >= 'a' && value <= 'z' ) || ( value >= 'A' && value <= 'Z' )
		if safe {
			encoded.WriteByte( value )
		} else {
			fmt.Fprintf( &encoded , "%%%02X" , value )
		}
	}
	result = fmt.Sprintf( `attachment; filename="%s"; filename*=UTF-8''%s` , plain.String() , encoded.String() )
	return
}

// myFile is the signed-in account's own, not deleted. Anything else is
// ErrNotFound, which does not say whether the id exists.
func ( handlers *Handlers ) myFile( c fiber.Ctx , file_id string ) ( file *models.File , err error ) {
	file , err = models.GetFile( handlers.Store , file_id )
	if err == nil && ( file.Deleted() || file.UserID != security.UserFrom( c ).ID ) {
		file , err = nil , db.ErrNotFound
	}
	return
}

type myStorageReply struct {
	UsedBytes  int64 `json:"used_bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
}

// myStorage says how much of the quota an account has used, so it finds out
// before an upload is refused rather than from the refusal.
//
// @route("GET /api/account/storage", summary="Your storage",
//     tag="Files", feature="files",
//     description="How many bytes your files use, and your quota (0 is no limit).",
//     mcp="my_storage", read_only=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) myStorage( c fiber.Ctx , in *apidoc.None ) ( reply *myStorageReply , err error ) {
	used , err := models.StorageUsed( handlers.Store , security.UserFrom( c ).ID )
	if err != nil { return }
	reply = &myStorageReply{ UsedBytes: used , QuotaBytes: handlers.cfg().Files.QuotaBytesPerUser }
	return
}

type allStorageReply struct {
	UsedBytes      int64  `json:"used_bytes"`
	DiskFreeBytes  uint64 `json:"disk_free_bytes,omitempty" jsonschema_description:"Left out where the system will not say."`
	DiskTotalBytes uint64 `json:"disk_total_bytes,omitempty" jsonschema_description:"Left out where the system will not say."`
}

// allStorage is every account's files together, and what room the disk has
// left, which no quota looks at. The disk figures are left out where the
// system will not say.
//
// @route("GET /api/admin/storage", summary="Storage for everyone",
//     tag="Files", role=ADMIN, feature="files",
//     description="How many bytes everyone's files use together, and the disk's free and total bytes where "
//         "the system says.",
//     mcp="admin_storage", read_only=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) allStorage( c fiber.Ctx , in *apidoc.None ) ( reply *allStorageReply , err error ) {
	used , err := models.StorageUsed( handlers.Store , 0 )
	if err != nil { return }
	reply = &allStorageReply{ UsedBytes: used }
	if free , total , space_err := disk.Space( handlers.cfg().Files.Dir ); space_err == nil {
		reply.DiskFreeBytes , reply.DiskTotalBytes = free , total
	}
	return
}

type listMyFilesRequest struct {
	Page
	Q    string `json:"q,omitempty" query:"q" jsonschema_description:"Keep files whose name contains this, in any case."`
	Sort string `json:"sort,omitempty" query:"sort" jsonschema:"enum=name,enum=size,enum=created_at" jsonschema_description:"Field to sort the whole list by. Rows without it go last."`
	Direction
}

type filesReply struct {
	Files []fileRow `json:"files"`
	Paged
}

// @route("GET /api/files", summary="List your files",
//     tag="Files", feature="files",
//     description="Your uploaded files, a page at a time: id, name, type, size, state (uploading or ready) "
//         "and when. next is the offset of the following page, or null on the last one.",
//     mcp="files_list", read_only=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) listMyFiles( c fiber.Ctx , in *listMyFilesRequest ) ( reply *filesReply , err error ) {
	files , err := models.ListFiles( handlers.Store , security.UserFrom( c ).ID )
	if err != nil { return }
	rows := []fileRow{}
	for _ , file := range files {
		rows = append( rows , newFileRow( file ) )
	}
	reply = &filesReply{}
	reply.Files , reply.Paged = sortedPage( matchRows( rows , in.Q , "name" ) , in.Sort , in.Direction , in.Page )
	return
}

type listAllFilesRequest struct {
	Page
	Q    string `json:"q,omitempty" query:"q" jsonschema_description:"Keep files whose name or owner contains this, in any case."`
	Sort string `json:"sort,omitempty" query:"sort" jsonschema:"enum=name,enum=user_name,enum=size,enum=created_at" jsonschema_description:"Field to sort the whole list by. Rows without it go last."`
	Direction
}

// anyFileRow is a file with whose it is.
type anyFileRow struct {
	fileRow
	UserID   uint64 `json:"user_id"`
	UserName string `json:"user_name"`
}

type allFilesReply struct {
	Files []anyFileRow `json:"files"`
	Paged
}

// listAllFiles adds who each file belongs to. The names, sizes and owners are
// what an admin needs to keep the disk in order.
//
// @route("GET /api/admin/files", summary="List everyone's files",
//     tag="Files", role=ADMIN, feature="files",
//     description="Every account's files, a page at a time, with whose each one is (user_id, user_name). "
//         "next is the offset of the following page, or null on the last one.",
//     mcp="admin_list_files", read_only=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) listAllFiles( c fiber.Ctx , in *listAllFilesRequest ) ( reply *allFilesReply , err error ) {
	files , err := models.ListFiles( handlers.Store , 0 )
	if err != nil { return }
	users , err := models.ListUsers( handlers.Store )
	if err != nil { return }
	names := map[ uint64 ]string{}
	for _ , user := range users {
		names[ user.ID ] = user.DisplayName
	}
	rows := []anyFileRow{}
	for _ , file := range files {
		rows = append( rows , anyFileRow{ fileRow: newFileRow( file ) , UserID: file.UserID , UserName: names[ file.UserID ] } )
	}
	reply = &allFilesReply{}
	reply.Files , reply.Paged = sortedPage( matchRows( rows , in.Q , "name" , "user_name" ) , in.Sort , in.Direction , in.Page )
	return
}

type createFileRequest struct {
	Name string `json:"name" error:"a file needs a name and a size"`
	Size int64  `json:"size" jsonschema:"minimum=0" error:"a file needs a name and a size"`
	Type string `json:"type,omitempty" jsonschema_description:"Shown in the list. Downloads are always application/octet-stream."`
}

// @route("POST /api/files", summary="Start an upload",
//     tag="Files", feature="files", status=201, errors=(409,),
//     description="Then send each piece in order and finish. A file over files.max_file_bytes, or one that "
//         "would pass files.quota_bytes_per_user, is a 409.",
//     reply_description="Created",
//     mcp_title="Start uploading a file",  // feature:mcp
//     mcp_description="Reserve room for a file and get where to send it. The bytes do not go through MCP: "  // feature:mcp
//         "POST each piece of chunk_bytes bytes (the last may be shorter), in order from index 0, as the "  // feature:mcp
//         "raw body (Content-Type: application/octet-stream) to chunk_url with {index} filled in and the "  // feature:mcp
//         "same Authorization: Bearer header this MCP connection uses. curl is a shell recipe that does "  // feature:mcp
//         "all of it. Then call files_finish_upload.",  // feature:mcp
//     mcp="files_start_upload", enabled=handlers.filesOn, call=handlers.startUploadTool,  // feature:mcp
// )
func ( handlers *Handlers ) createFile( c fiber.Ctx , in *createFileRequest ) ( reply *uploadReply , err error ) {
	file , create_err := models.CreateFile( handlers.Store , handlers.cfg() , security.UserFrom( c ).ID , in.Name , in.Type , in.Size )
	switch {
	case errors.Is( create_err , models.ErrBadFile ):
		err = refuse( fiber.StatusBadRequest , create_err.Error() )
		return
	case errors.Is( create_err , models.ErrFileTooLarge ) || errors.Is( create_err , models.ErrFileQuota ):
		err = refuse( fiber.StatusConflict , create_err.Error() )
		return
	case create_err != nil:
		err = create_err
		return
	}
	reply = newUploadReply( file )
	return
}

// getMyFile is how a client finds out where to resume: see received.
//
// @route("GET /api/files/{file_id}", summary="One of your files",
//     tag="Files", feature="files",
//     description="One file: its details, and for an unfinished upload how many pieces have arrived "
//         "(received), which is where to resume.",
//     mcp="files_get", read_only=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) getMyFile( c fiber.Ctx , in *fileIDPath ) ( reply *uploadReply , err error ) {
	file , get_err := handlers.myFile( c , in.FileID )
	if get_err != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	reply = newUploadReply( file )
	return
}

type fileChunkRequest struct {
	fileIDPath
	Index int `json:"index" path:"index" jsonschema:"minimum=0" error:"bad piece number"`
}

type fileChunkReply struct {
	Received int `json:"received"`
}

// putFileChunk takes the next piece. Out of order is a 409 carrying received,
// which is also what a retry of a piece that did arrive gets: the client goes
// on from there.
//
// @route("POST /api/files/{file_id}/chunks/{index}", summary="Send the next piece of an upload",
//     tag="Files", feature="files", errors=(409,), body_type="application/octet-stream",
//     description="The body is the raw bytes. Pieces go in order, from 0. A browser session sends its csrf "
//         "token in the X-CSRF-Token header. A 409 carries received, the piece to send next. Counts "
//         "against server.rate_limit_upload_max.",
//     mcp_http="File bytes: MCP messages are JSON, and clients cap their size. files_start_upload says "
//         "where to send each piece, with the same key.",
// )
func ( handlers *Handlers ) putFileChunk( c fiber.Ctx , in *fileChunkRequest ) ( reply *fileChunkReply , err error ) {
	if handlers.uploads.Hold( in.FileID ) == false {
		err = refuse( fiber.StatusConflict , "another request is writing this upload" )
		return
	}
	defer handlers.uploads.Release( in.FileID )

	file , get_err := handlers.myFile( c , in.FileID )
	if get_err != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	if file.State != models.FileUploading || in.Index != file.Received {
		err = refuse( fiber.StatusConflict , "that is not the next piece" , fiber.Map{ "received": file.Received } )
		return
	}
	if in.Index >= file.Chunks {
		err = refuse( fiber.StatusBadRequest , "bad piece number" )
		return
	}
	write_err := models.WriteFileChunk( handlers.Store , handlers.cfg() , file , in.Index , c.Body() )
	switch {
	case errors.Is( write_err , models.ErrFileChunkSize ):
		err = refuse( fiber.StatusBadRequest , write_err.Error() )
	case errors.Is( write_err , models.ErrFileMissing ):
		err = refuse( fiber.StatusNotFound , "upload_lost" )
	case write_err != nil:
		err = write_err
	default:
		reply = &fileChunkReply{ Received: in.Index + 1 }
	}
	return
}

// finishFile seals an upload whose pieces have all arrived.
//
// @route("POST /api/files/{file_id}/finish", summary="Finish an upload",
//     tag="Files", feature="files", errors=(409,),
//     description="Seal an upload once every piece has arrived; the file is then ready to download. Asked "
//         "again, it answers the same. If pieces are missing it says how many arrived (received).",
//     mcp="files_finish_upload", enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) finishFile( c fiber.Ctx , in *fileIDPath ) ( reply *fileRow , err error ) {
	if handlers.uploads.Hold( in.FileID ) == false {
		err = refuse( fiber.StatusConflict , "another request is writing this upload" )
		return
	}
	defer handlers.uploads.Release( in.FileID )

	file , get_err := handlers.myFile( c , in.FileID )
	if get_err != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	// Asked twice, the second answer is the same as the first.
	if file.State == models.FileReady {
		row := newFileRow( file )
		reply = &row
		return
	}
	if file.Received != file.Chunks {
		err = refuse( fiber.StatusConflict , "some of the file has not arrived" , fiber.Map{ "received": file.Received } )
		return
	}
	finished , finish_err := models.FinishFile( handlers.Store , handlers.cfg() , file )
	if errors.Is( finish_err , models.ErrFileMissing ) {
		err = refuse( fiber.StatusNotFound , "upload_lost" )
		return
	}
	if finish_err != nil {
		err = finish_err
		return
	}
	handlers.audit( c , logs.KindAdmin , "file uploaded" , fiber.Map{ "file_id": finished.ID , "size": finished.Size } )
	handlers.Webhooks.Emit( models.EventFileUploaded , map[string]any{ "file_id": finished.ID , "user_id": finished.UserID , "name": finished.Name , "size": finished.Size } ) // feature:webhooks
	row := newFileRow( finished )
	reply = &row
	return
}

// sendFile streams a finished file. It is only ever an attachment of an opaque
// type: nosniff is on every response, and nothing the uploader said about the
// file reaches the headers except its cleaned name.
func ( handlers *Handlers ) sendFile( c fiber.Ctx , file *models.File ) ( err error ) {
	if file.State != models.FileReady {
		err = notFound( c , "no such file" )
		return
	}
	reader , open_err := models.OpenFile( handlers.cfg() , file )
	if errors.Is( open_err , models.ErrFileMissing ) {
		log.Printf( "[files] file %s has a record but not the right data on disk" , file.ID )
		err = notFound( c , "no such file" )
		return
	}
	if open_err != nil {
		err = serverError( c , open_err )
		return
	}
	c.Set( fiber.HeaderContentType , fiber.MIMEOctetStream )
	c.Set( fiber.HeaderContentDisposition , contentDisposition( file.Name ) )
	c.Set( fiber.HeaderCacheControl , "private, no-store" )
	if c.Method() == fiber.MethodHead {
		c.Set( fiber.HeaderContentLength , strconv.FormatInt( file.Size , 10 ) )
		reader.Close()
		return
	}
	// Taken here: the reader runs after the handler, when c has been reused.
	connection := c.RequestCtx().Conn()
	reader.BeforeRead = func() { connection.SetWriteDeadline( time.Now().Add( fileWriteWait ) ) }
	// fasthttp closes the reader, and the fixed size means a piece that fails
	// to open ends the connection short rather than looking complete.
	c.Response().SetBodyStream( reader , int( file.Size ) )
	return
}

// @route("GET /api/files/{file_id}/download", summary="Download one of your files",
//     tag="Files", feature="files", reply_type="application/octet-stream",
//     reply_description="The file, always as an attachment",
//     mcp_title="Where to download a file",  // feature:mcp
//     mcp_description="The address of one of your files. GET it over HTTP with the same Authorization: "  // feature:mcp
//         "Bearer header this MCP connection uses; the bytes do not come through MCP.",  // feature:mcp
//     mcp="files_download_url", read_only=True, enabled=handlers.filesOn, call=handlers.downloadURLTool,  // feature:mcp
// )
func ( handlers *Handlers ) downloadMyFile( c fiber.Ctx , in *fileIDPath ) ( err error ) {
	file , get_err := handlers.myFile( c , in.FileID )
	if get_err != nil {
		err = notFound( c , "no such file" )
		return
	}
	err = handlers.sendFile( c , file )
	return
}

// @route("GET /api/admin/files/{file_id}/download", summary="Download anyone's file",
//     tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", reply_type="application/octet-stream",
//     description="Written to the activity log when the file is someone else's.",
//     reply_description="The file, always as an attachment",
// )
func ( handlers *Handlers ) downloadAnyFile( c fiber.Ctx , in *anyFileIDPath ) ( err error ) {
	file , get_err := models.GetFile( handlers.Store , in.FileID )
	if get_err != nil || file.Deleted() {
		err = notFound( c , "no such file" )
		return
	}
	if file.UserID != security.UserFrom( c ).ID {
		handlers.audit( c , logs.KindAdmin , "file opened by admin" , fiber.Map{ "file_id": file.ID , "user_id": file.UserID } )
	}
	err = handlers.sendFile( c , file )
	return
}

// renameFileBody is what a rename sends, either route.
type renameFileBody struct {
	Name   string  `json:"name" jsonschema_description:"Cleaned like an upload's name. 1-255 bytes." error:"a file name must be 1-255 bytes"`
	Expect *string `json:"expect,omitempty" jsonschema_description:"The name you think it has. Answers 409 if it has another."`
}

type renameMyFileRequest struct {
	fileIDPath
	renameFileBody
}

type renameAnyFileRequest struct {
	anyFileIDPath
	renameFileBody
}

type renamedFileReply struct {
	Ok   bool   `json:"ok"`
	Name string `json:"name" jsonschema_description:"As stored."`
}

// @route("POST /api/files/{file_id}/rename", summary="Rename a file",
//     tag="Files", feature="files", errors=(409,),
//     description="Give one of your files a new name. expect, when given, is the name you think it has "
//         "now; a rename made meanwhile is refused with the current name.",
//     mcp="files_rename", enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) renameMyFile( c fiber.Ctx , in *renameMyFileRequest ) ( reply *renamedFileReply , err error ) {
	reply , err = handlers.renameFile( c , in.FileID , in.renameFileBody , true )
	return
}

// @route("POST /api/admin/files/{file_id}/rename", summary="Rename any file",
//     tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", errors=(409,),
// )
func ( handlers *Handlers ) renameAnyFile( c fiber.Ctx , in *renameAnyFileRequest ) ( reply *renamedFileReply , err error ) {
	reply , err = handlers.renameFile( c , in.FileID , in.renameFileBody , false )
	return
}

// renameFile is the two rename routes. own_only is the difference: an admin may
// rename anyone's, as an admin may delete anyone's.
func ( handlers *Handlers ) renameFile( c fiber.Ctx , file_id string , body renameFileBody , own_only bool ) ( reply *renamedFileReply , err error ) {
	var file *models.File
	var get_err error
	if own_only {
		file , get_err = handlers.myFile( c , file_id )
	} else {
		file , get_err = models.GetFile( handlers.Store , file_id )
	}
	if get_err != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	name , rename_err := models.RenameFile( handlers.Store , file.ID , body.Name , body.Expect )
	var conflict_err *models.RenameConflictError
	switch {
	case errors.As( rename_err , &conflict_err ):
		err = refuse( fiber.StatusConflict , conflict_err.Error() , fiber.Map{ "name": conflict_err.Current } )
		return
	case errors.Is( rename_err , models.ErrBadFileName ):
		err = refuse( fiber.StatusBadRequest , rename_err.Error() )
		return
	case errors.Is( rename_err , db.ErrNotFound ):
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	case rename_err != nil:
		err = rename_err
		return
	}
	handlers.audit( c , logs.KindAdmin , "file renamed" , fiber.Map{ "file_id": file.ID , "user_id": file.UserID } )
	reply = &renamedFileReply{ Ok: true , Name: name }
	return
}

// @route("POST /api/files/{file_id}/delete", summary="Delete a file",
//     tag="Files", feature="files",
//     description="Move one of your files to the bin. It can be restored for ten minutes; after that it is "
//         "gone.",
//     mcp_note="files_restore brings it back.",  // feature:mcp
//     mcp="files_delete", destructive=True, enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) deleteMyFile( c fiber.Ctx , in *fileIDPath ) ( reply *okReply , err error ) {
	reply , err = handlers.deleteFile( c , in.FileID , true )
	return
}

// @route("POST /api/admin/files/{file_id}/delete", summary="Delete anyone's file",
//     tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files",
// )
func ( handlers *Handlers ) deleteAnyFile( c fiber.Ctx , in *anyFileIDPath ) ( reply *okReply , err error ) {
	reply , err = handlers.deleteFile( c , in.FileID , false )
	return
}

// deleteFile moves a file to the bin, where restore can bring it back, for
// the two delete routes. own_only is the difference between them, as for
// renaming.
func ( handlers *Handlers ) deleteFile( c fiber.Ctx , file_id string , own_only bool ) ( reply *okReply , err error ) {
	var file *models.File
	var get_err error
	if own_only {
		file , get_err = handlers.myFile( c , file_id )
	} else {
		file , get_err = models.GetFile( handlers.Store , file_id )
		if get_err == nil && file.Deleted() { get_err = db.ErrNotFound }
	}
	if get_err != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	if models.DeleteFile( handlers.Store , file.ID , security.UserFrom( c ).ID ) != nil {
		err = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file deleted" , fiber.Map{ "file_id": file.ID , "user_id": file.UserID } )
	handlers.Webhooks.Emit( models.EventFileDeleted , map[string]any{ "file_id": file.ID , "user_id": file.UserID , "name": file.Name } ) // feature:webhooks
	reply = &okReply{ Ok: true }
	return
}

// @route("POST /api/files/{file_id}/restore", summary="Restore a deleted file",
//     tag="Files", feature="files", errors=(409,),
//     description="Bring back a file you deleted, while it is still in the bin and your storage quota has "
//         "room.",
//     mcp="files_restore", enabled=handlers.filesOn,  // feature:mcp
// )
func ( handlers *Handlers ) restoreMyFile( c fiber.Ctx , in *fileIDPath ) ( reply *fileRow , err error ) {
	reply , err = handlers.restoreFile( c , in.FileID , false )
	return
}

// @route("POST /api/admin/files/{file_id}/restore", summary="Put back a deleted file",
//     tag="Files", access=SESSION_ONLY, role=ADMIN, feature="files", errors=(409,),
// )
func ( handlers *Handlers ) restoreAnyFile( c fiber.Ctx , in *anyFileIDPath ) ( reply *fileRow , err error ) {
	reply , err = handlers.restoreFile( c , in.FileID , true )
	return
}

// restoreFile answers an undo. Someone else's file looks like no file at all,
// unless the caller is an admin on the admin route.
func ( handlers *Handlers ) restoreFile( c fiber.Ctx , file_id string , as_admin bool ) ( reply *fileRow , err error ) {
	user := security.UserFrom( c )
	if as_admin == false {
		existing , get_err := models.GetFile( handlers.Store , file_id )
		if get_err != nil || existing.UserID != user.ID {
			err = refuse( fiber.StatusNotFound , models.ErrRestoreGone.Error() )
			return
		}
	}
	file , restore_err := models.RestoreFile( handlers.Store , handlers.cfg() , file_id , user.ID , as_admin )
	if errors.Is( restore_err , models.ErrFileQuota ) {
		err = refuse( fiber.StatusConflict , restore_err.Error() )
		return
	}
	if restore_err != nil {
		err = restoreRefusal( restore_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file restored" , fiber.Map{ "file_id": file_id } )
	row := newFileRow( file )
	reply = &row
	return
}
