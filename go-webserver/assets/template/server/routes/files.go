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

type createFileRequest struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Type      string `json:"type"`
	CSRFToken string `json:"csrf_token"`
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
type renameFileRequest struct {
	Name      string  `json:"name"`
	Expect    *string `json:"expect"`
	CSRFToken string  `json:"csrf_token"`
}

// fileRow picks the fields by hand, so the sealed key cannot reach a response
// by way of a change to the model.
func fileRow( file *models.File ) ( row fiber.Map ) {
	row = fiber.Map{
		"id":         file.ID,
		"name":       file.Name,
		"type":       file.Type,
		"size":       file.Size,
		"state":      file.State,
		"chunks":     file.Chunks,
		"received":   file.Received,
		"created_at": file.CreatedAt,
	}
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

// MyStorage says how much of the quota an account has used, so it finds out
// before an upload is refused rather than from the refusal.
func ( handlers *Handlers ) MyStorage( c fiber.Ctx ) ( err error ) {
	payload , refused := handlers.myStorage( c )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

func ( handlers *Handlers ) myStorage( c fiber.Ctx ) ( payload fiber.Map , refused *refusal ) {
	used , use_err := models.StorageUsed( handlers.Store , security.UserFrom( c ).ID )
	if use_err != nil {
		refused = internalFailure( use_err )
		return
	}
	payload = fiber.Map{ "used_bytes": used , "quota_bytes": handlers.cfg().Files.QuotaBytesPerUser }
	return
}

// AllStorage is every account's files together, and what room the disk has
// left, which no quota looks at. The disk figures are left out where the
// system will not say.
func ( handlers *Handlers ) AllStorage( c fiber.Ctx ) ( err error ) {
	payload , refused := handlers.allStorage()
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

func ( handlers *Handlers ) allStorage() ( payload fiber.Map , refused *refusal ) {
	used , use_err := models.StorageUsed( handlers.Store , 0 )
	if use_err != nil {
		refused = internalFailure( use_err )
		return
	}
	payload = fiber.Map{ "used_bytes": used }
	if free , total , space_err := disk.Space( handlers.cfg().Files.Dir ); space_err == nil {
		payload[ "disk_free_bytes" ] = free
		payload[ "disk_total_bytes" ] = total
	}
	return
}

func ( handlers *Handlers ) ListMyFiles( c fiber.Ctx ) ( err error ) {
	rows , list_err := handlers.myFileRows( c )
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	rows = matchRows( rows , c.Query( "q" ) , "name" )
	err = sendSorted( c , "files" , rows , "name" , "size" , "created_at" )
	return
}

// myFileRows is the caller's files, for ListMyFiles and the MCP files_list
// tool.
func ( handlers *Handlers ) myFileRows( c fiber.Ctx ) ( rows []fiber.Map , err error ) {
	files , err := models.ListFiles( handlers.Store , security.UserFrom( c ).ID )
	if err != nil { return }
	rows = []fiber.Map{}
	for _ , file := range files {
		rows = append( rows , fileRow( file ) )
	}
	return
}

// ListAllFiles adds who each file belongs to. The names, sizes and owners are
// what an admin needs to keep the disk in order.
func ( handlers *Handlers ) ListAllFiles( c fiber.Ctx ) ( err error ) {
	rows , list_err := handlers.allFileRows()
	if list_err != nil {
		err = serverError( c , list_err )
		return
	}
	rows = matchRows( rows , c.Query( "q" ) , "name" , "user_name" )
	err = sendSorted( c , "files" , rows , "name" , "user_name" , "size" , "created_at" )
	return
}

// allFileRows is everyone's files, for ListAllFiles and the MCP
// admin_list_files tool.
func ( handlers *Handlers ) allFileRows() ( rows []fiber.Map , err error ) {
	files , err := models.ListFiles( handlers.Store , 0 )
	if err != nil { return }
	users , err := models.ListUsers( handlers.Store )
	if err != nil { return }
	names := map[ uint64 ]string{}
	for _ , user := range users {
		names[ user.ID ] = user.DisplayName
	}
	rows = []fiber.Map{}
	for _ , file := range files {
		row := fileRow( file )
		row[ "user_id" ] = file.UserID
		row[ "user_name" ] = names[ file.UserID ]
		rows = append( rows , row )
	}
	return
}

func ( handlers *Handlers ) CreateFile( c fiber.Ctx ) ( err error ) {
	var body createFileRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	row , refused := handlers.createFile( c , body.Name , body.Type , body.Size )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.Status( fiber.StatusCreated ).JSON( row )
	return
}

// createFile starts an upload, for CreateFile and the MCP files_start_upload
// tool.
func ( handlers *Handlers ) createFile( c fiber.Ctx , name string , kind string , size int64 ) ( row fiber.Map , refused *refusal ) {
	file , create_err := models.CreateFile( handlers.Store , handlers.cfg() , security.UserFrom( c ).ID , name , kind , size )
	switch {
	case errors.Is( create_err , models.ErrBadFile ):
		refused = refuse( fiber.StatusBadRequest , create_err.Error() )
		return
	case errors.Is( create_err , models.ErrFileTooLarge ) || errors.Is( create_err , models.ErrFileQuota ):
		refused = refuse( fiber.StatusConflict , create_err.Error() )
		return
	case create_err != nil:
		refused = internalFailure( create_err )
		return
	}
	row = fileRow( file )
	row[ "chunk_bytes" ] = config.FileChunkBytes
	return
}

// GetMyFile is how a client finds out where to resume: see received.
func ( handlers *Handlers ) GetMyFile( c fiber.Ctx ) ( err error ) {
	row , refused := handlers.getMyFile( c , strings.Clone( c.Params( "file_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( row )
	return
}

func ( handlers *Handlers ) getMyFile( c fiber.Ctx , file_id string ) ( row fiber.Map , refused *refusal ) {
	file , get_err := handlers.myFile( c , file_id )
	if get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	row = fileRow( file )
	row[ "chunk_bytes" ] = config.FileChunkBytes
	return
}

// PutFileChunk takes the next piece. Out of order is a 409 carrying received,
// which is also what a retry of a piece that did arrive gets: the client goes
// on from there.
func ( handlers *Handlers ) PutFileChunk( c fiber.Ctx ) ( err error ) {
	if handlers.Guard.CheckCSRF( c , c.Get( fileCSRFHeader ) ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	index , parse_err := strconv.Atoi( c.Params( "index" ) )
	if parse_err != nil || index < 0 {
		err = badRequest( c , "bad piece number" )
		return
	}
	file_id := strings.Clone( c.Params( "file_id" ) )
	if handlers.uploads.Hold( file_id ) == false {
		err = conflict( c , "another request is writing this upload" , nil )
		return
	}
	defer handlers.uploads.Release( file_id )

	file , get_err := handlers.myFile( c , file_id )
	if get_err != nil {
		err = notFound( c , "no such file" )
		return
	}
	if file.State != models.FileUploading || index != file.Received {
		err = conflict( c , "that is not the next piece" , fiber.Map{ "received": file.Received } )
		return
	}
	if index >= file.Chunks {
		err = badRequest( c , "bad piece number" )
		return
	}
	write_err := models.WriteFileChunk( handlers.Store , handlers.cfg() , file , index , c.Body() )
	switch {
	case errors.Is( write_err , models.ErrFileChunkSize ):
		err = badRequest( c , write_err.Error() )
	case errors.Is( write_err , models.ErrFileMissing ):
		err = notFound( c , "upload_lost" )
	case write_err != nil:
		err = serverError( c , write_err )
	default:
		err = c.JSON( fiber.Map{ "received": index + 1 } )
	}
	return
}

func ( handlers *Handlers ) FinishFile( c fiber.Ctx ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	row , refused := handlers.finishFile( c , strings.Clone( c.Params( "file_id" ) ) )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( row )
	return
}

// finishFile seals an upload whose pieces have all arrived, for FinishFile and
// the MCP files_finish_upload tool.
func ( handlers *Handlers ) finishFile( c fiber.Ctx , file_id string ) ( row fiber.Map , refused *refusal ) {
	if handlers.uploads.Hold( file_id ) == false {
		refused = refuse( fiber.StatusConflict , "another request is writing this upload" )
		return
	}
	defer handlers.uploads.Release( file_id )

	file , get_err := handlers.myFile( c , file_id )
	if get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	// Asked twice, the second answer is the same as the first.
	if file.State == models.FileReady {
		row = fileRow( file )
		return
	}
	if file.Received != file.Chunks {
		refused = refuse( fiber.StatusConflict , "some of the file has not arrived" , fiber.Map{ "received": file.Received } )
		return
	}
	finished , finish_err := models.FinishFile( handlers.Store , handlers.cfg() , file )
	if errors.Is( finish_err , models.ErrFileMissing ) {
		refused = refuse( fiber.StatusNotFound , "upload_lost" )
		return
	}
	if finish_err != nil {
		refused = internalFailure( finish_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file uploaded" , fiber.Map{ "file_id": finished.ID , "size": finished.Size } )
	handlers.Webhooks.Emit( models.EventFileUploaded , map[string]any{ "file_id": finished.ID , "user_id": finished.UserID , "name": finished.Name , "size": finished.Size } ) // feature:webhooks
	row = fileRow( finished )
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

func ( handlers *Handlers ) DownloadMyFile( c fiber.Ctx ) ( err error ) {
	file , get_err := handlers.myFile( c , strings.Clone( c.Params( "file_id" ) ) )
	if get_err != nil {
		err = notFound( c , "no such file" )
		return
	}
	err = handlers.sendFile( c , file )
	return
}

func ( handlers *Handlers ) DownloadAnyFile( c fiber.Ctx ) ( err error ) {
	file , get_err := models.GetFile( handlers.Store , strings.Clone( c.Params( "file_id" ) ) )
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

// renameFile is the two rename routes. own_only is the difference: an admin may
// rename anyone's, as an admin may delete anyone's.
func ( handlers *Handlers ) renameFile( c fiber.Ctx , own_only bool ) ( err error ) {
	var body renameFileRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.renameFileAs( c , strings.Clone( c.Params( "file_id" ) ) , body.Name , body.Expect , own_only )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// renameFileAs is renameFile after the request is read, for it and the MCP
// files_rename tool.
func ( handlers *Handlers ) renameFileAs( c fiber.Ctx , file_id string , new_name string , expect *string , own_only bool ) ( payload fiber.Map , refused *refusal ) {
	var file *models.File
	var get_err error
	if own_only {
		file , get_err = handlers.myFile( c , file_id )
	} else {
		file , get_err = models.GetFile( handlers.Store , file_id )
	}
	if get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	name , rename_err := models.RenameFile( handlers.Store , file.ID , new_name , expect )
	var conflict_err *models.RenameConflictError
	switch {
	case errors.As( rename_err , &conflict_err ):
		refused = refuse( fiber.StatusConflict , conflict_err.Error() , fiber.Map{ "name": conflict_err.Current } )
		return
	case errors.Is( rename_err , models.ErrBadFileName ):
		refused = refuse( fiber.StatusBadRequest , rename_err.Error() )
		return
	case errors.Is( rename_err , db.ErrNotFound ):
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	case rename_err != nil:
		refused = internalFailure( rename_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file renamed" , fiber.Map{ "file_id": file.ID , "user_id": file.UserID } )
	payload = fiber.Map{ "ok": true , "name": name }
	return
}

func ( handlers *Handlers ) RenameMyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.renameFile( c , true )
	return
}

func ( handlers *Handlers ) RenameAnyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.renameFile( c , false )
	return
}

func ( handlers *Handlers ) DeleteMyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.deleteFile( c , true )
	return
}

func ( handlers *Handlers ) DeleteAnyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.deleteFile( c , false )
	return
}

func ( handlers *Handlers ) deleteFile( c fiber.Ctx , own_only bool ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	payload , refused := handlers.deleteFileAs( c , strings.Clone( c.Params( "file_id" ) ) , own_only )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( payload )
	return
}

// deleteFileAs moves a file to the bin, where restore can bring it back, for
// the two delete routes and the MCP files_delete tool. own_only is the
// difference between them, as for renaming.
func ( handlers *Handlers ) deleteFileAs( c fiber.Ctx , file_id string , own_only bool ) ( payload fiber.Map , refused *refusal ) {
	var file *models.File
	var get_err error
	if own_only {
		file , get_err = handlers.myFile( c , file_id )
	} else {
		file , get_err = models.GetFile( handlers.Store , file_id )
		if get_err == nil && file.Deleted() { get_err = db.ErrNotFound }
	}
	if get_err != nil {
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	if models.DeleteFile( handlers.Store , file.ID , security.UserFrom( c ).ID ) != nil {
		refused = refuse( fiber.StatusNotFound , "no such file" )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file deleted" , fiber.Map{ "file_id": file.ID , "user_id": file.UserID } )
	handlers.Webhooks.Emit( models.EventFileDeleted , map[string]any{ "file_id": file.ID , "user_id": file.UserID , "name": file.Name } ) // feature:webhooks
	payload = fiber.Map{ "ok": true }
	return
}

// restoreFile answers an undo. Someone else's file looks like no file at all,
// unless the caller is an admin on the admin route.
func ( handlers *Handlers ) restoreFile( c fiber.Ctx , as_admin bool ) ( err error ) {
	var body csrfOnlyRequest
	if c.Bind().JSON( &body ) != nil {
		err = badRequest( c , "malformed request body" )
		return
	}
	if handlers.Guard.CheckCSRF( c , body.CSRFToken ) == false {
		err = forbidden( c , "invalid csrf token" )
		return
	}
	row , refused := handlers.restoreFileAs( c , strings.Clone( c.Params( "file_id" ) ) , as_admin )
	if refused != nil {
		err = refused.send( c )
		return
	}
	err = c.JSON( row )
	return
}

// restoreFileAs is restoreFile after the request is read, for it and the MCP
// files_restore tool.
func ( handlers *Handlers ) restoreFileAs( c fiber.Ctx , file_id string , as_admin bool ) ( row fiber.Map , refused *refusal ) {
	user := security.UserFrom( c )
	if as_admin == false {
		existing , get_err := models.GetFile( handlers.Store , file_id )
		if get_err != nil || existing.UserID != user.ID {
			refused = refuse( fiber.StatusNotFound , models.ErrRestoreGone.Error() )
			return
		}
	}
	file , restore_err := models.RestoreFile( handlers.Store , handlers.cfg() , file_id , user.ID , as_admin )
	if errors.Is( restore_err , models.ErrFileQuota ) {
		refused = refuse( fiber.StatusConflict , restore_err.Error() )
		return
	}
	if restore_err != nil {
		refused = restoreRefusal( restore_err )
		return
	}
	handlers.audit( c , logs.KindAdmin , "file restored" , fiber.Map{ "file_id": file_id } )
	row = fileRow( file )
	return
}

func ( handlers *Handlers ) RestoreMyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.restoreFile( c , false )
	return
}

func ( handlers *Handlers ) RestoreAnyFile( c fiber.Ctx ) ( err error ) {
	err = handlers.restoreFile( c , true )
	return
}
